package eventschema

import (
	"fmt"
	"reflect"
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
			context.addObjectObservable(visit.enrichmentPath, *visit.objectDef.Observable, p.config.addEnumSiblings)
		} else if typeID, present := context.getObservableTypeID(
			visit.enrichmentPath, visit.attributeName, visit.attrDef,
		); present {
			context.addObjectObservable(visit.enrichmentPath, typeID, p.config.addEnumSiblings)
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
		context.addValueObservable(visit.enrichmentPath, typeID, visit.value, p.config.addEnumSiblings)
	}
}

func (p *enrichmentProcessor) addEnumSibling(context *processingContext, visit attributeVisit) {
	if !p.config.addEnumSiblings || visit.arrayIndex >= 0 {
		return
	}
	valueString := coerce.StringLenient(visit.value)
	enumDetail := visit.attrDef.Enum[valueString]
	reportOtherAddition := false
	siblingName := ""
	if visit.attrDef.Sibling != nil {
		siblingName = *visit.attrDef.Sibling
		_, siblingPresent := attributeValue(visit.item, siblingName)
		reportOtherAddition = !siblingPresent && isOtherEnumValue(visit.value) &&
			enumDetail != nil && enumDetail.Caption != ""
		if !siblingPresent &&
			(valueString == "" || enumDetail == nil || enumDetail.Caption == "") {
			context.addProcessorIssue(issuePhaseEnrichment, newProcessingDiagnostic(
				"enrichment_enum_sibling_not_added",
				fmt.Sprintf("Enum sibling %q was not added because enum value %v has no usable schema caption.",
					siblingName, visit.value),
				jsonish.Map{
					"attribute_path": visit.validationPath,
					"attribute":      visit.attributeName,
					"value":          visit.value,
					"sibling":        siblingName,
				},
			))
		}
	}
	context.addEnumSibling(visit.item, valueString, enumDetail, visit.attrDef)
	if reportOtherAddition {
		siblingPath := makeAttributePath(parentPath(visit.validationPath), siblingName)
		context.addProcessorIssue(issuePhaseEnrichment, newProcessingDiagnostic(
			"enrichment_enum_sibling_other_added",
			fmt.Sprintf("Enum sibling %q was added with caption %q for enum ID 99 because no source-specific sibling value was present.",
				siblingName, enumDetail.Caption),
			jsonish.Map{
				"attribute_path":      siblingPath,
				"attribute":           siblingName,
				"value":               enumDetail.Caption,
				"enum_attribute_path": visit.validationPath,
				"enum_attribute":      visit.attributeName,
				"enum_value":          visit.value,
			},
		))
	}
}

func (p *enrichmentProcessor) onEventDone(context *processingContext, event jsonish.Map) {
	if !p.config.addObservables {
		return
	}

	existing, present := event["observables"]
	if present && existing == nil {
		delete(event, "observables")
		present = false
	}
	if !present {
		p.addGeneratedObservables(context, event, nil, nil)
		return
	}

	existingObservables, ok := asSlice(existing)
	if !ok {
		if len(context.observables) > 0 {
			context.addProcessorIssue(issuePhaseEnrichment, newProcessingDiagnostic(
				"enrichment_observables_not_added_wrong_type",
				"Generated observables were not added because the event observables attribute is not an array.",
				jsonish.Map{
					"attribute_path":        "observables",
					"attribute":             "observables",
					"value":                 existing,
					"generated_observables": len(context.observables),
				},
			))
		}
		return
	}
	if len(existingObservables) == 0 {
		delete(event, "observables")
		existing = nil
	}
	p.addGeneratedObservables(context, event, existing, existingObservables)
}

type observableIdentity struct {
	name      string
	typeID    int64
	valueKind uint8
	value     string
}

const (
	objectObservableValue uint8 = iota
	nullObservableValue
	stringObservableValue
)

