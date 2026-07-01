package eventschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func TestEnrichmentRemovalSafelyRemovesScalarEnumSiblings(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	schema.classes[int64(1)].Attributes["mode_id"].Enum["99"] = &enumDefinition{Caption: "Other"}

	event := validValidationEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "Do"
	event["mode_id"] = json.Number("99")
	event["mode"] = "Other"
	event["status_ids"] = []any{json.Number("1")}
	event["statuses"] = []any{"Open"}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "class_name")
	assert.NotContains(event, "activity_name")
	assert.Equal("Other", event["mode"], "enum ID 99 siblings are always retained")
	assert.Contains(event, "statuses", "legacy enum array siblings are unsupported and retained")
	assert.Equal(2, result.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Equal(2, result.EnrichmentRemoval.EnumSiblingsRetained)
}

func TestEnrichmentRemovalForceRemovesMismatchedScalarEnumSibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["activity_name"] = "source-specific"

	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(
		WithForceRemoveEnumSiblings(),
		WithRemoveObservables(false),
	))
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "activity_name")
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRemoved)
}

func TestEnrichmentRemovalForceRetainsEnumID99Sibling(t *testing.T) {
	tests := []struct {
		name    string
		sibling string
	}{
		{name: "caption", sibling: "Other"},
		{name: "source-specific value", sibling: "source-specific"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			schema.classes[int64(1)].Attributes["mode_id"].Enum["99"] = &enumDefinition{Caption: "Other"}
			event := validValidationEvent()
			event["mode_id"] = json.Number("99")
			event["mode"] = test.sibling

			pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(
				WithForceRemoveEnumSiblings(),
				WithRemoveObservables(false),
			))
			result, err := pipeline.ProcessEvent(event)

			assert.NoError(err)
			assert.Equal(test.sibling, event["mode"])
			assert.Equal(0, result.EnrichmentRemoval.EnumSiblingsRemoved)
			assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
		})
	}
}

