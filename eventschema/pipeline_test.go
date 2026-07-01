package eventschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func TestNewEventProcessorPipelineRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name       string
		processors []EventProcessor
		wantError  string
	}{
		{name: "no processors", wantError: "at least one event processing action is required"},
		{
			name:       "duplicate validation",
			processors: []EventProcessor{NewValidation(), NewValidation()},
			wantError:  "validation processor may only be configured once",
		},
		{
			name:       "duplicate enrichment",
			processors: []EventProcessor{NewEnrichment(), NewEnrichment()},
			wantError:  "enrichment processor may only be configured once",
		},
		{
			name:       "duplicate removal",
			processors: []EventProcessor{NewEnrichmentRemoval(), NewEnrichmentRemoval()},
			wantError:  "enrichment-removal processor may only be configured once",
		},
		{
			name: "enrichment without action",
			processors: []EventProcessor{NewEnrichment(
				WithAddEnumSiblings(false),
				WithAddObservables(false),
			)},
			wantError: "enrichment processor must enable at least one action",
		},
		{
			name: "removal without action",
			processors: []EventProcessor{NewEnrichmentRemoval(
				WithRemoveEnumSiblings(false),
				WithRemoveObservables(false),
			)},
			wantError: "enrichment-removal processor must enable at least one action",
		},
		{
			name: "add and remove enum siblings",
			processors: []EventProcessor{
				NewEnrichment(WithAddObservables(false)),
				NewEnrichmentRemoval(WithRemoveObservables(false)),
			},
			wantError: "adding and removing enum siblings are mutually exclusive",
		},
		{
			name: "add and remove observables",
			processors: []EventProcessor{
				NewEnrichment(WithAddEnumSiblings(false)),
				NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
			},
			wantError: "adding and removing observables are mutually exclusive",
		},
		{
			name: "retain and force enum siblings",
			processors: []EventProcessor{NewEnrichmentRemoval(
				WithRemoveEnumSiblings(false),
				WithForceRemoveEnumSiblings(),
			)},
			wantError: "enrichment-removal processor forces and retains enum siblings",
		},
		{
			name: "force and retain observables",
			processors: []EventProcessor{NewEnrichmentRemoval(
				WithForceRemoveObservables(),
				WithRemoveObservables(false),
			)},
			wantError: "enrichment-removal processor forces and retains observables",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipeline, err := (&schemaImpl{}).NewEventProcessorPipeline(test.processors...)

			require.EqualError(t, err, test.wantError)
			require.Nil(t, pipeline)
		})
	}
}

func TestNewEventProcessorPipelineReportsAllConfigurationProblems(t *testing.T) {
	pipeline, err := (&schemaImpl{}).NewEventProcessorPipeline(
		NewEnrichment(WithAddEnumSiblings(false), WithAddObservables(false)),
		NewEnrichment(WithAddObservables(false)),
		NewEnrichmentRemoval(
			WithRemoveEnumSiblings(false),
			WithRemoveObservables(false),
			WithForceRemoveEnumSiblings(),
		),
	)

	require.Nil(t, pipeline)
	require.EqualError(t, err, "enrichment processor may only be configured once\n"+
		"enrichment processor at position 1 must enable at least one action\n"+
		"enrichment-removal processor forces and retains enum siblings\n"+
		"adding and removing enum siblings are mutually exclusive")
	joined, ok := err.(interface{ Unwrap() []error })
	require.True(t, ok)
	require.Len(t, joined.Unwrap(), 4)
}

func TestNewEventProcessorPipelineAllowsNonConflictingMutationProcesses(t *testing.T) {
	pipeline, err := (&schemaImpl{}).NewEventProcessorPipeline(
		NewEnrichment(WithAddObservables(false)),
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	)

	require.NoError(t, err)
	require.NotNil(t, pipeline)
}

func TestEventProcessorPipelineProcessesDistinctEventsConcurrently(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(), NewValidation())

	const workerCount = 16
	const eventsPerWorker = 25
	start := make(chan struct{})
	errorsFound := make(chan error, workerCount)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			<-start
			for range eventsPerWorker {
				event := validValidationEvent()
				event["mode_id"] = json.Number("1")
				event["ball"] = jsonish.Map{"green": "go"}

				result, err := pipeline.ProcessEvent(event)
				if err != nil {
					errorsFound <- err
					return
				}
				if len(result.Validation.Errors) != 0 || len(result.Validation.Warnings) != 0 {
					errorsFound <- fmt.Errorf("unexpected validation result: %+v", result.Validation)
					return
				}
				if event["class_name"] != "Alpha" || event["activity_name"] != "Do" || event["mode"] != "Known" {
					errorsFound <- fmt.Errorf("event was not enriched as expected: %v", event)
					return
				}
				if _, present := event["observables"]; !present {
					errorsFound <- errors.New("event observables were not enriched")
					return
				}
			}
		}()
	}

	close(start)
	workers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		assert.NoError(err)
	}
}
