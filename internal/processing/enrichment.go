package processing

import (
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/observable"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
)

type enrichmentProcessor struct {
	enumSiblingsEnabled  bool
	observablesEnabled   bool
	pathNotation         pathstyle.Style
	observableTypes      observableTypeSelector
	classObservableTries map[int64]*classObservableTrie
	objectObservability  observable.ObjectObservability
	issueSuppression     issueSuppression
}

func (p *enrichmentProcessor) onClass(context *processContext) {
	if p.classObservableTries != nil {
		context.classObservableTrie = p.classObservableTries[context.class.Uid]
	}
}

func (p *enrichmentProcessor) onObject(
	context *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	objectDef *schema.ObjectDefinition,
) {
	if !p.observablesEnabled {
		return
	}
	if objectDef.Observable != nil {
		if p.observableTypes.allows(*objectDef.Observable) {
			context.addObservable(
				&context.path, *objectDef.Observable, nil, p.pathNotation, p.enumSiblingsEnabled,
			)
		}
	} else if typeID, present := p.observableTypeID(context, attributeName, attrDef); present {
		context.addObservable(&context.path, typeID, nil, p.pathNotation, p.enumSiblingsEnabled)
	}
}

func (p *enrichmentProcessor) onObjectWrongType(
	context *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	if !p.observablesEnabled || !p.attributeMayGenerate(context, attributeName, attrDef) {
		return
	}
	if !context.suppressesIssue(p.issueSuppression, issue.EnrichmentObservableNotAddedWrongType) {
		attributePath := context.path.String(pathstyle.ArrayIndexed)
		context.addProcessorIssue(
			issue.SourceEnrichment,
			issue.EnrichmentObservableNotAddedWrongType,
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      attributeName,
			},
			"Observable was not added for "+strconv.Quote(attributePath)+" because its value is not an object.",
		)
	}
}

func (p *enrichmentProcessor) onAttribute(
	context *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
	status attributeState,
) {
	if status == attributeEnum {
		p.addEnumSibling(context, item, value, attributeName, attrDef, arrayIndex)
		return
	}
	if status == attributeArrayWrongType && p.observablesEnabled {
		_, primitiveObservable := p.observableTypeID(context, attributeName, attrDef)
		objectObservable := attrDef.Type == "object_t" && p.attributeMayGenerate(context, attributeName, attrDef)
		if primitiveObservable || objectObservable {
			if !context.suppressesIssue(p.issueSuppression, issue.EnrichmentObservableNotAddedWrongType) {
				attributePath := context.path.String(pathstyle.ArrayIndexed)
				context.addProcessorIssue(
					issue.SourceEnrichment,
					issue.EnrichmentObservableNotAddedWrongType,
					jsonish.Map{
						"attribute_path": attributePath,
						"attribute":      attributeName,
					},
					"Observable was not added for "+strconv.Quote(attributePath)+" because its value is not an array.",
				)
			}
		}
		return
	}
	if status != attributePrimitive || attrDef.Enum != nil || !p.observablesEnabled {
		return
	}
	p.addScalarValueObservable(context, attributeName, attrDef, value)
}

func (p *enrichmentProcessor) onArrayElement(
	context *processContext,
	values eventvalue.ArrayView,
	index int,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	status attributeState,
) {
	if status != attributePrimitive || attrDef.Enum != nil || !p.observablesEnabled {
		return
	}
	p.addScalarValueObservable(context, attributeName, attrDef, values.At(index))
}