func (p *enrichmentProcessor) addGeneratedObservables(
	context *processingContext,
	event jsonish.Map,
	existing any,
	existingObservables []any,
) {
	seen := make(map[observableIdentity]string, len(existingObservables)+len(context.observables))
	for _, observable := range existingObservables {
		if identity, ok := getObservableIdentity(observable); ok {
			seen[identity] = "existing"
		}
	}

	generated := make([]jsonish.Map, 0, len(context.observables))
	for _, observable := range context.observables {
		identity, ok := getObservableIdentity(observable)
		if ok {
			if duplicateOf, duplicate := seen[identity]; duplicate {
				duplicateDescription := duplicateOf
				if duplicateOf == "generated" {
					duplicateDescription = "earlier generated"
				}
				context.addProcessorIssue(issuePhaseEnrichment, newProcessingDiagnostic(
					"enrichment_observable_duplicate_skipped",
					fmt.Sprintf("Generated observable %q was skipped because it duplicates an %s observable.",
						identity.name, duplicateDescription),
					jsonish.Map{
						"attribute_path": identity.name,
						"attribute":      "observables",
						"observable":     observable,
						"duplicate_of":   duplicateOf,
					},
				))
				continue
			}
			seen[identity] = "generated"
		}
		generated = append(generated, observable)
	}

	if len(generated) == 0 {
		return
	}
	if existing == nil {
		event["observables"] = generated
	} else {
		event["observables"] = appendObservableMaps(existing, existingObservables, generated)
	}
	context.result.Enrichment.ObservablesAdded += len(generated)
}

func getObservableIdentity(value any) (observableIdentity, bool) {
	observable, ok := value.(jsonish.Map)
	if !ok {
		return observableIdentity{}, false
	}
	name, ok := observable["name"].(string)
	if !ok {
		return observableIdentity{}, false
	}
	typeID, ok := getInt64Value(observable["type_id"])
	if !ok {
		return observableIdentity{}, false
	}
	identity := observableIdentity{name: name, typeID: typeID, valueKind: objectObservableValue}
	value, present := observable["value"]
	if !present {
		return identity, true
	}
	if value == nil {
		identity.valueKind = nullObservableValue
		return identity, true
	}
	valueString, ok := value.(string)
	if !ok {
		return observableIdentity{}, false
	}
	identity.valueKind = stringObservableValue
	identity.value = valueString
	return identity, true
}

func appendObservableMaps(existing any, existingObservables []any, generated []jsonish.Map) any {
	switch values := existing.(type) {
	case []jsonish.Map:
		result := make([]jsonish.Map, 0, len(values)+len(generated))
		result = append(result, values...)
		return append(result, generated...)
	case []any:
		result := make([]any, 0, len(values)+len(generated))
		result = append(result, values...)
		for _, observable := range generated {
			result = append(result, observable)
		}
		return result
	}

	reflected := reflect.ValueOf(existing)
	if reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array {
		resultType := reflected.Type()
		if reflected.Kind() == reflect.Array {
			resultType = reflect.SliceOf(reflected.Type().Elem())
		}
		result := reflect.MakeSlice(resultType, 0, reflected.Len()+len(generated))
		for index := range reflected.Len() {
			result = reflect.Append(result, reflected.Index(index))
		}
		for _, observable := range generated {
			value := reflect.ValueOf(observable)
			if value.Type().AssignableTo(resultType.Elem()) {
				result = reflect.Append(result, value)
				continue
			}
			if value.Type().ConvertibleTo(resultType.Elem()) {
				result = reflect.Append(result, value.Convert(resultType.Elem()))
				continue
			}
			return appendObservableMapsAsAny(existingObservables, generated)
		}
		return result.Interface()
	}
	return appendObservableMapsAsAny(existingObservables, generated)
}

func appendObservableMapsAsAny(existing []any, generated []jsonish.Map) []any {
	result := make([]any, 0, len(existing)+len(generated))
	result = append(result, existing...)
	for _, observable := range generated {
		result = append(result, observable)
	}
	return result
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
	if _, siblingPresent := attributeValue(item, sibling); siblingPresent {
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

func (c *processingContext) addObjectObservable(
	attributePath string,
	observableTypeID int64,
	addTypeSibling bool,
) {
	observable := jsonish.Map{
		"name":    attributePath,
		"type_id": observableTypeID,
	}
	if addTypeSibling {
		if typeStr, present := c.observableTypes[observableTypeID]; present {
			observable["type"] = typeStr
		}
	}
	c.observables = append(c.observables, observable)
}

func (c *processingContext) addValueObservable(
	attributePath string,
	observableTypeID int64,
	value any,
	addTypeSibling bool,
) {
	valueStr := coerce.StringLenient(value)
	if valueStr == "" {
		return
	}
	observable := jsonish.Map{
		"name":    attributePath,
		"type_id": observableTypeID,
		"value":   valueStr,
	}
	if addTypeSibling {
		if typeStr, present := c.observableTypes[observableTypeID]; present {
			observable["type"] = typeStr
		}
	}
	c.observables = append(c.observables, observable)
}
