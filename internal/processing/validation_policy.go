package processing

import (
	"fmt"

	"github.com/ocsf/ocsf-toolkit/validation"
)

const (
	validationClassUIDUnknownMask                         uint64 = 1 << validation.ClassUIDUnknown
	validationProfileUnknownMask                          uint64 = 1 << validation.ProfileUnknown
	validationAttributeEnumValueUnknownMask               uint64 = 1 << validation.AttributeEnumValueUnknown
	validationAttributeEnumArrayValueUnknownMask          uint64 = 1 << validation.AttributeEnumArrayValueUnknown
	validationAttributeEnumSiblingWithoutEnumMask         uint64 = 1 << validation.AttributeEnumSiblingWithoutEnum
	validationSchemaBugObjectMissingMask                  uint64 = 1 << validation.SchemaBugObjectMissing
	validationAttributeEnumArraySiblingMissingMask        uint64 = 1 << validation.AttributeEnumArraySiblingMissing
	validationAttributeEnumArraySiblingIncorrectMask      uint64 = 1 << validation.AttributeEnumArraySiblingIncorrect
	validationAttributeEnumArraySiblingLengthMismatchMask uint64 = 1 <<
		validation.AttributeEnumArraySiblingLengthMismatch
	validationSchemaBugTypeMissingMask                   uint64 = 1 << validation.SchemaBugTypeMissing
	validationSchemaBugPrimitiveTypeUnknownMask          uint64 = 1 << validation.SchemaBugPrimitiveTypeUnknown
	validationAttributeValueNotInSuperTypeValuesMask     uint64 = 1 << validation.AttributeValueNotInSuperTypeValues
	validationAttributeValueNotInTypeValuesMask          uint64 = 1 << validation.AttributeValueNotInTypeValues
	validationAttributeValueExceedsSuperTypeRangeMask    uint64 = 1 << validation.AttributeValueExceedsSuperTypeRange
	validationAttributeValueExceedsRangeMask             uint64 = 1 << validation.AttributeValueExceedsRange
	validationAttributeValueExceedsSuperTypeMaxLenMask   uint64 = 1 << validation.AttributeValueExceedsSuperTypeMaxLen
	validationAttributeValueExceedsMaxLenMask            uint64 = 1 << validation.AttributeValueExceedsMaxLen
	validationSchemaBugTypeRegexInvalidMask              uint64 = 1 << validation.SchemaBugTypeRegexInvalid
	validationAttributeUnknownMask                       uint64 = 1 << validation.AttributeUnknown
	validationAttributeRequiresProfileMask               uint64 = 1 << validation.AttributeRequiresProfile
	validationVersionInvalidFormatMask                   uint64 = 1 << validation.VersionInvalidFormat
	validationVersionIncompatibleInitialDevelopmentMask  uint64 = 1 << validation.VersionIncompatibleInitialDevelopment
	validationVersionIncompatiblePrereleaseMask          uint64 = 1 << validation.VersionIncompatiblePrerelease
	validationVersionIncompatibleLaterMask               uint64 = 1 << validation.VersionIncompatibleLater
	validationTypeUIDExpectedValueOverflowMask           uint64 = 1 << validation.TypeUIDExpectedValueOverflow
	validationTypeUIDIncorrectMask                       uint64 = 1 << validation.TypeUIDIncorrect
	validationConstraintFailedMask                       uint64 = 1 << validation.ConstraintFailed
	validationConstraintUnknownMask                      uint64 = 1 << validation.ConstraintUnknown
	validationObservableNameInvalidSyntaxMask            uint64 = 1 << validation.ObservableNameInvalidSyntax
	validationObservableNameInvalidReferenceMask         uint64 = 1 << validation.ObservableNameInvalidReference
	validationObservablePathNotFoundMask                 uint64 = 1 << validation.ObservablePathNotFound
	validationObservablePathNotObjectMask                uint64 = 1 << validation.ObservablePathNotObject
	validationObservableValueNotFoundMask                uint64 = 1 << validation.ObservableValueNotFound
	validationAttributeRequiredMissingMask               uint64 = 1 << validation.AttributeRequiredMissing
	validationAttributeWrongTypeMask                     uint64 = 1 << validation.AttributeWrongType
	validationClassDeprecatedMask                        uint64 = 1 << validation.ClassDeprecated
	validationAttributeDeprecatedMask                    uint64 = 1 << validation.AttributeDeprecated
	validationAttributeTypeDeprecatedMask                uint64 = 1 << validation.AttributeTypeDeprecated
	validationObjectDeprecatedMask                       uint64 = 1 << validation.ObjectDeprecated
	validationAttributeRecommendedMissingMask            uint64 = 1 << validation.AttributeRecommendedMissing
	validationAttributeEnumSiblingSuspiciousMask         uint64 = 1 << validation.AttributeEnumSiblingSuspicious
	validationAttributeEnumSiblingIncorrectMask          uint64 = 1 << validation.AttributeEnumSiblingIncorrect
	validationAttributeEnumValueDeprecatedMask           uint64 = 1 << validation.AttributeEnumValueDeprecated
	validationAttributeEnumArrayValueDeprecatedMask      uint64 = 1 << validation.AttributeEnumArrayValueDeprecated
	validationAttributeValueSuperTypeRegexNotMatchedMask uint64 = 1 <<
		validation.AttributeValueSuperTypeRegexNotMatched
	validationAttributeValueRegexNotMatchedMask uint64 = 1 << validation.AttributeValueRegexNotMatched
	validationVersionEarlierMask                uint64 = 1 << validation.VersionEarlier
	validationObservableNamePathNotationMask    uint64 = 1 << validation.ObservableNamePathNotation
	validationClassUIDMissingMask               uint64 = 1 << validation.ClassUIDMissing
	validationClassUIDWrongTypeMask             uint64 = 1 << validation.ClassUIDWrongType

	// All masks except 0 set to true.
	allValidationCodesMask       uint64 = ^uint64(0) &^ 1
	defaultValidationIgnoredMask uint64 = validationAttributeRecommendedMissingMask
	defaultValidationWarningMask uint64 = validationClassDeprecatedMask |
		validationAttributeDeprecatedMask |
		validationAttributeTypeDeprecatedMask |
		validationObjectDeprecatedMask |
		validationAttributeEnumSiblingSuspiciousMask |
		validationAttributeEnumSiblingIncorrectMask |
		validationAttributeEnumValueDeprecatedMask |
		validationAttributeEnumArrayValueDeprecatedMask |
		validationAttributeValueSuperTypeRegexNotMatchedMask |
		validationAttributeValueRegexNotMatchedMask |
		validationVersionEarlierMask |
		validationObservableNamePathNotationMask
	defaultValidationErrorMask uint64 = allValidationCodesMask &^
		(defaultValidationIgnoredMask | defaultValidationWarningMask)
	requirementValidationMask uint64 = validationAttributeRequiredMissingMask |
		validationAttributeRecommendedMissingMask
	enumValueValidationMask uint64 = validationAttributeEnumValueUnknownMask |
		validationAttributeEnumArrayValueUnknownMask |
		validationAttributeEnumValueDeprecatedMask |
		validationAttributeEnumArrayValueDeprecatedMask
	enumSiblingValidationMask uint64 = validationAttributeEnumSiblingWithoutEnumMask |
		validationAttributeEnumArraySiblingMissingMask |
		validationAttributeEnumArraySiblingIncorrectMask |
		validationAttributeEnumArraySiblingLengthMismatchMask |
		validationAttributeEnumSiblingSuspiciousMask |
		validationAttributeEnumSiblingIncorrectMask
	primitiveValueValidationMask uint64 = validationSchemaBugTypeMissingMask |
		validationSchemaBugPrimitiveTypeUnknownMask |
		validationAttributeValueNotInSuperTypeValuesMask |
		validationAttributeValueNotInTypeValuesMask |
		validationAttributeValueExceedsSuperTypeRangeMask |
		validationAttributeValueExceedsRangeMask |
		validationAttributeValueExceedsSuperTypeMaxLenMask |
		validationAttributeValueExceedsMaxLenMask |
		validationSchemaBugTypeRegexInvalidMask |
		validationAttributeValueSuperTypeRegexNotMatchedMask |
		validationAttributeValueRegexNotMatchedMask
	attributeDeprecationValidationMask uint64 = validationAttributeDeprecatedMask |
		validationAttributeTypeDeprecatedMask
	constraintValidationMask uint64 = validationConstraintFailedMask | validationConstraintUnknownMask
	versionValidationMask    uint64 = validationVersionInvalidFormatMask |
		validationVersionIncompatibleInitialDevelopmentMask |
		validationVersionIncompatiblePrereleaseMask |
		validationVersionIncompatibleLaterMask |
		validationVersionEarlierMask
	typeUIDValidationMask    uint64 = validationTypeUIDExpectedValueOverflowMask | validationTypeUIDIncorrectMask
	observableValidationMask uint64 = validationObservableNameInvalidSyntaxMask |
		validationObservableNameInvalidReferenceMask |
		validationObservablePathNotFoundMask |
		validationObservablePathNotObjectMask |
		validationObservableValueNotFoundMask |
		validationObservableNamePathNotationMask
	eventWalkValidationMask uint64 = validationAttributeRequiresProfileMask |
		validationAttributeUnknownMask |
		requirementValidationMask |
		validationAttributeWrongTypeMask |
		enumValueValidationMask |
		enumSiblingValidationMask |
		primitiveValueValidationMask |
		attributeDeprecationValidationMask |
		validationObjectDeprecatedMask |
		validationSchemaBugObjectMissingMask |
		constraintValidationMask
)

