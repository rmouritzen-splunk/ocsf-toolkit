package processing

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
)

var errNilEvent = errors.New("event is nil")
var errUninitializedSchema = errors.New("schema is not initialized; create it with eventpipeline.NewSchema")
var errUninitializedPipeline = errors.New(
	"event processor pipeline is not initialized; create it with eventpipeline.NewPipeline",
)

type enumLookupStatus uint8

const (
	enumLookupFound enumLookupStatus = iota
	enumLookupValueUnusable
	enumLookupDefinitionMissing
)

type processContext struct {
	compiled *schema.Compiled
	pipeline *Pipeline
	result   Result

	class               *schema.ClassDefinition
	activeProfiles      schema.ProfileSet
	classObservableTrie *classObservableTrie
	observables         []jsonish.Map
	// observableDiagnosticPaths is allocated only when the configured observable notation omits concrete array
	// indexes. Non-empty entries retain the canonical indexed source path for diagnostics.
	observableDiagnosticPaths []string
	// generatedObservablesStart identifies a suffix appended by this processing pass and therefore semantically
	// valid by construction. A negative value means no trusted suffix exists. Existing observables retain their
	// original order.
	generatedObservablesStart        int
	generatedObservablesPathNotation pathstyle.Style
	processingTraversalLimitReported bool
	path                             eventpath.Path
	// jsonNumberStyleKnown and jsonNumberUsesFloatSyntax record whether the first numeric enum json.Number seen in
	// this event contained .eE. They only select whether later lookups try the exact schema string map before
	// normalization; misses always normalize, so mixed encodings remain correct.
	jsonNumberStyleKnown      bool
	jsonNumberUsesFloatSyntax bool
}

// Result is the internal mutable result accumulated while processing an event. The public eventpipeline result wraps
// these values in an opaque concrete value after traversal completes.
type Result struct {
	Validation        eventresult.ValidationResult
	Enrichment        eventresult.EnrichmentResult
	EnrichmentRemoval eventresult.EnrichmentRemovalResult
	Issues            []eventresult.ProcessingIssue
	SuppressedIssues  int
}

// addProcessorIssue appends a diagnostic that its caller has already decided to report. Callers must apply any
// applicable issue suppression before constructing the diagnostic so suppressed paths avoid rendering paths, formatting
// messages, and allocating detail maps. This final append intentionally does not repeat the suppression check.
func (c *processContext) addProcessorIssue(
	source issue.Source,
	code issue.IssueCode,
	details jsonish.Map,
	message string,
) {
	issue := eventresult.ProcessingIssue{
		Source:  source,
		Code:    code,
		Message: sanitizeDiagnosticMessage(message),
		Details: details,
	}
	c.result.Issues = append(c.result.Issues, issue)
}

func (c *processContext) suppressesIssue(suppression issueSuppression, code issue.IssueCode) bool {
	if !suppression.suppresses(code) {
		return false
	}
	c.result.SuppressedIssues++
	return true
}

func lookupEnumDefinition(
	context *processContext,
	attrDef *schema.ItemAttributeDefinition,
	value any,
) (*schema.EnumDefinition, enumLookupStatus) {
	switch attrDef.PrimitiveType {
	case "string_t":
		text, ok := eventvalue.AsString(value)
		if !ok {
			return nil, enumLookupValueUnusable
		}
		definition := attrDef.Enum[text]
		if definition == nil {
			return nil, enumLookupDefinitionMissing
		}
		return definition, enumLookupFound
	case "integer_t", "long_t":
		return lookupNumericEnumDefinition(context, attrDef, value)
	default:
		text, ok := eventvalue.FormatScalar(value)
		if !ok {
			return nil, enumLookupValueUnusable
		}
		definition := attrDef.Enum[text]
		if definition == nil {
			return nil, enumLookupDefinitionMissing
		}
		return definition, enumLookupFound
	}
}

