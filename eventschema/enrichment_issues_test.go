package eventschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func TestEnrichmentReportsEnumSiblingItCannotAdd(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["activity_id"] = json.Number("1234")

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddObservables(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "activity_name")
	issues := issuesWithCode(result.Issues, "enrichment_enum_sibling_not_added")
	assert.Len(issues, 1)
	assert.Equal(issuePhaseEnrichment, issues[0].Phase)
}

func TestEnrichmentReportsObservableObjectWithWrongType(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	event := jsonish.Map{
		"class_uid": json.Number("1"),
		"ball":      "not an object",
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	issues := issuesWithCode(result.Issues, "enrichment_observable_not_added_wrong_type")
	assert.Len(issues, 1)
	assert.Equal("ball", issues[0].AttributePath)
}

func TestEnrichmentAddsObjectObservableDefinedOnAttribute(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	observableTypeID := int64(2000)
	schema.classes[int64(1)].Attributes["ball"].Observable = &observableTypeID
	event := jsonish.Map{
		"class_uid": json.Number("1"),
		"ball":      jsonish.Map{"green": "go"},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(2, result.Enrichment.ObservablesAdded)
	observables, ok := event["observables"].([]jsonish.Map)
	assert.True(ok)
	assert.Contains(observables, jsonish.Map{"name": "ball", "type_id": observableTypeID})
}

func TestEnrichmentReportsObservableArrayWithWrongType(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	observableTypeID := int64(1000)
	schema.classes[int64(1)].Attributes["statuses"].Observable = &observableTypeID
	event := validValidationEvent()
	event["statuses"] = "not an array"

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	issues := issuesWithCode(result.Issues, "enrichment_observable_not_added_wrong_type")
	assert.Len(issues, 1)
	assert.Equal("statuses", issues[0].AttributePath)
}

func TestEnrichmentPreservesExistingObservablesAndReportsGeneratedEntries(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	existing := []any{jsonish.Map{"name": "red", "type_id": json.Number("1000"), "value": "existing"}}
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"ball":        jsonish.Map{"green": "go"},
		"observables": existing,
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(existing, event["observables"])
	assert.Zero(result.Enrichment.ObservablesAdded)
	issues := issuesWithCode(result.Issues, "enrichment_observables_not_added_existing")
	assert.Len(issues, 1)
	assert.Equal(1, issues[0].Details["generated_observables"])
}
