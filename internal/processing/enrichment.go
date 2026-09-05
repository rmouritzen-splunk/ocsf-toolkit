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
	enumSiblingsEnabled        bool
	observablesEnabled         bool
	deduplicateGenerated       bool
	reportObservableDuplicates bool
	pathNotation               pathstyle.Style
	observableTypes            observableTypeSelector
	classObservableTries       map[int64]*classObservableTrie
	objectObservability        observable.ObjectObservability
}

func (p *enrichmentProcessor) onClass(context *processContext) {
	if p.classObservableTries != nil {
		context.classObservableTrie = p.classObservableTries[context.class.Uid]
	}
	if !p.observablesEnabled {
		return
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
				p.deduplicateGenerated, p.reportObservableDuplicates,
			)
		}
	} else if typeID, present := p.observableTypeID(context, attributeName, attrDef); present {
		context.addObservable(
			&context.path, typeID, nil, p.pathNotation, p.enumSiblingsEnabled,
			p.deduplicateGenerated, p.reportObservableDuplicates,
		)
	}
}

func (p *enrichmentProcessor) onObjectWrongType(
	context *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) error {
	if !p.observablesEnabled || !p.attributeMayGenerate(context, attributeName, attrDef) {
		return nil
	}
	if !context.ignoresIssue(issueEnrichmentObservableNotAddedWrongTypeMask) {
		attributePath := context.path.String(pathstyle.ArrayIndexed)
		return context.addProcessorIssue(
			issue.SourceEnrichment,
			issue.EnrichmentObservableNotAddedWrongType,
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      attributeName,
			},
			"Observable was not added for "+strconv.Quote(attributePath)+" because its value is not an object.",
		)
	}
	return nil
}

