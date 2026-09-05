// Package validation defines machine-readable codes and levels for OCSF validation findings.
package validation

import "github.com/ocsf/ocsf-toolkit/internal/coderegistry"

// Code identifies an OCSF validation condition independently of its effective reporting level.
type Code uint8

// Code names and ordinals are public compatibility contracts. During v1, retain the name and ordinal of a code that
// stops being emitted and add "Deprecated: this code is no longer emitted." to its documentation. Never reorder
// existing codes; append new codes immediately before codeCount.
const (
	// None indicates the absence of a validation code and is not a valid reportable code.
	None Code = iota

	// ClassUIDUnknown reports a class_uid absent from the schema.
	ClassUIDUnknown
	// ProfileUnknown reports an unknown metadata profile.
	ProfileUnknown
	// AttributeEnumValueUnknown reports an unknown scalar enum value.
	AttributeEnumValueUnknown
	// AttributeEnumArrayValueUnknown reports an unknown enum-array value.
	AttributeEnumArrayValueUnknown
	// AttributeEnumSiblingWithoutEnum reports an enum sibling without its integral enum attribute.
	AttributeEnumSiblingWithoutEnum
	// SchemaBugObjectMissing reports an object type absent from the schema.
	SchemaBugObjectMissing
	// AttributeEnumArraySiblingMissing reports an enum-array element without a sibling element.
	AttributeEnumArraySiblingMissing
	// AttributeEnumArraySiblingIncorrect reports an incorrect enum-array sibling element.
	AttributeEnumArraySiblingIncorrect
	// AttributeEnumArraySiblingLengthMismatch reports enum and sibling arrays with different lengths.
	AttributeEnumArraySiblingLengthMismatch
	// SchemaBugTypeMissing reports an attribute type absent from the schema.
	SchemaBugTypeMissing
	// SchemaBugPrimitiveTypeUnknown reports an unsupported primitive schema type.
	SchemaBugPrimitiveTypeUnknown
	// AttributeValueNotInSuperTypeValues reports a value excluded by its supertype.
	AttributeValueNotInSuperTypeValues
	// AttributeValueNotInTypeValues reports a value excluded by its type.
	AttributeValueNotInTypeValues
	// AttributeValueExceedsSuperTypeRange reports a value outside its supertype range.
	AttributeValueExceedsSuperTypeRange
	// AttributeValueExceedsRange reports a value outside its type range.
	AttributeValueExceedsRange
	// AttributeValueExceedsSuperTypeMaxLen reports a value longer than its supertype permits.
	AttributeValueExceedsSuperTypeMaxLen
	// AttributeValueExceedsMaxLen reports a value longer than its type permits.
	AttributeValueExceedsMaxLen
	// SchemaBugTypeRegexInvalid reports an invalid schema regex.
	SchemaBugTypeRegexInvalid
	// AttributeUnknown reports an attribute absent from the active schema definition.
	AttributeUnknown
	// AttributeRequiresProfile reports an attribute defined by a profile that is not active for the event.
	AttributeRequiresProfile
	// VersionInvalidFormat reports a metadata version that is not valid semantic versioning.
	VersionInvalidFormat
	// VersionIncompatibleInitialDevelopment reports incompatible initial-development versions.
	VersionIncompatibleInitialDevelopment
	// VersionIncompatiblePrerelease reports incompatible prerelease versions.
	VersionIncompatiblePrerelease
	// VersionIncompatibleLater reports an event version later than the loaded schema.
	VersionIncompatibleLater
	// TypeUIDExpectedValueOverflow reports a derived type_uid outside signed 64-bit range.
	TypeUIDExpectedValueOverflow
	// TypeUIDIncorrect reports a type_uid inconsistent with class_uid and activity_id.
	TypeUIDIncorrect
	// ConstraintFailed reports an unsatisfied schema constraint.
	ConstraintFailed
	// ConstraintUnknown reports an unsupported schema constraint.
	ConstraintUnknown
	// ObservableNameInvalidSyntax reports invalid observable path syntax.
	ObservableNameInvalidSyntax
	// ObservableNameInvalidReference reports an observable path absent from the event-class schema.
	ObservableNameInvalidReference
	// ObservablePathNotFound reports an observable path absent from the event.
	ObservablePathNotFound
	// ObservablePathNotObject reports a valueless observable path that does not resolve to an object.
	ObservablePathNotObject
	// ObservableValueNotFound reports an observable value absent from its named event path.
	ObservableValueNotFound
	// AttributeRequiredMissing reports a missing required attribute.
	AttributeRequiredMissing
	// AttributeWrongType reports a value incompatible with its schema type.
	AttributeWrongType

	// ClassDeprecated reports use of a deprecated event class.
	ClassDeprecated
	// AttributeDeprecated reports use of a deprecated attribute.
	AttributeDeprecated
	// AttributeTypeDeprecated reports use of a deprecated dictionary type by a non-deprecated attribute.
	AttributeTypeDeprecated
	// ObjectDeprecated reports use of a deprecated object type.
	ObjectDeprecated
	// AttributeRecommendedMissing reports a missing recommended attribute.
	AttributeRecommendedMissing
	// AttributeEnumSiblingSuspicious reports a suspicious sibling for enum ID 99.
	AttributeEnumSiblingSuspicious
	// AttributeEnumSiblingIncorrect reports a scalar enum sibling inconsistent with its enum value.
	AttributeEnumSiblingIncorrect
	// AttributeEnumValueDeprecated reports use of a deprecated scalar enum value.
	AttributeEnumValueDeprecated
	// AttributeEnumArrayValueDeprecated reports use of a deprecated enum-array value.
	AttributeEnumArrayValueDeprecated
	// AttributeValueSuperTypeRegexNotMatched reports a value that does not match its supertype regex.
	AttributeValueSuperTypeRegexNotMatched
	// AttributeValueRegexNotMatched reports a value that does not match its type regex.
	AttributeValueRegexNotMatched
	// VersionEarlier reports an event version earlier than the loaded compatible schema.
	VersionEarlier
	// ObservableNamePathNotation reports an observable name that does not use the preferred notation.
	ObservableNamePathNotation
	// ClassUIDMissing reports a missing class_uid that prevents class resolution.
	ClassUIDMissing
	// ClassUIDWrongType reports a class_uid whose type prevents class resolution.
	ClassUIDWrongType

	codeCount
)

