package processing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestEnumSiblingRetentionReasonsHaveStableStrings(t *testing.T) {
	expected := map[enumSiblingRetentionReason]string{
		enumSiblingRetentionReasonSiblingValueWrongType: "sibling_value_wrong_type",
		enumSiblingRetentionReasonEnumValueMissing:      "enum_value_missing",
		enumSiblingRetentionReasonEnumValueWrongType:    "enum_value_wrong_type",
		enumSiblingRetentionReasonEnumValueUnknown:      "enum_value_unknown",
		enumSiblingRetentionReasonSiblingValueMismatch:  "sibling_value_mismatch",
	}

	require.Len(t, expected, int(enumSiblingRetentionReasonCount)-1)
	for offset := range int(enumSiblingRetentionReasonCount) - 1 {
		reason := enumSiblingRetentionReason(offset + 1)
		require.Equal(
			t, expected[reason], reason.String(), "unexpected string for enum sibling retention reason %d", reason,
		)
	}
	require.Empty(t, invalidEnumSiblingRetentionReason.String())
	require.Empty(t, enumSiblingRetentionReasonCount.String())
}

func TestRecordEnumSiblingRetentionRejectsUnexpectedReason(t *testing.T) {
	err := recordEnumSiblingRetention(
		&processContext{}, nil, "mode_id", "mode", enumSiblingRetentionReasonCount, "", issueSuppression{},
	)
	require.ErrorContains(t, err, "unexpected enum sibling retention reason")
}

func TestEnrichmentRemovalSafelyRemovesScalarAndArrayEnumSiblings(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	schema.compiled.Classes[int64(1)].Attributes["mode_id"].Enum["99"] = &enumDefinition{Caption: "Other"}

	event := validValidationEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "Do"
	event["mode_id"] = json.Number("99")
	event["mode"] = "Other"
	event["status_ids"] = []any{json.Number("1")}
	event["statuses"] = []any{"Open"}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "class_name")
	assert.NotContains(event, "activity_name")
	assert.Equal("Other", event["mode"], "enum ID 99 siblings are always retained")
	assert.NotContains(event, "statuses")
	assert.Equal(3, result.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Empty(result.Issues)
}

func TestEnrichmentForceRemovalRemovesIntegralEnumArraySibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "Do"
	event["status_ids"] = []any{json.Number("1")}
	event["statuses"] = []any{"Open"}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichmentRemoval(WithForceRemoveEnumSiblings(), WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "statuses")
	assert.Equal(3, result.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Zero(result.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Empty(result.Issues)
}

func TestEnrichmentRemovalRetainsStringEnumSibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	siblingName := "state_name"
	schema.compiled.Classes[int64(1)].Attributes["state"].Sibling = &siblingName
	schema.compiled.Classes[int64(1)].Attributes[siblingName] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
	}
	event := validValidationEvent()
	event["state"] = "open"
	event[siblingName] = "Open"

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal("Open", event[siblingName])
	assert.Zero(result.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Empty(result.Issues)
}

func TestEnrichmentSafeRemovalRetainsMismatchedIntegralEnumArraySibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("2")}
	event["statuses"] = []any{"Open", "Unexpected"}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(event, "statuses")
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Len(issuesWithCode(result.Issues, "issue_enrichment_removal_enum_sibling_not_removed"), 1)
}

func TestEnrichmentForceRemovalRetainsIntegralEnumArraySiblingContainingOther(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	schema.compiled.Classes[int64(1)].Attributes["status_ids"].Enum["99"] = &enumDefinition{Caption: "Other"}
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("99")}
	event["statuses"] = []any{"Open", "Source-specific"}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichmentRemoval(WithForceRemoveEnumSiblings(), WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(event, "statuses")
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Empty(result.Issues)
}

func TestEnrichmentSafeRemovalRetainsIntegralEnumArraySiblingContainingOther(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	schema.compiled.Classes[int64(1)].Attributes["status_ids"].Enum["99"] = &enumDefinition{Caption: "Other"}
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("99")}
	event["statuses"] = []any{"Open", "Source-specific"}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(event, "statuses")
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Empty(result.Issues)
}