func compileValidationPolicy(rules []ValidationPolicyRule) (levelPolicy, error) {
	policy := defaultValidationPolicy()
	for _, rule := range rules {
		if rule.All {
			if !rule.Level.Valid() {
				return policy, fmt.Errorf("validation policy has invalid all-code level %d", rule.Level)
			}
			setValidationPolicyMask(&policy, allValidationCodesMask, rule.Level)
			continue
		}
		if !rule.Code.Valid() {
			return policy, fmt.Errorf("validation policy has unknown validation code %d", rule.Code)
		}
		if !rule.Level.Valid() {
			return policy, fmt.Errorf("validation policy has invalid level %d for %s", rule.Level, rule.Code)
		}
		setValidationPolicyLevel(&policy, rule.Code, rule.Level)
	}
	return policy, nil
}

func defaultValidationPolicy() levelPolicy {
	return levelPolicy{
		ignored: defaultValidationIgnoredMask,
		warning: defaultValidationWarningMask,
		error:   defaultValidationErrorMask,
	}
}

func setValidationPolicyLevel(policy *levelPolicy, code validation.Code, level validation.Level) {
	setValidationPolicyMask(policy, uint64(1)<<code, level)
}

func setValidationPolicyMask(policy *levelPolicy, mask uint64, level validation.Level) {
	policy.ignored &^= mask
	policy.warning &^= mask
	policy.error &^= mask
	switch level {
	case validation.LevelIgnored:
		policy.ignored |= mask
	case validation.LevelWarning:
		policy.warning |= mask
	case validation.LevelError:
		policy.error |= mask
	}
}
