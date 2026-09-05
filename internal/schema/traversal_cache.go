package schema

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/schemaresult"
)

// EnsureTraversalCache initializes immutable schema data used by event traversal.
func (s *Compiled) EnsureTraversalCache() {
	s.traversalCacheOnce.Do(func() {
		classIDs := make([]int64, 0, len(s.Classes))
		for classID := range s.Classes {
			classIDs = append(classIDs, classID)
		}
		slices.Sort(classIDs)
		for _, classID := range classIDs {
			class := s.Classes[classID]
			if class != nil {
				setItemTraversalCache(s, "class", class.Name, &class.ItemDefinition)
				s.initializeItemNumericEnums(&class.ItemDefinition)
			}
		}
		for _, objectName := range sortedKeys(s.Objects) {
			object := s.Objects[objectName]
			if object != nil {
				setItemTraversalCache(s, "object", objectName, &object.ItemDefinition)
				s.initializeItemNumericEnums(&object.ItemDefinition)
			}
		}
	})
}

// InitializationIssues returns nonfatal schema conditions found while constructing event-processing caches.
func (s *Compiled) InitializationIssues() []schemaresult.InitializationIssue {
	s.EnsureTraversalCache()
	issues := make([]schemaresult.InitializationIssue, len(s.initializationIssues))
	for index, found := range s.initializationIssues {
		found.Details = maps.Clone(found.Details)
		issues[index] = found
	}
	return issues
}

func setItemTraversalCache(schema *Compiled, itemType, itemName string, item *ItemDefinition) {
	item.OrderedAttributes = make([]OrderedAttribute, 0, len(item.Attributes))
	for name, definition := range item.Attributes {
		if definition != nil {
			// Loaded schemas have already passed type-inheritance cycle validation. A validation-enabled pipeline
			// independently propagates this defensive error for internally constructed schemas.
			definition.PrimitiveType, _ = schema.ResolvePrimitiveType(definition.Type)
			if definition.ObjectType != nil {
				definition.ResolvedObject = schema.Objects[*definition.ObjectType]
			}
		}
		item.OrderedAttributes = append(item.OrderedAttributes, OrderedAttribute{Name: name, Definition: definition})
	}
	sort.Slice(item.OrderedAttributes, func(left, right int) bool {
		return item.OrderedAttributes[left].Name < item.OrderedAttributes[right].Name
	})
	for _, attribute := range item.OrderedAttributes {
		definition := attribute.Definition
		if definition == nil || definition.Enum == nil || definition.Sibling == nil {
			continue
		}
		sibling := item.Attributes[*definition.Sibling]
		found := enumSiblingInitializationIssue(itemType, itemName, attribute.Name, definition, sibling)
		if found != nil {
			schema.initializationIssues = append(schema.initializationIssues, *found)
			continue
		}
		definition.ResolvedEnumSibling = sibling
		sibling.ResolvedEnumAttribute = definition
		sibling.ResolvedEnumAttributeName = attribute.Name
	}
	item.OrderedConstraintKeys = sortedKeys(item.Constraints)
}

func enumSiblingInitializationIssue(
	itemType string,
	itemName string,
	attributeName string,
	attribute *ItemAttributeDefinition,
	sibling *ItemAttributeDefinition,
) *schemaresult.InitializationIssue {
	siblingName := *attribute.Sibling
	baseDetails := jsonish.Map{
		"item_type": itemType,
		"item_name": itemName,
		"attribute": attributeName,
		"sibling":   siblingName,
	}
	if attribute.Type != "integer_t" && attribute.Type != "long_t" {
		baseDetails["attribute_type"] = attribute.Type
		baseDetails["attribute_is_array"] = isArray(attribute)
		return &schemaresult.InitializationIssue{
			Code: issue.AtInitSchemaEnumSiblingSourceNotIntegral,
			Message: fmt.Sprintf(
				"Schema %s %q enum attribute %q with sibling %q must have direct type integer_t or long_t.",
				itemType, itemName, attributeName, siblingName,
			),
			Details: baseDetails,
		}
	}
	if sibling == nil {
		return &schemaresult.InitializationIssue{
			Code: issue.AtInitSchemaEnumSiblingTargetNotFound,
			Message: fmt.Sprintf(
				"Schema %s %q enum attribute %q names missing sibling %q.",
				itemType, itemName, attributeName, siblingName,
			),
			Details: baseDetails,
		}
	}
	if sibling.Enum != nil {
		baseDetails["sibling_type"] = sibling.Type
		baseDetails["sibling_is_array"] = isArray(sibling)
		return &schemaresult.InitializationIssue{
			Code: issue.AtInitSchemaEnumSiblingTargetIsEnum,
			Message: fmt.Sprintf(
				"Schema %s %q enum attribute %q names sibling %q, which is itself an enum.",
				itemType, itemName, attributeName, siblingName,
			),
			Details: baseDetails,
		}
	}
	if sibling.Type != "string_t" || isArray(attribute) != isArray(sibling) {
		baseDetails["expected_type"] = "string_t"
		baseDetails["expected_is_array"] = isArray(attribute)
		baseDetails["actual_type"] = sibling.Type
		baseDetails["actual_is_array"] = isArray(sibling)
		return &schemaresult.InitializationIssue{
			Code: issue.AtInitSchemaEnumSiblingTargetNotString,
			Message: fmt.Sprintf(
				"Schema %s %q enum attribute %q names sibling %q with an incompatible direct type or array shape.",
				itemType, itemName, attributeName, siblingName,
			),
			Details: baseDetails,
		}
	}
	return nil
}
