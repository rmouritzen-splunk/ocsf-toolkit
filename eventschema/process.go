package eventschema

import (
	"errors"
	"sort"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

var errNilEvent = errors.New("event is nil")

type processingContext struct {
	*schemaImpl
	pipeline *eventProcessorPipelineImpl
	result   ProcessingResult

	class                *classDefinition
	activeProfiles       map[string]struct{}
	classObservables     map[string]int64
	observables          []jsonish.Map
	observableResolution *observableResolution
	stopped              bool
}

type classVisit struct {
	event    jsonish.Map
	classUID int64
	profiles []string
	status   classVisitStatus
}

type processingDiagnostic struct {
	code    string
	message string
	details jsonish.Map
}

func newProcessingDiagnostic(code, message string, details jsonish.Map) *processingDiagnostic {
	return &processingDiagnostic{code: code, message: message, details: details}
}

func (c *processingContext) addProcessorIssue(phase string, diagnostic *processingDiagnostic) {
	issue := ProcessingIssue{
		Phase:    phase,
		Severity: issueSeverityWarning,
		Code:     diagnostic.code,
		Message:  diagnostic.message,
		Details:  diagnostic.details,
	}
	if attributePath, ok := diagnostic.details["attribute_path"].(string); ok {
		issue.AttributePath = attributePath
	}
	if attribute, ok := diagnostic.details["attribute"].(string); ok {
		issue.Attribute = attribute
	}
	if value, present := diagnostic.details["value"]; present {
		issue.Value = value
	}
	c.result.Issues = append(c.result.Issues, issue)
}

type classVisitStatus int

const (
	classVisitResolved classVisitStatus = iota
	classVisitUIDMissing
	classVisitUIDWrongType
	classVisitUIDUnknown
)

type itemVisit struct {
	item                    jsonish.Map
	validationParentPath    string
	itemDefinition          *commonItemDefinition
	validateItemConstraints bool
	allowUnknownAttributes  bool
}

type attributeVisit struct {
	item           jsonish.Map
	value          any
	validationPath string
	enrichmentPath string
	attributeName  string
	attrDef        *itemAttributeDefinition
	arrayIndex     int
	status         attributeVisitStatus
}

type attributeVisitStatus int

const (
	attributeVisitPresent attributeVisitStatus = iota
	attributeVisitMissing
	attributeVisitArrayWrongType
	attributeVisitEnum
	attributeVisitPrimitive
)

type objectVisit struct {
	value          any
	objectValue    jsonish.Map
	validationPath string
	enrichmentPath string
	attributeName  string
	attrDef        *itemAttributeDefinition
	objectDef      *objectDefinition
	objectType     string
	status         objectVisitStatus
}

type objectVisitStatus int

const (
	objectVisitValid objectVisitStatus = iota
	objectVisitWrongType
	objectVisitSchemaMissing
)

type eventProcessVisitor interface {
	onClass(*processingContext, classVisit)
	onClassDone(*processingContext, itemVisit)
	onObject(*processingContext, objectVisit)
	onObjectDone(*processingContext, itemVisit)
	onAttribute(*processingContext, attributeVisit)
	onEventDone(*processingContext, jsonish.Map)
}

type eventProcessVisitorBase struct{}

func (eventProcessVisitorBase) onClass(*processingContext, classVisit)         {}
func (eventProcessVisitorBase) onClassDone(*processingContext, itemVisit)      {}
func (eventProcessVisitorBase) onObject(*processingContext, objectVisit)       {}
func (eventProcessVisitorBase) onObjectDone(*processingContext, itemVisit)     {}
func (eventProcessVisitorBase) onAttribute(*processingContext, attributeVisit) {}
func (eventProcessVisitorBase) onEventDone(*processingContext, jsonish.Map)    {}

func (p *eventProcessorPipelineImpl) ProcessEvent(event jsonish.Map) (ProcessingResult, error) {
	if event == nil {
		return ProcessingResult{}, errNilEvent
	}

	context := processingContext{
		schemaImpl: p.schema,
		pipeline:   p,
	}

	context.resolveClass(event)
	if context.stopped {
		return context.result, nil
	}

	profiles := context.validateAndReturnProfiles(event)
	context.activeProfiles = makeProfileSet(profiles)
	context.visitClass(classVisit{event: event, classUID: context.class.Uid, profiles: profiles, status: classVisitResolved})

	context.processClass("", "", event, &context.class.commonItemDefinition)
	context.visitEventDone(event)

	return context.result, nil
}

func (c *processingContext) visitClass(visit classVisit) {
	for _, processor := range c.pipeline.transforms {
		processor.onClass(c, visit)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onClass(c, visit)
	}
}

func (c *processingContext) visitClassDone(visit itemVisit) {
	for _, processor := range c.pipeline.transforms {
		processor.onClassDone(c, visit)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onClassDone(c, visit)
	}
}

func (c *processingContext) visitObject(visit objectVisit) {
	for _, processor := range c.pipeline.transforms {
		processor.onObject(c, visit)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onObject(c, visit)
	}
}

func (c *processingContext) visitObjectDone(visit itemVisit) {
	for _, processor := range c.pipeline.transforms {
		processor.onObjectDone(c, visit)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onObjectDone(c, visit)
	}
}

func (c *processingContext) visitAttribute(visit attributeVisit) {
	for _, processor := range c.pipeline.transforms {
		processor.onAttribute(c, visit)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onAttribute(c, visit)
	}
}

func (c *processingContext) visitEventDone(event jsonish.Map) {
	for _, processor := range c.pipeline.transforms {
		processor.onEventDone(c, event)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onEventDone(c, event)
	}
}

func (p transformEventProcessor) onClass(context *processingContext, visit classVisit) {
	if p.enrichment != nil {
		p.enrichment.onClass(context, visit)
	} else {
		p.removal.onClass(context, visit)
	}
}

func (p transformEventProcessor) onClassDone(context *processingContext, visit itemVisit) {
	if p.enrichment != nil {
		p.enrichment.onClassDone(context, visit)
	} else {
		p.removal.onClassDone(context, visit)
	}
}

func (p transformEventProcessor) onObject(context *processingContext, visit objectVisit) {
	if p.enrichment != nil {
		p.enrichment.onObject(context, visit)
	} else {
		p.removal.onObject(context, visit)
	}
}

func (p transformEventProcessor) onObjectDone(context *processingContext, visit itemVisit) {
	if p.enrichment != nil {
		p.enrichment.onObjectDone(context, visit)
	} else {
		p.removal.onObjectDone(context, visit)
	}
}

func (p transformEventProcessor) onAttribute(context *processingContext, visit attributeVisit) {
	if p.enrichment != nil {
		p.enrichment.onAttribute(context, visit)
	} else {
		p.removal.onAttribute(context, visit)
	}
}

func (p transformEventProcessor) onEventDone(context *processingContext, event jsonish.Map) {
	if p.enrichment != nil {
		p.enrichment.onEventDone(context, event)
	} else {
		p.removal.onEventDone(context, event)
	}
}

func (c *processingContext) processClass(
	validationParentPath string,
	enrichmentParentPath string,
	item jsonish.Map,
	itemDefinition *commonItemDefinition,
) {
	c.processItem(validationParentPath, enrichmentParentPath, item, itemDefinition, false, false, c.visitClassDone)
}

func (c *processingContext) processItem(
	validationParentPath string,
	enrichmentParentPath string,
	item jsonish.Map,
	itemDefinition *commonItemDefinition,
	validateItemConstraints bool,
	allowUnknownAttributes bool,
	done func(itemVisit),
) {
	if itemDefinition == nil {
		return
	}
	metadata := itemDefinition.processing
	for _, attributeName := range metadata.attributeNames {
		attrDef := itemDefinition.Attributes[attributeName]
		if !c.attributeActive(attrDef) {
			continue
		}
		value, present := attributeValue(item, attributeName)
		validationPath, enrichmentPath := c.makeProcessingPaths(
			validationParentPath,
			enrichmentParentPath,
			attributeName,
		)

		if !present {
			c.visitAttribute(attributeVisit{
				item:           item,
				validationPath: validationPath,
				enrichmentPath: enrichmentPath,
				attributeName:  attributeName,
				attrDef:        attrDef,
				arrayIndex:     -1,
				status:         attributeVisitMissing,
			})
			continue
		}

		c.processAttribute(
			item,
			value,
			validationPath,
			enrichmentPath,
			attributeName,
			attrDef,
		)
	}

	done(itemVisit{
		item:                    item,
		validationParentPath:    validationParentPath,
		itemDefinition:          itemDefinition,
		validateItemConstraints: validateItemConstraints,
		allowUnknownAttributes:  allowUnknownAttributes,
	})
}

func (c *processingContext) makeProcessingPaths(
	validationParentPath string,
	enrichmentParentPath string,
	attributeName string,
) (string, string) {
	if c.pipeline.needsValidationPath && c.pipeline.needsEnrichmentPath &&
		validationParentPath == enrichmentParentPath {
		path := makeAttributePath(validationParentPath, attributeName)
		return path, path
	}
	var validationPath, enrichmentPath string
	if c.pipeline.needsValidationPath {
		validationPath = makeAttributePath(validationParentPath, attributeName)
	}
	if c.pipeline.needsEnrichmentPath {
		enrichmentPath = makeAttributePath(enrichmentParentPath, attributeName)
	}
	return validationPath, enrichmentPath
}

func (c *processingContext) attributeActive(attributeDefinition *itemAttributeDefinition) bool {
	if attributeDefinition == nil {
		return false
	}
	if len(attributeDefinition.Profiles) == 0 {
		return true
	}
	for _, profile := range attributeDefinition.Profiles {
		if _, present := c.activeProfiles[profile]; present {
			return true
		}
	}
	return false
}

func makeProfileSet(profiles []string) map[string]struct{} {
	if len(profiles) == 0 {
		return nil
	}
	profileSet := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		profileSet[profile] = struct{}{}
	}
	return profileSet
}