// onEnumSiblingPairAttributes handles an enum and its resolved string sibling of the same shape instead of
// onAttribute.
func (p *enrichmentProcessor) onEnumSiblingPairAttributes(
	context *processContext,
	item jsonish.Map,
	enumAttributeName string,
	enumAttrDef *schema.ItemAttributeDefinition,
	siblingAttributeName string,
	siblingAttrDef *schema.ItemAttributeDefinition,
) {
	enumValue, enumPresent := eventvalue.Attribute(item, enumAttributeName)
	if enumPresent {
		context.path.PushAttribute(enumAttributeName)
		if enumAttrDef.IsArray != nil && *enumAttrDef.IsArray {
			p.addEnumArraySibling(context, item, enumValue, enumAttributeName, enumAttrDef, siblingAttributeName)
		} else {
			p.addEnumSibling(context, item, enumValue, enumAttributeName, enumAttrDef, -1)
		}
		context.path.Pop()
	}
	if !p.observablesEnabled {
		return
	}
	siblingValue, siblingPresent := eventvalue.Attribute(item, siblingAttributeName)
	if !siblingPresent {
		return
	}
	context.path.PushAttribute(siblingAttributeName)
	if siblingAttrDef.IsArray != nil && *siblingAttrDef.IsArray {
		p.addArrayValueObservables(context, siblingAttributeName, siblingAttrDef, siblingValue)
	} else {
		p.addScalarValueObservable(context, siblingAttributeName, siblingAttrDef, siblingValue)
	}
	context.path.Pop()
}

func (p *enrichmentProcessor) addEnumArraySibling(
	context *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	siblingName string,
) {
	if !p.enumSiblingsEnabled {
		return
	}
	if _, present := eventvalue.Attribute(item, siblingName); present {
		return
	}
	values, ok := eventvalue.NewArrayView(value)
	if !ok {
		return
	}
	captions := make([]string, values.Len())
	reportOtherAddition := false
	for index := range values.Len() {
		detail, lookupStatus := lookupEnumDefinition(context, attrDef, values.At(index))
		if lookupStatus != enumLookupFound || detail.Caption == "" {
			if !context.suppressesIssue(p.issueSuppression, issue.EnrichmentEnumSiblingNotAdded) {
				attributePath := context.path.String(pathstyle.ArrayIndexed)
				context.addProcessorIssue(
					issue.SourceEnrichment,
					issue.EnrichmentEnumSiblingNotAdded,
					jsonish.Map{
						"attribute_path": attributePath,
						"attribute":      attributeName,
						"sibling":        siblingName,
					},
					"Enum sibling "+strconv.Quote(siblingName)+
						" was not added because an enum array value has no usable schema caption.",
				)
			}
			return
		}
		captions[index] = detail.Caption
		if enumArrayIsOther(context, attrDef, values, index) {
			reportOtherAddition = true
		}
	}
	item[siblingName] = captions
	context.result.Enrichment.EnumSiblingsAdded++
	if reportOtherAddition &&
		!context.suppressesIssue(p.issueSuppression, issue.EnrichmentEnumSiblingOtherAdded) {
		enumPath := context.path.String(pathstyle.ArrayIndexed)
		siblingPath := context.path.SiblingString(siblingName, pathstyle.ArrayIndexed)
		context.addProcessorIssue(
			issue.SourceEnrichment,
			issue.EnrichmentEnumSiblingOtherAdded,
			jsonish.Map{
				"attribute_path":      siblingPath,
				"attribute":           siblingName,
				"enum_attribute_path": enumPath,
				"enum_attribute":      attributeName,
			},
			"Enum sibling "+strconv.Quote(siblingName)+
				" was added with a schema caption for an enum ID 99 array element"+
				" because no source-specific sibling value was present.",
		)
	}
}

func enumArrayIsOther(
	context *processContext,
	attrDef *schema.ItemAttributeDefinition,
	values eventvalue.ArrayView,
	index int,
) bool {
	if context.compiled.TypeDerivedFrom(attrDef.Type, "string_t") {
		return false
	}
	var value int64
	var integral bool
	if attrDef.PrimitiveType == "long_t" {
		value, integral = values.AsLongAt(index)
	} else {
		value, integral = values.AsIntegerAt(index)
	}
	return integral && value == 99
}

