package eventpipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestEngineeringInvariantPipelineAndProcessingResultStateRemainPrivate(t *testing.T) {
	// Engineering invariant test: Pipeline and ProcessingResult remain opaque concrete values while their private
	// representations and public method sets may grow without changing existing callers.
	for _, publicType := range []reflect.Type{reflect.TypeFor[Pipeline](), reflect.TypeFor[ProcessingResult]()} {
		require.Equal(t, reflect.Struct, publicType.Kind())
		for fieldIndex := range publicType.NumField() {
			require.NotEmptyf(
				t,
				publicType.Field(fieldIndex).PkgPath,
				"%s field %s must remain unexported",
				publicType.Name(),
				publicType.Field(fieldIndex).Name,
			)
		}
	}
}

func TestRemovedSuppressionCountsRemainAbsent(t *testing.T) {
	processingResultType := reflect.TypeFor[ProcessingResult]()
	_, hasSuppressedIssueCount := processingResultType.MethodByName("SuppressedIssueCount")
	require.False(t, hasSuppressedIssueCount)

	validationResultType := reflect.TypeFor[eventresult.ValidationResult]()
	_, hasIgnoredErrorCount := validationResultType.FieldByName("IgnoredErrorCount")
	require.False(t, hasIgnoredErrorCount)
	_, hasIgnoredWarningCount := validationResultType.FieldByName("IgnoredWarningCount")
	require.False(t, hasIgnoredWarningCount)
}

func TestProcessingResultDoesNotImplementJSONUnmarshaler(t *testing.T) {
	_, unmarshalsJSON := any((*ProcessingResult)(nil)).(json.Unmarshaler)
	require.False(t, unmarshalsJSON, "ProcessingResult must not expose unused JSON unmarshalling")
}

func TestProcessingResultJSONMarshalPreservesStableShape(t *testing.T) {
	original := ProcessingResult{state: processingResultState{
		validation: eventresult.ValidationResult{
			Findings: []eventresult.ValidationFinding{{
				Level:   validation.LevelError,
				Code:    validation.AttributeRequiredMissing,
				Message: "required attribute is missing",
			}},
		},
		enrichment: eventresult.EnrichmentResult{EnumSiblingsAdded: 2, ObservablesAdded: 3},
		enrichmentRemoval: eventresult.EnrichmentRemovalResult{
			EnumSiblingsRemoved:  4,
			EnumSiblingsRetained: 5,
			ObservablesRemoved:   6,
			ObservablesRetained:  7,
		},
		issues: []eventresult.ProcessingIssue{{
			Source:  issue.SourceProcessing,
			Code:    issue.EventTraversalLimited,
			Message: "processing was limited",
		}},
	}}

	encoded, err := json.Marshal(original)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"validation":{"findings":[
			{"level":"error","code":"validation_attribute_required_missing","message":"required attribute is missing"}
		]},
		"enrichment":{"enum_siblings_added":2,"observables_added":3},
		"enrichment_removal":{"enum_siblings_removed":4,"enum_siblings_retained":5,
			"observables_removed":6,"observables_retained":7},
		"issues":[{"source":"processing","code":"issue_event_traversal_limited","message":"processing was limited"}]
	}`, string(encoded))
}

func TestSchemaRetainsCompiledSchemaDirectly(t *testing.T) {
	loaded := &Schema{}
	require.IsType(t, (*schema.Compiled)(nil), loaded.compiled)
}

func TestNewPipelineRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		options []PipelineOption
		wantErr string
	}{
		{name: "no options", wantErr: "at least one event processing action is required"},
		{
			name: "enrichment without action",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.None),
				WithObservables(enrichment.None),
			},
			wantErr: "at least one event processing action is required",
		},
		{
			name: "enrichment path notation without observables",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithEnrichmentObservablePathNotation(pathstyle.ArrayIndexed),
			},
			wantErr: "observable path notation is configured without adding observables",
		},
		{
			name: "observable IDs without observable enrichment",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithObservables(enrichment.None, 1000),
			},
			wantErr: "observable type IDs are configured without adding observables",
		},
		{
			name: "invalid enrichment path notation",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithObservables(enrichment.Add),
				WithEnrichmentObservablePathNotation(pathstyle.Style("invalid")),
			},
			wantErr: `invalid observable path notation "invalid"`,
		},
		{
			name:    "invalid validation path notation",
			options: []PipelineOption{WithValidation(WithValidationObservablePathNotation(pathstyle.Style("invalid")))},
			wantErr: `validation has invalid observable path notation "invalid"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipeline, err := newPipelineForSchema(newSchema(&schema.Compiled{}), test.options...)

			require.EqualError(t, err, test.wantErr)
			require.Nil(t, pipeline)
		})
	}
}

