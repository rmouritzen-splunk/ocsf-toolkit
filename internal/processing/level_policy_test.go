package processing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestEngineeringInvariantLevelPolicyMasksMatchPublicCodes(t *testing.T) {
	// Engineering invariant test: every public validation and issue code must fit in the internal uint64 policy
	// masks, occur in exactly one default level mask, and retain its public default and ignorable classification.
	t.Run("validation", func(t *testing.T) {
		policy := defaultValidationPolicy()
		var codesMask uint64
		for _, code := range validation.Codes() {
			require.Greater(t, code, validation.None)
			require.Less(t, code, validation.Code(64))
			mask, present := validationMasksByCode[code]
			require.True(t, present)
			require.Equal(t, uint64(1)<<code, mask)
			codesMask |= mask
			require.Equal(t, 1, matchingLevelMaskCount(policy, mask))
			level, reported := (&validationProcessor{policy: policy}).findingLevel(nil, code)
			require.Equal(t, code.DefaultLevel(), level)
			require.Equal(t, level != validation.LevelIgnored, reported)
		}
		require.Len(t, validationMasksByCode, len(validation.Codes()))
		require.Equal(t, ^uint64(0)&^1, allValidationCodesMask)
		require.Equal(t, codesMask, allValidationCodesMask&codesMask)
	})

	t.Run("issue", func(t *testing.T) {
		policy := defaultIssuePolicy()
		var codesMask uint64
		var mandatoryMask uint64
		for _, code := range issue.Codes() {
			require.Greater(t, code, issue.None)
			require.Less(t, code, issue.Code(64))
			mask, present := issueMasksByCode[code]
			require.True(t, present)
			require.Equal(t, uint64(1)<<code, mask)
			codesMask |= mask
			require.Equal(t, 1, matchingLevelMaskCount(policy, mask))
			require.Equal(t, code.DefaultLevel(), effectiveIssueLevel(policy, code))
			if !code.Ignorable() {
				mandatoryMask |= mask
			}
		}
		require.Len(t, issueMasksByCode, len(issue.Codes()))
		require.Equal(t, ^uint64(0)&^1, allIssueCodesMask)
		require.Equal(t, codesMask, allIssueCodesMask&codesMask)
		require.Equal(t, mandatoryIssueMask, mandatoryMask)
		require.Equal(t, allIssueCodesMask&^mandatoryMask, ignorableIssueMask)
		require.Equal(t, codesMask&^mandatoryMask, ignorableIssueMask&codesMask)
	})
}

func TestEngineeringInvariantLevelPolicyOverridesEveryPublicCode(t *testing.T) {
	// Engineering invariant test: compiling a code-specific override must move only that public code's bit to the
	// requested level mask, for every validation and issue code that the registries expose.
	t.Run("validation", func(t *testing.T) {
		for _, target := range validation.Codes() {
			for _, level := range []validation.Level{
				validation.LevelIgnored,
				validation.LevelWarning,
				validation.LevelError,
			} {
				t.Run(target.String()+"/"+level.String(), func(t *testing.T) {
					baseline := validation.LevelWarning
					if level == baseline {
						baseline = validation.LevelError
					}
					policy, err := compileValidationPolicy([]ValidationPolicyRule{
						{All: true, Level: baseline},
						{Code: target, Level: level},
					})
					require.NoError(t, err)

					for _, code := range validation.Codes() {
						expected := baseline
						if code == target {
							expected = level
						}
						require.Equal(t, expected, (&validationProcessor{policy: policy}).effectiveFindingLevel(code))
						require.Equal(t, 1, matchingLevelMaskCount(policy, validationMasksByCode[code]))
					}

					processor := validationProcessor{policy: policy}
					context := processContext{}
					processor.addFinding(&context, target, nil, "diagnostic message")
					if level == validation.LevelIgnored {
						require.Empty(t, context.result.Validation.Findings)
					} else {
						require.Equal(t, []eventresult.ValidationFinding{{
							Level: level, Code: target, Message: "diagnostic message",
						}}, context.result.Validation.Findings)
					}
				})
			}
		}
	})

	t.Run("issue", func(t *testing.T) {
		for _, target := range issue.Codes() {
			for _, level := range []issue.Level{
				issue.LevelIgnored,
				issue.LevelWarning,
				issue.LevelError,
			} {
				t.Run(target.String()+"/"+level.String(), func(t *testing.T) {
					config := IssuePolicyConfig{LevelRules: []IssueLevelRule{
						{All: true, Level: issue.LevelWarning},
						{Code: target, Level: level},
					}}
					policy, err := compileIssuePolicy(config.LevelRules)
					if level == issue.LevelIgnored && !target.Ignorable() {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)

					for _, code := range issue.Codes() {
						expected := issue.LevelWarning
						if code == target {
							expected = level
						}
						require.Equal(t, expected, effectiveIssueLevel(policy, code))
						require.Equal(t, 1, matchingLevelMaskCount(policy, issueMasksByCode[code]))
					}

					context := processContext{pipelineImpl: &PipelineImpl{issuePolicy: policy}}
					issueErr := context.addProcessorIssue(issue.SourceProcessing, target, nil, "diagnostic message")
					switch level {
					case issue.LevelIgnored:
						require.NoError(t, issueErr)
						require.Empty(t, context.result.Issues)
					case issue.LevelWarning:
						require.NoError(t, issueErr)
						require.Equal(t, []eventresult.ProcessingIssue{{
							Source: issue.SourceProcessing, Code: target, Message: "diagnostic message",
						}}, context.result.Issues)
					case issue.LevelError:
						require.Error(t, issueErr)
						require.Empty(t, context.result.Issues)
					}
				})
			}
		}
	})
}