func (p *enrichmentProcessor) onAttribute(
	context *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
	status attributeState,
) error {
	if status == attributeEnum {
		return p.addEnumSibling(context, item, value, attributeName, attrDef, arrayIndex)
	}
	if status == attributeArrayWrongType && p.observablesEnabled {
		_, primitiveObservable := p.observableTypeID(context, attributeName, attrDef)
		objectObservable := attrDef.Type == "object_t" && p.attributeMayGenerate(context, attributeName, attrDef)
		if primitiveObservable || objectObservable {
			if !context.ignoresIssue(issueEnrichmentObservableNotAddedWrongTypeMask) {
				attributePath := context.path.String(pathstyle.ArrayIndexed)
				return context.addProcessorIssue(
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
		return nil
	}
	if status != attributePrimitive || attrDef.Enum != nil || !p.observablesEnabled {
		return nil
	}
	return p.addScalarValueObservable(context, attributeName, attrDef, value)
}

func (p *enrichmentProcessor) onArrayElement(
	context *processContext,
	values eventvalue.ArrayView,
	index int,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	status attributeState,
) error {
	if status != attributePrimitive || attrDef.Enum != nil || !p.observablesEnabled {
		return nil
	}
	return p.addScalarValueObservable(context, attributeName, attrDef, values.At(index))
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
) error {
	enumValue := item[enumAttributeName]
	if enumValue != nil {
		context.path.PushAttribute(enumAttributeName)
		var err error
		if enumAttrDef.IsArray != nil && *enumAttrDef.IsArray {
			err = p.addEnumArraySibling(
				context, item, enumValue, enumAttributeName, enumAttrDef, siblingAttributeName,
			)
		} else {
			err = p.addEnumSibling(context, item, enumValue, enumAttributeName, enumAttrDef, -1)
		}
		context.path.Pop()
		if err != nil {
			return err
		}
	}
	if !p.observablesEnabled {
		return nil
	}
	siblingValue := item[siblingAttributeName]
	if siblingValue == nil {
		return nil
	}
	context.path.PushAttribute(siblingAttributeName)
	var err error
	if siblingAttrDef.IsArray != nil && *siblingAttrDef.IsArray {
		err = p.addArrayValueObservables(context, siblingAttributeName, siblingAttrDef, siblingValue)
	} else {
		err = p.addScalarValueObservable(context, siblingAttributeName, siblingAttrDef, siblingValue)
	}
	context.path.Pop()
	return err
}

func (p *enrichmentProcessor) addEnumArraySibling(
	context *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	siblingName string,
) error {
	if !p.enumSiblingsEnabled {
		return nil
	}
	if item[siblingName] != nil {
		return nil
	}
	values, ok := eventvalue.NewArrayView(value)
	if !ok {
		return nil
	}
	captions := make([]string, values.Len())
	reportOtherAddition := false
	for index := range values.Len() {
		detail, lookupStatus := lookupEnumDefinition(context, attrDef, values.At(index))
		if lookupStatus != enumLookupFound || detail.Caption == "" {
			if !context.ignoresIssue(issueEnrichmentEnumSiblingNotAddedMask) {
				attributePath := context.path.String(pathstyle.ArrayIndexed)
				return context.addProcessorIssue(
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
			return nil
		}
		captions[index] = detail.Caption
		if enumArrayIsOther(context, attrDef, values, index) {
			reportOtherAddition = true
		}
	}
	item[siblingName] = captions
	context.result.Enrichment.EnumSiblingsAdded++
	if reportOtherAddition &&
		!context.ignoresIssue(issueEnrichmentEnumSiblingOtherAddedMask) {
		enumPath := context.path.String(pathstyle.ArrayIndexed)
		siblingPath := context.path.SiblingString(siblingName, pathstyle.ArrayIndexed)
		return context.addProcessorIssue(
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
	return nil
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
) error {
	values, ok := eventvalue.NewArrayView(value)
	if !ok {
		if _, observable := p.observableTypeID(context, attributeName, attrDef); observable &&
			!context.ignoresIssue(issueEnrichmentObservableNotAddedWrongTypeMask) {
			attributePath := context.path.String(pathstyle.ArrayIndexed)
			return context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentObservableNotAddedWrongType,
				jsonish.Map{"attribute_path": attributePath, "attribute": attributeName},
				"Observable was not added for "+strconv.Quote(attributePath)+" because its value is not an array.",
			)
		}
		return nil
	}
	for index := range values.Len() {
		context.path.PushArrayIndex(index)
		err := p.addScalarValueObservable(context, attributeName, attrDef, values.At(index))
		context.path.Pop()
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *enrichmentProcessor) addScalarValueObservable(
	context *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	value any,
) error {
	typeID, present := p.observableTypeID(context, attributeName, attrDef)
	if !present {
		return nil
	}
	if context.compiled.TypeDerivedFrom(attrDef.Type, "json_t") {
		if !context.ignoresIssue(issueEnrichmentObservableNotAddedJSONTypeMask) {
			attributePath := context.path.String(pathstyle.ArrayIndexed)
			if err := context.addProcessorIssue(
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
			); err != nil {
				return err
			}
		}
		return nil
	}
	valueString, valueSupported := eventvalue.FormatScalar(value)
	_, valueIsString := eventvalue.AsString(value)
	if !valueSupported || valueString == "" && !valueIsString {
		if !context.ignoresIssue(issueEnrichmentObservableNotAddedWrongTypeMask) {
			attributePath := context.path.String(pathstyle.ArrayIndexed)
			if err := context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentObservableNotAddedWrongType,
				jsonish.Map{
					"attribute_path": attributePath,
					"attribute":      attributeName,
				},
				"Observable was not added for "+strconv.Quote(attributePath)+
					" because its value is not a supported scalar.",
			); err != nil {
				return err
			}
		}
		return nil
	}
	context.addObservable(
		&context.path, typeID, &valueString, p.pathNotation, p.enumSiblingsEnabled,
		p.deduplicateGenerated, p.reportObservableDuplicates,
	)
	return nil
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
) error {
	if !p.enumSiblingsEnabled || isArrayElement(arrayIndex) ||
		!schema.AttributeActive(attrDef.ResolvedEnumSibling, context.activeProfiles) {
		return nil
	}
	enumDetail, lookupStatus := lookupEnumDefinition(context, attrDef, value)
	reportOtherAddition := false
	siblingName := ""
	if attrDef.Sibling != nil {
		siblingName = *attrDef.Sibling
		siblingMissing := item[siblingName] == nil
		reportOtherAddition = siblingMissing &&
			attributeIsOtherEnumValue(value) &&
			enumDetail != nil && enumDetail.Caption != ""
		if siblingMissing &&
			(lookupStatus != enumLookupFound || enumDetail.Caption == "") &&
			!context.ignoresIssue(issueEnrichmentEnumSiblingNotAddedMask) {
			attributePath := context.path.String(pathstyle.ArrayIndexed)
			if err := context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentEnumSiblingNotAdded,
				jsonish.Map{
					"attribute_path": attributePath,
					"attribute":      attributeName,
					"sibling":        siblingName,
				},
				"Enum sibling "+strconv.Quote(siblingName)+
					" was not added because the enum value has no usable schema caption.",
			); err != nil {
				return err
			}
		}
	}
	context.addEnumSibling(item, enumDetail, attrDef)
	if reportOtherAddition && !context.ignoresIssue(issueEnrichmentEnumSiblingOtherAddedMask) {
		enumPath := context.path.String(pathstyle.ArrayIndexed)
		siblingPath := context.path.SiblingString(siblingName, pathstyle.ArrayIndexed)
		if err := context.addProcessorIssue(
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
		); err != nil {
			return err
		}
	}
	return nil
}

func (p *enrichmentProcessor) onEventDone(context *processContext, event jsonish.Map) error {
	if !p.observablesEnabled {
		return nil
	}

	existing := event["observables"]
	if existing == nil {
		return p.addGeneratedObservables(context, event, nil)
	}

	existingObservables, ok := observable.NewCollection(existing)
	if !ok {
		if len(context.observables) > 0 &&
			!context.ignoresIssue(issueEnrichmentObservablesNotAddedWrongTypeMask) {
			if err := context.addProcessorIssue(
				issue.SourceEnrichment,
				issue.EnrichmentObservablesNotAddedWrongType,
				jsonish.Map{
					"attribute_path":        "observables",
					"attribute":             "observables",
					"generated_observables": len(context.observables),
				},
				"Generated observables were not added because the event observables attribute is not an array.",
			); err != nil {
				return err
			}
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
	if len(context.observables) == 0 && !p.reportObservableDuplicates {
		return nil
	}
	generated := context.observables
	if p.reportObservableDuplicates {
		analysis := observable.AnalyzeDuplicates(existing, generated, p.deduplicateGenerated)
		for _, duplicate := range analysis.Duplicates {
			if err := p.reportObservableDuplicate(context, duplicate); err != nil {
				return err
			}
		}
		generated = analysis.AcceptedGenerated
	}
	return p.appendGeneratedObservables(context, event, existing, generated)
}

func (p *enrichmentProcessor) appendGeneratedObservables(
	context *processContext,
	event jsonish.Map,
	existing *observable.Collection,
	generated []jsonish.Map,
) error {
	if len(generated) == 0 {
		return nil
	}
	generatedObservablesFirstIndex := 0
	if existing == nil {
		event["observables"] = generated
	} else {
		appended, err := existing.Append(generated)
		if err != nil {
			return err
		}
		event["observables"] = appended
		generatedObservablesFirstIndex = existing.Len()
	}
	context.result.Enrichment.ObservablesAdded += len(generated)
	context.generatedObservablesFirstIndex = generatedObservablesFirstIndex
	return nil
}

func (p *enrichmentProcessor) reportObservableDuplicate(
	context *processContext,
	duplicate observable.Duplicate,
) error {
	attributePath := "observables[" + strconv.Itoa(duplicate.Occurrence.Index) + "]"
	attribute := "observables"
	description := "Existing observable " + strconv.Itoa(duplicate.Occurrence.Index)
	if duplicate.Occurrence.Origin == observable.ObservableOriginGenerated {
		attributePath = context.observableDiagnosticPath(duplicate.Occurrence.Index, duplicate.Name, p.pathNotation)
		attribute = terminalObservableAttribute(attributePath)
		description = "Generated observable for path " + strconv.Quote(attributePath)
	}
	return context.addProcessorIssue(
		issue.SourceEnrichment,
		issue.ObservableDuplicate,
		jsonish.Map{
			"attribute_path":      attributePath,
			"attribute":           attribute,
			"observable_origin":   string(duplicate.Occurrence.Origin),
			"observable_index":    duplicate.Occurrence.Index,
			"duplicate_of_origin": string(duplicate.First.Origin),
			"duplicate_of_index":  duplicate.First.Index,
		},
		description+" duplicates "+string(duplicate.First.Origin)+" observable "+
			strconv.Itoa(duplicate.First.Index)+".",
	)
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
	if item[sibling] != nil {
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
	deduplicateGenerated bool,
	reportObservableDuplicates bool,
) {
	name := path.String(pathNotation)
	if deduplicateGenerated && !reportObservableDuplicates &&
		!c.generatedObservableIdentities.Add(name, observableTypeID, value) {
		return
	}
	typeIDValue, present := c.compiled.ObservableTypeIDValue(observableTypeID)
	if !present {
		typeIDValue = observableTypeID
	}
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
	c.addGeneratedObservable(path, observable, pathNotation, reportObservableDuplicates)
}

func (c *processContext) addGeneratedObservable(
	path *eventpath.Path,
	observable jsonish.Map,
	style pathstyle.Style,
	reportObservableDuplicates bool,
) {
	index := len(c.observables)
	c.observables = append(c.observables, observable)
	if !reportObservableDuplicates {
		return
	}
	if style == pathstyle.ArrayIndexed || style == pathstyle.JSONPath || !path.HasArrayIndex() {
		if c.observableDiagnostics != nil {
			c.observableDiagnostics.generatedIndexedPaths = append(
				c.observableDiagnostics.generatedIndexedPaths, "",
			)
		}
		return
	}
	if c.observableDiagnostics == nil {
		c.observableDiagnostics = &observableDiagnosticState{
			generatedIndexedPaths: make([]string, index, index+1),
		}
	}
	c.observableDiagnostics.generatedIndexedPaths = append(
		c.observableDiagnostics.generatedIndexedPaths,
		path.String(pathstyle.ArrayIndexed),
	)
}

func (c *processContext) observableDiagnosticPath(index int, name string, style pathstyle.Style) string {
	if c.observableDiagnostics != nil && index < len(c.observableDiagnostics.generatedIndexedPaths) &&
		c.observableDiagnostics.generatedIndexedPaths[index] != "" {
		return c.observableDiagnostics.generatedIndexedPaths[index]
	}
	if style == pathstyle.JSONPath {
		return strings.TrimPrefix(name, "$.")
	}
	return name
}
