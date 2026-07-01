package eventschema

import (
	"fmt"
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/coerce"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

const issuePhaseEnrichment = "enrichment"

type enrichmentProcessor struct {
	eventProcessVisitorBase
	config enrichmentConfig
}

func (p *enrichmentProcessor) onObject(
	context *processingContext,
	visit objectVisit,
) {
	if !p.config.addObservables {
		return
	}
	if visit.status == objectVisitWrongType &&
		context.objectAttributeMayGenerateObservable(visit.enrichmentPath, visit.attributeName, visit.attrDef) {
		context.addProcessorIssue(issuePhaseEnrichment, newProcessingDiagnostic(
			"enrichment_observable_not_added_wrong_type",
			fmt.Sprintf("Observable was not added for %q because its value is not an object.", visit.validationPath),
			jsonish.Map{
				"attribute_path": visit.validationPath,
				"attribute":      visit.attributeName,
				"value":          visit.value,
			},
		))
		return
	}
	if visit.status == objectVisitValid {
		if visit.objectDef.Observable != nil {
			context.addObjectObservable(visit.enrichmentPath, *visit.objectDef.Observable)
		} else if typeID, present := context.getObservableTypeID(
			visit.enrichmentPath, visit.attributeName, visit.attrDef,
		); present {
			context.addObjectObservable(visit.enrichmentPath, typeID)
		}
	}
}

func (p *enrichmentProcessor) onAttribute(
	context *processingContext,
	visit attributeVisit,
) {
	if visit.status == attributeVisitEnum {
		p.addEnumSibling(context, visit)
		return
	}
	if visit.status == attributeVisitArrayWrongType && p.config.addObservables {
		_, primitiveObservable := context.getObservableTypeID(visit.enrichmentPath, visit.attributeName, visit.attrDef)
		objectObservable := visit.attrDef.Type == "object_t" &&
			context.objectAttributeMayGenerateObservable(visit.enrichmentPath, visit.attributeName, visit.attrDef)
		if primitiveObservable || objectObservable {
			context.addProcessorIssue(issuePhaseEnrichment, newProcessingDiagnostic(
				"enrichment_observable_not_added_wrong_type",
				fmt.Sprintf("Observable was not added for %q because its value is not an array.", visit.validationPath),
				jsonish.Map{
					"attribute_path": visit.validationPath,
					"attribute":      visit.attributeName,
					"value":          visit.value,
				},
			))
		}
		return
	}
	if visit.status != attributeVisitPrimitive || visit.attrDef.Enum != nil || !p.config.addObservables {
		return
	}
	if typeID, present := context.getObservableTypeID(visit.enrichmentPath, visit.attributeName, visit.attrDef); present {
		context.addValueObservable(visit.enrichmentPath, typeID, visit.value)
	}
}

func (p *enrichmentProcessor) addEnumSibling(context *processingContext, visit attributeVisit) {
	if !p.config.addEnumSiblings || visit.arrayIndex >= 0 {
		return
	}
	valueString := coerce.StringLenient(visit.value)
	enumDetail := visit.attrDef.Enum[valueString]
	if visit.attrDef.Sibling != nil {
		if _, siblingPresent := visit.item[*visit.attrDef.Sibling]; !siblingPresent &&
			(valueString == "" || enumDetail == nil || enumDetail.Caption == "") {
			context.addProcessorIssue(issuePhaseEnrichment, newProcessingDiagnostic(
				"enrichment_enum_sibling_not_added",
				fmt.Sprintf("Enum sibling %q was not added because enum value %v has no usable schema caption.",
					*visit.attrDef.Sibling, visit.value),
				jsonish.Map{
					"attribute_path": visit.validationPath,
					"attribute":      visit.attributeName,
					"value":          visit.value,
					"sibling":        *visit.attrDef.Sibling,
				},
			))
		}
	}
	context.addEnumSibling(visit.item, valueString, enumDetail, visit.attrDef)
}

func (p *enrichmentProcessor) onEventDone(context *processingContext, event jsonish.Map) {
	if !p.config.addObservables || len(context.observables) == 0 {
		return
	}
	if existing, present := event["observables"]; present && existing != nil {
		if values, ok := asSlice(existing); !ok || len(values) > 0 {
			context.addProcessorIssue(issuePhaseEnrichment, newProcessingDiagnostic(
				"enrichment_observables_not_added_existing",
				"Generated observables were not added because the event already contains observables.",
				jsonish.Map{
					"attribute_path":        "observables",
					"attribute":             "observables",
					"generated_observables": len(context.observables),
				},
			))
			return
		}
	}
	event["observables"] = context.observables
	context.result.Enrichment.ObservablesAdded += len(context.observables)
}

func (c *processingContext) addEnumSiblings() bool {
	return c.enrichment != nil && c.enrichment.config.addEnumSiblings
}

func (c *processingContext) addEnumSibling(
	item jsonish.Map,
	valueString string,
	enumDetail *enumDefinition,
	attrDef *itemAttributeDefinition,
) {
	if attrDef == nil || attrDef.Sibling == nil || enumDetail == nil || enumDetail.Caption == "" {
		return
	}
	sibling := *attrDef.Sibling
	if _, siblingPresent := item[sibling]; siblingPresent {
		return
	}
	if valueString == "" {
		return
	}
	item[sibling] = enumDetail.Caption
	c.result.Enrichment.EnumSiblingsAdded++
}

func (c *processingContext) getObservableTypeID(
	attributePath string,
	attribute string,
	attrDef *itemAttributeDefinition,
) (int64, bool) {
	if attrDef != nil {
		typeDef, present := c.dictionary.Types.Attributes[attrDef.Type]
		if present && typeDef != nil && typeDef.Observable != nil {
			return *typeDef.Observable, true
		}
	}
	dictAttrDef, dictAttrPresent := c.dictionary.Attributes[attribute]
	if dictAttrPresent && dictAttrDef != nil && dictAttrDef.Observable != nil {
		return *dictAttrDef.Observable, true
	}
	if attrDef != nil && attrDef.Observable != nil {
		return *attrDef.Observable, true
	}
	if c.classObservables != nil {
		typeID, present := c.classObservables[attributePath]
		return typeID, present
	}
	return 0, false
}

func (c *processingContext) objectAttributeMayGenerateObservable(
	attributePath string,
	attributeName string,
	attrDef *itemAttributeDefinition,
) bool {
	if attrDef == nil || attrDef.ObjectType == nil {
		return false
	}
	objectDef := c.objects[*attrDef.ObjectType]
	if objectDef == nil {
		return false
	}
	if objectDef.Observable != nil || c.classObservableBelow(attributePath) {
		return true
	}
	if _, present := c.getObservableTypeID(attributePath, attributeName, attrDef); present {
		return true
	}
	return c.objectDefinitionMayGenerateObservable(objectDef, make(map[*objectDefinition]struct{}))
}

func (c *processingContext) classObservableBelow(attributePath string) bool {
	prefix := attributePath + "."
	for path := range c.classObservables {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (c *processingContext) objectDefinitionMayGenerateObservable(
	objectDef *objectDefinition,
	seen map[*objectDefinition]struct{},
) bool {
	if objectDef == nil {
		return false
	}
	if _, visited := seen[objectDef]; visited {
		return false
	}
	seen[objectDef] = struct{}{}
	defer delete(seen, objectDef)

	for attributeName, attrDef := range objectDef.Attributes {
		if attrDef == nil {
			continue
		}
		if attrDef.Observable != nil {
			return true
		}
		if dictAttrDef := c.dictionary.Attributes[attributeName]; dictAttrDef != nil && dictAttrDef.Observable != nil {
			return true
		}
		if attrDef.Type == "object_t" && attrDef.ObjectType != nil {
			if nested := c.objects[*attrDef.ObjectType]; nested != nil &&
				(nested.Observable != nil || c.objectDefinitionMayGenerateObservable(nested, seen)) {
				return true
			}
			continue
		}
		if typeDef := c.dictionary.Types.Attributes[attrDef.Type]; typeDef != nil && typeDef.Observable != nil {
			return true
		}
	}
	return false
}

func (c *processingContext) addObjectObservable(attributePath string, observableTypeID int64) {
	observable := jsonish.Map{
		"name":    attributePath,
		"type_id": observableTypeID,
	}
	if c.addEnumSiblings() {
		if typeStr, present := c.observableTypes[observableTypeID]; present {
			observable["type"] = typeStr
		}
	}
	c.observables = append(c.observables, observable)
}

func (c *processingContext) addValueObservable(attributePath string, observableTypeID int64, value any) {
	valueStr := coerce.StringLenient(value)
	if valueStr == "" {
		return
	}
	observable := jsonish.Map{
		"name":    attributePath,
		"type_id": observableTypeID,
		"value":   valueStr,
	}
	if c.addEnumSiblings() {
		if typeStr, present := c.observableTypes[observableTypeID]; present {
			observable["type"] = typeStr
		}
	}
	c.observables = append(c.observables, observable)
}
