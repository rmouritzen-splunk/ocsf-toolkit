package processing

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/stretchr/testify/require"
)

func TestIssueSuppressionNeverSuppressesMandatoryCodes(t *testing.T) {
	suppressAll := issueSuppression{configured: true}
	suppressSelected := issueSuppression{
		configured: true,
		codes:      map[issue.IssueCode]struct{}{issue.EventTraversalLimited: {}},
	}

	require.False(t, suppressAll.suppresses(issue.EventTraversalLimited))
	require.False(t, suppressSelected.suppresses(issue.EventTraversalLimited))
}

func TestIssueSuppressionSuppressesConfiguredTolerableCodes(t *testing.T) {
	require.False(t, (issueSuppression{}).suppresses(issue.EnrichmentEnumSiblingNotAdded))
	require.True(t, (issueSuppression{configured: true}).suppresses(issue.EnrichmentEnumSiblingNotAdded))
	require.True(t, (issueSuppression{
		configured: true,
		codes:      map[issue.IssueCode]struct{}{issue.EnrichmentEnumSiblingNotAdded: {}},
	}).suppresses(issue.EnrichmentEnumSiblingNotAdded))
}

func TestIssueSuppressionSuppressesOnlyCodesInMultiCodeSelection(t *testing.T) {
	suppression := issueSuppression{
		configured: true,
		codes: map[issue.IssueCode]struct{}{
			issue.EnrichmentEnumSiblingNotAdded:        {},
			issue.EnrichmentObservableDuplicateSkipped: {},
		},
	}

	require.True(t, suppression.suppresses(issue.EnrichmentEnumSiblingNotAdded))
	require.True(t, suppression.suppresses(issue.EnrichmentObservableDuplicateSkipped))
	require.False(t, suppression.suppresses(issue.EnrichmentEnumSiblingOtherAdded))
}
