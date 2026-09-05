package processing

import (
	"fmt"
	"strconv"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/observable"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
)

type enumSiblingRetentionReason uint8

const (
	invalidEnumSiblingRetentionReason enumSiblingRetentionReason = iota
	enumSiblingRetentionReasonSiblingValueWrongType
	enumSiblingRetentionReasonEnumValueMissing
	enumSiblingRetentionReasonEnumValueWrongType
	enumSiblingRetentionReasonEnumValueUnknown
	enumSiblingRetentionReasonSiblingValueMismatch
	enumSiblingRetentionReasonCount
)

var enumSiblingRetentionReasonNames = [enumSiblingRetentionReasonCount]string{
	enumSiblingRetentionReasonSiblingValueWrongType: "sibling_value_wrong_type",
	enumSiblingRetentionReasonEnumValueMissing:      "enum_value_missing",
	enumSiblingRetentionReasonEnumValueWrongType:    "enum_value_wrong_type",
	enumSiblingRetentionReasonEnumValueUnknown:      "enum_value_unknown",
	enumSiblingRetentionReasonSiblingValueMismatch:  "sibling_value_mismatch",
}

func (reason enumSiblingRetentionReason) String() string {
	if reason <= invalidEnumSiblingRetentionReason || reason >= enumSiblingRetentionReasonCount {
		return ""
	}
	return enumSiblingRetentionReasonNames[reason]
}

// enrichmentSafeRemovalProcessor removes enum siblings and observables that it can verify against the schema,
// retaining anything it cannot verify.
type enrichmentSafeRemovalProcessor struct {
	enumSiblingsEnabled bool
	observablesEnabled  bool
	// deferObservablesRemoval is set by pipeline construction (see NewPipelineImpl) when a separate unit in this
	// pipeline adds enum siblings during the attribute walk. Analyzing observables against sibling data requires
	// that walk to finish first, so this unit's observable removal moves from onClass to onClassDone instead of
	// running ahead of its own enum-sibling removal below.
	deferObservablesRemoval bool
}

func (p *enrichmentSafeRemovalProcessor) onClass(context *processContext, event jsonish.Map) error {
	// Analyze and remove observables before other event mutation so references to removable enum
	// siblings are evaluated against the original event. Validation then walks only retained entries.
	if !p.deferObservablesRemoval {
		return removeObservablesSafely(context, event, p.observablesEnabled)
	}
	return nil
}

func (p *enrichmentSafeRemovalProcessor) onClassDone(context *processContext, item jsonish.Map) error {
	if p.deferObservablesRemoval {
		return removeObservablesSafely(context, item, p.observablesEnabled)
	}
	return nil
}

func (p *enrichmentSafeRemovalProcessor) onAttribute(
	context *processContext,
	item jsonish.Map,
	_ string,
	attrDef *schema.ItemAttributeDefinition,
	status attributeState,
) {
	retainUnsupportedEnumSibling(context, item, attrDef, status, p.enumSiblingsEnabled)
}

func (p *enrichmentSafeRemovalProcessor) onEnumSiblingPairAttributes(
	context *processContext,
	item jsonish.Map,
	enumAttributeName string,
	enumAttrDef *schema.ItemAttributeDefinition,
	siblingAttributeName string,
) error {
	return removeEnumSiblingPair(
		context, item, enumAttributeName, enumAttrDef, siblingAttributeName,
		p.enumSiblingsEnabled, false,
	)
}

// enrichmentForceRemovalProcessor removes supported scalar and array enums with string siblings and observables
// without verifying them against the schema. An enum sibling whose scalar or array enum value includes 99 ("Other")
// is always retained because it contains unverifiable source data rather than enrichment output.
type enrichmentForceRemovalProcessor struct {
	enumSiblingsEnabled     bool
	observablesEnabled      bool
	deferObservablesRemoval bool
}

func (p *enrichmentForceRemovalProcessor) onClass(context *processContext, event jsonish.Map) {
	if !p.deferObservablesRemoval {
		forceRemoveObservablesWithoutAnalysis(context, event, p.observablesEnabled)
	}
}

func (p *enrichmentForceRemovalProcessor) onClassDone(context *processContext, item jsonish.Map) {
	if p.deferObservablesRemoval {
		forceRemoveObservablesWithoutAnalysis(context, item, p.observablesEnabled)
	}
}