func sortedAttributeNames(attributes map[string]*itemAttributeDefinition) []string {
	names := make([]string, 0, len(attributes))
	for name := range attributes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (si *schemaImpl) ensureProcessingMetadata() {
	si.processingMetadataOnce.Do(func() {
		for _, class := range si.classes {
			if class != nil {
				class.processing = makeItemProcessingMetadata(&class.commonItemDefinition)
			}
		}
		for _, object := range si.objects {
			if object != nil {
				object.processing = makeItemProcessingMetadata(&object.commonItemDefinition)
			}
		}
	})
}

func makeItemProcessingMetadata(item *commonItemDefinition) itemProcessingMetadata {
	if item == nil {
		return itemProcessingMetadata{}
	}
	constraintKeys := make([]string, 0, len(item.Constraints))
	for key := range item.Constraints {
		constraintKeys = append(constraintKeys, key)
	}
	sort.Strings(constraintKeys)
	return itemProcessingMetadata{
		attributeNames: sortedAttributeNames(item.Attributes),
		constraintKeys: constraintKeys,
	}
}

func (c *processingContext) processAttribute(
	item jsonish.Map,
	value any,
	validationPath string,
	enrichmentPath string,
	attributeName string,
	attrDef *itemAttributeDefinition,
) {
	if attrDef == nil {
		return
	}

	c.visitAttribute(attributeVisit{
		item:           item,
		value:          value,
		validationPath: validationPath,
		enrichmentPath: enrichmentPath,
		attributeName:  attributeName,
		attrDef:        attrDef,
		arrayIndex:     -1,
		status:         attributeVisitPresent,
	})

	if attrDef.IsArray != nil && *attrDef.IsArray {
		c.processArray(item, value, validationPath, enrichmentPath, attributeName, attrDef)
		return
	}

	c.processValue(item, value, validationPath, enrichmentPath, attributeName, attrDef, -1)
}

func (c *processingContext) processArray(
	item jsonish.Map,
	value any,
	validationPath string,
	enrichmentPath string,
	attributeName string,
	attrDef *itemAttributeDefinition,
) {
	values, ok := newArrayView(value)
	if !ok {
		c.visitAttribute(attributeVisit{
			item:           item,
			value:          value,
			validationPath: validationPath,
			enrichmentPath: enrichmentPath,
			attributeName:  attributeName,
			attrDef:        attrDef,
			arrayIndex:     -1,
			status:         attributeVisitArrayWrongType,
		})
		return
	}

	for index := range values.Len() {
		element := values.At(index)
		elementValidationPath := validationPath
		if c.pipeline.needsValidationPath {
			elementValidationPath = makeArrayElementPath(validationPath, index)
		}
		c.processValue(item, element, elementValidationPath, enrichmentPath, attributeName, attrDef, index)
	}
}

func (c *processingContext) processValue(
	containingItem jsonish.Map,
	value any,
	validationPath string,
	enrichmentPath string,
	attributeName string,
	attrDef *itemAttributeDefinition,
	arrayIndex int,
) {
	visit := attributeVisit{
		item:           containingItem,
		value:          value,
		validationPath: validationPath,
		enrichmentPath: enrichmentPath,
		attributeName:  attributeName,
		attrDef:        attrDef,
		arrayIndex:     arrayIndex,
	}
	if attrDef.Enum != nil {
		visit.status = attributeVisitEnum
		c.visitAttribute(visit)
	}

	if attrDef.Type == "object_t" {
		c.processObjectValue(value, validationPath, enrichmentPath, attributeName, attrDef)
		return
	}

	visit.status = attributeVisitPrimitive
	c.visitAttribute(visit)
}

func (c *processingContext) processObjectValue(
	value any,
	validationPath string,
	enrichmentPath string,
	attributeName string,
	attrDef *itemAttributeDefinition,
) {
	objectValue, ok := value.(jsonish.Map)
	if !ok {
		c.visitObject(objectVisit{
			value:          value,
			validationPath: validationPath,
			enrichmentPath: enrichmentPath,
			attributeName:  attributeName,
			attrDef:        attrDef,
			status:         objectVisitWrongType,
		})
		return
	}

	if attrDef.ObjectType == nil {
		return
	}
	objectDef, present := c.objects[*attrDef.ObjectType]
	if !present || objectDef == nil {
		c.visitObject(objectVisit{
			value:          value,
			objectValue:    objectValue,
			validationPath: validationPath,
			enrichmentPath: enrichmentPath,
			attributeName:  attributeName,
			attrDef:        attrDef,
			objectType:     *attrDef.ObjectType,
			status:         objectVisitSchemaMissing,
		})
		return
	}

	c.visitObject(objectVisit{
		value:          value,
		objectValue:    objectValue,
		validationPath: validationPath,
		enrichmentPath: enrichmentPath,
		attributeName:  attributeName,
		attrDef:        attrDef,
		objectDef:      objectDef,
		objectType:     *attrDef.ObjectType,
		status:         objectVisitValid,
	})

	c.processItem(
		validationPath,
		enrichmentPath,
		objectValue,
		&objectDef.commonItemDefinition,
		true,
		*attrDef.ObjectType == "object",
		c.visitObjectDone,
	)
}