func (p *enrichmentProcessor) addArrayValueObservables(
	context *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	value any,
) {
	values, ok := eventvalue.NewArrayView(value)
	if !ok {
		if _, observable := p.observableTypeID(context, attributeName, attrDef); observable &&
			!context.suppressesIssue(p.issueSuppression, issue.EnrichmentObservableNotAddedWrongType) {
			attributePath := context.path.String(pathstyle.ArrayIndexed)
			context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentObservableNotAddedWrongType,
				jsonish.Map{"attribute_path": attributePath, "attribute": attributeName},
				"Observable was not added for "+strconv.Quote(attributePath)+" because its value is not an array.",
			)
		}
		return
	}
	for index := range values.Len() {
		context.path.PushArrayIndex(index)
		p.addScalarValueObservable(context, attributeName, attrDef, values.At(index))
		context.path.Pop()
	}
}

func (p *enrichmentProcessor) addScalarValueObservable(
	context *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	value any,
) {
	typeID, present := p.observableTypeID(context, attributeName, attrDef)
	if !present {
		return
	}
	if context.compiled.TypeDerivedFrom(attrDef.Type, "json_t") {
		if !context.suppressesIssue(p.issueSuppression, issue.EnrichmentObservableNotAddedJSONType) {
			attributePath := context.path.String(pathstyle.ArrayIndexed)
			context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentObservableNotAddedJSONType,
				jsonish.Map{
					"attribute_path": attributePath,
					"attribute":      attributeName,
					"type":           attrDef.Type,
				},
				"Observable was not added for "+strconv.Quote(attributePath)+" because schema type "+
					strconv.Quote(attrDef.Type)+" has ambiguous scalar and"+
					" structured value semantics and is not supported as an observable source."+
					" If this use case is needed, file an issue at https://github.com/ocsf/ocsf-toolkit/issues.",
			)
		}
		return
	}
	valueString, valueSupported := eventvalue.FormatScalar(value)
	_, valueIsString := eventvalue.AsString(value)
	if !valueSupported || valueString == "" && !valueIsString {
		if !context.suppressesIssue(p.issueSuppression, issue.EnrichmentObservableNotAddedWrongType) {
			attributePath := context.path.String(pathstyle.ArrayIndexed)
			context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentObservableNotAddedWrongType,
				jsonish.Map{
					"attribute_path": attributePath,
					"attribute":      attributeName,
				},
				"Observable was not added for "+strconv.Quote(attributePath)+
					" because its value is not a supported scalar.",
			)
		}
		return
	}
	context.addObservable(&context.path, typeID, &valueString, p.pathNotation, p.enumSiblingsEnabled)
}

func (p *enrichmentProcessor) observableTypeID(
	context *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) (int64, bool) {
	if typeID, present := observable.TypeID(context.compiled, attributeName, attrDef); present {
		return typeID, p.observableTypes.allows(typeID)
	}
	return context.classObservableTrie.TypeID(&context.path)
}

func (p *enrichmentProcessor) attributeMayGenerate(
	context *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) bool {
	if context.classObservableTrie.HasDeclarationAtOrBelow(&context.path) {
		return true
	}
	return observable.AttributeMayGenerate(
		context.compiled,
		attributeName,
		attrDef,
		p.observableTypes.allows,
		p.objectObservability,
	)
}