func TestValidationPolicyChangesLevelsAndIgnoresConfiguredFindings(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	newEvent := func() jsonish.Map {
		event := validValidationEvent()
		delete(event, "name")
		delete(event, "red")
		return event
	}
	find := func(result ProcessingResult, code validation.Code) (eventresult.ValidationFinding, bool) {
		for _, finding := range result.Validation().Findings {
			if finding.Code == code {
				return finding, true
			}
		}
		return eventresult.ValidationFinding{}, false
	}

	result, err := mustNewPipeline(assert, schema, WithValidation(
		WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelWarning),
		WithValidationLevel(validation.AttributeRecommendedMissing, validation.LevelError),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	required, present := find(result, validation.AttributeRequiredMissing)
	assert.True(present)
	assert.Equal(validation.LevelWarning, required.Level)
	recommended, present := find(result, validation.AttributeRecommendedMissing)
	assert.True(present)
	assert.Equal(validation.LevelError, recommended.Level)

	result, err = mustNewPipeline(assert, schema, WithValidation(
		WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelIgnored),
		WithValidationLevel(validation.AttributeRecommendedMissing, validation.LevelIgnored),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	_, present = find(result, validation.AttributeRequiredMissing)
	assert.False(present)
	_, present = find(result, validation.AttributeRecommendedMissing)
	assert.False(present)

	result, err = mustNewPipeline(assert, schema, WithValidation(
		WithAllValidationLevels(validation.LevelIgnored),
		WithValidationLevel(validation.AttributeDeprecated, validation.LevelWarning),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	assert.Empty(result.Validation().Findings)

	result, err = mustNewPipeline(assert, schema, WithValidation(
		WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelWarning),
		WithValidationLevel(validation.AttributeRecommendedMissing, validation.LevelError),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	required, present = find(result, validation.AttributeRequiredMissing)
	assert.True(present)
	assert.Equal(validation.LevelWarning, required.Level)
	recommended, present = find(result, validation.AttributeRecommendedMissing)
	assert.True(present)
	assert.Equal(validation.LevelError, recommended.Level)
}

func TestInvariantRecommendedMissingDefaultsToIgnoredAndCanBeEnabledByLevel(t *testing.T) {
	// Invariant test: validation skips missing recommended attributes at the default ignored level and reports them
	// only when validation-level policy enables them as warning or error.
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	newEvent := func() jsonish.Map {
		event := validValidationEvent()
		delete(event, "red")
		return event
	}
	findRecommendedMissing := func(result ProcessingResult) (eventresult.ValidationFinding, bool) {
		for _, finding := range result.Validation().Findings {
			if finding.Code == validation.AttributeRecommendedMissing {
				return finding, true
			}
		}
		return eventresult.ValidationFinding{}, false
	}

	result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(newEvent())
	assert.NoError(err)
	_, present := findRecommendedMissing(result)
	assert.False(present)

	result, err = mustNewPipeline(assert, schema, WithValidation(
		WithValidationLevel(validation.AttributeRecommendedMissing, validation.LevelWarning),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	finding, present := findRecommendedMissing(result)
	assert.True(present)
	assert.Equal(validation.LevelWarning, finding.Level)
}

func TestValidationLevelPolicyAcceptsExplicitCurrentDefault(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	delete(event, "name")
	delete(event, "red")

	result, err := mustNewPipeline(assert, schema, WithValidation(
		WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelError),
		WithValidationLevel(validation.AttributeRecommendedMissing, validation.LevelWarning),
	)).ProcessEvent(event)

	assert.NoError(err)
	for _, finding := range result.Validation().Findings {
		switch finding.Code {
		case validation.AttributeRequiredMissing:
			assert.Equal(validation.LevelError, finding.Level)
		case validation.AttributeRecommendedMissing:
			assert.Equal(validation.LevelWarning, finding.Level)
		}
	}
}

func TestIssueLevelOptionsRejectRepeatedCode(t *testing.T) {
	schema := makeValidationTestSchema(require.New(t))

	pipeline, err := NewPipeline(
		WithSchema(schema),
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.None),
		WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelWarning),
		WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelIgnored),
	)

	require.Nil(t, pipeline)
	var duplicate *PipelineOptionIssueLevelDuplicateCodeError
	require.ErrorAs(t, err, &duplicate)
	require.Equal(t, issue.EnrichmentEnumSiblingNotAdded, duplicate.Code())
	require.Equal(t, PipelineOptionIssueLevel, duplicate.Option())
}

func TestIgnoredIssueDoesNotChangeEnrichmentRemovalResults(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["activity_id"] = json.Number("1234")
	event["activity_name"] = "source-specific"

	result, err := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Remove),
		WithObservables(enrichment.None),
		WithAllIssueLevels(issue.LevelIgnored),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal("source-specific", event["activity_name"])
	assert.Equal(1, result.EnrichmentRemoval().EnumSiblingsRetained)
	assert.Empty(result.Issues())
}

func TestIssueLevelsIgnoreNoneOneMultipleAndAllCodes(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	newEvent := func() jsonish.Map {
		event := validValidationEvent()
		event["activity_id"] = json.Number("1234")
		event["ball"] = jsonish.Map{"green": "go"}
		event["observables"] = []any{
			jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
		}
		return event
	}
	issueCodesFor := func(options ...PipelineOption) []issue.Code {
		pipeline := mustNewPipeline(assert, schema, append([]PipelineOption{
			WithEnumSiblings(enrichment.Add),
			WithObservables(enrichment.Add),
		}, options...)...)
		result, err := pipeline.ProcessEvent(newEvent())
		assert.NoError(err)
		codes := make([]issue.Code, len(result.Issues()))
		for i, found := range result.Issues() {
			codes[i] = found.Code
		}
		return codes
	}

	assert.ElementsMatch(
		[]issue.Code{issue.EnrichmentEnumSiblingNotAdded, issue.EnrichmentObservableDuplicateSkipped},
		issueCodesFor(),
		"ignoring none reports every ignorable issue",
	)
	assert.ElementsMatch(
		[]issue.Code{issue.EnrichmentObservableDuplicateSkipped},
		issueCodesFor(WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelIgnored)),
		"ignoring one code leaves the other reported",
	)
	assert.Empty(
		issueCodesFor(
			WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelIgnored),
			WithIssueLevel(issue.EnrichmentObservableDuplicateSkipped, issue.LevelIgnored),
		),
		"ignoring multiple selected codes ignores each of them",
	)
	assert.Empty(
		issueCodesFor(WithAllIssueLevels(issue.LevelIgnored)),
		"an all-code ignored level omits every ignorable issue",
	)
}