func lookupNumericEnumDefinition(
	context *processContext,
	attrDef *schema.ItemAttributeDefinition,
	value any,
) (*schema.EnumDefinition, enumLookupStatus) {
	if number, ok := value.(json.Number); ok {
		text := number.String()
		if !context.jsonNumberStyleKnown {
			context.jsonNumberStyleKnown = true
			context.jsonNumberUsesFloatSyntax = strings.ContainsAny(text, ".eE")
		}
		if !context.jsonNumberUsesFloatSyntax && attrDef.NumericEnumKeysCanonical() {
			if definition := attrDef.Enum[text]; definition != nil {
				return definition, enumLookupFound
			}
		}
	}
	var integer int64
	var ok bool
	if attrDef.PrimitiveType == "long_t" {
		integer, ok = eventvalue.AsLong(value)
	} else {
		integer, ok = eventvalue.AsInteger(value)
	}
	if !ok {
		return nil, enumLookupValueUnusable
	}
	definition := attrDef.NumericEnumDefinition(integer)
	if definition == nil {
		return nil, enumLookupDefinitionMissing
	}
	return definition, enumLookupFound
}

func attributeIsOtherEnumValue(value any) bool {
	return eventvalue.IsOtherEnumValue(value)
}

func isArrayElement(arrayIndex int) bool {
	return arrayIndex != -1
}

func isScalarArrayAttribute(attrDef *schema.ItemAttributeDefinition) bool {
	switch attrDef.PrimitiveType {
	case "boolean_t", "float_t", "integer_t", "long_t", "string_t":
		return true
	default:
		return false
	}
}

type attributeState int

const (
	attributePresent attributeState = iota
	attributeMissing
	attributeArrayWrongType
	attributeEnum
	attributePrimitive
)

// ProcessEvent adds or removes enrichment and/or validates event in place.
//
// The event map and any nested maps or slices it contains must not be accessed or mutated
// concurrently while ProcessEvent is running. Events are nested structures of map[string]any
// objects, slices, fixed arrays, and supported primitives: nil, bool, string, json.Number, signed
// Go integer types, float32, and float64. jsonish.Map is a helper alias for map[string]any, not a
// required distinct type. OCSF integer_t and long_t values use signed 64-bit semantics. Unsigned
// integers, structs, and other Go map types are not supported. When source data is JSON, prefer
// the jsonio decoding helpers so numbers remain json.Number.
//
// Processing is not transactional. When enrichment removal, enrichment, or future mutating
// processors are enabled, the event may be partially modified if ProcessEvent returns an error.
// Callers that need to preserve the original event should deep-copy it before processing.
//
// Invalid events are reported in the returned Result. The public eventpipeline pipeline exposes it as an opaque
// eventpipeline.ProcessingResult. The error return is for an
// uninitialized pipeline, processor failures, or unusable caller input, not OCSF validation
// failures. Results and errors do not repeat event values in diagnostic text or details.
func (p *Pipeline) ProcessEvent(event jsonish.Map) (Result, error) {
	if p == nil || p.compiled == nil {
		return Result{}, errUninitializedPipeline
	}
	if event == nil {
		return Result{}, errNilEvent
	}
	context := processContext{
		compiled:                  p.compiled,
		pipeline:                  p,
		generatedObservablesStart: -1,
	}

	classResolved, err := context.resolveClass(event)
	if err != nil {
		return context.result, err
	}
	if !classResolved {
		return context.result, nil
	}

	context.activeProfiles = schema.EventProfileSet(event)
	for _, processor := range p.mutations {
		if err := processor.onClass(&context, event); err != nil {
			return context.result, err
		}
	}
	if p.validation != nil {
		p.validation.onClass(&context, event)
	}

	if p.requiresEventWalk {
		if err := context.visitItem(event, &context.class.ItemDefinition); err != nil {
			return context.result, err
		}
		if err := context.visitClassDone(event, &context.class.ItemDefinition); err != nil {
			return context.result, err
		}
	}
	if err := context.visitEventDone(event); err != nil {
		return context.result, err
	}

	return context.result, nil
}