func TestEngineeringInvariantIgnoredValidationWorkMasksRequireEveryCode(t *testing.T) {
	// Engineering invariant test: shared validation work is skipped only when every finding that the work can emit is
	// ignored; enabling any one of those findings must retain the shared work.
	workMasks := []struct {
		name string
		mask uint64
	}{
		{"requirements", requirementValidationMask},
		{"enum values", enumValueValidationMask},
		{"enum siblings", enumSiblingValidationMask},
		{"primitive values", primitiveValueValidationMask},
		{"attribute deprecation", attributeDeprecationValidationMask},
		{"constraints", constraintValidationMask},
		{"version", versionValidationMask},
		{"type uid", typeUIDValidationMask},
		{"observables", observableValidationMask},
		{"event walk", eventWalkValidationMask},
	}

	for _, work := range workMasks {
		t.Run(work.name, func(t *testing.T) {
			require.NotZero(t, work.mask)
			policy := levelPolicy{ignored: work.mask}
			require.True(t, policy.isIgnored(work.mask))

			enabledCode := work.mask & -work.mask
			policy.ignored &^= enabledCode
			policy.warning |= enabledCode
			require.False(t, policy.isIgnored(work.mask))
		})
	}
}

func matchingLevelMaskCount(policy levelPolicy, mask uint64) int {
	count := 0
	if policy.isIgnored(mask) {
		count++
	}
	if policy.isWarning(mask) {
		count++
	}
	if policy.isError(mask) {
		count++
	}
	return count
}

var validationMasksByCode = map[validation.Code]uint64{
	validation.ClassUIDUnknown:                         validationClassUIDUnknownMask,
	validation.ProfileUnknown:                          validationProfileUnknownMask,
	validation.AttributeEnumValueUnknown:               validationAttributeEnumValueUnknownMask,
	validation.AttributeEnumArrayValueUnknown:          validationAttributeEnumArrayValueUnknownMask,
	validation.AttributeEnumSiblingWithoutEnum:         validationAttributeEnumSiblingWithoutEnumMask,
	validation.SchemaBugObjectMissing:                  validationSchemaBugObjectMissingMask,
	validation.AttributeEnumArraySiblingMissing:        validationAttributeEnumArraySiblingMissingMask,
	validation.AttributeEnumArraySiblingIncorrect:      validationAttributeEnumArraySiblingIncorrectMask,
	validation.AttributeEnumArraySiblingLengthMismatch: validationAttributeEnumArraySiblingLengthMismatchMask,
	validation.SchemaBugTypeMissing:                    validationSchemaBugTypeMissingMask,
	validation.SchemaBugPrimitiveTypeUnknown:           validationSchemaBugPrimitiveTypeUnknownMask,
	validation.AttributeValueNotInSuperTypeValues:      validationAttributeValueNotInSuperTypeValuesMask,
	validation.AttributeValueNotInTypeValues:           validationAttributeValueNotInTypeValuesMask,
	validation.AttributeValueExceedsSuperTypeRange:     validationAttributeValueExceedsSuperTypeRangeMask,
	validation.AttributeValueExceedsRange:              validationAttributeValueExceedsRangeMask,
	validation.AttributeValueExceedsSuperTypeMaxLen:    validationAttributeValueExceedsSuperTypeMaxLenMask,
	validation.AttributeValueExceedsMaxLen:             validationAttributeValueExceedsMaxLenMask,
	validation.SchemaBugTypeRegexInvalid:               validationSchemaBugTypeRegexInvalidMask,
	validation.AttributeUnknown:                        validationAttributeUnknownMask,
	validation.AttributeRequiresProfile:                validationAttributeRequiresProfileMask,
	validation.VersionInvalidFormat:                    validationVersionInvalidFormatMask,
	validation.VersionIncompatibleInitialDevelopment:   validationVersionIncompatibleInitialDevelopmentMask,
	validation.VersionIncompatiblePrerelease:           validationVersionIncompatiblePrereleaseMask,
	validation.VersionIncompatibleLater:                validationVersionIncompatibleLaterMask,
	validation.TypeUIDExpectedValueOverflow:            validationTypeUIDExpectedValueOverflowMask,
	validation.TypeUIDIncorrect:                        validationTypeUIDIncorrectMask,
	validation.ConstraintFailed:                        validationConstraintFailedMask,
	validation.ConstraintUnknown:                       validationConstraintUnknownMask,
	validation.ObservableNameInvalidSyntax:             validationObservableNameInvalidSyntaxMask,
	validation.ObservableNameInvalidReference:          validationObservableNameInvalidReferenceMask,
	validation.ObservablePathNotFound:                  validationObservablePathNotFoundMask,
	validation.ObservablePathNotObject:                 validationObservablePathNotObjectMask,
	validation.ObservableValueNotFound:                 validationObservableValueNotFoundMask,
	validation.AttributeRequiredMissing:                validationAttributeRequiredMissingMask,
	validation.AttributeWrongType:                      validationAttributeWrongTypeMask,
	validation.ClassDeprecated:                         validationClassDeprecatedMask,
	validation.AttributeDeprecated:                     validationAttributeDeprecatedMask,
	validation.AttributeTypeDeprecated:                 validationAttributeTypeDeprecatedMask,
	validation.ObjectDeprecated:                        validationObjectDeprecatedMask,
	validation.AttributeRecommendedMissing:             validationAttributeRecommendedMissingMask,
	validation.AttributeEnumSiblingSuspicious:          validationAttributeEnumSiblingSuspiciousMask,
	validation.AttributeEnumSiblingIncorrect:           validationAttributeEnumSiblingIncorrectMask,
	validation.AttributeEnumValueDeprecated:            validationAttributeEnumValueDeprecatedMask,
	validation.AttributeEnumArrayValueDeprecated:       validationAttributeEnumArrayValueDeprecatedMask,
	validation.AttributeValueSuperTypeRegexNotMatched:  validationAttributeValueSuperTypeRegexNotMatchedMask,
	validation.AttributeValueRegexNotMatched:           validationAttributeValueRegexNotMatchedMask,
	validation.VersionEarlier:                          validationVersionEarlierMask,
	validation.ObservableNamePathNotation:              validationObservableNamePathNotationMask,
	validation.ClassUIDMissing:                         validationClassUIDMissingMask,
	validation.ClassUIDWrongType:                       validationClassUIDWrongTypeMask,
}

