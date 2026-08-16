package processing

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/stretchr/testify/require"
)

func TestPipelineConfigValidateReportsInvalidActions(t *testing.T) {
	config := PipelineConfig{
		EnumSiblingsAction: enrichment.Action("invalid"),
		ObservablesAction:  enrichment.Action("invalid"),
		Observables:        ObservablesConfig{PathNotation: pathstyle.Simple},
	}

	err := config.Validate()

	require.EqualError(t, err, `invalid enum siblings action "invalid"`+"\n"+
		`invalid observables action "invalid"`)
}

func TestPipelineConfigValidateReportsInvalidObservablesConfiguration(t *testing.T) {
	config := PipelineConfig{
		EnumSiblingsAction: enrichment.None,
		ObservablesAction:  enrichment.None,
		Observables: ObservablesConfig{
			PathNotation:           pathstyle.Style("invalid"),
			PathNotationConfigured: true,
		},
	}

	err := config.Validate()

	require.EqualError(t, err, "at least one event processing action is required\n"+
		"observable path notation is configured without adding observables")
}

func TestPipelineConfigValidateReportsInvalidObservablePathNotationWhenAddingObservables(t *testing.T) {
	config := PipelineConfig{
		EnumSiblingsAction: enrichment.None,
		ObservablesAction:  enrichment.Add,
		Observables: ObservablesConfig{
			PathNotation: pathstyle.Style("invalid"),
		},
	}

	err := config.Validate()

	require.EqualError(t, err, `invalid observable path notation "invalid"`)
}