func (p *enrichmentForceRemovalProcessor) onAttribute(
	context *processContext,
	item jsonish.Map,
	_ string,
	attrDef *schema.ItemAttributeDefinition,
	status attributeState,
) {
	retainUnsupportedEnumSibling(context, item, attrDef, status, p.enumSiblingsEnabled)
}

func (p *enrichmentForceRemovalProcessor) onEnumSiblingPairAttributes(
	context *processContext,
	item jsonish.Map,
	enumAttributeName string,
	enumAttrDef *schema.ItemAttributeDefinition,
	siblingAttributeName string,
) error {
	return removeEnumSiblingPair(
		context, item, enumAttributeName, enumAttrDef, siblingAttributeName,
		p.enumSiblingsEnabled, true,
	)
}

func removeEnumSiblingPair(
	context *processContext,
	item jsonish.Map,
	enumAttributeName string,
	enumAttrDef *schema.ItemAttributeDefinition,
	siblingAttributeName string,
	enabled bool,
	force bool,
) error {
	if !enabled {
		return nil
	}
	siblingValue, siblingPresent := item[siblingAttributeName]
	if !siblingPresent {
		return nil
	}
	if siblingValue == nil {
		delete(item, siblingAttributeName)
		context.result.EnrichmentRemoval.EnumSiblingsRemoved++
		return nil
	}
	if enumAttrDef.IsArray != nil && *enumAttrDef.IsArray {
		return removeEnumArraySiblingPair(
			context, item, enumAttributeName, enumAttrDef, siblingAttributeName, siblingValue, force,
		)
	}
	siblingString, ok := eventvalue.AsString(siblingValue)
	if !ok {
		return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
			enumSiblingRetentionReasonSiblingValueWrongType, "")
	}
	enumValue, enumPresent := eventvalue.Attribute(item, enumAttributeName)
	if !enumPresent {
		return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
			enumSiblingRetentionReasonEnumValueMissing, "")
	}
	enumDetail, other, ok := enumSiblingValueDefinition(context, enumAttrDef, enumValue)
	if !ok {
		return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
			enumSiblingRetentionReasonEnumValueWrongType, "")
	}
	if other {
		context.result.EnrichmentRemoval.EnumSiblingsRetained++
		return nil
	}
	if !force {
		if enumDetail == nil {
			return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
				enumSiblingRetentionReasonEnumValueUnknown, "")
		}
		if siblingString != enumDetail.Caption {
			return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
				enumSiblingRetentionReasonSiblingValueMismatch, enumDetail.Caption)
		}
	}

	delete(item, siblingAttributeName)
	context.result.EnrichmentRemoval.EnumSiblingsRemoved++
	return nil
}

func enumSiblingValueDefinition(
	context *processContext,
	attrDef *schema.ItemAttributeDefinition,
	value any,
) (definition *schema.EnumDefinition, other bool, ok bool) {
	definition, lookupStatus := lookupEnumDefinition(context, attrDef, value)
	other = (attrDef.PrimitiveType == "integer_t" || attrDef.PrimitiveType == "long_t") &&
		eventvalue.IsOtherEnumValue(value)
	return definition, other, lookupStatus != enumLookupValueUnusable
}

func removeEnumArraySiblingPair(
	context *processContext,
	item jsonish.Map,
	enumAttributeName string,
	enumAttrDef *schema.ItemAttributeDefinition,
	siblingAttributeName string,
	siblingValue any,
	force bool,
) error {
	siblingValues, ok := eventvalue.NewArrayView(siblingValue)
	if !ok {
		return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
			enumSiblingRetentionReasonSiblingValueWrongType, "")
	}
	enumValue, enumPresent := eventvalue.Attribute(item, enumAttributeName)
	if !enumPresent {
		return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
			enumSiblingRetentionReasonEnumValueMissing, "")
	}
	enumValues, ok := eventvalue.NewArrayView(enumValue)
	if !ok {
		return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
			enumSiblingRetentionReasonEnumValueWrongType, "")
	}
	if !force && enumValues.Len() != siblingValues.Len() {
		return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
			enumSiblingRetentionReasonSiblingValueMismatch, "an equally sized caption array")
	}
	for index := range enumValues.Len() {
		detail, other, valid := enumArraySiblingValueDefinition(
			context, enumAttrDef, enumValues, index,
		)
		if !valid {
			return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
				enumSiblingRetentionReasonEnumValueWrongType, "")
		}
		if other {
			context.result.EnrichmentRemoval.EnumSiblingsRetained++
			return nil
		}
		if force {
			continue
		}
		if detail == nil {
			return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
				enumSiblingRetentionReasonEnumValueUnknown, "")
		}
		siblingString, stringValue := siblingValues.AsStringAt(index)
		if !stringValue || siblingString != detail.Caption {
			return recordEnumSiblingRetention(context, &context.path, enumAttributeName, siblingAttributeName,
				enumSiblingRetentionReasonSiblingValueMismatch, detail.Caption)
		}
	}
	delete(item, siblingAttributeName)
	context.result.EnrichmentRemoval.EnumSiblingsRemoved++
	return nil
}

