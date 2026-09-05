package processing

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/stretchr/testify/require"
)

func TestIssuePolicyAllIgnoredNeverIgnoresMandatoryCodes(t *testing.T) {
	policy, err := compileIssuePolicy([]IssueLevelRule{{All: true, Level: issue.LevelIgnored}})
	require.NoError(t, err)

	require.Equal(t, issue.LevelWarning, effectiveIssueLevel(policy, issue.EventTraversalLimited))
}

func TestIssuePolicyResolvesConfiguredTolerableCodeLevels(t *testing.T) {
	require.Equal(t, issue.LevelWarning, effectiveIssueLevel(levelPolicy{}, issue.EnrichmentEnumSiblingNotAdded))
	policy, err := compileIssuePolicy([]IssueLevelRule{{
		Code: issue.EnrichmentEnumSiblingNotAdded, Level: issue.LevelIgnored,
	}})
	require.NoError(t, err)
	require.Equal(t, issue.LevelIgnored, effectiveIssueLevel(policy, issue.EnrichmentEnumSiblingNotAdded))
}

func TestIssuePolicyUsesExplicitLevelsOverAllLevel(t *testing.T) {
	policy, err := compileIssuePolicy([]IssueLevelRule{
		{All: true, Level: issue.LevelIgnored},
		{Code: issue.EnrichmentEnumSiblingNotAdded, Level: issue.LevelWarning},
	})
	require.NoError(t, err)

	require.Equal(t, issue.LevelWarning, effectiveIssueLevel(policy, issue.EnrichmentEnumSiblingNotAdded))
	require.Equal(t, issue.LevelIgnored, effectiveIssueLevel(policy, issue.EnrichmentObservableDuplicateSkipped))
}