func TestEnrichmentSafeRemovalComparesEnumArraysByIndex(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("2")}
	event["statuses"] = []any{"Closed", "Open"}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(event, "statuses")
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Len(issuesWithCode(result.Issues, "issue_enrichment_removal_enum_sibling_not_removed"), 1)
}

func TestEnrichmentSafeRemovalRetainsEnumArraysWithDifferentLengths(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("2")}
	event["statuses"] = []any{"Open"}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(event, "statuses")
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
	issues := issuesWithCode(result.Issues, "issue_enrichment_removal_enum_sibling_not_removed")
	assert.Len(issues, 1)
	assert.Equal("sibling_value_mismatch", issues[0].Details["reason"])
}

func TestEnrichmentSafeRemovalRetainsStringEnumArraySibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	statusIDs := schema.compiled.Classes[int64(1)].Attributes["status_ids"]
	statusIDs.Type = "string_t"
	statusIDs.Enum = map[string]*enumDefinition{"99": {Caption: "Ninety-nine"}}
	event := validValidationEvent()
	event["status_ids"] = []any{"99"}
	event["statuses"] = []any{"Ninety-nine"}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]any{"Ninety-nine"}, event["statuses"])
	assert.Zero(result.EnrichmentRemoval.EnumSiblingsRemoved)
}

func TestEnrichmentRemovalRemovesNullEnumSiblingRegardlessOfSupport(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)

	event := validValidationEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "Do"
	event["status_ids"] = []any{json.Number("1")}
	event["statuses"] = nil

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "statuses", "a null sibling is equivalent to a missing one and is always removed")
	assert.Equal(
		3,
		result.EnrichmentRemoval.EnumSiblingsRemoved,
		"class_name, activity_name, and the null (unsupported array) statuses are all removed",
	)
	assert.Zero(result.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Empty(result.Issues, "removing a null sibling is unconditional and does not need an issue")
}

func TestEnrichmentRemovalReportsSupportedEnumSiblingItCannotSafelyRemove(t *testing.T) {
	tests := []struct {
		name      string
		enumValue any
		sibling   any
		reason    string
	}{
		{
			name:      "sibling value mismatch",
			enumValue: json.Number("1"),
			sibling:   "source-specific",
			reason:    "sibling_value_mismatch",
		},
		{
			name:      "sibling value wrong type",
			enumValue: json.Number("1"),
			sibling:   json.Number("1"),
			reason:    "sibling_value_wrong_type",
		},
		{name: "enum value wrong type", enumValue: "1", sibling: "Do", reason: "enum_value_wrong_type"},
		{
			name:      "enum value unknown",
			enumValue: json.Number("1234"),
			sibling:   "source-specific",
			reason:    "enum_value_unknown",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			event := validValidationEvent()
			event["activity_id"] = test.enumValue
			event["activity_name"] = test.sibling

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichmentRemoval(WithRemoveObservables(false)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal(test.sibling, event["activity_name"])
			assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
			issues := issuesWithCode(result.Issues, "issue_enrichment_removal_enum_sibling_not_removed")
			assert.Len(issues, 1)
			assert.Equal(test.reason, issues[0].Details["reason"])
			assert.Equal("activity_name", issues[0].Details["attribute_path"])
		})
	}
}

func TestEnrichmentRemovalRetainsEnumSiblingWhenEnumValueMissing(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	delete(event, "activity_id")
	event["activity_name"] = "Do"

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.Equal("Do", event["activity_name"])
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRetained)
	issues := issuesWithCode(result.Issues, "issue_enrichment_removal_enum_sibling_not_removed")
	assert.Len(issues, 1)
	assert.Equal("enum_value_missing", issues[0].Details["reason"])
	assert.Equal("activity_name", issues[0].Details["attribute_path"])
}