func (c *processContext) resolveClass(event jsonish.Map) (bool, error) {
	class, _, status := c.compiled.ResolveEventClass(event)
	switch status {
	case schema.ClassUIDMissing:
		if c.pipeline.validation != nil {
			c.pipeline.validation.onClassUIDMissing(c)
		}
		c.addProcessorIssue(
			issue.SourceProcessing,
			issue.ClassUIDMissing,
			jsonish.Map{"attribute_path": "class_uid", "attribute": "class_uid"},
			`The "class_uid" attribute is missing, preventing further event processing.`,
		)
		return false, nil
	case schema.ClassUIDWrongType:
		if c.pipeline.validation != nil {
			c.pipeline.validation.onClassUIDWrongType(c, event["class_uid"])
		}
		c.addProcessorIssue(
			issue.SourceProcessing,
			issue.ClassUIDWrongType,
			jsonish.Map{"attribute_path": "class_uid", "attribute": "class_uid"},
			`The "class_uid" attribute has the wrong type, preventing further event processing.`,
		)
		return false, nil
	case schema.ClassUIDUnknown:
		if c.pipeline.validation != nil {
			c.pipeline.validation.onClassUIDUnknown(c)
		}
		c.addProcessorIssue(
			issue.SourceProcessing,
			issue.ClassUIDUnknown,
			jsonish.Map{"attribute_path": "class_uid", "attribute": "class_uid"},
			`The "class_uid" value does not identify a class in the schema, preventing further event processing.`,
		)
		return false, nil
	case schema.ClassResolved:
		c.class = class
		return true, nil
	default:
		return false, fmt.Errorf("unexpected class resolution status %d", status)
	}
}

func (c *processContext) visitClassDone(
	item jsonish.Map,
	itemDefinition *schema.ItemDefinition,
) error {
	for _, processor := range c.pipeline.mutations {
		if err := processor.onClassDone(c, item, itemDefinition); err != nil {
			return err
		}
	}
	c.visitUnknownAttributes(item, itemDefinition)
	return nil
}

func (c *processContext) visitObject(
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	objectDef *schema.ObjectDefinition,
) {
	for _, processor := range c.pipeline.mutations {
		processor.onObject(c, attributeName, attrDef, objectDef)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onObject(c, attributeName, objectDef)
	}
}

func (c *processContext) visitObjectWrongType(
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	for _, processor := range c.pipeline.mutations {
		processor.onObjectWrongType(c, attributeName, attrDef)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onObjectWrongType(c, value, attributeName, attrDef)
	}
}

func (c *processContext) visitObjectDone(
	item jsonish.Map,
	objectDefinition *schema.ObjectDefinition,
	objectType string,
) {
	if c.pipeline.validation != nil {
		if objectType != "object" {
			c.visitUnknownAttributes(item, &objectDefinition.ItemDefinition)
		}
		c.pipeline.validation.onObjectDone(c, item, objectDefinition)
	}
}

func (c *processContext) visitEventDone(event jsonish.Map) error {
	for _, processor := range c.pipeline.mutations {
		if err := processor.onEventDone(c, event); err != nil {
			return err
		}
	}
	if c.pipeline.validation != nil {
		return c.pipeline.validation.onEventDone(c, event)
	}
	return nil
}

