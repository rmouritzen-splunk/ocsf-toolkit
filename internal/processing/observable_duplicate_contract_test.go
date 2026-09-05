package processing

import (
	"encoding/json"
	"testing"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

// Invariant test: duplicate validation is optional and produces no finding under the toolkit's default policy.
func TestInvariantObservableDuplicateValidationDefaultsToIgnored(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": int64(1000), "value": "go"},
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewValidation()).ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(issuesWithCode(result.Validation.Findings, "validation_observable_duplicate"))
}

// Invariant test: enabled duplicate validation reports collisions using final observable-array indexes.
func TestInvariantObservableDuplicateValidationReportsFinalArrayIndexes(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": int64(1000), "value": "go"},
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewValidation(WithValidationLevel(validation.ObservableDuplicate, validation.LevelWarning)),
	).ProcessEvent(event)

	assert.NoError(err)
	duplicates := issuesWithCode(result.Validation.Findings, "validation_observable_duplicate")
	assert.Len(duplicates, 1)
	assert.Equal("observables[1]", duplicates[0].Details["attribute_path"])
	assert.Equal(1, duplicates[0].Details["observable_index"])
	assert.Equal(0, duplicates[0].Details["duplicate_of_index"])
}

// Invariant test: an enabled duplicate issue is the sole owner when issue and validation diagnostics are both enabled.
func TestInvariantDuplicateIssueTakesPrecedenceOverDuplicateValidation(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": int64(1000), "value": "go"},
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
	}
	event["balls"] = []any{
		jsonish.Map{"green": "same"},
		jsonish.Map{"green": "same"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddEnumSiblings(false)),
		duplicateObservableWarningConfig(),
		NewValidation(WithValidationLevel(validation.ObservableDuplicate, validation.LevelWarning)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Len(issuesWithCode(result.Issues, "issue_observable_duplicate"), 3)
	assert.Empty(issuesWithCode(result.Validation.Findings, "validation_observable_duplicate"))
}

// Invariant test: generated deduplication mutates the observable array before validation inspects its final state.
func TestInvariantGeneratedDeduplicationRemovesCandidatesBeforeDuplicateValidation(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	event := validValidationEvent()
	event["balls"] = []any{
		jsonish.Map{"green": "same"},
		jsonish.Map{"green": "same"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(
			WithAddEnumSiblings(false),
			WithObservableDeduplication(enrichment.ObservableDeduplicationGenerated),
		),
		NewValidation(WithValidationLevel(validation.ObservableDuplicate, validation.LevelWarning)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	assert.Empty(issuesWithCode(result.Validation.Findings, "validation_observable_duplicate"))
}
