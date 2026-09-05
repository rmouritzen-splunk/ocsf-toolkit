package processing

import (
	"fmt"

	"github.com/ocsf/ocsf-toolkit/issue"
)

const (
	issueEventTraversalLimitedMask                    uint64 = 1 << issue.EventTraversalLimited
	issueEnrichmentObservableNotAddedWrongTypeMask    uint64 = 1 << issue.EnrichmentObservableNotAddedWrongType
	issueEnrichmentObservableNotAddedJSONTypeMask     uint64 = 1 << issue.EnrichmentObservableNotAddedJSONType
	issueEnrichmentEnumSiblingNotAddedMask            uint64 = 1 << issue.EnrichmentEnumSiblingNotAdded
	issueEnrichmentEnumSiblingOtherAddedMask          uint64 = 1 << issue.EnrichmentEnumSiblingOtherAdded
	issueEnrichmentObservablesNotAddedWrongTypeMask   uint64 = 1 << issue.EnrichmentObservablesNotAddedWrongType
	issueEnrichmentObservableDuplicateSkippedMask     uint64 = 1 << issue.EnrichmentObservableDuplicateSkipped
	issueEnrichmentRemovalEnumSiblingNotRemovedMask   uint64 = 1 << issue.EnrichmentRemovalEnumSiblingNotRemoved
	issueObservableArrayWrongTypeMask                 uint64 = 1 << issue.ObservableArrayWrongType
	issueObservableElementWrongTypeMask               uint64 = 1 << issue.ObservableElementWrongType
	issueObservableNameMissingMask                    uint64 = 1 << issue.ObservableNameMissing
	issueObservableNameWrongTypeMask                  uint64 = 1 << issue.ObservableNameWrongType
	issueObservableNameInvalidSyntaxMask              uint64 = 1 << issue.ObservableNameInvalidSyntax
	issueObservableNameInvalidReferenceMask           uint64 = 1 << issue.ObservableNameInvalidReference
	issueObservablePathNotFoundMask                   uint64 = 1 << issue.ObservablePathNotFound
	issueObservablePathNotObjectMask                  uint64 = 1 << issue.ObservablePathNotObject
	issueObservableValueWrongTypeMask                 uint64 = 1 << issue.ObservableValueWrongType
	issueObservableValueNotFoundMask                  uint64 = 1 << issue.ObservableValueNotFound
	issueClassUIDMissingMask                          uint64 = 1 << issue.ClassUIDMissing
	issueClassUIDWrongTypeMask                        uint64 = 1 << issue.ClassUIDWrongType
	issueClassUIDUnknownMask                          uint64 = 1 << issue.ClassUIDUnknown
	issueAtInitSchemaEnumSiblingSourceNotIntegralMask uint64 = 1 << issue.AtInitSchemaEnumSiblingSourceNotIntegral
	issueAtInitSchemaEnumSiblingTargetNotFoundMask    uint64 = 1 << issue.AtInitSchemaEnumSiblingTargetNotFound
	issueAtInitSchemaEnumSiblingTargetIsEnumMask      uint64 = 1 << issue.AtInitSchemaEnumSiblingTargetIsEnum
	issueAtInitSchemaEnumSiblingTargetNotStringMask   uint64 = 1 << issue.AtInitSchemaEnumSiblingTargetNotString

	// All masks except 0 set to true.
	allIssueCodesMask  uint64 = ^uint64(0) &^ 1
	mandatoryIssueMask uint64 = issueEventTraversalLimitedMask |
		issueClassUIDMissingMask |
		issueClassUIDWrongTypeMask |
		issueClassUIDUnknownMask
	ignorableIssueMask uint64 = allIssueCodesMask &^ mandatoryIssueMask
)

func compileIssuePolicy(rules []IssueLevelRule) (levelPolicy, error) {
	policy := defaultIssuePolicy()
	for _, rule := range rules {
		if rule.All {
			if !rule.Level.Valid() {
				return policy, fmt.Errorf("issue policy has invalid all-code level %d", rule.Level)
			}
			mask := allIssueCodesMask
			if rule.Level == issue.LevelIgnored {
				mask = ignorableIssueMask
			}
			setIssuePolicyMask(&policy, mask, rule.Level)
			continue
		}
		if !rule.Code.Valid() {
			return policy, fmt.Errorf("issue policy has unknown issue code %d", rule.Code)
		}
		if !rule.Level.Valid() {
			return policy, fmt.Errorf("issue policy has invalid level %d for %s", rule.Level, rule.Code)
		}
		if rule.Level == issue.LevelIgnored && !rule.Code.Ignorable() {
			return policy, fmt.Errorf("issue policy cannot ignore mandatory issue code %s", rule.Code)
		}
		setIssuePolicyLevel(&policy, rule.Code, rule.Level)
	}
	return policy, nil
}

func defaultIssuePolicy() levelPolicy {
	return levelPolicy{warning: allIssueCodesMask}
}

func setIssuePolicyLevel(policy *levelPolicy, code issue.Code, level issue.Level) {
	setIssuePolicyMask(policy, uint64(1)<<code, level)
}

func setIssuePolicyMask(policy *levelPolicy, mask uint64, level issue.Level) {
	policy.ignored &^= mask
	policy.warning &^= mask
	policy.error &^= mask
	switch level {
	case issue.LevelIgnored:
		policy.ignored |= mask
	case issue.LevelWarning:
		policy.warning |= mask
	case issue.LevelError:
		policy.error |= mask
	}
}

func effectiveIssueLevel(policy levelPolicy, code issue.Code) issue.Level {
	mask := uint64(1) << code
	switch {
	case policy.isIgnored(mask):
		return issue.LevelIgnored
	case policy.isWarning(mask):
		return issue.LevelWarning
	case policy.isError(mask):
		return issue.LevelError
	default:
		return code.DefaultLevel()
	}
}