func (c *processContext) visitItem(
	item jsonish.Map,
	itemDefinition *schema.ItemDefinition,
) error {
	if itemDefinition == nil {
		return nil
	}
	for _, attribute := range itemDefinition.OrderedAttributes {
		attributeName := attribute.Name
		attrDef := attribute.Definition
		if !schema.AttributeActive(attrDef, c.activeProfiles) {
			value, present := eventvalue.Attribute(item, attributeName)
			if present && c.pipeline.validation != nil {
				if attrDef == nil {
					c.pipeline.validation.onUnknownAttribute(c, attributeName, itemDefinition)
				} else {
					c.path.PushAttribute(attributeName)
					c.pipeline.validation.onInactiveAttribute(c, item, value, attributeName, attrDef, itemDefinition)
					c.path.Pop()
				}
			}
			continue
		}
		if attrDef != nil && attrDef.ResolvedEnumAttribute != nil &&
			schema.AttributeActive(attrDef.ResolvedEnumAttribute, c.activeProfiles) {
			// The linked enum owns the pair dispatch.
			continue
		}
		if attrDef != nil && attrDef.ResolvedEnumSibling != nil &&
			schema.AttributeActive(attrDef.ResolvedEnumSibling, c.activeProfiles) {
			if err := c.visitEnumSiblingPairAttributes(item, attributeName, attrDef); err != nil {
				return err
			}
			continue
		}
		value, present := eventvalue.Attribute(item, attributeName)
		c.path.PushAttribute(attributeName)

		if !present {
			if err := c.visitAttribute(item, nil, attributeName, attrDef, -1, attributeMissing); err != nil {
				return err
			}
		} else {
			if err := c.visitAttribute(item, value, attributeName, attrDef, -1, attributePresent); err != nil {
				return err
			}
		}
		c.path.Pop()
	}
	return nil
}

// visitEnumSiblingPairAttributes fires the alternative pair visit for an enum and its resolved
// string sibling of the same scalar or array shape. Processors read the current values synchronously, so later
// processors observe earlier mutations.
func (c *processContext) visitEnumSiblingPairAttributes(
	item jsonish.Map,
	enumAttributeName string,
	enumAttrDef *schema.ItemAttributeDefinition,
) error {
	siblingAttrDef := enumAttrDef.ResolvedEnumSibling
	siblingAttributeName := *enumAttrDef.Sibling

	for _, processor := range c.pipeline.mutations {
		if err := processor.onEnumSiblingPairAttributes(
			c, item, enumAttributeName, enumAttrDef, siblingAttributeName, siblingAttrDef,
		); err != nil {
			return err
		}
	}
	if c.pipeline.validation != nil {
		return c.pipeline.validation.onEnumSiblingPairAttributes(
			c, item, enumAttributeName, enumAttrDef, siblingAttributeName, siblingAttrDef,
		)
	}
	return nil
}

func (c *processContext) visitAttribute(
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
	status attributeState,
) error {
	if status < attributePresent || status > attributePrimitive {
		return fmt.Errorf("unexpected attribute state %d", status)
	}
	if status == attributePresent && attrDef == nil {
		return fmt.Errorf("present attribute %q has no definition", attributeName)
	}

	for _, processor := range c.pipeline.mutations {
		processor.onAttribute(c, item, value, attributeName, attrDef, arrayIndex, status)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.onAttribute(c, item, value, attributeName, attrDef, arrayIndex, status)
	}
	if status != attributePresent {
		return nil
	}

	if attrDef.IsArray != nil && *attrDef.IsArray {
		return c.visitArray(item, value, attributeName, attrDef)
	}

	return c.visitValue(item, value, attributeName, attrDef, -1)
}

func (c *processContext) visitArray(
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) error {
	values, ok := eventvalue.NewArrayView(value)
	if !ok {
		return c.visitAttribute(item, value, attributeName, attrDef, -1, attributeArrayWrongType)
	}
	if isScalarArrayAttribute(attrDef) {
		for index := range values.Len() {
			c.path.PushArrayIndex(index)
			err := c.visitScalarArrayValue(item, values, attributeName, attrDef, index)
			c.path.Pop()
			if err != nil {
				return err
			}
		}
		return nil
	}

	for index := range values.Len() {
		c.path.PushArrayIndex(index)
		element := values.At(index)
		if err := c.visitValue(item, element, attributeName, attrDef, index); err != nil {
			return err
		}
		c.path.Pop()
	}
	return nil
}

