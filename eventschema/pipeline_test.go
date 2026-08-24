package eventschema

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
	"github.com/ocsf/ocsf-toolkit/internal/processing"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestPipelineOwnsPublicMethodSet(t *testing.T) {
	publicType := reflect.TypeFor[Pipeline]()
	require.Equal(t, 1, publicType.NumField())
	require.Equal(t, reflect.TypeFor[*processing.Pipeline](), publicType.Field(0).Type)

	publicPointerType := reflect.TypeFor[*Pipeline]()
	require.Equal(t, 1, publicPointerType.NumMethod())
	_, ok := publicPointerType.MethodByName("ProcessEvent")
	require.True(t, ok)
}

func TestProcessingResultIsOpaqueConcreteValueWithAccessors(t *testing.T) {
	resultType := reflect.TypeFor[ProcessingResult]()
	require.Equal(t, 1, resultType.NumField())
	require.NotEmpty(t, resultType.Field(0).PkgPath, "the result state must remain unexported")

	for _, method := range []string{"Validation", "Enrichment", "EnrichmentRemoval", "Issues", "SuppressedIssueCount"} {
		_, ok := resultType.MethodByName(method)
		require.Truef(t, ok, "ProcessingResult.%s is missing", method)
	}
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
			SuppressedErrorCount:   1,
			SuppressedWarningCount: 2,
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
		suppressedIssues: 8,
	}}

	encoded, err := json.Marshal(original)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"validation":{"findings":[
			{"level":"error","code":"validation_attribute_required_missing","message":"required attribute is missing"}
		],"suppressed_error_count":1,"suppressed_warning_count":2},
		"enrichment":{"enum_siblings_added":2,"observables_added":3},
		"enrichment_removal":{"enum_siblings_removed":4,"enum_siblings_retained":5,
			"observables_removed":6,"observables_retained":7},
		"issues":[{"source":"processing","code":"issue_event_traversal_limited","message":"processing was limited"}],
		"suppressed_issue_count":8
	}`, string(encoded))
}

func TestSchemaUsesInternalProcessingSchemaDirectly(t *testing.T) {
	schema := &Schema{}
	require.IsType(t, (*processing.PipelineFactory)(nil), schema.pipelineFactory)
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
		{
			name: "unknown typed issue suppression code",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithSuppressIssues(issue.IssueCode(255)),
			},
			wantErr: "issue suppression has unknown issue codes: 255",
		},
		{
			name: "unknown string issue suppression code",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Remove),
				WithSuppressIssuesByStrings("issue_unknown_z", "issue_unknown_a"),
			},
			wantErr: `issue suppression has unknown issue codes: "issue_unknown_a", "issue_unknown_z"`,
		},
		{
			name: "mandatory issue suppression code",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithSuppressIssues(issue.EventTraversalLimited),
			},
			wantErr: "issue suppression cannot suppress mandatory issue codes: issue_event_traversal_limited",
		},
		{
			name: "mandatory class resolution issue suppression code",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithSuppressIssues(issue.ClassUIDMissing),
			},
			wantErr: "issue suppression cannot suppress mandatory issue codes: issue_class_uid_missing",
		},
		{
			name: "unknown validation policy code",
			options: []PipelineOption{WithValidation(
				WithSuppressValidation(validation.Code(255)),
			)},
			wantErr: "validation policy has unknown validation code 255",
		},
		{
			name: "mandatory validation suppression code",
			options: []PipelineOption{WithValidation(
				WithSuppressValidation(validation.ClassUIDMissing),
			)},
			wantErr: "validation policy cannot suppress mandatory validation codes: validation_class_uid_missing",
		},
		{
			name: "conflicting validation policy actions",
			options: []PipelineOption{WithValidation(
				WithSuppressValidation(validation.AttributeRequiredMissing),
				WithValidationErrorsAsWarnings(validation.AttributeRequiredMissing),
			)},
			wantErr: "validation policy has conflicting actions for validation_attribute_required_missing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipeline, err := (&Schema{}).NewPipeline(test.options...)

			require.EqualError(t, err, test.wantErr)
			require.Nil(t, pipeline)
		})
	}
}

func TestValidationPolicyChangesLevelsAndTracksSuppressionByDefaultLevel(t *testing.T) {
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
		WithWarnOnMissingRecommended(),
		WithValidationErrorsAsWarnings(validation.AttributeRequiredMissing),
		WithValidationWarningsAsErrors(validation.AttributeRecommendedMissing),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	required, present := find(result, validation.AttributeRequiredMissing)
	assert.True(present)
	assert.Equal(validation.LevelWarning, required.Level)
	recommended, present := find(result, validation.AttributeRecommendedMissing)
	assert.True(present)
	assert.Equal(validation.LevelError, recommended.Level)

	result, err = mustNewPipeline(assert, schema, WithValidation(
		WithWarnOnMissingRecommended(),
		WithSuppressValidation(validation.AttributeRequiredMissing, validation.AttributeRecommendedMissing),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	_, present = find(result, validation.AttributeRequiredMissing)
	assert.False(present)
	_, present = find(result, validation.AttributeRecommendedMissing)
	assert.False(present)
	assert.Equal(1, result.Validation().SuppressedErrorCount)
	assert.Equal(1, result.Validation().SuppressedWarningCount)

	result, err = mustNewPipeline(assert, schema, WithValidation(
		WithWarnOnMissingRecommended(),
		WithSuppressValidation(),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	assert.Empty(result.Validation().Findings)
	assert.GreaterOrEqual(result.Validation().SuppressedErrorCount, 1)
	assert.Equal(1, result.Validation().SuppressedWarningCount)

	result, err = mustNewPipeline(assert, schema, WithValidation(
		WithWarnOnMissingRecommended(),
		WithValidationErrorsAsWarnings(),
		WithValidationWarningsAsErrors(),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	required, present = find(result, validation.AttributeRequiredMissing)
	assert.True(present)
	assert.Equal(validation.LevelWarning, required.Level)
	recommended, present = find(result, validation.AttributeRecommendedMissing)
	assert.True(present)
	assert.Equal(validation.LevelError, recommended.Level)
}

func TestValidationLevelPolicyAcceptsExplicitCurrentDefault(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	delete(event, "name")
	delete(event, "red")

	result, err := mustNewPipeline(assert, schema, WithValidation(
		WithWarnOnMissingRecommended(),
		WithValidationWarningsAsErrors(validation.AttributeRequiredMissing),
		WithValidationErrorsAsWarnings(validation.AttributeRecommendedMissing),
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

func TestIssueSuppressionOptionsUseLastValueAndCopyCodes(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	selected := []issue.IssueCode{issue.EnrichmentEnumSiblingOtherAdded}
	selection := WithSuppressIssues(selected...)
	selected[0] = issue.EnrichmentEnumSiblingNotAdded

	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.None),
		WithSuppressIssues(),
		selection,
	)
	event := validValidationEvent()
	event["activity_id"] = json.Number("1234")

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Len(result.Issues(), 1, "the last option suppresses only the copied enum-other code")
	assert.Equal(issue.EnrichmentEnumSiblingNotAdded, result.Issues()[0].Code)

	pipeline = mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.None),
		WithSuppressIssuesByStrings(issue.EnrichmentEnumSiblingNotAdded.String()),
		WithSuppressIssues(),
	)
	event = validValidationEvent()
	event["activity_id"] = json.Number("1234")
	result, err = pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(result.Issues(), "the final empty list suppresses every suppressible issue")
	assert.Equal(1, result.SuppressedIssueCount())

	pipeline = mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.None),
		WithSuppressIssuesByStrings(issue.EnrichmentEnumSiblingNotAdded.String()),
	)
	event = validValidationEvent()
	event["activity_id"] = json.Number("1234")
	result, err = pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(result.Issues(), "the string-based option suppresses its selected issue")
	assert.Equal(1, result.SuppressedIssueCount())
}

func TestIssueSuppressionDoesNotChangeEnrichmentRemovalResults(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["activity_id"] = json.Number("1234")
	event["activity_name"] = "source-specific"

	result, err := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Remove),
		WithObservables(enrichment.None),
		WithSuppressIssues(),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Equal("source-specific", event["activity_name"])
	assert.Equal(1, result.EnrichmentRemoval().EnumSiblingsRetained)
	assert.Empty(result.Issues())
	assert.Equal(1, result.SuppressedIssueCount())
}

func TestWithSuppressIssuesSuppressesNoneOneMultipleAndAllCodes(t *testing.T) {
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
	issueCodesFor := func(options ...PipelineOption) []issue.IssueCode {
		pipeline := mustNewPipeline(assert, schema, append([]PipelineOption{
			WithEnumSiblings(enrichment.Add),
			WithObservables(enrichment.Add),
		}, options...)...)
		result, err := pipeline.ProcessEvent(newEvent())
		assert.NoError(err)
		codes := make([]issue.IssueCode, len(result.Issues()))
		for i, found := range result.Issues() {
			codes[i] = found.Code
		}
		return codes
	}

	assert.ElementsMatch(
		[]issue.IssueCode{issue.EnrichmentEnumSiblingNotAdded, issue.EnrichmentObservableDuplicateSkipped},
		issueCodesFor(),
		"suppressing none reports every suppressible issue",
	)
	assert.ElementsMatch(
		[]issue.IssueCode{issue.EnrichmentObservableDuplicateSkipped},
		issueCodesFor(WithSuppressIssues(issue.EnrichmentEnumSiblingNotAdded)),
		"suppressing one code leaves the other reported",
	)
	assert.Empty(
		issueCodesFor(
			WithSuppressIssues(issue.EnrichmentEnumSiblingNotAdded, issue.EnrichmentObservableDuplicateSkipped),
		),
		"suppressing multiple selected codes suppresses each of them",
	)
	assert.Empty(
		issueCodesFor(WithSuppressIssues()),
		"suppressing with an empty list suppresses every suppressible issue",
	)
}

func TestNewPipelineValidatesSelectedObservableTypeIDs(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)

	pipeline, err := schema.NewPipeline(
		WithEnumSiblings(enrichment.None),
		WithObservables(enrichment.Add, 3000, 1000, -1, 3000),
	)

	assert.Nil(pipeline)
	assert.EqualError(err, "enrichment processor has unknown observable type IDs: -1, 3000")
}

func TestWithObservablesCopiesIDsAndLastOptionWins(t *testing.T) {
	assert := require.New(t)
	schema := makeTestSchema(assert)
	selected := []int64{1000}
	selection := WithObservables(enrichment.Add, selected...)
	selected[0] = 3000

	pipeline, err := schema.NewPipeline(
		WithObservables(enrichment.Add, 3000),
		selection,
	)

	assert.NoError(err)
	event := jsonish.Map{"class_uid": json.Number("1"), "ball": jsonish.Map{"green": "go"}}
	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Equal(1, result.Enrichment().ObservablesAdded)
}

func TestSchemaZeroValueCannotBuildPipeline(t *testing.T) {
	var schema Schema
	pipeline, err := schema.NewPipeline(WithValidation())

	require.ErrorIs(t, err, errUninitializedSchema)
	require.Nil(t, pipeline)
}

func TestNewPipelineReportsAllConfigurationProblems(t *testing.T) {
	pipeline, err := newSchema(&schema.Compiled{}).NewPipeline(
		WithObservables(enrichment.None, 1000),
		WithEnrichmentObservablePathNotation(pathstyle.ArrayIndexed),
		WithSuppressIssues(issue.IssueCode(255)),
	)

	require.Nil(t, pipeline)
	require.EqualError(t, err, "at least one event processing action is required\n"+
		"observable path notation is configured without adding observables\n"+
		"observable type IDs are configured without adding observables\n"+
		"issue suppression has unknown issue codes: 255")
	joined, ok := err.(interface{ Unwrap() []error })
	require.True(t, ok)
	require.Len(t, joined.Unwrap(), 4)
}

func TestNewPipelineAllowsDifferentEnumSiblingsAndObservablesActions(t *testing.T) {
	pipeline, err := newSchema(&schema.Compiled{}).NewPipeline(
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
		WithSuppressIssues(),
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
			_, err := schema.NewPipeline(
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

			pipeline, err := schema.NewPipeline(WithValidation())

			assert.Nil(pipeline)
			assert.EqualError(err, test.wantError)
		})
	}
}