func TestEnrichmentRemovalIssueUsesIndexedNestedPath(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	modeSibling := "mode"
	schema.compiled.Objects["ball"].Attributes["mode_id"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{
			Type:    "integer_t",
			Sibling: &modeSibling,
			Enum:    map[string]*enumDefinition{"1": {Caption: "Known"}},
		},
	}
	schema.compiled.Objects["ball"].Attributes["mode"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
	}
	event := validValidationEvent()
	event["balls"] = []any{
		jsonish.Map{"green": "first", "mode_id": json.Number("1"), "mode": "Known"},
		jsonish.Map{"green": "second", "mode_id": json.Number("1"), "mode": "source-specific"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithRemoveObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	issues := issuesWithCode(result.Issues, "issue_enrichment_removal_enum_sibling_not_removed")
	assert.Len(issues, 1)
	assert.Equal("balls[1].mode", issues[0].Details["attribute_path"])
	assert.Equal("balls[1].mode_id", issues[0].Details["enum_attribute_path"])
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
			schema.compiled.Classes[int64(1)].Attributes["mode_id"].Enum["99"] = &enumDefinition{Caption: "Other"}
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

func TestEnrichmentRemovalTreatsNullAttributesAsMissing(t *testing.T) {
	t.Run("enum sibling", func(t *testing.T) {
		assert := require.New(t)
		schema := makeValidationTestSchema(assert)
		event := validValidationEvent()
		event["mode_id"] = json.Number("1")
		event["mode"] = nil

		result, err := mustNewEventProcessorPipeline(
			assert,
			schema,
			NewEnrichmentRemoval(WithRemoveObservables(false)),
		).ProcessEvent(event)

		assert.NoError(err)
		assert.NotContains(event, "mode")
		assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRemoved)
		assert.Zero(result.EnrichmentRemoval.EnumSiblingsRetained)
	})

	t.Run("observables", func(t *testing.T) {
		assert := require.New(t)
		schema := makeValidationTestSchema(assert)
		event := validValidationEvent()
		event["observables"] = nil

		result, err := mustNewEventProcessorPipeline(
			assert,
			schema,
			NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
		).ProcessEvent(event)

		assert.NoError(err)
		assert.NotContains(event, "observables")
		assert.Zero(result.EnrichmentRemoval.ObservablesRemoved)
		assert.Zero(result.EnrichmentRemoval.ObservablesRetained)
		assert.Empty(result.Issues)
	})
}

func TestEnrichmentRemovalRemovesEmptyObservables(t *testing.T) {
	testCases := []struct {
		name  string
		force bool
	}{
		{name: "safe"},
		{name: "forced", force: true},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			event := validValidationEvent()
			event["observables"] = []any{}
			options := []EnrichmentRemovalOption{WithRemoveEnumSiblings(false)}
			if test.force {
				options = append(options, WithForceRemoveObservables())
			}

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichmentRemoval(options...),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.NotContains(event, "observables")
			assert.Zero(result.EnrichmentRemoval.ObservablesRemoved)
			assert.Zero(result.EnrichmentRemoval.ObservablesRetained)
			assert.Empty(result.Issues)
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

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(3, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	observables, ok := event["observables"].([]any)
	assert.True(ok)
	assert.Len(observables, 1)
	assert.Equal(issue.ObservableValueNotFound, result.Issues[0].Code)
}

func TestEnrichmentRemovalFiltersNamedObservableSlice(t *testing.T) {
	type observableList []jsonish.Map

	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = observableList{
		{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
		{"name": "ball.green", "type_id": json.Number("1000"), "value": "other"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	remaining, ok := event["observables"].(observableList)
	assert.True(ok)
	assert.Len(remaining, 1)
	assert.Equal("other", remaining[0]["value"])
}

func TestEnrichmentRemovalFiltersFixedObservableArray(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = [2]jsonish.Map{
		{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
		{"name": "ball.green", "type_id": json.Number("1000"), "value": "other"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	remaining, ok := event["observables"].([]jsonish.Map)
	assert.True(ok)
	assert.Len(remaining, 1)
	assert.Equal("other", remaining[0]["value"])
}

func TestEnrichmentRemovalMatchesObservableValuesAfterScalarStringConversion(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["port"] = json.Number("443")
	event["observables"] = []any{
		jsonish.Map{"name": "port", "type_id": 11, "value": "443"},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
}

func TestEnrichmentRemovalTreatsMissingObservablePathAsNull(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": 1000, "value": nil},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Empty(result.Issues)
}

func TestEnrichmentRemovalOmitsExplicitNullFromObservableIssueDetails(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": 1000, "value": nil},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(event, "observables")
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	issues := issuesWithCode(result.Issues, "issue_observable_value_not_found")
	assert.Len(issues, 1)
	assert.Equal("observables[0].value", issues[0].Details["attribute_path"])
	assert.NotContains(issues[0].Details, "value")
}

func TestSafeEnrichmentRemovalStopsWithoutResolvedClass(t *testing.T) {
	tests := []struct {
		name     string
		classUID any
		present  bool
	}{
		{name: "missing"},
		{name: "wrong type", classUID: "wrong", present: true},
		{name: "unknown", classUID: json.Number("999"), present: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			empty := []any{}
			event := jsonish.Map{"observables": empty}
			if test.present {
				event["class_uid"] = test.classUID
			}

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal(empty, event["observables"])
			assert.Zero(result.EnrichmentRemoval.ObservablesRemoved)
			assert.Zero(result.EnrichmentRemoval.ObservablesRetained)
		})
	}
}

func TestValidationTreatsMissingObservablePathAsNull(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": nil},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewValidation()).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(
		issueCodes(findingsAtLevel(result.Validation.Findings, validation.LevelError)),
		"validation_observable_path_not_found",
	)
	assert.NotContains(
		issueCodes(findingsAtLevel(result.Validation.Findings, validation.LevelError)),
		"validation_observable_value_not_found",
	)
}

func TestEnrichmentRemovalTreatsMissingArrayBranchesAsNull(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	event := validValidationEvent()
	event["balls"] = []any{
		jsonish.Map{"green": "present"},
		jsonish.Map{},
	}
	event["observables"] = []any{
		jsonish.Map{"name": "balls[].green", "type_id": 1000, "value": nil},
		jsonish.Map{"name": "balls[2].green", "type_id": 1000, "value": nil},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(2, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Empty(result.Issues)
}

func TestEnrichmentRemovalDoesNotTreatWrongTypeObservablePathAsNull(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = "wrong type"
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": 1000, "value": nil},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(event, "observables")
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	assert.Equal(issue.ObservablePathNotFound, result.Issues[0].Code)
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

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(len(forms), result.EnrichmentRemoval.ObservablesRemoved)
	assert.Empty(result.Issues)
}

func TestEnrichmentGeneratesSupportedObservablePathNotation(t *testing.T) {
	tests := []struct {
		name  string
		style pathstyle.Style
		want  []string
	}{
		{name: "simple", style: pathstyle.Simple, want: []string{"balls.green", "balls.green"}},
		{name: "brackets", style: pathstyle.ArrayBrackets, want: []string{"balls[].green", "balls[].green"}},
		{name: "wildcard", style: pathstyle.ArrayWildcard, want: []string{"balls[*].green", "balls[*].green"}},
		{name: "indexed", style: pathstyle.ArrayIndexed, want: []string{"balls[0].green", "balls[1].green"}},
		{name: "JSONPath", style: pathstyle.JSONPath, want: []string{"$.balls[0].green", "$.balls[1].green"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			addObservableArrayTestAttributes(schema)
			event := validValidationEvent()
			event["balls"] = []any{
				jsonish.Map{"green": "first"},
				jsonish.Map{"green": "second"},
			}

			result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(
				WithAddEnumSiblings(false),
				WithEnrichmentObservablePathNotation(test.style),
			)).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal(2, result.Enrichment.ObservablesAdded)
			observables, ok := event["observables"].([]jsonish.Map)
			assert.True(ok)
			names := make([]string, len(observables))
			for index, observable := range observables {
				name, ok := observable["name"].(string)
				assert.True(ok)
				names[index] = name
			}
			assert.Equal(test.want, names)

			validationResult, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewValidation(WithValidationObservablePathNotation(test.style)),
			).ProcessEvent(event)
			assert.NoError(err)
			assert.Empty(findingsAtLevel(validationResult.Validation.Findings, validation.LevelError))
			assert.Empty(issuesWithCode(
				findingsAtLevel(validationResult.Validation.Findings, validation.LevelWarning),
				"validation_observable_name_path_notation_unexpected",
			))
		})
	}
}

func TestGeneratedObservableSuffixPassesIndependentValidation(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "generated"}
	event["observables"] = []any{
		jsonish.Map{"name": "name", "type_id": json.Number("1000"), "value": "event name"},
	}

	enrichmentResult, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddEnumSiblings(false)),
	).ProcessEvent(event)
	assert.NoError(err)
	assert.Equal(1, enrichmentResult.Enrichment.ObservablesAdded)

	validationResult, err := mustNewEventProcessorPipeline(assert, schema, NewValidation()).ProcessEvent(event)
	assert.NoError(err)
	assert.Empty(findingsAtLevel(validationResult.Validation.Findings, validation.LevelError))
}

func TestCombinedProcessingChecksGeneratedObservableNotationOnlyWhenNeeded(t *testing.T) {
	tests := []struct {
		name         string
		pathStyle    pathstyle.Style
		preferred    pathstyle.Style
		wantWarnings int
	}{
		{name: "same style", pathStyle: pathstyle.ArrayIndexed, preferred: pathstyle.ArrayIndexed},
		{
			name:         "different array style",
			pathStyle:    pathstyle.Simple,
			preferred:    pathstyle.ArrayIndexed,
			wantWarnings: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			addObservableArrayTestAttributes(schema)
			event := validValidationEvent()
			event["balls"] = []any{
				jsonish.Map{"green": "first"},
				jsonish.Map{"green": "second"},
			}

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichment(
					WithAddEnumSiblings(false),
					WithEnrichmentObservablePathNotation(test.pathStyle),
				),
				NewValidation(WithValidationObservablePathNotation(test.preferred)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Len(
				issuesWithCode(
					findingsAtLevel(result.Validation.Findings, validation.LevelWarning),
					"validation_observable_name_path_notation_unexpected",
				),
				test.wantWarnings,
			)
		})
	}
}

func TestCombinedProcessingValidatesExistingObservablePrefix(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "generated"}
	event["observables"] = []any{
		jsonish.Map{"name": "name", "type_id": json.Number("1000"), "value": "wrong"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddEnumSiblings(false)),
		NewValidation(),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	issues := issuesWithCode(
		findingsAtLevel(result.Validation.Findings, validation.LevelError),
		"validation_observable_value_not_found",
	)
	assert.Len(issues, 1)
	assert.Equal("observables[0].value", issues[0].Details["attribute_path"])
}

func TestValidationOptionWarnsAboutObservablePathNotation(t *testing.T) {
	tests := []struct {
		name  string
		style pathstyle.Style
		path  string
	}{
		{name: "simple", style: pathstyle.Simple, path: "balls.green"},
		{name: "brackets", style: pathstyle.ArrayBrackets, path: "balls[].green"},
		{name: "wildcard", style: pathstyle.ArrayWildcard, path: "balls[*].green"},
		{name: "indexed", style: pathstyle.ArrayIndexed, path: "balls[1].green"},
		{name: "JSONPath", style: pathstyle.JSONPath, path: "$.balls[1].green"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			addObservableArrayTestAttributes(schema)
			event := validValidationEvent()
			event["balls"] = []any{
				jsonish.Map{"green": "first"},
				jsonish.Map{"green": "second"},
			}
			event["observables"] = []any{
				jsonish.Map{"name": test.path, "type_id": json.Number("1000"), "value": "second"},
			}

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewValidation(WithValidationObservablePathNotation(test.style)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Empty(issuesWithCode(
				findingsAtLevel(result.Validation.Findings, validation.LevelWarning),
				"validation_observable_name_path_notation_unexpected",
			))
		})
	}

	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	event := validValidationEvent()
	event["balls"] = []any{jsonish.Map{"green": "first"}}
	event["observables"] = []any{
		jsonish.Map{"name": "balls.green", "type_id": json.Number("1000"), "value": "first"},
	}
	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewValidation(WithValidationObservablePathNotation(pathstyle.ArrayIndexed)),
	).ProcessEvent(event)
	assert.NoError(err)
	warnings := issuesWithCode(
		findingsAtLevel(result.Validation.Findings, validation.LevelWarning),
		"validation_observable_name_path_notation_unexpected",
	)
	assert.Len(warnings, 1)
	assert.Equal("observables[0].name", warnings[0].Details["attribute_path"])
	assert.Equal(pathstyle.ArrayIndexed, warnings[0].Details["preferred_path_notation"])
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

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveEnumSiblings(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Equal(2, result.EnrichmentRemoval.ObservablesRemoved)
}

func TestEnrichmentRemovalForceRemovesMalformedObservableWithoutAnalysis(t *testing.T) {
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
	assert.Empty(result.Issues)
}

func TestEnrichmentRemovalReportsMalformedObservables(t *testing.T) {
	tests := []struct {
		name       string
		observable any
		prepare    func(jsonish.Map)
		wantCode   issue.IssueCode
	}{
		{
			name:       "attribute is not an array",
			observable: "wrong type",
			wantCode:   issue.ObservableArrayWrongType,
		},
		{
			name:       "element is not an object",
			observable: []any{"wrong type"},
			wantCode:   issue.ObservableElementWrongType,
		},
		{
			name:       "name is missing",
			observable: []any{jsonish.Map{"value": "go"}},
			wantCode:   issue.ObservableNameMissing,
		},
		{
			name:       "name has wrong type",
			observable: []any{jsonish.Map{"name": 7, "value": "go"}},
			wantCode:   issue.ObservableNameWrongType,
		},
		{
			name:       "name is not defined by schema",
			observable: []any{jsonish.Map{"name": "unknown.value", "value": "go"}},
			wantCode:   issue.ObservableNameInvalidReference,
		},
		{
			name:       "name has invalid syntax",
			observable: []any{jsonish.Map{"name": "ball[bad].green", "value": "go"}},
			wantCode:   issue.ObservableNameInvalidSyntax,
		},
		{
			name:       "path does not resolve in event",
			observable: []any{jsonish.Map{"name": "ball.green", "value": "go"}},
			wantCode:   issue.ObservablePathNotFound,
		},
		{
			name:       "value has wrong type",
			observable: []any{jsonish.Map{"name": "ball.green", "value": 7}},
			prepare: func(event jsonish.Map) {
				event["ball"] = jsonish.Map{"green": "go"}
			},
			wantCode: issue.ObservableValueWrongType,
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
			assert.Equal(issue.SourceEnrichmentRemoval, result.Issues[0].Source)
		})
	}
}

func TestForceObservableRemovalStopsWithoutResolvedClass(t *testing.T) {
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
	assert.Contains(event, "observables")
	assert.Zero(result.EnrichmentRemoval.ObservablesRemoved)
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
	assert.Empty(
		findingsAtLevel(result.Validation.Findings, validation.LevelError),
		"validation should inspect the final event after removal",
	)
	assert.Empty(result.Issues, "forced removal should not inspect observable entries")
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
	assert.Contains(
		issueCodes(findingsAtLevel(result.Validation.Findings, validation.LevelError)),
		"validation_observable_value_not_found",
	)
	assert.Contains(
		issueCodes(findingsAtLevel(result.Validation.Findings, validation.LevelError)),
		"validation_observable_path_not_object",
	)
}

func TestObservableIssueDoesNotRepeatEventValues(t *testing.T) {
	assert := require.New(t)
	const sensitiveValue = "sensitive-event-value-7f4c2a"
	event := validValidationEvent()
	event["observables"] = []any{jsonish.Map{
		"name":    sensitiveValue,
		"type_id": json.Number("1000"),
		"value":   sensitiveValue,
	}}

	result, err := mustNewEventProcessorPipeline(
		assert,
		makeValidationTestSchema(assert),
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotEmpty(result.Issues)
	encoded, err := json.Marshal(result.Issues)
	assert.NoError(err)
	assert.NotContains(string(encoded), sensitiveValue)
}

func TestValidationRejectsIncorrectObservableNames(t *testing.T) {
	tests := []struct {
		name       string
		observable jsonish.Map
		wantCode   string
		wantPath   string
	}{
		{
			name:       "missing",
			observable: jsonish.Map{"type_id": json.Number("1000"), "value": "go"},
			wantCode:   "validation_attribute_required_missing",
			wantPath:   "observables[0].name",
		},
		{
			name:       "null",
			observable: jsonish.Map{"name": nil, "type_id": json.Number("1000"), "value": "go"},
			wantCode:   "validation_attribute_required_missing",
			wantPath:   "observables[0].name",
		},
		{
			name:       "wrong type",
			observable: jsonish.Map{"name": 7, "type_id": json.Number("1000"), "value": "go"},
			wantCode:   "validation_attribute_wrong_type",
			wantPath:   "observables[0].name",
		},
		{
			name:       "invalid syntax",
			observable: jsonish.Map{"name": "ball[bad].green", "type_id": json.Number("1000"), "value": "go"},
			wantCode:   "validation_observable_name_invalid_syntax",
			wantPath:   "observables[0].name",
		},
		{
			name:       "observables self reference",
			observable: jsonish.Map{"name": "observables.name", "type_id": json.Number("1000"), "value": "go"},
			wantCode:   "validation_observable_name_invalid_reference",
			wantPath:   "observables[0].name",
		},
		{
			name:       "undefined reference",
			observable: jsonish.Map{"name": "unknown.value", "type_id": json.Number("1000"), "value": "go"},
			wantCode:   "validation_observable_name_invalid_reference",
			wantPath:   "observables[0].name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			event := validValidationEvent()
			event["observables"] = []any{test.observable}

			result, err := mustNewEventProcessorPipeline(assert, schema, NewValidation()).ProcessEvent(event)

			assert.NoError(err)
			issues := issuesWithCode(findingsAtLevel(result.Validation.Findings, validation.LevelError), test.wantCode)
			assert.Len(issues, 1)
			assert.Equal(test.wantPath, issues[0].Details["attribute_path"])
		})
	}
}

func TestEnrichmentRemovalAndValidationAnalyzeTheirOwnEventState(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "missing"},
	}

	pipeline := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
		NewValidation(),
	)
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	removalIssues := issuesWithCode(result.Issues, "issue_observable_value_not_found")
	assert.Len(removalIssues, 1)
	assert.Equal("observables[1].value", removalIssues[0].Details["attribute_path"])
	validationFindings := issuesWithCode(
		findingsAtLevel(result.Validation.Findings, validation.LevelError),
		"validation_observable_value_not_found",
	)
	assert.Len(validationFindings, 1)
	assert.Equal("observables[0].value", validationFindings[0].Details["attribute_path"])
}

func TestEnrichmentRemovalAnalyzesObservablesAfterEnumSiblingsAreAdded(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["observables"] = []any{
		jsonish.Map{"name": "activity_name", "value": "Do"},
	}

	pipeline := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddObservables(false)),
		NewEnrichmentRemoval(WithRemoveEnumSiblings(false)),
	)
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(
		"Do", event["activity_name"], "enum-sibling add must run before observable removal analyzes this value",
	)
	assert.Equal(
		1,
		result.EnrichmentRemoval.ObservablesRemoved,
		"activity_name now matches the added sibling, so the observable is safely removable",
	)
	assert.Empty(result.Issues, "observable removal must not report a false positive path-not-found issue")
}