func TestNewPipelineValidatesSelectedObservableTypeIDs(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)

	pipeline, err := newPipelineForSchema(schema,
		WithEnumSiblings(enrichment.None),
		WithObservables(enrichment.Add, 3000, 1000, -1, 3000),
	)

	assert.Nil(pipeline)
	assert.EqualError(err, "enrichment processor has unknown observable type IDs: -1, 3000")
}

func TestWithObservablesCopiesIDs(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	selected := []int64{1000}
	selection := WithObservables(enrichment.Add, selected...)
	selected[0] = 3000

	pipeline, err := newPipelineForSchema(schema, selection)

	assert.NoError(err)
	event := jsonish.Map{"class_uid": json.Number("1"), "ball": jsonish.Map{"green": "go"}}
	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Equal(1, result.Enrichment().ObservablesAdded)
}

func TestSchemaZeroValueCannotBuildPipeline(t *testing.T) {
	var schema Schema
	pipeline, err := newPipelineForSchema(&schema, WithValidation())

	require.ErrorIs(t, err, errUninitializedSchema)
	require.Nil(t, pipeline)
}

func TestNewPipelineRequiresSchema(t *testing.T) {
	pipeline, err := NewPipeline(WithValidation())

	require.ErrorIs(t, err, errSchemaNotConfigured)
	require.Nil(t, pipeline)
}

