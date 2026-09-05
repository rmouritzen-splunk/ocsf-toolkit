package validation_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/validation"
)

type validationCodeContract struct {
	code         validation.Code
	name         string
	defaultLevel validation.Level
}

func errorCode(code validation.Code, name string) validationCodeContract {
	return validationCodeContract{code: code, name: name, defaultLevel: validation.LevelError}
}

func warningCode(code validation.Code, name string) validationCodeContract {
	return validationCodeContract{code: code, name: name, defaultLevel: validation.LevelWarning}
}

func ignoredCode(code validation.Code, name string) validationCodeContract {
	return validationCodeContract{code: code, name: name, defaultLevel: validation.LevelIgnored}
}

var validationCodeContracts = []validationCodeContract{
	errorCode(validation.ClassUIDUnknown, "validation_class_uid_unknown"),
	errorCode(validation.ProfileUnknown, "validation_profile_unknown"),
	errorCode(validation.AttributeEnumValueUnknown, "validation_attribute_enum_value_unknown"),
	errorCode(validation.AttributeEnumArrayValueUnknown, "validation_attribute_enum_array_value_unknown"),
	errorCode(
		validation.AttributeEnumSiblingWithoutEnum,
		"validation_attribute_enum_sibling_without_enum",
	),
	errorCode(validation.SchemaBugObjectMissing, "validation_schema_bug_object_missing"),
	errorCode(
		validation.AttributeEnumArraySiblingMissing,
		"validation_attribute_enum_array_sibling_missing",
	),
	errorCode(
		validation.AttributeEnumArraySiblingIncorrect,
		"validation_attribute_enum_array_sibling_incorrect",
	),
	errorCode(
		validation.AttributeEnumArraySiblingLengthMismatch,
		"validation_attribute_enum_array_sibling_length_mismatch",
	),
	errorCode(validation.SchemaBugTypeMissing, "validation_schema_bug_type_missing"),
	errorCode(validation.SchemaBugPrimitiveTypeUnknown, "validation_schema_bug_primitive_type_unknown"),
	errorCode(
		validation.AttributeValueNotInSuperTypeValues,
		"validation_attribute_value_not_in_super_type_values",
	),
	errorCode(validation.AttributeValueNotInTypeValues, "validation_attribute_value_not_in_type_values"),
	errorCode(
		validation.AttributeValueExceedsSuperTypeRange,
		"validation_attribute_value_exceeds_super_type_range",
	),
	errorCode(validation.AttributeValueExceedsRange, "validation_attribute_value_exceeds_range"),
	errorCode(
		validation.AttributeValueExceedsSuperTypeMaxLen,
		"validation_attribute_value_exceeds_super_type_max_len",
	),
	errorCode(validation.AttributeValueExceedsMaxLen, "validation_attribute_value_exceeds_max_len"),
	errorCode(validation.SchemaBugTypeRegexInvalid, "validation_schema_bug_type_regex_invalid"),
	errorCode(validation.AttributeUnknown, "validation_attribute_unknown"),
	errorCode(validation.AttributeRequiresProfile, "validation_attribute_requires_profile"),
	errorCode(validation.VersionInvalidFormat, "validation_version_invalid_format"),
	errorCode(
		validation.VersionIncompatibleInitialDevelopment,
		"validation_version_incompatible_initial_development",
	),
	errorCode(validation.VersionIncompatiblePrerelease, "validation_version_incompatible_prerelease"),
	errorCode(validation.VersionIncompatibleLater, "validation_version_incompatible_later"),
	errorCode(validation.TypeUIDExpectedValueOverflow, "validation_type_uid_expected_value_overflow"),
	errorCode(validation.TypeUIDIncorrect, "validation_type_uid_incorrect"),
	errorCode(validation.ConstraintFailed, "validation_constraint_failed"),
	errorCode(validation.ConstraintUnknown, "validation_constraint_unknown"),
	errorCode(validation.ObservableNameInvalidSyntax, "validation_observable_name_invalid_syntax"),
	errorCode(validation.ObservableNameInvalidReference, "validation_observable_name_invalid_reference"),
	errorCode(validation.ObservablePathNotFound, "validation_observable_path_not_found"),
	errorCode(validation.ObservablePathNotObject, "validation_observable_path_not_object"),
	errorCode(validation.ObservableValueNotFound, "validation_observable_value_not_found"),
	errorCode(validation.AttributeRequiredMissing, "validation_attribute_required_missing"),
	errorCode(validation.AttributeWrongType, "validation_attribute_wrong_type"),
	warningCode(validation.ClassDeprecated, "validation_class_deprecated"),
	warningCode(validation.AttributeDeprecated, "validation_attribute_deprecated"),
	warningCode(validation.AttributeTypeDeprecated, "validation_attribute_type_deprecated"),
	warningCode(validation.ObjectDeprecated, "validation_object_deprecated"),
	{
		code:         validation.AttributeRecommendedMissing,
		name:         "validation_attribute_recommended_missing",
		defaultLevel: validation.LevelIgnored,
	},
	warningCode(
		validation.AttributeEnumSiblingSuspicious,
		"validation_attribute_enum_sibling_suspicious_other",
	),
	warningCode(validation.AttributeEnumSiblingIncorrect, "validation_attribute_enum_sibling_incorrect"),
	warningCode(validation.AttributeEnumValueDeprecated, "validation_attribute_enum_value_deprecated"),
	warningCode(
		validation.AttributeEnumArrayValueDeprecated,
		"validation_attribute_enum_array_value_deprecated",
	),
	warningCode(
		validation.AttributeValueSuperTypeRegexNotMatched,
		"validation_attribute_value_super_type_regex_not_matched",
	),
	warningCode(validation.AttributeValueRegexNotMatched, "validation_attribute_value_regex_not_matched"),
	warningCode(validation.VersionEarlier, "validation_version_earlier"),
	warningCode(
		validation.ObservableNamePathNotation,
		"validation_observable_name_path_notation_unexpected",
	),
	errorCode(validation.ClassUIDMissing, "validation_class_uid_missing"),
	errorCode(validation.ClassUIDWrongType, "validation_class_uid_wrong_type"),
	ignoredCode(validation.ObservableDuplicate, "validation_observable_duplicate"),
}

func TestInvariantValidationCodeContract(t *testing.T) {
	// Invariant test: every v1 validation code retains its exported constant, ordinal, text and JSON identity,
	// default level, and ignorable classification; additions append to this independently reviewed manifest.
	wantCodes := make([]validation.Code, len(validationCodeContracts))
	for index, contract := range validationCodeContracts {
		t.Run(contract.name, func(t *testing.T) {
			wantCodes[index] = contract.code
			require.Equal(t, index+1, int(contract.code))
			require.True(t, contract.code.Valid())
			require.Equal(t, contract.name, contract.code.String())
			require.Equal(t, contract.defaultLevel, contract.code.DefaultLevel())
			require.True(t, contract.code.Ignorable())
			require.NotEmpty(t, contract.code.Description())

			parsed, ok := validation.ParseCode(contract.name)
			require.True(t, ok)
			require.Equal(t, contract.code, parsed)

			encoded, err := json.Marshal(contract.code)
			require.NoError(t, err)
			require.Equal(t, `"`+contract.name+`"`, string(encoded))
			var decoded validation.Code
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, contract.code, decoded)
		})
	}
	require.Equal(t, wantCodes, validation.Codes())
}
