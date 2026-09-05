package processing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestEnrichmentReportsEnumSiblingItCannotAdd(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["activity_id"] = json.Number("1234")

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddObservables(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "activity_name")
	issues := issuesWithCode(result.Issues, "issue_enrichment_enum_sibling_not_added")
	assert.Len(issues, 1)
	assert.Equal(issue.SourceEnrichment, issues[0].Source)
}

func TestEnrichmentAddsIntegralEnumArraySibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "Do"
	event["status_ids"] = []any{json.Number("1"), json.Number("2")}
	delete(event, "statuses")

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichment(WithAddObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]string{"Open", "Closed"}, event["statuses"])
	assert.Equal(1, result.Enrichment.EnumSiblingsAdded)
	assert.Empty(result.Issues)
}

func TestEnrichmentDoesNotAddStringEnumSibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	siblingName := "state_name"
	schema.Classes[int64(1)].Attributes["state"].Sibling = &siblingName
	schema.Classes[int64(1)].Attributes[siblingName] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
	}
	event := validValidationEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "Do"
	event["state"] = "open"

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichment(WithAddObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, siblingName)
	assert.Zero(result.Enrichment.EnumSiblingsAdded)
	assert.Empty(result.Issues)
}

func TestEnrichmentDoesNotAddEmptyStringEnumSibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	activityID := schema.Classes[int64(1)].Attributes["activity_id"]
	activityID.Type = "string_t"
	activityID.Enum = map[string]*enumDefinition{"": {Caption: "Empty"}}
	event := validValidationEvent()
	event["class_name"] = "Alpha"
	event["activity_id"] = ""
	delete(event, "activity_name")

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichment(WithAddObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "activity_name")
	assert.Zero(result.Enrichment.EnumSiblingsAdded)
	assert.Empty(result.Issues)
}

func TestIntegralEnumNormalizationAcceptsIntegralNumericRepresentations(t *testing.T) {
	for _, test := range []struct {
		name   string
		number any
	}{
		{name: "JSON decimal", number: json.Number("1.0")},
		{name: "JSON exponent", number: json.Number("1e0")},
		{name: "float64", number: float64(1)},
		{name: "float32", number: float32(1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			event := validValidationEvent()
			event["activity_id"] = test.number
			delete(event, "activity_name")
			event["status_ids"] = []any{test.number}
			event["statuses"] = []any{"Open"}

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichment(WithAddObservables(false)),
				NewValidation(),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal("Do", event["activity_name"])
			assert.NotContains(
				issueCodes(result.Validation.Findings),
				"validation_attribute_enum_value_unknown",
			)
			assert.NotContains(
				issueCodes(result.Validation.Findings),
				"validation_attribute_enum_array_value_unknown",
			)
		})
	}
}

func TestEnrichmentDoesNotPartiallyAddIntegralEnumArraySibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "Do"
	event["status_ids"] = []any{json.Number("1"), json.Number("1234")}
	delete(event, "statuses")

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichment(WithAddObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "statuses")
	assert.Zero(result.Enrichment.EnumSiblingsAdded)
	assert.Len(issuesWithCode(result.Issues, "issue_enrichment_enum_sibling_not_added"), 1)
}

