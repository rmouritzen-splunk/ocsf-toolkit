// Package issue defines machine-readable codes, sources, and policy levels for event-processing issues.
package issue

import "github.com/ocsf/ocsf-toolkit/internal/coderegistry"

// Code identifies an event-processing condition.
type Code uint8

// Code names and ordinals are public compatibility contracts. During v1, retain the name and ordinal of a code that
// stops being emitted and add "Deprecated: this code is no longer emitted." to its documentation. Never reorder
// existing codes; append new codes immediately before issueCodeCount.
const (
	// None indicates the absence of an issue code and is not a valid reportable code.
	None Code = iota
	// EventTraversalLimited reports that schema-guided processing stopped at a recursive-object boundary.
	EventTraversalLimited
	// EnrichmentObservableNotAddedWrongType reports an unsupported observable source value.
	EnrichmentObservableNotAddedWrongType
	// EnrichmentObservableNotAddedJSONType reports an ambiguous json_t observable source.
	EnrichmentObservableNotAddedJSONType
	// EnrichmentEnumSiblingNotAdded reports an enum value without a usable schema caption.
	EnrichmentEnumSiblingNotAdded
	// EnrichmentEnumSiblingOtherAdded reports a synthesized enum ID 99 sibling.
	EnrichmentEnumSiblingOtherAdded
	// EnrichmentObservablesNotAddedWrongType reports an invalid top-level observables value.
	EnrichmentObservablesNotAddedWrongType
	// EnrichmentObservableDuplicateSkipped reports a generated observable duplicate.
	EnrichmentObservableDuplicateSkipped
	// EnrichmentRemovalEnumSiblingNotRemoved reports an enum sibling retained because safe removal failed.
	EnrichmentRemovalEnumSiblingNotRemoved
	// ObservableArrayWrongType reports that observables is not an array.
	ObservableArrayWrongType
	// ObservableElementWrongType reports a non-object observables array element.
	ObservableElementWrongType
	// ObservableNameMissing reports an observable without a name.
	ObservableNameMissing
	// ObservableNameWrongType reports a non-string observable name.
	ObservableNameWrongType
	// ObservableNameInvalidSyntax reports invalid observable path syntax.
	ObservableNameInvalidSyntax
	// ObservableNameInvalidReference reports an observable path not defined by the event class.
	ObservableNameInvalidReference
	// ObservablePathNotFound reports an observable path absent from the event.
	ObservablePathNotFound
	// ObservablePathNotObject reports a valueless observable path that does not resolve to an object.
	ObservablePathNotObject
	// ObservableValueWrongType reports an observable value that is neither a string nor null.
	ObservableValueWrongType
	// ObservableValueNotFound reports an observable value absent from its named event path.
	ObservableValueNotFound
	// ClassUIDMissing reports that processing stopped because class_uid is missing.
	ClassUIDMissing
	// ClassUIDWrongType reports that processing stopped because class_uid has the wrong type.
	ClassUIDWrongType
	// ClassUIDUnknown reports that processing stopped because class_uid does not identify a schema class.
	ClassUIDUnknown
	// AtInitSchemaEnumSiblingSourceNotIntegral reports an enum sibling source without a direct integral type.
	AtInitSchemaEnumSiblingSourceNotIntegral
	// AtInitSchemaEnumSiblingTargetNotFound reports an enum sibling target absent from its class or object.
	AtInitSchemaEnumSiblingTargetNotFound
	// AtInitSchemaEnumSiblingTargetIsEnum reports an enum sibling target that is itself an enum.
	AtInitSchemaEnumSiblingTargetIsEnum
	// AtInitSchemaEnumSiblingTargetNotString reports a target without the required direct string type and shape.
	AtInitSchemaEnumSiblingTargetNotString
	issueCodeCount
)