func TestEnrichmentRemovalForceRemovesEnumSiblingsBeforeObservablesAreAnalyzed(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["activity_name"] = "Do"
	event["observables"] = []any{
		jsonish.Map{"name": "activity_name", "value": "Do"},
	}

	pipeline := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichmentRemoval(WithForceRemoveEnumSiblings(), WithRemoveObservables(true)),
	)
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "activity_name", "force-removed enum siblings are deleted regardless of order")
	assert.Equal(
		0,
		result.EnrichmentRemoval.ObservablesRemoved,
		"activity_name is already gone by the time observable removal analyzes this value, so it cannot be"+
			" verified and must be retained; enum-sibling work always runs before observable work, with no exception",
	)
	assert.Equal(1, result.EnrichmentRemoval.ObservablesRetained)
	assert.Equal(1, result.EnrichmentRemoval.EnumSiblingsRemoved)
	issues := issuesWithCode(result.Issues, "issue_observable_path_not_found")
	assert.Len(issues, 1, "the deleted sibling is a genuine, not false-positive, path-not-found issue")
	assert.Equal("observables[0].name", issues[0].Details["attribute_path"])
}

func addObservableArrayTestAttributes(schema *PipelineFactory) {
	trueValue := true
	ballType := "ball"
	schema.compiled.Classes[int64(1)].Attributes["balls"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{
			Type:       "object_t",
			ObjectType: &ballType,
			IsArray:    &trueValue,
		},
	}
	schema.compiled.Objects["ball"].Attributes["children"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{
			Type:       "object_t",
			ObjectType: &ballType,
			IsArray:    &trueValue,
		},
	}
}