func (c *processContext) visitScalarArrayValue(
	containingItem jsonish.Map,
	values eventvalue.ArrayView,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
) error {
	if attrDef.Enum != nil {
		for _, processor := range c.pipeline.mutations {
			processor.onArrayElement(c, values, arrayIndex, attributeName, attrDef, attributeEnum)
		}
		if c.pipeline.validation != nil {
			c.pipeline.validation.validateArrayEnum(
				c, containingItem, values, arrayIndex, attributeName, attrDef,
			)
		}
	}
	for _, processor := range c.pipeline.mutations {
		processor.onArrayElement(c, values, arrayIndex, attributeName, attrDef, attributePrimitive)
	}
	if c.pipeline.validation != nil {
		c.pipeline.validation.validateArrayPrimitiveValue(
			c, values, arrayIndex, attributeName, attrDef, &c.path,
		)
	}
	return nil
}

func (c *processContext) visitValue(
	containingItem jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
) error {
	if attrDef.Enum != nil {
		if err := c.visitAttribute(
			containingItem, value, attributeName, attrDef, arrayIndex, attributeEnum,
		); err != nil {
			return err
		}
	}

	if attrDef.Type == "object_t" {
		return c.visitObjectValue(value, attributeName, attrDef)
	}

	return c.visitAttribute(containingItem, value, attributeName, attrDef, arrayIndex, attributePrimitive)
}

func (c *processContext) visitObjectValue(
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) error {
	objectValue, ok := value.(jsonish.Map)
	if !ok {
		c.visitObjectWrongType(value, attributeName, attrDef)
		return nil
	}

	if attrDef.ObjectType == nil {
		return nil
	}
	objectDef := attrDef.ResolvedObject
	if objectDef == nil {
		if c.pipeline.validation != nil {
			c.pipeline.validation.onObjectSchemaMissing(c, attributeName, *attrDef.ObjectType)
		}
		return nil
	}

	// Every occurrence of an OCSF data dictionary attribute has the same type. A repeated object attribute name on the
	// current path therefore recurs through the same object relationship. Thus recursion can be detected by looking for
	// repeats of this object's attribute name on the event path.
	if c.path.HasPriorAttribute(attributeName) {
		attributePath := c.path.String(pathstyle.ArrayIndexed)
		c.addProcessingTraversalLimitIssue(attributePath, attributeName, *attrDef.ObjectType)
		return nil
	}

	c.visitObject(attributeName, attrDef, objectDef)
	if err := c.visitItem(objectValue, &objectDef.ItemDefinition); err != nil {
		return err
	}
	c.visitObjectDone(objectValue, objectDef, *attrDef.ObjectType)
	return nil
}

func (c *processContext) visitUnknownAttributes(item jsonish.Map, itemDefinition *schema.ItemDefinition) {
	if c.pipeline.validation == nil {
		return
	}
	var attributeNames []string
	for attributeName, value := range item {
		if value == nil {
			continue
		}
		if _, defined := itemDefinition.Attributes[attributeName]; defined {
			continue
		}
		attributeNames = append(attributeNames, attributeName)
	}
	sort.Strings(attributeNames)
	for _, attributeName := range attributeNames {
		c.pipeline.validation.onUnknownAttribute(c, attributeName, itemDefinition)
	}
}

func (c *processContext) addProcessingTraversalLimitIssue(attributePath, attribute, objectType string) {
	if c.processingTraversalLimitReported {
		return
	}
	c.processingTraversalLimitReported = true
	details := jsonish.Map{"attribute_path": attributePath, "attribute": attribute}
	if objectType != "" {
		details["object_type"] = objectType
	}
	c.addProcessorIssue(
		issue.SourceProcessing,
		issue.EventTraversalLimited,
		details,
		"Event processing did not inspect content beyond at least one recursive object relationship"+
			" because it reached the supported traversal limit;"+
			" the first affected path was "+strconv.Quote(attributePath)+".",
	)
}