var issueCodeInfos = [issueCodeCount]coderegistry.Info{
	EventTraversalLimited: {
		Name:        "issue_event_traversal_limited",
		Description: "Schema-guided processing stopped at a recursive-object boundary.",
		Mandatory:   true,
	},
	EnrichmentObservableNotAddedWrongType: {
		Name:        "issue_enrichment_observable_not_added_wrong_type",
		Description: "The observable source value has an unsupported type.",
	},
	EnrichmentObservableNotAddedJSONType: {
		Name:        "issue_enrichment_observable_not_added_json_type",
		Description: "The observable source value has the ambiguous json_t type.",
	},
	EnrichmentEnumSiblingNotAdded: {
		Name:        "issue_enrichment_enum_sibling_not_added",
		Description: "The enum value has no usable schema caption for its sibling.",
	},
	EnrichmentEnumSiblingOtherAdded: {
		Name:        "issue_enrichment_enum_sibling_other_added",
		Description: "A sibling was synthesized using the enum ID 99 caption.",
	},
	EnrichmentObservablesNotAddedWrongType: {
		Name:        "issue_enrichment_observables_not_added_wrong_type",
		Description: "The top-level observables attribute has an invalid value.",
	},
	EnrichmentObservableDuplicateSkipped: {
		Name:        "issue_enrichment_observable_duplicate_skipped",
		Description: "The generated observable duplicates one already present.",
	},
	EnrichmentRemovalEnumSiblingNotRemoved: {
		Name:        "issue_enrichment_removal_enum_sibling_not_removed",
		Description: "The enum sibling was retained because safe removal failed.",
	},
	ObservableArrayWrongType: {
		Name:        "issue_observable_array_wrong_type",
		Description: "The observables attribute is not an array.",
	},
	ObservableElementWrongType: {
		Name:        "issue_observable_element_wrong_type",
		Description: "An observables array element is not an object.",
	},
	ObservableNameMissing: {
		Name:        "issue_observable_name_missing",
		Description: "An observable is missing its name.",
	},
	ObservableNameWrongType: {
		Name:        "issue_observable_name_wrong_type",
		Description: "An observable's name is not a string.",
	},
	ObservableNameInvalidSyntax: {
		Name:        "issue_observable_name_invalid_syntax",
		Description: "An observable's name has invalid path syntax.",
	},
	ObservableNameInvalidReference: {
		Name:        "issue_observable_name_invalid_reference",
		Description: "The observable name is not defined by the event's class.",
	},
	ObservablePathNotFound: {
		Name:        "issue_observable_path_not_found",
		Description: "The observable's named path was not found in the event.",
	},
	ObservablePathNotObject: {
		Name:        "issue_observable_path_not_object",
		Description: "A valueless observable does not resolve to an object.",
	},
	ObservableValueWrongType: {
		Name:        "issue_observable_value_wrong_type",
		Description: "The observable's value is neither a string nor null.",
	},
	ObservableValueNotFound: {
		Name:        "issue_observable_value_not_found",
		Description: "The observable's value was not found at its named path.",
	},
	ClassUIDMissing: {
		Name:        "issue_class_uid_missing",
		Description: "The class_uid is missing.",
		Mandatory:   true,
	},
	ClassUIDWrongType: {
		Name:        "issue_class_uid_wrong_type",
		Description: "The class_uid has the wrong type.",
		Mandatory:   true,
	},
	ClassUIDUnknown: {
		Name:        "issue_class_uid_unknown",
		Description: "The class_uid is not defined in the schema.",
		Mandatory:   true,
	},
	AtInitSchemaEnumSiblingSourceNotIntegral: {
		Name:        "issue_at_init_schema_enum_sibling_source_not_integral",
		Description: "An enum with a sibling does not have the direct type integer_t or long_t.",
	},
	AtInitSchemaEnumSiblingTargetNotFound: {
		Name:        "issue_at_init_schema_enum_sibling_target_not_found",
		Description: "An enum sibling target is absent from its class or object.",
	},
	AtInitSchemaEnumSiblingTargetIsEnum: {
		Name:        "issue_at_init_schema_enum_sibling_target_is_enum",
		Description: "An enum sibling target is itself an enum.",
	},
	AtInitSchemaEnumSiblingTargetNotString: {
		Name:        "issue_at_init_schema_enum_sibling_target_not_string",
		Description: "The enum sibling target is not a direct, same-shaped string_t.",
	},
}

var issueCodeRegistry = coderegistry.New[Code]("issue code", issueCodeInfos[:])

// Codes returns every valid Code in declaration order.
func Codes() []Code {
	return issueCodeRegistry.Codes()
}

// Valid reports whether code is defined by this toolkit version.
func (code Code) Valid() bool {
	return issueCodeRegistry.Valid(code)
}

// DefaultLevel returns the toolkit's default handling level for code, or an invalid level for an invalid code. Every
// issue code currently defaults to Warning.
func (code Code) DefaultLevel() Level {
	if !code.Valid() {
		return Level(0)
	}
	return LevelWarning
}

// Ignorable reports whether issue-level policy may omit code. Issues that report incomplete event processing are
// mandatory and cannot be ignored.
func (code Code) Ignorable() bool {
	return issueCodeRegistry.Ignorable(code)
}

// String returns the stable external representation of code, or an empty string for an invalid code.
func (code Code) String() string {
	return issueCodeRegistry.String(code)
}

// Description returns a short human-readable explanation of code, or an empty string for an invalid code.
func (code Code) Description() string {
	return issueCodeRegistry.Description(code)
}

// ParseCode resolves a stable external issue-code representation.
func ParseCode(value string) (Code, bool) {
	return issueCodeRegistry.Parse(value)
}

// MarshalText returns the stable external representation used by JSON encoders.
func (code Code) MarshalText() ([]byte, error) {
	return issueCodeRegistry.MarshalText(code)
}

// UnmarshalText resolves a stable external representation used by JSON decoders.
func (code *Code) UnmarshalText(text []byte) error {
	return issueCodeRegistry.UnmarshalText(text, code)
}