func enumArraySiblingValueDefinition(
	context *processContext,
	attrDef *schema.ItemAttributeDefinition,
	values eventvalue.ArrayView,
	index int,
) (definition *schema.EnumDefinition, other bool, ok bool) {
	value := values.At(index)
	definition, lookupStatus := lookupEnumDefinition(context, attrDef, value)
	switch attrDef.PrimitiveType {
	case "integer_t":
		integer, integral := values.AsIntegerAt(index)
		other = integral && integer == 99
	case "long_t":
		integer, integral := values.AsLongAt(index)
		other = integral && integer == 99
	}
	return definition, other, lookupStatus != enumLookupValueUnusable
}

func retainUnsupportedEnumSibling(
	context *processContext,
	item jsonish.Map,
	attrDef *schema.ItemAttributeDefinition,
	status attributeState,
	enabled bool,
) {
	if !enabled || status != attributePresent || attrDef == nil || attrDef.Enum == nil ||
		attrDef.Sibling == nil || attrDef.ResolvedEnumSibling != nil {
		return
	}
	siblingName := *attrDef.Sibling
	siblingValue, present := item[siblingName]
	if !present {
		return
	}
	if siblingValue == nil {
		delete(item, siblingName)
		context.result.EnrichmentRemoval.EnumSiblingsRemoved++
		return
	}
	context.result.EnrichmentRemoval.EnumSiblingsRetained++
}

func recordEnumSiblingRetention(
	context *processContext,
	path *eventpath.Path,
	enumAttribute string,
	siblingAttribute string,
	reason enumSiblingRetentionReason,
	expectedCaption string,
) error {
	if reason <= invalidEnumSiblingRetentionReason || reason >= enumSiblingRetentionReasonCount {
		return fmt.Errorf("unexpected enum sibling retention reason %d", reason)
	}
	context.result.EnrichmentRemoval.EnumSiblingsRetained++
	if context.ignoresIssue(issueEnrichmentRemovalEnumSiblingNotRemovedMask) {
		return nil
	}

	var message string
	switch reason {
	case enumSiblingRetentionReasonSiblingValueWrongType:
		message = "Enum sibling " + strconv.Quote(siblingAttribute) +
			" was not removed because its value is not a string."
	case enumSiblingRetentionReasonEnumValueMissing:
		message = "Enum sibling " + strconv.Quote(siblingAttribute) + " was not removed because enum attribute " +
			strconv.Quote(enumAttribute) + " is missing."
	case enumSiblingRetentionReasonEnumValueWrongType:
		message = "Enum sibling " + strconv.Quote(siblingAttribute) + " was not removed because enum attribute " +
			strconv.Quote(enumAttribute) + " has the wrong value type."
	case enumSiblingRetentionReasonEnumValueUnknown:
		message = "Enum sibling " + strconv.Quote(siblingAttribute) +
			" was not removed because the enum value is not defined."
	case enumSiblingRetentionReasonSiblingValueMismatch:
		message = "Enum sibling " + strconv.Quote(siblingAttribute) +
			" was not removed because its value does not match schema caption " + strconv.Quote(expectedCaption) + "."
	default:
		return fmt.Errorf("unexpected enum sibling retention reason %d", reason)
	}
	details := jsonish.Map{
		"attribute_path":      path.ChildString(siblingAttribute, pathstyle.ArrayIndexed),
		"attribute":           siblingAttribute,
		"enum_attribute_path": path.ChildString(enumAttribute, pathstyle.ArrayIndexed),
		"enum_attribute":      enumAttribute,
		"reason":              reason.String(),
	}
	if reason == enumSiblingRetentionReasonSiblingValueMismatch {
		details["expected_value"] = expectedCaption
	}
	return context.addProcessorIssue(
		issue.SourceEnrichmentRemoval,
		issue.EnrichmentRemovalEnumSiblingNotRemoved,
		details,
		message,
	)
}