// codeInfo contains the stable metadata associated with a Code.
type codeInfo struct {
	name         string
	description  string
	defaultLevel Level
}

var codeInfos = [codeCount]codeInfo{
	ClassUIDUnknown: {
		"validation_class_uid_unknown",
		"The class_uid is not defined in the schema.",
		LevelError,
	},
	ProfileUnknown: {
		"validation_profile_unknown",
		"A metadata profile is not defined by the schema.",
		LevelError,
	},
	AttributeEnumValueUnknown: {
		"validation_attribute_enum_value_unknown",
		"A scalar enum value is not defined by the schema.",
		LevelError,
	},
	AttributeEnumArrayValueUnknown: {
		"validation_attribute_enum_array_value_unknown",
		"An enum-array value is not defined by the schema.",
		LevelError,
	},
	AttributeEnumSiblingWithoutEnum: {
		"validation_attribute_enum_sibling_without_enum",
		"An enum sibling is present without its integral enum attribute.",
		LevelError,
	},
	SchemaBugObjectMissing: {
		"validation_schema_bug_object_missing",
		"An object type referenced by the schema is missing.",
		LevelError,
	},
	AttributeEnumArraySiblingMissing: {
		"validation_attribute_enum_array_sibling_missing",
		"An enum-array element has no corresponding sibling element.",
		LevelError,
	},
	AttributeEnumArraySiblingIncorrect: {
		"validation_attribute_enum_array_sibling_incorrect",
		"An enum-array sibling element does not match its enum value.",
		LevelError,
	},
	AttributeEnumArraySiblingLengthMismatch: {
		"validation_attribute_enum_array_sibling_length_mismatch",
		"An enum array and its sibling array have different lengths.",
		LevelError,
	},
	SchemaBugTypeMissing: {
		"validation_schema_bug_type_missing",
		"An attribute type referenced by the schema is missing.",
		LevelError,
	},
	SchemaBugPrimitiveTypeUnknown: {
		"validation_schema_bug_primitive_type_unknown",
		"A primitive schema type is not supported.",
		LevelError,
	},
	AttributeValueNotInSuperTypeValues: {
		"validation_attribute_value_not_in_super_type_values",
		"A value is not in its supertype's allowed values.",
		LevelError,
	},
	AttributeValueNotInTypeValues: {
		"validation_attribute_value_not_in_type_values",
		"A value is not in its type's allowed values.",
		LevelError,
	},
	AttributeValueExceedsSuperTypeRange: {
		"validation_attribute_value_exceeds_super_type_range",
		"A value is outside its supertype's permitted range.",
		LevelError,
	},
	AttributeValueExceedsRange: {
		"validation_attribute_value_exceeds_range",
		"A value is outside its type's permitted range.",
		LevelError,
	},
	AttributeValueExceedsSuperTypeMaxLen: {
		"validation_attribute_value_exceeds_super_type_max_len",
		"A value is longer than its supertype permits.",
		LevelError,
	},
	AttributeValueExceedsMaxLen: {
		"validation_attribute_value_exceeds_max_len",
		"A value is longer than its type permits.",
		LevelError,
	},
	SchemaBugTypeRegexInvalid: {
		"validation_schema_bug_type_regex_invalid",
		"A regular expression defined by the schema is invalid.",
		LevelError,
	},
	AttributeUnknown: {
		"validation_attribute_unknown",
		"An attribute is not defined by the active schema.",
		LevelError,
	},
	AttributeRequiresProfile: {
		"validation_attribute_requires_profile",
		"An attribute is defined by the schema but requires a profile that is not active for the event.",
		LevelError,
	},
	VersionInvalidFormat: {
		"validation_version_invalid_format",
		"The event metadata version is not valid semantic versioning.",
		LevelError,
	},
	VersionIncompatibleInitialDevelopment: {
		"validation_version_incompatible_initial_development",
		"The event and schema use incompatible initial-development versions.",
		LevelError,
	},
	VersionIncompatiblePrerelease: {
		"validation_version_incompatible_prerelease",
		"The event and schema use incompatible prerelease versions.",
		LevelError,
	},
	VersionIncompatibleLater: {
		"validation_version_incompatible_later",
		"The event version is later than the loaded schema version.",
		LevelError,
	},
	TypeUIDExpectedValueOverflow: {
		"validation_type_uid_expected_value_overflow",
		"The type_uid derived from class_uid and activity_id exceeds the signed 64-bit range.",
		LevelError,
	},
	TypeUIDIncorrect: {
		"validation_type_uid_incorrect",
		"The type_uid is inconsistent with class_uid and activity_id.",
		LevelError,
	},
	ConstraintFailed: {
		"validation_constraint_failed",
		"A schema constraint is not satisfied.",
		LevelError,
	},
	ConstraintUnknown: {
		"validation_constraint_unknown",
		"A schema constraint is not supported.",
		LevelError,
	},
	ObservableNameInvalidSyntax: {
		"validation_observable_name_invalid_syntax",
		"An observable name has invalid path syntax.",
		LevelError,
	},
	ObservableNameInvalidReference: {
		"validation_observable_name_invalid_reference",
		"An observable name is not defined by the event class.",
		LevelError,
	},
	ObservablePathNotFound: {
		"validation_observable_path_not_found",
		"An observable path is absent from the event.",
		LevelError,
	},
	ObservablePathNotObject: {
		"validation_observable_path_not_object",
		"A valueless observable path does not resolve to an object.",
		LevelError,
	},
	ObservableValueNotFound: {
		"validation_observable_value_not_found",
		"An observable value is absent from its named event path.",
		LevelError,
	},
	AttributeRequiredMissing: {
		"validation_attribute_required_missing",
		"A required attribute is missing.",
		LevelError,
	},
	AttributeWrongType: {
		"validation_attribute_wrong_type",
		"An attribute value is incompatible with its schema type.",
		LevelError,
	},
	ClassDeprecated: {
		"validation_class_deprecated",
		"The event class is deprecated.",
		LevelWarning,
	},
	AttributeDeprecated: {
		"validation_attribute_deprecated",
		"An attribute is deprecated.",
		LevelWarning,
	},
	AttributeTypeDeprecated: {
		"validation_attribute_type_deprecated",
		"A non-deprecated attribute uses a deprecated dictionary type.",
		LevelWarning,
	},
	ObjectDeprecated: {
		"validation_object_deprecated",
		"An object type is deprecated.",
		LevelWarning,
	},
	AttributeRecommendedMissing: {
		"validation_attribute_recommended_missing",
		"A recommended attribute is missing.",
		LevelIgnored,
	},
	AttributeEnumSiblingSuspicious: {
		"validation_attribute_enum_sibling_suspicious_other",
		"An enum ID 99 sibling is suspicious.",
		LevelWarning,
	},
	AttributeEnumSiblingIncorrect: {
		"validation_attribute_enum_sibling_incorrect",
		"A scalar enum sibling does not match its enum value.",
		LevelWarning,
	},
	AttributeEnumValueDeprecated: {
		"validation_attribute_enum_value_deprecated",
		"A scalar enum value is deprecated.",
		LevelWarning,
	},
	AttributeEnumArrayValueDeprecated: {
		"validation_attribute_enum_array_value_deprecated",
		"An enum-array value is deprecated.",
		LevelWarning,
	},
	AttributeValueSuperTypeRegexNotMatched: {
		"validation_attribute_value_super_type_regex_not_matched",
		"A value does not match its supertype's regular expression.",
		LevelWarning,
	},
	AttributeValueRegexNotMatched: {
		"validation_attribute_value_regex_not_matched",
		"A value does not match its type's regular expression.",
		LevelWarning,
	},
	VersionEarlier: {
		"validation_version_earlier",
		"The event version is earlier than the compatible loaded schema version.",
		LevelWarning,
	},
	ObservableNamePathNotation: {
		"validation_observable_name_path_notation_unexpected",
		"An observable name does not use the configured path notation.",
		LevelWarning,
	},
	ClassUIDMissing: {
		"validation_class_uid_missing",
		"The class_uid is missing.",
		LevelError,
	},
	ClassUIDWrongType: {
		"validation_class_uid_wrong_type",
		"The class_uid has the wrong type.",
		LevelError,
	},
}