func (p *enrichmentProcessor) addEnumSibling(
	context *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
) {
	if !p.enumSiblingsEnabled || isArrayElement(arrayIndex) ||
		!schema.AttributeActive(attrDef.ResolvedEnumSibling, context.activeProfiles) {
		return
	}
	enumDetail, lookupStatus := lookupEnumDefinition(context, attrDef, value)
	reportOtherAddition := false
	siblingName := ""
	if attrDef.Sibling != nil {
		siblingName = *attrDef.Sibling
		_, siblingPresent := eventvalue.Attribute(item, siblingName)
		reportOtherAddition = !siblingPresent &&
			attributeIsOtherEnumValue(value) &&
			enumDetail != nil && enumDetail.Caption != ""
		if !siblingPresent &&
			(lookupStatus != enumLookupFound || enumDetail.Caption == "") &&
			!context.suppressesIssue(p.issueSuppression, issue.EnrichmentEnumSiblingNotAdded) {
			attributePath := context.path.String(pathstyle.ArrayIndexed)
			context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentEnumSiblingNotAdded,
				jsonish.Map{
					"attribute_path": attributePath,
					"attribute":      attributeName,
					"sibling":        siblingName,
				},
				"Enum sibling "+strconv.Quote(siblingName)+
					" was not added because the enum value has no usable schema caption.",
			)
		}
	}
	context.addEnumSibling(item, enumDetail, attrDef)
	if reportOtherAddition && !context.suppressesIssue(p.issueSuppression, issue.EnrichmentEnumSiblingOtherAdded) {
		enumPath := context.path.String(pathstyle.ArrayIndexed)
		siblingPath := context.path.SiblingString(siblingName, pathstyle.ArrayIndexed)
		context.addProcessorIssue(
			issue.SourceEnrichment,
			issue.EnrichmentEnumSiblingOtherAdded,
			jsonish.Map{
				"attribute_path":      siblingPath,
				"attribute":           siblingName,
				"enum_attribute_path": enumPath,
				"enum_attribute":      attributeName,
			},
			"Enum sibling "+strconv.Quote(siblingName)+" was added with caption "+
				strconv.Quote(enumDetail.Caption)+" for enum ID 99"+
				" because no source-specific sibling value was present.",
		)
	}
}

func (p *enrichmentProcessor) onEventDone(context *processContext, event jsonish.Map) error {
	if !p.observablesEnabled {
		return nil
	}

	existing, present := event["observables"]
	if present && existing == nil {
		delete(event, "observables")
		present = false
	}
	if !present {
		return p.addGeneratedObservables(context, event, nil)
	}

	existingObservables, ok := observable.NewCollection(existing)
	if !ok {
		if len(context.observables) > 0 &&
			!context.suppressesIssue(p.issueSuppression, issue.EnrichmentObservablesNotAddedWrongType) {
			context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentObservablesNotAddedWrongType,
				jsonish.Map{
					"attribute_path":        "observables",
					"attribute":             "observables",
					"generated_observables": len(context.observables),
				},
				"Generated observables were not added because the event observables attribute is not an array.",
			)
		}
		return nil
	}
	if existingObservables.Len() == 0 {
		delete(event, "observables")
		return p.addGeneratedObservables(context, event, nil)
	}
	return p.addGeneratedObservables(context, event, &existingObservables)
}

func (p *enrichmentProcessor) addGeneratedObservables(
	context *processContext,
	event jsonish.Map,
	existing *observable.Collection,
) error {
	if len(context.observables) == 0 {
		return nil
	}
	if existing == nil && len(context.observables) == 1 {
		event["observables"] = context.observables
		context.result.Enrichment.ObservablesAdded = 1
		context.generatedObservablesStart = 0
		context.generatedObservablesPathNotation = p.pathNotation
		return nil
	}
	deduplication := observable.DeduplicateGenerated(existing, context.observables)
	for _, duplicate := range deduplication.Duplicates {
		if context.suppressesIssue(p.issueSuppression, issue.EnrichmentObservableDuplicateSkipped) {
			continue
		}
		duplicateDescription := string(duplicate.Source)
		if duplicate.Source == observable.DuplicateGenerated {
			duplicateDescription = "earlier generated"
		}
		attributePath := context.observableDiagnosticPath(duplicate.GeneratedIndex, duplicate.Name, p.pathNotation)
		context.addProcessorIssue(
			issue.SourceEnrichment,
			issue.EnrichmentObservableDuplicateSkipped,
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      terminalObservableAttribute(attributePath),
				"duplicate_of":   string(duplicate.Source),
			},
			"Generated observable for path "+strconv.Quote(attributePath)+
				" was skipped because it duplicates an "+duplicateDescription+" observable.",
		)
	}
	generated := deduplication.Accepted

	if len(generated) == 0 {
		return nil
	}
	if existing == nil {
		event["observables"] = generated
	} else {
		appended, err := existing.Append(generated)
		if err != nil {
			return err
		}
		event["observables"] = appended
	}
	context.result.Enrichment.ObservablesAdded += len(generated)
	if existing != nil {
		context.generatedObservablesStart = existing.Len()
	}
	context.generatedObservablesPathNotation = p.pathNotation
	return nil
}