func TestEnrichmentRemovalSafelyRemovesScalarAndObjectObservables(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["balls"] = []any{
		jsonish.Map{"green": "first"},
		jsonish.Map{"green": "second"},
	}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
		jsonish.Map{"name": "ball", "type_id": json.Number("1000")},
		jsonish.Map{"name": "balls[].green", "type_id": json.Number("1000"), "value": "second"},
		jsonish.Map{"name": "balls[0].green", "type_id": json.Number("1000"), "value": "second"},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(3, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	observables, ok := event["observables"].([]any)
	assert.True(ok)
	assert.Len(observables, 1)
	assert.Equal("observable_value_not_found", result.Issues[0].Code)
}

func TestEnrichmentRemovalMatchesObservableValuesAfterScalarStringConversion(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["port"] = json.Number("443")
	event["observables"] = []any{
		jsonish.Map{"name": "port", "type_id": 11, "value": "443"},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
}

func TestEnrichmentRemovalSupportsAllObservableArrayPathForms(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	forms := []string{"balls.green", "balls[].green", "balls[*].green", "balls[1].green", "$.balls[1].green"}
	observables := make([]any, 0, len(forms))
	for _, name := range forms {
		observables = append(observables, jsonish.Map{
			"name": name, "type_id": json.Number("1000"), "value": "second",
		})
	}
	event := validValidationEvent()
	event["balls"] = []any{jsonish.Map{"green": "first"}, jsonish.Map{"green": "second"}}
	event["observables"] = observables

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(len(forms), result.EnrichmentRemoval.ObservablesRemoved)
	assert.Empty(result.Issues)
}

func TestEnrichmentRemovalResolvesNestedObjectArrays(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	event := validValidationEvent()
	event["balls"] = []any{
		jsonish.Map{
			"green": "outer",
			"children": []any{
				jsonish.Map{"green": "inner-first"},
				jsonish.Map{"green": "inner-second"},
			},
		},
	}
	event["observables"] = []any{
		jsonish.Map{"name": "balls[].children.green", "type_id": 1000, "value": "inner-second"},
		jsonish.Map{"name": "balls[0].children[0].green", "type_id": 1000, "value": "inner-first"},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(2, result.EnrichmentRemoval.ObservablesRemoved)
}

func TestEnrichmentRemovalReportsMalformedObservableAndForceRemovesIt(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["observables"] = []any{jsonish.Map{"name": "ball[nope].green", "value": "go"}}

	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(
		WithRemoveEnumSiblings(false),
		WithForceRemoveObservables(),
	))
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Len(result.Issues, 1)
	assert.Equal(issuePhaseEnrichmentRemoval, result.Issues[0].Phase)
	assert.Equal("observable_name_invalid_syntax", result.Issues[0].Code)
}

func TestEnrichmentRemovalReportsMalformedObservables(t *testing.T) {
	tests := []struct {
		name       string
		observable any
		prepare    func(jsonish.Map)
		wantCode   string
	}{
		{
			name:       "attribute is not an array",
			observable: "wrong type",
			wantCode:   "observable_array_wrong_type",
		},
		{
			name:       "element is not an object",
			observable: []any{"wrong type"},
			wantCode:   "observable_element_wrong_type",
		},
		{
			name:       "name is missing",
			observable: []any{jsonish.Map{"value": "go"}},
			wantCode:   "observable_name_missing",
		},
		{
			name:       "name has wrong type",
			observable: []any{jsonish.Map{"name": 7, "value": "go"}},
			wantCode:   "observable_name_wrong_type",
		},
		{
			name:       "name is not defined by schema",
			observable: []any{jsonish.Map{"name": "unknown.value", "value": "go"}},
			wantCode:   "observable_name_invalid_reference",
		},
		{
			name:       "path does not resolve in event",
			observable: []any{jsonish.Map{"name": "ball.green", "value": "go"}},
			wantCode:   "observable_path_not_found",
		},
		{
			name:       "value has wrong type",
			observable: []any{jsonish.Map{"name": "ball.green", "value": 7}},
			prepare: func(event jsonish.Map) {
				event["ball"] = jsonish.Map{"green": "go"}
			},
			wantCode: "observable_value_wrong_type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			event := validValidationEvent()
			event["observables"] = test.observable
			if test.prepare != nil {
				test.prepare(event)
			}

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Contains(event, "observables")
			assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
			assert.Len(result.Issues, 1)
			assert.Equal(test.wantCode, result.Issues[0].Code)
			assert.Equal(issuePhaseEnrichmentRemoval, result.Issues[0].Phase)
		})
	}
}

func TestForceObservableRemovalRunsWithoutResolvedClass(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := jsonish.Map{
		"observables": []any{jsonish.Map{"name": "anything", "value": "value"}},
	}

	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(
		WithRemoveEnumSiblings(false),
		WithForceRemoveObservables(),
	))
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
}

func TestValidationIgnoresObservableEntriesRemovedBeforeTraversal(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": "wrong type", "value": "go"},
		jsonish.Map{"name": "ball[bad].green", "type_id": "wrong type", "value": "missing"},
	}

	pipeline := mustNewEventProcessorPipeline(assert, schema,
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false), WithForceRemoveObservables()),
		NewValidation(),
	)
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Empty(result.Validation.Errors, "validation should inspect the final event after removal")
	assert.Contains(issueCodes(result.Issues), "observable_name_invalid_syntax")
}

func TestValidationUsesObservableValuesAndObjectReferences(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "missing"},
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000")},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewValidation()).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "observable_value_not_found")
	assert.Contains(issueCodes(result.Validation.Errors), "observable_path_not_object")
}

func TestEnrichmentRemovalAndValidationSharePreRemovalObservableAnalysis(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "missing"},
	}

	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false)), NewValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	assert.Contains(issueCodes(result.Validation.Errors), "observable_value_not_found")
}

func addObservableArrayTestAttributes(schema *schemaImpl) {
	trueValue := true
	ballType := "ball"
	schema.classes[int64(1)].Attributes["balls"] = &itemAttributeDefinition{
		commonAttributeDefinition: commonAttributeDefinition{
			Type:       "object_t",
			ObjectType: &ballType,
			IsArray:    &trueValue,
		},
	}
	schema.objects["ball"].Attributes["children"] = &itemAttributeDefinition{
		commonAttributeDefinition: commonAttributeDefinition{
			Type:       "object_t",
			ObjectType: &ballType,
			IsArray:    &trueValue,
		},
	}
}