func TestNewPipelineReportsSchemaStateBeforeSemanticPolicyErrors(t *testing.T) {
	tests := []struct {
		name    string
		schema  *Schema
		wantErr error
	}{
		{name: "missing", wantErr: errSchemaNotConfigured},
		{name: "uninitialized", schema: &Schema{}, wantErr: errUninitializedSchema},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := []PipelineOption{
				WithValidation(WithValidationLevel(validation.Code(255), validation.LevelWarning)),
			}
			if test.schema != nil {
				options = append([]PipelineOption{WithSchema(test.schema)}, options...)
			}

			pipeline, err := NewPipeline(options...)

			require.ErrorIs(t, err, test.wantErr)
			require.Nil(t, pipeline)
		})
	}
}

func TestPipelineOptionsRejectDuplicateSingletons(t *testing.T) {
	valid := makeValidationTestSchema(require.New(t))

	for _, test := range []struct {
		name    string
		options []PipelineOption
		option  PipelineOptionName
	}{
		{
			name:    "schema",
			options: []PipelineOption{WithSchema(valid), WithSchema(valid), WithValidation()},
			option:  PipelineOptionSchema,
		},
		{
			name: "enum siblings",
			options: []PipelineOption{
				WithSchema(valid), WithEnumSiblings(enrichment.Add), WithEnumSiblings(enrichment.Remove),
			},
			option: PipelineOptionEnumSiblings,
		},
		{
			name: "observables",
			options: []PipelineOption{
				WithSchema(valid), WithObservables(enrichment.Add), WithObservables(enrichment.Remove),
			},
			option: PipelineOptionObservables,
		},
		{
			name: "enrichment observable path notation",
			options: []PipelineOption{
				WithSchema(valid),
				WithObservables(enrichment.Add),
				WithEnrichmentObservablePathNotation(pathstyle.Simple),
				WithEnrichmentObservablePathNotation(pathstyle.ArrayIndexed),
			},
			option: PipelineOptionEnrichmentObservablePathNotation,
		},
		{
			name:    "validation",
			options: []PipelineOption{WithSchema(valid), WithValidation(), WithValidation()},
			option:  PipelineOptionValidation,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pipeline, err := NewPipeline(test.options...)

			require.Nil(t, pipeline)
			var duplicate *PipelineOptionDuplicateError
			require.ErrorAs(t, err, &duplicate)
			require.Equal(t, test.option, duplicate.Option())
		})
	}
}

func TestNewPipelineReportsFirstConfigurationProblem(t *testing.T) {
	pipeline, err := newPipelineForSchema(newSchema(&schema.Compiled{}),
		WithObservables(enrichment.None, 1000),
		WithEnrichmentObservablePathNotation(pathstyle.ArrayIndexed),
		WithIssueLevel(issue.Code(255), issue.LevelWarning),
	)

	require.Nil(t, pipeline)
	require.EqualError(t, err, "at least one event processing action is required")
	_, ok := err.(interface{ Unwrap() []error })
	require.False(t, ok)
	require.NotContains(t, err.Error(), "observable path notation is configured without adding observables")
	require.NotContains(t, err.Error(), "observable type IDs are configured without adding observables")
	require.NotContains(t, err.Error(), "issue policy has unknown issue code 255")
}

