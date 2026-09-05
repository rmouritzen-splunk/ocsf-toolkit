package processing

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

func TestPipelineConfigCompilationReportsInvalidActions(t *testing.T) {
	config := PipelineConfig{
		EnumSiblingsAction: enrichment.Action("invalid"),
		ObservablesAction:  enrichment.Action("invalid"),
		Observables:        ObservablesConfig{PathNotation: pathstyle.Simple},
	}

	_, err := config.validateAndCompileLevelPolicies()

	require.EqualError(t, err, `invalid enum siblings action "invalid"`)
}

func TestPipelineConfigCompilationReportsInvalidObservablesConfiguration(t *testing.T) {
	config := PipelineConfig{
		EnumSiblingsAction: enrichment.None,
		ObservablesAction:  enrichment.None,
		Observables: ObservablesConfig{
			PathNotation:           pathstyle.Style("invalid"),
			PathNotationConfigured: true,
		},
	}

	_, err := config.validateAndCompileLevelPolicies()

	require.EqualError(t, err, "at least one event processing action is required")
}

func TestPipelineConfigCompilationReportsInvalidObservablePathNotationWhenAddingObservables(t *testing.T) {
	config := PipelineConfig{
		EnumSiblingsAction: enrichment.None,
		ObservablesAction:  enrichment.Add,
		Observables: ObservablesConfig{
			PathNotation: pathstyle.Style("invalid"),
		},
	}

	_, err := config.validateAndCompileLevelPolicies()

	require.EqualError(t, err, `invalid observable path notation "invalid"`)
}

func TestIssuePolicyCompilationReturnsFirstProblem(t *testing.T) {
	_, err := compileIssuePolicy([]IssueLevelRule{
		{Code: issue.Code(255), Level: issue.LevelWarning},
		{Code: issue.EnrichmentEnumSiblingNotAdded, Level: issue.Level(255)},
	})

	require.EqualError(t, err, "issue policy has unknown issue code 255")
	_, unwrapsErrors := err.(interface{ Unwrap() []error })
	require.False(t, unwrapsErrors)
}

func TestValidationPolicyCompilationReturnsFirstProblem(t *testing.T) {
	_, err := compileValidationPolicy([]ValidationPolicyRule{
		{Code: validation.Code(255), Level: validation.LevelWarning},
		{Code: validation.AttributeUnknown, Level: validation.Level(255)},
	})

	require.EqualError(t, err, "validation policy has unknown validation code 255")
	_, unwrapsErrors := err.(interface{ Unwrap() []error })
	require.False(t, unwrapsErrors)
}