func TestEnrichmentIssueDoesNotRepeatEventValues(t *testing.T) {
	assert := require.New(t)
	const sensitiveValue = "sensitive-event-value-7f4c2a"
	event := validValidationEvent()
	event["activity_id"] = sensitiveValue

	result, err := mustNewEventProcessorPipeline(
		assert,
		makeValidationTestSchema(assert),
		NewEnrichment(WithAddObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotEmpty(result.Issues)
	encoded, err := json.Marshal(result.Issues)
	assert.NoError(err)
	assert.NotContains(string(encoded), sensitiveValue)
}

func TestEnrichmentReportsOtherCaptionAddedForEnumID99(t *testing.T) {
	tests := []struct {
		name          string
		withNullValue bool
	}{
		{name: "missing sibling"},
		{name: "null sibling", withNullValue: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			event := validValidationEvent()
			event["activity_id"] = json.Number("99")
			event["type_uid"] = json.Number("199")
			if test.withNullValue {
				event["activity_name"] = nil
			}

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichment(WithAddObservables(false)),
				NewValidation(),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal("Other", event["activity_name"])
			issues := issuesWithCode(result.Issues, "issue_enrichment_enum_sibling_other_added")
			assert.Len(issues, 1)
			assert.Equal(issue.SourceEnrichment, issues[0].Source)
			assert.Equal("activity_name", issues[0].Details["attribute_path"])
			assert.NotContains(issues[0].Details, "value")
			assert.NotContains(issues[0].Details, "enum_value")
			assert.Contains(
				issueCodes(findingsAtLevel(result.Validation.Findings, validation.LevelWarning)),
				"validation_attribute_enum_sibling_suspicious_other",
			)
		})
	}
}

func TestEnrichmentAddsParallelIntegralEnumArraySiblingContainingOther(t *testing.T) {
	for _, test := range []struct {
		name        string
		nullSibling bool
	}{
		{name: "missing sibling"},
		{name: "null sibling", nullSibling: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			schema.Classes[int64(1)].Attributes["status_ids"].Enum["99"] =
				&enumDefinition{Caption: "Other"}
			event := validValidationEvent()
			event["status_ids"] = []any{json.Number("1"), json.Number("99"), json.Number("2")}
			if test.nullSibling {
				event["statuses"] = nil
			}

			result, err := mustNewEventProcessorPipeline(
				assert, schema, NewEnrichment(WithAddObservables(false)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal([]string{"Open", "Other", "Closed"}, event["statuses"])
			issues := issuesWithCode(result.Issues, "issue_enrichment_enum_sibling_other_added")
			assert.Len(issues, 1)
			assert.Equal("statuses", issues[0].Details["attribute_path"])
		})
	}
}

func TestEnrichmentDoesNotAddStringEnumArraySibling(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	statusIDs := schema.Classes[int64(1)].Attributes["status_ids"]
	statusIDs.Type = "string_t"
	statusIDs.Enum = map[string]*enumDefinition{"99": {Caption: "Ninety-nine"}}
	event := validValidationEvent()
	event["status_ids"] = []any{"99"}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichment(WithAddObservables(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "statuses")
	assert.Empty(issuesWithCode(result.Issues, "issue_enrichment_enum_sibling_other_added"))
}

func TestEnrichmentReportsObservableObjectWithWrongType(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	event := jsonish.Map{
		"class_uid": json.Number("1"),
		"ball":      "not an object",
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false))).
		ProcessEvent(event)

	assert.NoError(err)
	issues := issuesWithCode(result.Issues, "issue_enrichment_observable_not_added_wrong_type")
	assert.Len(issues, 1)
	assert.Equal("ball", issues[0].Details["attribute_path"])
}

func TestEnrichmentAddsObjectObservableDefinedOnAttribute(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	observableTypeID := int64(2000)
	schema.Classes[int64(1)].Attributes["ball"].Observable = &observableTypeID
	event := jsonish.Map{
		"class_uid": json.Number("1"),
		"ball":      jsonish.Map{"green": "go"},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(2, result.Enrichment.ObservablesAdded)
	observables, ok := event["observables"].([]jsonish.Map)
	assert.True(ok)
	assert.Contains(observables, jsonish.Map{"name": "ball", "type_id": observableTypeID})
}

func TestEnrichmentFiltersEveryObservableDeclarationSourceByTypeID(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	selectedTypeID := int64(2000)
	excludedTypeID := int64(1000)
	schema.ObservableTypes[selectedTypeID] = "Selected"
	schema.Objects["ball"].Observable = &selectedTypeID
	schema.Dictionary.Attributes["dictionary_observable"] = &commonAttributeDefinition{
		Type:       "string_t",
		Observable: &selectedTypeID,
	}
	schema.Dictionary.Attributes["direct_observable"] = &commonAttributeDefinition{Type: "string_t"}
	schema.Dictionary.Attributes["typed_observable"] = &commonAttributeDefinition{Type: "observable_string_t"}
	schema.Dictionary.Types.Attributes["observable_string_t"] = &typeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t", Observable: &excludedTypeID},
	}
	class := schema.Classes[int64(1)]
	class.Attributes["dictionary_observable"] = &itemAttributeDefinition{
		CommonAttributeDefinition: *schema.Dictionary.Attributes["dictionary_observable"],
	}
	class.Attributes["direct_observable"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t", Observable: &excludedTypeID},
	}
	class.Attributes["typed_observable"] = &itemAttributeDefinition{
		CommonAttributeDefinition: *schema.Dictionary.Attributes["typed_observable"],
	}
	event := jsonish.Map{
		"class_uid":             json.Number("1"),
		"ball":                  jsonish.Map{"green": "go"},
		"dictionary_observable": "selected",
		"direct_observable":     "excluded",
		"typed_observable":      "excluded",
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddEnumSiblings(false), WithAddObservables(true, selectedTypeID)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(2, result.Enrichment.ObservablesAdded)
	assert.Empty(result.Issues)
	assert.ElementsMatch([]jsonish.Map{
		{"name": "ball", "type_id": selectedTypeID},
		{"name": "dictionary_observable", "type_id": selectedTypeID, "value": "selected"},
	}, event["observables"])
}

func TestEnrichmentDoesNotReportMalformedExcludedObservableSources(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	selectedTypeID := int64(2000)
	schema.ObservableTypes[selectedTypeID] = "Selected"
	event := jsonish.Map{
		"class_uid": json.Number("1"),
		"ball":      "not an object",
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddEnumSiblings(false), WithAddObservables(true, selectedTypeID)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(result.Issues)
	assert.Zero(result.Enrichment.ObservablesAdded)
	assert.NotContains(event, "observables")
}

func TestEnrichmentReportsObservableArrayWithWrongType(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	observableTypeID := int64(1000)
	schema.Classes[int64(1)].Attributes["statuses"].Observable = &observableTypeID
	event := validValidationEvent()
	event["statuses"] = "not an array"

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false))).
		ProcessEvent(event)

	assert.NoError(err)
	issues := issuesWithCode(result.Issues, "issue_enrichment_observable_not_added_wrong_type")
	assert.Len(issues, 1)
	assert.Equal("statuses", issues[0].Details["attribute_path"])
}