var issueMasksByCode = map[issue.Code]uint64{
	issue.EventTraversalLimited:                    issueEventTraversalLimitedMask,
	issue.EnrichmentObservableNotAddedWrongType:    issueEnrichmentObservableNotAddedWrongTypeMask,
	issue.EnrichmentObservableNotAddedJSONType:     issueEnrichmentObservableNotAddedJSONTypeMask,
	issue.EnrichmentEnumSiblingNotAdded:            issueEnrichmentEnumSiblingNotAddedMask,
	issue.EnrichmentEnumSiblingOtherAdded:          issueEnrichmentEnumSiblingOtherAddedMask,
	issue.EnrichmentObservablesNotAddedWrongType:   issueEnrichmentObservablesNotAddedWrongTypeMask,
	issue.EnrichmentObservableDuplicateSkipped:     issueEnrichmentObservableDuplicateSkippedMask,
	issue.EnrichmentRemovalEnumSiblingNotRemoved:   issueEnrichmentRemovalEnumSiblingNotRemovedMask,
	issue.ObservableArrayWrongType:                 issueObservableArrayWrongTypeMask,
	issue.ObservableElementWrongType:               issueObservableElementWrongTypeMask,
	issue.ObservableNameMissing:                    issueObservableNameMissingMask,
	issue.ObservableNameWrongType:                  issueObservableNameWrongTypeMask,
	issue.ObservableNameInvalidSyntax:              issueObservableNameInvalidSyntaxMask,
	issue.ObservableNameInvalidReference:           issueObservableNameInvalidReferenceMask,
	issue.ObservablePathNotFound:                   issueObservablePathNotFoundMask,
	issue.ObservablePathNotObject:                  issueObservablePathNotObjectMask,
	issue.ObservableValueWrongType:                 issueObservableValueWrongTypeMask,
	issue.ObservableValueNotFound:                  issueObservableValueNotFoundMask,
	issue.ClassUIDMissing:                          issueClassUIDMissingMask,
	issue.ClassUIDWrongType:                        issueClassUIDWrongTypeMask,
	issue.ClassUIDUnknown:                          issueClassUIDUnknownMask,
	issue.AtInitSchemaEnumSiblingSourceNotIntegral: issueAtInitSchemaEnumSiblingSourceNotIntegralMask,
	issue.AtInitSchemaEnumSiblingTargetNotFound:    issueAtInitSchemaEnumSiblingTargetNotFoundMask,
	issue.AtInitSchemaEnumSiblingTargetIsEnum:      issueAtInitSchemaEnumSiblingTargetIsEnumMask,
	issue.AtInitSchemaEnumSiblingTargetNotString:   issueAtInitSchemaEnumSiblingTargetNotStringMask,
}