var codeRegistry = coderegistry.New[Code]("validation code", codeRegistryInfos())

// codeRegistryInfos projects codeInfos' name and description into the shared registry's table. Default levels remain
// in codeInfos because the shared registry has no notion of validation levels.
func codeRegistryInfos() []coderegistry.Info {
	infos := make([]coderegistry.Info, len(codeInfos))
	for index, info := range codeInfos {
		infos[index] = coderegistry.Info{Name: info.name, Description: info.description}
	}
	return infos
}

// Valid reports whether code is defined by this toolkit version.
func (code Code) Valid() bool {
	return codeRegistry.Valid(code)
}

// Codes returns every valid Code in declaration order.
func Codes() []Code {
	return codeRegistry.Codes()
}

// DefaultLevel returns the toolkit's default reporting level for code, or an invalid level for an invalid code.
func (code Code) DefaultLevel() Level {
	if !code.Valid() {
		return invalidLevel
	}
	return codeInfos[code].defaultLevel
}

// Ignorable reports whether validation-level policy may omit code. Every validation finding is ignorable; mandatory
// processing issues independently report conditions such as class-resolution failures that prevent further work.
func (code Code) Ignorable() bool {
	return codeRegistry.Ignorable(code)
}

// String returns the stable external representation of code, or an empty string for an invalid code.
func (code Code) String() string {
	return codeRegistry.String(code)
}

// Description returns a short human-readable explanation of code, or an empty string for an invalid code.
func (code Code) Description() string {
	return codeRegistry.Description(code)
}

// ParseCode resolves a stable external validation-code representation.
func ParseCode(value string) (Code, bool) {
	return codeRegistry.Parse(value)
}

// MarshalText returns the stable external representation used by JSON encoders.
func (code Code) MarshalText() ([]byte, error) {
	return codeRegistry.MarshalText(code)
}

// UnmarshalText resolves a stable external representation used by JSON decoders.
func (code *Code) UnmarshalText(text []byte) error {
	return codeRegistry.UnmarshalText(text, code)
}