func TestEnrichmentAddsObservableForEmptyString(t *testing.T) {
	for _, attributeType := range []string{"string_t", "integer_t"} {
		t.Run(attributeType, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			observableTypeID := int64(1000)
			attribute := schema.Classes[int64(1)].Attributes["name"]
			attribute.Type = attributeType
			attribute.Observable = &observableTypeID
			event := validValidationEvent()
			event["name"] = ""

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichment(WithAddEnumSiblings(false)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal(1, result.Enrichment.ObservablesAdded)
			assert.Empty(result.Issues)
			assert.Equal(
				[]jsonish.Map{{"name": "name", "type_id": observableTypeID, "value": ""}}, event["observables"],
			)
		})
	}
}

func TestEnrichmentReportsStructuredScalarObservableValue(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "object", value: jsonish.Map{"key": "value"}},
		{name: "array", value: []any{"value"}},
		{name: "typed array", value: []string{"value"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			observableTypeID := int64(1000)
			schema.Classes[int64(1)].Attributes["name"].Observable = &observableTypeID
			event := validValidationEvent()
			event["name"] = test.value

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichment(WithAddEnumSiblings(false)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.NotContains(event, "observables")
			assert.Zero(result.Enrichment.ObservablesAdded)
			issues := issuesWithCode(result.Issues, "issue_enrichment_observable_not_added_wrong_type")
			assert.Len(issues, 1)
			assert.Equal("name", issues[0].Details["attribute_path"])
		})
	}
}