func TestNewPipelineAllowsDifferentEnumSiblingsAndObservablesActions(t *testing.T) {
	pipeline, err := newPipelineForSchema(newSchema(&schema.Compiled{}),
		WithEnumSiblings(enrichment.Remove),
		WithObservables(enrichment.Add),
	)

	require.NoError(t, err)
	require.NotNil(t, pipeline)
}

func TestPipelineProcessesDistinctEventsConcurrently(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
		WithValidation(),
	)

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
				errorCount := len(findingsAtLevel(result.Validation().Findings, validation.LevelError))
				warningCount := len(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))
				if errorCount != 0 || warningCount != 0 {
					errorsFound <- fmt.Errorf("unexpected validation result: %+v", result.Validation())
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

func TestPipelineStopsAfterRepeatedObjectAttribute(t *testing.T) {
	assert := require.New(t)
	trueValue := true
	personType := "person"
	ldapPersonType := "ldap_person"
	stateSibling := "state"
	objectObservableType := int64(7)

	stateAttribute := func() *itemAttributeDefinition {
		return &itemAttributeDefinition{CommonAttributeDefinition: commonAttributeDefinition{
			Type:    "integer_t",
			Sibling: &stateSibling,
			Enum: map[string]*enumDefinition{
				"1": {Caption: "Known"},
			},
		}}
	}
	objectAttribute := func(objectType string) *itemAttributeDefinition {
		return &itemAttributeDefinition{CommonAttributeDefinition: commonAttributeDefinition{
			Type:       "object_t",
			ObjectType: &objectType,
		}}
	}

	class := &classDefinition{
		Uid: 1,
		ItemDefinition: commonItemDefinition{
			Name: "test_class",
			Attributes: map[string]*itemAttributeDefinition{
				"class_uid": {CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
				"people": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type:       "object_t",
						ObjectType: &personType,
						IsArray:    &trueValue,
					},
				},
			},
		},
	}
	person := &objectDefinition{ItemDefinition: commonItemDefinition{
		Name: "person",
		Attributes: map[string]*itemAttributeDefinition{
			"state_id":    stateAttribute(),
			"state":       {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
			"ldap_person": objectAttribute(ldapPersonType),
		},
	}}
	ldapPerson := &objectDefinition{
		Observable: &objectObservableType,
		ItemDefinition: commonItemDefinition{
			Name: "ldap_person",
			Attributes: map[string]*itemAttributeDefinition{
				"state_id": stateAttribute(),
				"state":    {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
				"manager":  objectAttribute(personType),
			},
		},
	}
	compiledSchema := newSchema(&schema.Compiled{
		Classes: map[int64]*classDefinition{1: class},
		Objects: map[string]*objectDefinition{
			personType:     person,
			ldapPersonType: ldapPerson,
		},
		Dictionary: &dictionaryDefinition{
			Attributes: map[string]*commonAttributeDefinition{},
			Types:      &typesDefinition{Attributes: map[string]*typeDefinition{}},
		},
	})
	pipeline := mustNewPipeline(assert, compiledSchema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
		WithAllIssueLevels(issue.LevelIgnored),
		WithValidation(),
	)
	recursivePerson := func() jsonish.Map {
		return jsonish.Map{
			"state_id": 1,
			"ldap_person": jsonish.Map{
				"state_id": 1,
				"manager": jsonish.Map{
					"state_id":    1,
					"ldap_person": jsonish.Map{"state_id": 1},
				},
			},
		}
	}
	event := jsonish.Map{
		"class_uid": 1,
		"people":    []any{recursivePerson(), recursivePerson()},
	}

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	people, ok := event["people"].([]any)
	assert.True(ok)
	firstPerson, ok := people[0].(jsonish.Map)
	assert.True(ok)
	assert.Equal("Known", firstPerson["state"])
	firstLDAPPerson, ok := firstPerson["ldap_person"].(jsonish.Map)
	assert.True(ok)
	assert.Equal("Known", firstLDAPPerson["state"])
	manager, ok := firstLDAPPerson["manager"].(jsonish.Map)
	assert.True(ok)
	assert.Equal("Known", manager["state"])
	duplicateLDAPPerson, ok := manager["ldap_person"].(jsonish.Map)
	assert.True(ok)
	assert.NotContains(duplicateLDAPPerson, "state")
	assert.Equal(6, result.Enrichment().EnumSiblingsAdded)
	assert.Equal(1, result.Enrichment().ObservablesAdded)
	observables, ok := event["observables"].([]jsonish.Map)
	assert.True(ok)
	assert.Equal("people.ldap_person", observables[0]["name"])
	enrichmentIssues := issuesWithCode(result.Issues(), "issue_event_traversal_limited")
	assert.Len(enrichmentIssues, 1)
	assert.Equal(issue.SourceProcessing, enrichmentIssues[0].Source)
	assert.Equal("people[0].ldap_person.manager.ldap_person", enrichmentIssues[0].Details["attribute_path"])
	enrichmentWarningFindings := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
	assert.Empty(issuesWithCode(enrichmentWarningFindings, "issue_event_traversal_limited"))

	validationOnlyEvent := jsonish.Map{"class_uid": 1, "people": []any{recursivePerson(), recursivePerson()}}
	validationOnlyResult, err := mustNewPipeline(assert, compiledSchema, WithValidation()).
		ProcessEvent(validationOnlyEvent)
	assert.NoError(err)
	validationOnlyIssues := issuesWithCode(validationOnlyResult.Issues(), "issue_event_traversal_limited")
	assert.Len(validationOnlyIssues, 1)
	validationOnlyWarningFindings := findingsAtLevel(
		validationOnlyResult.Validation().Findings, validation.LevelWarning,
	)
	assert.Empty(issuesWithCode(validationOnlyWarningFindings, "issue_event_traversal_limited"))

	removalResult, err := mustNewPipeline(
		assert,
		compiledSchema,
		WithEnumSiblings(enrichment.Remove), WithObservables(enrichment.None),
	).ProcessEvent(event)
	assert.NoError(err)
	removalIssues := issuesWithCode(removalResult.Issues(), "issue_event_traversal_limited")
	assert.Len(removalIssues, 1)
	assert.Equal(issue.SourceProcessing, removalIssues[0].Source)
	assert.Equal("people[0].ldap_person.manager.ldap_person", removalIssues[0].Details["attribute_path"])

	observableRemovalOnlyEvent := jsonish.Map{"class_uid": 1, "people": []any{recursivePerson()}}
	observableRemovalOnlyResult, err := mustNewPipeline(
		assert,
		compiledSchema,
		WithEnumSiblings(enrichment.None), WithObservables(enrichment.Remove),
	).ProcessEvent(observableRemovalOnlyEvent)
	assert.NoError(err)
	assert.Empty(issuesWithCode(observableRemovalOnlyResult.Issues(), "issue_event_traversal_limited"))

	observableRemovalOnlyEvent["observables"] = []any{jsonish.Map{
		"name": "people.ldap_person.manager.ldap_person.state_id", "type_id": 1, "value": "1",
	}}
	observableRemovalOnlyResult, err = mustNewPipeline(
		assert,
		compiledSchema,
		WithEnumSiblings(enrichment.None), WithObservables(enrichment.Remove),
	).ProcessEvent(observableRemovalOnlyEvent)
	assert.NoError(err)
	observableTraversalIssues := issuesWithCode(observableRemovalOnlyResult.Issues(), "issue_event_traversal_limited")
	assert.Len(observableTraversalIssues, 1)
	assert.Equal("observables[0]", observableTraversalIssues[0].Details["attribute_path"])
}

func TestEngineeringInvariantErrorLevelObservableTraversalIssueStopsProcessing(t *testing.T) {
	// Engineering invariant test: a mandatory traversal-limit issue promoted to error must stop processing and return
	// a ProcessingIssueError regardless of whether schema walking or observable analysis detects the limit.
	assert := require.New(t)
	personType := "person"
	objectAttribute := func(objectType string) *itemAttributeDefinition {
		return &itemAttributeDefinition{CommonAttributeDefinition: commonAttributeDefinition{
			Type:       "object_t",
			ObjectType: &objectType,
		}}
	}
	loaded := newSchema(&schema.Compiled{
		Classes: map[int64]*classDefinition{1: {
			Uid: 1,
			ItemDefinition: commonItemDefinition{Attributes: map[string]*itemAttributeDefinition{
				"class_uid": {CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
				"person":    objectAttribute(personType),
			}},
		}},
		Objects: map[string]*objectDefinition{personType: {
			ItemDefinition: commonItemDefinition{Attributes: map[string]*itemAttributeDefinition{
				"manager": objectAttribute(personType),
			}},
		}},
		Dictionary: &dictionaryDefinition{
			Attributes: map[string]*commonAttributeDefinition{},
			Types:      &typesDefinition{Attributes: map[string]*typeDefinition{}},
		},
	})
	pipeline := mustNewPipeline(assert, loaded,
		WithIssueLevel(issue.EventTraversalLimited, issue.LevelError),
		WithValidation(
			WithAllValidationLevels(validation.LevelIgnored),
			WithValidationLevel(validation.ObservablePathNotFound, validation.LevelWarning),
		),
	)
	event := jsonish.Map{
		"class_uid": 1,
		"observables": []any{jsonish.Map{
			"name": "person.manager.manager.manager",
		}},
	}

	result, err := pipeline.ProcessEvent(event)

	assert.Equal(ProcessingResult{}, result)
	var issueErr *ProcessingIssueError
	assert.ErrorAs(err, &issueErr)
	assert.Equal(issue.EventTraversalLimited, issueErr.Issue().Code)
	assert.Equal(issue.SourceProcessing, issueErr.Issue().Source)
}

func TestPipelineConstructionIsConcurrent(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)

	const workerCount = 16
	start := make(chan struct{})
	errorsFound := make(chan error, workerCount)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			<-start
			_, err := newPipelineForSchema(schema,
				WithEnumSiblings(enrichment.Add),
				WithObservables(enrichment.Add),
				WithValidation(),
			)
			if err != nil {
				errorsFound <- err
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

func TestNewPipelineRejectsInvalidAllowedValues(t *testing.T) {
	tests := []struct {
		name      string
		typeName  string
		typeDef   *typeDefinition
		wantError string
	}{
		{
			name:     "integer outside int64",
			typeName: "bad_integer_t",
			typeDef: &typeDefinition{
				CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"},
				Values:                    []any{json.Number("9223372036854775808")},
			},
			wantError: "type \"bad_integer_t\" allowed value at index 0 is not a signed 64-bit integer",
		},
		{
			name:     "non-finite float",
			typeName: "bad_float_t",
			typeDef: &typeDefinition{
				CommonAttributeDefinition: commonAttributeDefinition{Type: "float_t"},
				Values:                    []any{math.Inf(1)},
			},
			wantError: "type \"bad_float_t\" allowed value at index 0 is not a finite float64",
		},
		{
			name:     "wrong string value type",
			typeName: "bad_string_t",
			typeDef: &typeDefinition{
				CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
				Values:                    []any{true},
			},
			wantError: "type \"bad_string_t\" allowed value at index 0 is not a string",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			schema.compiledForTest().Dictionary.Types.Attributes[test.typeName] = test.typeDef

			pipeline, err := newPipelineForSchema(schema, WithValidation())

			assert.Nil(pipeline)
			assert.EqualError(err, test.wantError)
		})
	}
}
