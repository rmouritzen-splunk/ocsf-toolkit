package eventschema

import (
	"errors"
	"sort"

	"github.com/ocsf/ocsf-processor/jsonish"
)

var errNilEvent = errors.New("event is nil")

type processingContext struct {
	*schemaImpl
	result     ProcessingResult
	processors []eventProcessVisitor
	validation *validationProcessor
	enrichment *enrichmentProcessor

	class            *classDefinition
	activeProfiles   map[string]struct{}
	classObservables map[string]int64
	observables      []jsonish.Map
	stopped          bool
}

type classVisit struct {
	event    jsonish.Map
	classUID int64
	profiles []string
	status   classVisitStatus
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
	filteredAttributes      map[string]*itemAttributeDefinition
	validateItemConstraints bool
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

type eventProcessorImpl struct {
	schema     *schemaImpl
	processors []eventProcessVisitor
}

func (si *schemaImpl) NewEventProcessor(processes ...EventProcess) EventProcessor {
	var config processorConfig
	for _, process := range processes {
		if process != nil {
			process.applyProcess(&config)
		}
	}
	factories := make([]processorFactory, 0, len(config.factories)+len(config.validationFactories))
	factories = append(factories, config.factories...)
	factories = append(factories, config.validationFactories...)
	processors := make([]eventProcessVisitor, 0, len(factories))
	for _, factory := range factories {
		processors = append(processors, factory())
	}
	return &eventProcessorImpl{
		schema:     si,
		processors: processors,
	}
}

func (p *eventProcessorImpl) ProcessEvent(event jsonish.Map) (ProcessingResult, error) {
	return p.schema.processEvent(event, p.processors)
}

func (si *schemaImpl) processEvent(event jsonish.Map, processors []eventProcessVisitor) (ProcessingResult, error) {
	if event == nil {
		return ProcessingResult{}, errNilEvent
	}

	context := &processingContext{
		schemaImpl: si,
		processors: processors,
	}
	for _, processor := range processors {
		switch processor := processor.(type) {
		case *validationProcessor:
			context.validation = processor
		case *enrichmentProcessor:
			context.enrichment = processor
		}
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
	for _, processor := range c.processors {
		processor.onClass(c, visit)
	}
}

func (c *processingContext) visitClassDone(visit itemVisit) {
	for _, processor := range c.processors {
		processor.onClassDone(c, visit)
	}
}

func (c *processingContext) visitObject(visit objectVisit) {
	for _, processor := range c.processors {
		processor.onObject(c, visit)
	}
}

func (c *processingContext) visitObjectDone(visit itemVisit) {
	for _, processor := range c.processors {
		processor.onObjectDone(c, visit)
	}
}

func (c *processingContext) visitAttribute(visit attributeVisit) {
	for _, processor := range c.processors {
		processor.onAttribute(c, visit)
	}
}

func (c *processingContext) visitEventDone(event jsonish.Map) {
	for _, processor := range c.processors {
		processor.onEventDone(c, event)
	}
}

func (c *processingContext) processClass(
	validationParentPath string,
	enrichmentParentPath string,
	item jsonish.Map,
	itemDefinition *commonItemDefinition,
) {
	c.processItem(validationParentPath, enrichmentParentPath, item, itemDefinition, false, c.visitClassDone)
}

func (c *processingContext) processItem(
	validationParentPath string,
	enrichmentParentPath string,
	item jsonish.Map,
	itemDefinition *commonItemDefinition,
	validateItemConstraints bool,
	done func(itemVisit),
) {
	if itemDefinition == nil {
		return
	}
	filteredAttributes := c.filterAttributes(itemDefinition.Attributes)

	attributeNames := sortedAttributeNames(filteredAttributes)
	for _, attributeName := range attributeNames {
		attrDef := filteredAttributes[attributeName]
		value, present := item[attributeName]
		validationPath := makeAttributePath(validationParentPath, attributeName)
		enrichmentPath := makeAttributePath(enrichmentParentPath, attributeName)

		if !present || value == nil {
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
		filteredAttributes:      filteredAttributes,
		validateItemConstraints: validateItemConstraints,
	})
}

func (c *processingContext) filterAttributes(
	attributes map[string]*itemAttributeDefinition,
) map[string]*itemAttributeDefinition {
	if len(attributes) == 0 {
		return attributes
	}
	filtered := make(map[string]*itemAttributeDefinition, len(attributes))
	for attributeName, attributeDefinition := range attributes {
		if attributeDefinition == nil {
			continue
		}
		if len(attributeDefinition.Profiles) == 0 {
			filtered[attributeName] = attributeDefinition
			continue
		}
		for _, profile := range attributeDefinition.Profiles {
			if _, present := c.activeProfiles[profile]; present {
				filtered[attributeName] = attributeDefinition
				break
			}
		}
	}
	return filtered
}

func makeProfileSet(profiles []string) map[string]struct{} {
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
	values, ok := asSlice(value)
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

	for index, element := range values {
		elementValidationPath := makeArrayElementPath(validationPath, index)
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

	c.processItem(validationPath, enrichmentPath, objectValue, &objectDef.commonItemDefinition, true, c.visitObjectDone)
}