func removeObservablesSafely(
	context *processContext,
	event jsonish.Map,
	enabled bool,
) error {
	if !enabled {
		return nil
	}
	value, present := event["observables"]
	if !present {
		return nil
	}
	if value == nil {
		delete(event, "observables")
		return nil
	}

	analysis, observables, isArray, err := observable.Analyze(
		event, context.class, context.compiled.Objects, context.activeProfiles,
	)
	if err != nil {
		return err
	}
	var diagnosticPath eventpath.Path
	diagnosticPath.PushAttribute("observables")
	for index, entry := range analysis.Entries {
		diagnosticPath.PushArrayIndex(index)
		if entry.Problem == observable.ProblemTraversalLimited {
			err := context.addProcessingTraversalLimitIssue(
				diagnosticPath.String(pathstyle.ArrayIndexed), "observables", "",
			)
			diagnosticPath.Pop()
			if err != nil {
				return err
			}
			continue
		}
		if code, mask, defined := observableIssueCode(entry.Problem); defined && !context.ignoresIssue(mask) {
			if diagnostic, present := observable.EntryToDiagnostic(entry, index, &diagnosticPath); present {
				if err := context.addProcessorIssue(
					issue.SourceEnrichmentRemoval,
					code,
					diagnostic.Details,
					diagnostic.Message,
				); err != nil {
					diagnosticPath.Pop()
					return err
				}
			}
		}
		diagnosticPath.Pop()
	}

	if !isArray {
		context.result.EnrichmentRemoval.ObservablesRetained++
		return nil
	}

	removeCount := analysis.RemovableCount()
	context.result.EnrichmentRemoval.ObservablesRemoved += removeCount
	context.result.EnrichmentRemoval.ObservablesRetained += observables.Len() - removeCount
	if removeCount == observables.Len() {
		delete(event, "observables")
		return nil
	}
	if removeCount == 0 {
		return nil
	}
	filtered, err := observables.FilterRemovable(analysis.Entries, removeCount)
	if err != nil {
		return err
	}
	event["observables"] = filtered
	return nil
}

func observableIssueCode(problem observable.Problem) (issue.Code, uint64, bool) {
	switch problem {
	case observable.ProblemArrayWrongType:
		return issue.ObservableArrayWrongType, issueObservableArrayWrongTypeMask, true
	case observable.ProblemElementWrongType:
		return issue.ObservableElementWrongType, issueObservableElementWrongTypeMask, true
	case observable.ProblemNameMissing:
		return issue.ObservableNameMissing, issueObservableNameMissingMask, true
	case observable.ProblemNameWrongType:
		return issue.ObservableNameWrongType, issueObservableNameWrongTypeMask, true
	case observable.ProblemNameInvalidSyntax:
		return issue.ObservableNameInvalidSyntax, issueObservableNameInvalidSyntaxMask, true
	case observable.ProblemNameInvalidReference:
		return issue.ObservableNameInvalidReference, issueObservableNameInvalidReferenceMask, true
	case observable.ProblemPathNotFound:
		return issue.ObservablePathNotFound, issueObservablePathNotFoundMask, true
	case observable.ProblemPathNotObject:
		return issue.ObservablePathNotObject, issueObservablePathNotObjectMask, true
	case observable.ProblemValueWrongType:
		return issue.ObservableValueWrongType, issueObservableValueWrongTypeMask, true
	case observable.ProblemValueNotFound:
		return issue.ObservableValueNotFound, issueObservableValueNotFoundMask, true
	default:
		return issue.None, 0, false
	}
}

func forceRemoveObservablesWithoutAnalysis(context *processContext, event jsonish.Map, enabled bool) {
	if !enabled {
		return
	}
	value, present := event["observables"]
	if !present {
		return
	}
	if observables, ok := eventvalue.NewArrayView(value); ok {
		context.result.EnrichmentRemoval.ObservablesRemoved += observables.Len()
	}
	delete(event, "observables")
}