// terminalObservableAttribute extracts the final attribute from a rendered observable path. It is used only while
// reporting a duplicate, avoiding per-observable tracking solely for an uncommon diagnostic.
func terminalObservableAttribute(path string) string {
	attribute := path
	if separator := strings.LastIndexByte(attribute, '.'); separator >= 0 {
		attribute = attribute[separator+1:]
	}
	if array := strings.IndexByte(attribute, '['); array >= 0 {
		attribute = attribute[:array]
	}
	return attribute
}

func (c *processContext) addEnumSibling(
	item jsonish.Map,
	enumDetail *schema.EnumDefinition,
	attrDef *schema.ItemAttributeDefinition,
) {
	if attrDef == nil || attrDef.Sibling == nil || enumDetail == nil || enumDetail.Caption == "" {
		return
	}
	sibling := *attrDef.Sibling
	if _, siblingPresent := eventvalue.Attribute(item, sibling); siblingPresent {
		return
	}
	item[sibling] = enumDetail.CaptionValue()
	c.result.Enrichment.EnumSiblingsAdded++
}

// addObservable adds a generated observable for path, describing an object attribute when value is nil, or a
// scalar attribute's value when value is non-nil (including an empty string, which is a legitimate scalar value).
func (c *processContext) addObservable(
	path *eventpath.Path,
	observableTypeID int64,
	value *string,
	pathNotation pathstyle.Style,
	enumSiblingsEnabled bool,
) {
	typeIDValue, present := c.compiled.ObservableTypeIDValue(observableTypeID)
	if !present {
		typeIDValue = observableTypeID
	}
	name := path.String(pathNotation)
	observable := jsonish.Map{
		"name":    name,
		"type_id": typeIDValue,
	}
	if value != nil {
		observable["value"] = *value
	}
	if enumSiblingsEnabled {
		if typeValue, present := c.compiled.ObservableTypeValue(observableTypeID); present {
			observable["type"] = typeValue
		}
	}
	c.addGeneratedObservable(path, observable, pathNotation)
}

func (c *processContext) addGeneratedObservable(
	path *eventpath.Path,
	observable jsonish.Map,
	style pathstyle.Style,
) {
	index := len(c.observables)
	c.observables = append(c.observables, observable)
	if style == pathstyle.ArrayIndexed || style == pathstyle.JSONPath || !path.HasArrayIndex() {
		if c.observableDiagnosticPaths != nil {
			c.observableDiagnosticPaths = append(c.observableDiagnosticPaths, "")
		}
		return
	}
	if c.observableDiagnosticPaths == nil {
		c.observableDiagnosticPaths = make([]string, index, index+1)
	}
	c.observableDiagnosticPaths = append(c.observableDiagnosticPaths, path.String(pathstyle.ArrayIndexed))
}

func (c *processContext) observableDiagnosticPath(index int, name string, style pathstyle.Style) string {
	if index < len(c.observableDiagnosticPaths) && c.observableDiagnosticPaths[index] != "" {
		return c.observableDiagnosticPaths[index]
	}
	if style == pathstyle.JSONPath {
		return strings.TrimPrefix(name, "$.")
	}
	return name
}