func TestEnrichmentSkipsJSONTypeObservableDeclarations(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "structured value", value: jsonish.Map{"key": "value"}},
		{name: "JSON encoded string", value: `{"key":"value"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			observableTypeID := int64(1000)
			attribute := schema.Classes[int64(1)].Attributes["name"]
			attribute.Type = "json_t"
			attribute.Observable = &observableTypeID
			event := validValidationEvent()
			event["name"] = test.value

			result, err := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichment(WithAddEnumSiblings(false)),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.NotContains(event, "observables")
			assert.Zero(result.Enrichment.ObservablesAdded)
			issues := issuesWithCode(result.Issues, "issue_enrichment_observable_not_added_json_type")
			assert.Len(issues, 1)
			assert.Equal("name", issues[0].Details["attribute_path"])
			assert.Equal("json_t", issues[0].Details["type"])
			assert.Contains(issues[0].Message, "https://github.com/ocsf/ocsf-toolkit/issues")
		})
	}
}

func TestEnrichmentAppendsGeneratedObservablesToExistingObservables(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	existing := []any{jsonish.Map{"name": "red", "type_id": json.Number("1000"), "value": "existing"}}
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"ball":        jsonish.Map{"green": "go"},
		"observables": existing,
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false))).
		ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]any{
		existing[0],
		jsonish.Map{"name": "ball.green", "type_id": int64(1000), "value": "go"},
	}, event["observables"])
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	assert.Empty(result.Issues)
}

// Invariant test: generated deduplication retains a generated candidate that duplicates an existing observable.
func TestInvariantGeneratedDeduplicationDoesNotCompareExistingObservables(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	existing := []any{jsonish.Map{
		"name":    "ball.green",
		"type_id": json.Number("1000"),
		"type":    "Existing caption is ignored for identity",
		"value":   "go",
		"extra":   true,
	}}
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"ball":        jsonish.Map{"green": "go"},
		"observables": existing,
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(
			WithAddEnumSiblings(false),
			WithObservableDeduplication(enrichment.ObservableDeduplicationGenerated),
		),
		duplicateObservableWarningConfig(),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]any{
		existing[0],
		jsonish.Map{"name": "ball.green", "type_id": int64(1000), "value": "go"},
	}, event["observables"])
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	issues := issuesWithCode(result.Issues, "issue_observable_duplicate")
	assert.Len(issues, 1)
	assert.Equal("ball.green", issues[0].Details["attribute_path"])
	assert.Equal("green", issues[0].Details["attribute"])
	assert.Equal("generated", issues[0].Details["observable_origin"])
	assert.Equal(0, issues[0].Details["observable_index"])
	assert.Equal("existing", issues[0].Details["duplicate_of_origin"])
	assert.Equal(0, issues[0].Details["duplicate_of_index"])
	assert.Equal(
		`Generated observable for path "ball.green" duplicates existing observable 0.`,
		issues[0].Message,
	)
}

// Invariant test: duplicate issues independently detect existing-existing, generated-existing, and
// generated-generated identities without enabling deduplication.
func TestInvariantObservableDuplicateIssueCoversEveryOriginPair(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	existing := jsonish.Map{"name": "ball.green", "type_id": int64(1000), "value": "go"}
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{existing, jsonish.Map{
		"name": "ball.green", "type_id": json.Number("1000"), "value": "go",
	}}
	event["balls"] = []any{
		jsonish.Map{"green": "same"},
		jsonish.Map{"green": "same"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddEnumSiblings(false)),
		duplicateObservableWarningConfig(),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(3, result.Enrichment.ObservablesAdded)
	duplicates := issuesWithCode(result.Issues, "issue_observable_duplicate")
	assert.Len(duplicates, 3)
	assert.Equal("existing", duplicates[0].Details["observable_origin"])
	assert.Equal("existing", duplicates[0].Details["duplicate_of_origin"])
	assert.Equal("generated", duplicates[1].Details["observable_origin"])
	assert.Equal("existing", duplicates[1].Details["duplicate_of_origin"])
	assert.Equal("generated", duplicates[2].Details["observable_origin"])
	assert.Equal("generated", duplicates[2].Details["duplicate_of_origin"])
}

func TestEnrichmentDoesNotNormalizeObservableNamesWhenFindingDuplicates(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	existing := []any{jsonish.Map{"name": "ball[].green", "type_id": int64(1000), "value": "go"}}
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"ball":        jsonish.Map{"green": "go"},
		"observables": existing,
	}

	result, err := mustNewEventProcessorPipeline(assert, schema,
		NewEnrichment(WithAddEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]any{
		existing[0],
		jsonish.Map{"name": "ball.green", "type_id": int64(1000), "value": "go"},
	}, event["observables"])
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	assert.Empty(result.Issues)
}

// Invariant test: ignored observable deduplication appends every generated candidate without duplicate diagnostics.
func TestInvariantIgnoredObservableDeduplicationKeepsGeneratedDuplicates(t *testing.T) {
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
		NewEnrichment(WithAddEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]jsonish.Map{
		{"name": "balls.green", "type_id": int64(1000), "value": "same"},
		{"name": "balls.green", "type_id": int64(1000), "value": "same"},
	}, event["observables"])
	assert.Equal(2, result.Enrichment.ObservablesAdded)
	assert.Empty(result.Issues)
}

// Invariant test: generated deduplication remains active while duplicate issue reporting stays ignored.
func TestInvariantGeneratedObservableDeduplicationIsIndependentOfDuplicateDiagnostics(t *testing.T) {
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
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]jsonish.Map{{"name": "balls.green", "type_id": int64(1000), "value": "same"}},
		event["observables"])
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	assert.Empty(result.Issues)
}

// Invariant test: generated duplicate identity uses the exact configured name, integral type ID, and scalar value;
// only path styles that omit concrete array indexes can collapse distinct array locations.
func TestInvariantGeneratedObservableDeduplicationAcrossPathStyles(t *testing.T) {
	tests := []struct {
		name             string
		style            pathstyle.Style
		observableNames  []string
		duplicatePaths   []string
		observablesAdded int
	}{
		{
			name: "simple", style: pathstyle.Simple,
			observableNames: []string{"balls.green"}, duplicatePaths: []string{"balls[1].green"}, observablesAdded: 1,
		},
		{
			name: "brackets", style: pathstyle.ArrayBrackets,
			observableNames: []string{"balls[].green"}, duplicatePaths: []string{"balls[1].green"}, observablesAdded: 1,
		},
		{
			name: "wildcard", style: pathstyle.ArrayWildcard,
			observableNames:  []string{"balls[*].green"},
			duplicatePaths:   []string{"balls[1].green"},
			observablesAdded: 1,
		},
		{
			name: "indexed", style: pathstyle.ArrayIndexed,
			observableNames: []string{"balls[0].green", "balls[1].green"}, duplicatePaths: []string{},
			observablesAdded: 2,
		},
		{
			name: "JSONPath", style: pathstyle.JSONPath,
			observableNames: []string{"$.balls[0].green", "$.balls[1].green"}, duplicatePaths: []string{},
			observablesAdded: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
					WithEnrichmentObservablePathNotation(test.style),
					WithObservableDeduplication(enrichment.ObservableDeduplicationGenerated),
				),
				duplicateObservableWarningConfig(),
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal(test.observablesAdded, result.Enrichment.ObservablesAdded)
			observables, ok := event["observables"].([]jsonish.Map)
			assert.True(ok)
			observableNames := make([]string, len(observables))
			for index, observable := range observables {
				observableNames[index], ok = observable["name"].(string)
				assert.True(ok)
			}
			assert.Equal(test.observableNames, observableNames)
			duplicates := issuesWithCode(result.Issues, "issue_observable_duplicate")
			duplicatePaths := make([]string, len(duplicates))
			for index, duplicate := range duplicates {
				duplicatePaths[index], ok = duplicate.Details["attribute_path"].(string)
				assert.True(ok)
				assert.Equal("generated", duplicate.Details["duplicate_of_origin"])
			}
			assert.Equal(test.duplicatePaths, duplicatePaths)
		})
	}
}

// Engineering invariant test: duplicate detection must preserve traversal order even when equal identities are not
// adjacent, and duplicate diagnostics must retain their concrete indexed source paths.
func TestEngineeringInvariantGeneratedObservableDeduplicationDoesNotAssumeAdjacency(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	objectObservableTypeID := int64(2000)
	schema.ObservableTypes[objectObservableTypeID] = "Object"
	schema.Objects["ball"].Observable = &objectObservableTypeID
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
		duplicateObservableWarningConfig(),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]jsonish.Map{
		{"name": "balls", "type_id": objectObservableTypeID},
		{"name": "balls.green", "type_id": int64(1000), "value": "same"},
	}, event["observables"])
	assert.Equal(2, result.Enrichment.ObservablesAdded)
	duplicates := issuesWithCode(result.Issues, "issue_observable_duplicate")
	assert.Len(duplicates, 2)
	assert.Equal("balls[1]", duplicates[0].Details["attribute_path"])
	assert.Equal("balls[1].green", duplicates[1].Details["attribute_path"])
}

// Invariant test: an original observable takes source precedence over every generated occurrence of the same
// identity, regardless of generated repetition.
func TestInvariantExistingObservableTakesGeneratedDuplicatePrecedence(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	existing := jsonish.Map{"name": "balls.green", "type_id": json.Number("1000"), "value": "same"}
	event := validValidationEvent()
	event["observables"] = []any{existing}
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
		duplicateObservableWarningConfig(),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]any{
		existing,
		jsonish.Map{"name": "balls.green", "type_id": int64(1000), "value": "same"},
	}, event["observables"])
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	duplicates := issuesWithCode(result.Issues, "issue_observable_duplicate")
	assert.Len(duplicates, 2)
	assert.Equal("balls[0].green", duplicates[0].Details["attribute_path"])
	assert.Equal("balls[1].green", duplicates[1].Details["attribute_path"])
	assert.Equal("existing", duplicates[0].Details["duplicate_of_origin"])
	assert.Equal("existing", duplicates[1].Details["duplicate_of_origin"])
}

// Engineering invariant test: a malformed destination prevents all generated observable insertion and therefore
// reports only its raw candidate count, without generated-to-generated duplicate diagnostics.
func TestEngineeringInvariantMalformedObservableDestinationDefersAllGeneratedDeduplication(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	event := validValidationEvent()
	event["observables"] = "not an array"
	event["balls"] = []any{
		jsonish.Map{"green": "same"},
		jsonish.Map{"green": "same"},
	}

	result, err := mustNewEventProcessorPipeline(
		assert, schema, NewEnrichment(WithAddEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal("not an array", event["observables"])
	assert.Zero(result.Enrichment.ObservablesAdded)
	wrongType := issuesWithCode(result.Issues, "issue_enrichment_observables_not_added_wrong_type")
	assert.Len(wrongType, 1)
	assert.Equal(2, wrongType[0].Details["generated_observables"])
	assert.Empty(issuesWithCode(result.Issues, "issue_observable_duplicate"))
}

// Invariant test: nil and empty observable destinations have no existing identities, while successful enrichment
// replaces them with the same deduplicated generated array.
func TestInvariantGeneratedObservableDeduplicationTreatsNilAndEmptyDestinationsAsNoExisting(t *testing.T) {
	for _, test := range []struct {
		name        string
		destination any
	}{
		{name: "nil", destination: nil},
		{name: "empty", destination: []any{}},
		{name: "typed empty", destination: []jsonish.Map{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			addObservableArrayTestAttributes(schema)
			event := validValidationEvent()
			event["observables"] = test.destination
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
			).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal([]jsonish.Map{
				{"name": "balls.green", "type_id": int64(1000), "value": "same"},
			}, event["observables"])
			assert.Equal(1, result.Enrichment.ObservablesAdded)
			assert.Empty(result.Issues)
		})
	}
}

// Invariant test: issue policy controls only duplicate diagnostics; it never changes which generated observables are
// suppressed, and an error-level duplicate remains deferred until completed-event processing before observable append.
func TestInvariantGeneratedObservableDuplicateIssuePolicyDoesNotChangeDeduplication(t *testing.T) {
	for _, test := range []struct {
		name       string
		level      issue.Level
		issueCount int
		wantError  bool
	}{
		{name: "ignored", level: issue.LevelIgnored},
		{name: "warning", level: issue.LevelWarning, issueCount: 1},
		{name: "error", level: issue.LevelError, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			addObservableArrayTestAttributes(schema)
			event := validValidationEvent()
			event["balls"] = []any{
				jsonish.Map{"green": "same"},
				jsonish.Map{"green": "same"},
			}
			pipeline := mustNewEventProcessorPipeline(
				assert,
				schema,
				NewEnrichment(
					WithAddEnumSiblings(false),
					WithObservableDeduplication(enrichment.ObservableDeduplicationGenerated),
				),
				PipelineConfig{
					EnumSiblingsAction: enrichment.None,
					ObservablesAction:  enrichment.None,
					IssuePolicy: IssuePolicyConfig{
						LevelRules: []IssueLevelRule{{
							Code: issue.ObservableDuplicate, Level: test.level,
						}},
					},
				},
			)

			result, err := pipeline.ProcessEvent(event)

			if test.wantError {
				assert.Error(err)
				assert.NotContains(event, "observables")
				assert.Zero(result.Enrichment.ObservablesAdded)
				assert.Empty(result.Issues)
				return
			}
			assert.NoError(err)
			assert.Equal([]jsonish.Map{
				{"name": "balls.green", "type_id": int64(1000), "value": "same"},
			}, event["observables"])
			assert.Equal(1, result.Enrichment.ObservablesAdded)
			assert.Len(result.Issues, test.issueCount)
		})
	}
}

// Invariant test: generated-only deduplication does not inspect a nonempty existing observable collection.
func TestInvariantGeneratedDeduplicationDoesNotInspectExistingObservables(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	existing := jsonish.Map{"name": "existing", "type_id": int64(1000), "value": "value"}
	event := validValidationEvent()
	event["observables"] = []any{existing}
	event["balls"] = []any{
		jsonish.Map{"green": "same"},
		jsonish.Map{"green": "same"},
	}
	pipeline := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(
			WithAddEnumSiblings(false),
			WithObservableDeduplication(enrichment.ObservableDeduplicationGenerated),
		),
		PipelineConfig{
			EnumSiblingsAction: enrichment.None,
			ObservablesAction:  enrichment.None,
			IssuePolicy: IssuePolicyConfig{
				LevelRules: []IssueLevelRule{{
					Code: issue.ObservableDuplicate, Level: issue.LevelIgnored,
				}},
			},
		},
	)

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]any{
		existing,
		jsonish.Map{"name": "balls.green", "type_id": int64(1000), "value": "same"},
	}, event["observables"])
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	assert.Empty(result.Issues)
}

// Engineering invariant test: validation must receive zero as its exclusive upper bound when enrichment creates an
// observable array, including when multiple observables are generated.
func TestEngineeringInvariantGeneratedObservablesInNewArraySetValidationUpperBound(t *testing.T) {
	context := processContext{
		observables: []jsonish.Map{
			{"name": "first", "type_id": int64(1), "value": "one"},
			{"name": "second", "type_id": int64(1), "value": "two"},
		},
		generatedObservablesFirstIndex: -1,
	}
	event := jsonish.Map{}
	processor := enrichmentProcessor{pathNotation: pathstyle.Simple}

	err := processor.addGeneratedObservables(&context, event, nil)

	require.NoError(t, err)
	require.Equal(t, 0, context.generatedObservablesFirstIndex)
}

func TestTerminalObservableAttribute(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		path      string
		attribute string
	}{
		{path: "value", attribute: "value"},
		{path: "values[1]", attribute: "values"},
		{path: "object.value", attribute: "value"},
		{path: "objects[].value", attribute: "value"},
		{path: "objects[*].value", attribute: "value"},
		{path: "objects[1].value", attribute: "value"},
		{path: "$.objects[1].value", attribute: "value"},
	} {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.attribute, terminalObservableAttribute(test.path))
		})
	}
}

func TestEnrichmentTreatsNullValueAsOmittedForDuplicates(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	objectObservableTypeID := int64(2000)
	schema.Classes[int64(1)].Attributes["ball"].Observable = &objectObservableTypeID
	existing := jsonish.Map{"name": "ball", "type_id": objectObservableTypeID, "value": nil}
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"ball":        jsonish.Map{"green": "go"},
		"observables": []any{existing},
	}

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddEnumSiblings(false)),
		duplicateObservableWarningConfig(),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]any{
		existing,
		jsonish.Map{"name": "ball", "type_id": objectObservableTypeID},
		jsonish.Map{"name": "ball.green", "type_id": int64(1000), "value": "go"},
	}, event["observables"])
	assert.Equal(2, result.Enrichment.ObservablesAdded)
	issues := issuesWithCode(result.Issues, "issue_observable_duplicate")
	assert.Len(issues, 1)
	assert.Equal("existing", issues[0].Details["duplicate_of_origin"])
}

func duplicateObservableWarningConfig() PipelineConfig {
	return PipelineConfig{
		EnumSiblingsAction: enrichment.None,
		ObservablesAction:  enrichment.None,
		IssuePolicy: IssuePolicyConfig{LevelRules: []IssueLevelRule{{
			Code: issue.ObservableDuplicate, Level: issue.LevelWarning,
		}}},
	}
}

// Invariant test: enrichment does not normalize a nil observables attribute when it has nothing to add.
func TestInvariantEnrichmentPreservesNilObservablesWhenNothingIsGenerated(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["observables"] = nil

	result, err := mustNewEventProcessorPipeline(
		assert,
		schema,
		NewEnrichment(WithAddEnumSiblings(false)),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(event, "observables")
	assert.Nil(event["observables"])
	assert.Zero(result.Enrichment.ObservablesAdded)
}

func TestEnrichmentReportsWrongTypeExistingObservables(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"ball":        jsonish.Map{"green": "go"},
		"observables": "not an array",
	}

	result, err := mustNewEventProcessorPipeline(assert, schema,
		NewEnrichment(WithAddEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal("not an array", event["observables"])
	assert.Zero(result.Enrichment.ObservablesAdded)
	issues := issuesWithCode(result.Issues, "issue_enrichment_observables_not_added_wrong_type")
	assert.Len(issues, 1)
	assert.Equal(1, issues[0].Details["generated_observables"])
}

func TestEnrichmentAppendsToTypedObservableSlice(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	existing := []jsonish.Map{{"name": "red", "type_id": int64(1000), "value": "existing"}}
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"ball":        jsonish.Map{"green": "go"},
		"observables": existing,
	}

	result, err := mustNewEventProcessorPipeline(assert, schema,
		NewEnrichment(WithAddEnumSiblings(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal([]jsonish.Map{
		existing[0],
		{"name": "ball.green", "type_id": int64(1000), "value": "go"},
	}, event["observables"])
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	assert.Empty(result.Issues)
}

func TestEnrichmentRemovesEmptyObservablesWhenNothingIsGenerated(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"observables": []any{},
	}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment()).ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(event, "observables")
	assert.Zero(result.Enrichment.ObservablesAdded)
	assert.Empty(result.Issues)
}

func TestEnrichmentLeavesEmptyObservablesWhenObservableEnrichmentIsDisabled(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	empty := []any{}
	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"observables": empty,
	}

	result, err := mustNewEventProcessorPipeline(assert, schema,
		NewEnrichment(WithAddObservables(false))).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(empty, event["observables"])
	assert.Empty(result.Issues)
}

func TestEnrichmentStopsWithoutResolvedClass(t *testing.T) {
	tests := []struct {
		name     string
		classUID any
		present  bool
		code     string
	}{
		{name: "missing", code: "issue_class_uid_missing"},
		{name: "wrong type", classUID: "wrong", present: true, code: "issue_class_uid_wrong_type"},
		{name: "unknown", classUID: json.Number("999"), present: true, code: "issue_class_uid_unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeTestSchema(assert)
			empty := []any{}
			event := jsonish.Map{"observables": empty}
			if test.present {
				event["class_uid"] = test.classUID
			}

			result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment()).ProcessEvent(event)

			assert.NoError(err)
			assert.Equal(empty, event["observables"])
			assert.Zero(result.Enrichment.EnumSiblingsAdded)
			assert.Zero(result.Enrichment.ObservablesAdded)
			assert.Equal([]string{test.code}, issueCodes(result.Issues))
		})
	}
}

func TestValidationProcessesScalarEnumAndSiblingTogether(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	schema.Classes[1].Attributes["mode"].Requirement = "recommended"

	t.Run("enrichment satisfies earlier sibling requirement", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["mode_id"] = json.Number("1")
		pipeline := mustNewEventProcessorPipeline(
			assert,
			schema,
			NewEnrichment(WithAddObservables(false)),
			NewValidation(WithValidationLevel(validation.AttributeRecommendedMissing, validation.LevelWarning)),
		)

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		assert.Equal("Known", event["mode"])
		assert.NotContains(
			issueCodes(findingsAtLevel(result.Validation.Findings, validation.LevelWarning)),
			"validation_attribute_recommended_missing",
		)
	})

	t.Run("sibling requirement applies when enum exists", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["mode_id"] = json.Number("1")
		pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation(
			WithValidationLevel(validation.AttributeRecommendedMissing, validation.LevelWarning),
		))

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		issues := issuesWithCode(
			findingsAtLevel(result.Validation.Findings, validation.LevelWarning),
			"validation_attribute_recommended_missing",
		)
		assert.Len(issues, 1)
		assert.Equal("mode", issues[0].Details["attribute_path"])
	})

	t.Run("sibling requirement does not apply without enum", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation(
			WithValidationLevel(validation.AttributeRecommendedMissing, validation.LevelWarning),
		))

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		for _, issue := range issuesWithCode(
			findingsAtLevel(result.Validation.Findings, validation.LevelWarning),
			"validation_attribute_recommended_missing",
		) {
			assert.NotEqual("mode", issue.Details["attribute_path"])
		}
	})

	t.Run("sibling cannot exist without enum", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["mode"] = "Known"
		pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		issues := issuesWithCode(
			findingsAtLevel(result.Validation.Findings, validation.LevelError),
			"validation_attribute_enum_sibling_without_enum",
		)
		assert.Len(issues, 1)
		assert.Equal("mode", issues[0].Details["attribute_path"])
		assert.Equal("mode_id", issues[0].Details["enum_attribute"])
	})

	t.Run("sibling structure is validated with enum", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["mode_id"] = json.Number("1")
		event["mode"] = json.Number("1")
		pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		issues := issuesWithCode(
			findingsAtLevel(result.Validation.Findings, validation.LevelError),
			"validation_attribute_wrong_type",
		)
		assert.Len(issues, 1)
		assert.Equal("mode", issues[0].Details["attribute_path"])
	})
}

func TestValidationProcessesStringEnumAsOrdinaryAttribute(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["state"] = "other"

	result, err := mustNewEventProcessorPipeline(assert, schema, NewValidation()).ProcessEvent(event)

	assert.NoError(err)
	issues := issuesWithCode(
		findingsAtLevel(result.Validation.Findings, validation.LevelError),
		"validation_attribute_enum_value_unknown",
	)
	assert.Len(issues, 1)
	assert.Equal("state", issues[0].Details["attribute_path"])
}

func TestEnrichmentAddsObservableFromEnumSibling(t *testing.T) {
	tests := []struct {
		name             string
		event            jsonish.Map
		wantEnumSiblings int
	}{
		{
			name:             "freshly added sibling value",
			event:            jsonish.Map{"class_uid": json.Number("1"), "mode_id": json.Number("1")},
			wantEnumSiblings: 2,
		},
		{
			name:             "pre-existing sibling value",
			event:            jsonish.Map{"class_uid": json.Number("1"), "mode": "Known"},
			wantEnumSiblings: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			observableTypeID := int64(2000)
			schema.Classes[int64(1)].Attributes["mode"].Observable = &observableTypeID

			result, err := mustNewEventProcessorPipeline(assert, schema, NewEnrichment()).ProcessEvent(test.event)

			assert.NoError(err)
			assert.Equal("Known", test.event["mode"])
			assert.Equal(test.wantEnumSiblings, result.Enrichment.EnumSiblingsAdded)
			assert.Equal(1, result.Enrichment.ObservablesAdded)
			assert.Equal(
				[]jsonish.Map{{"name": "mode", "type_id": observableTypeID, "value": "Known"}},
				test.event["observables"],
			)
		})
	}
}
