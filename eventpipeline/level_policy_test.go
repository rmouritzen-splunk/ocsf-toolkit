package eventpipeline

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

func TestWithValidationLevelSetsExplicitFindingDisposition(t *testing.T) {
	assert := require.New(t)
	loaded := makeValidationTestSchema(assert)
	newEvent := func() jsonish.Map {
		event := validValidationEvent()
		delete(event, "name")
		return event
	}

	warningResult, err := mustNewPipeline(assert, loaded, WithValidation(
		WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelWarning),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	assert.Equal(validation.LevelWarning, findingForCode(
		assert, warningResult, validation.AttributeRequiredMissing,
	).Level)

	ignoredResult, err := mustNewPipeline(assert, loaded, WithValidation(
		WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelIgnored),
	)).ProcessEvent(newEvent())
	assert.NoError(err)
	assert.False(hasFindingCode(ignoredResult, validation.AttributeRequiredMissing))
}

func TestWithIssueLevelSupportsIgnoredWarningAndError(t *testing.T) {
	assert := require.New(t)
	loaded := makeValidationTestSchema(assert)
	newEvent := func() jsonish.Map {
		event := validValidationEvent()
		event["activity_id"] = json.Number("1234")
		return event
	}
	newPipeline := func(level issue.Level) *Pipeline {
		return mustNewPipeline(assert, loaded,
			WithEnumSiblings(enrichment.Add),
			WithObservables(enrichment.None),
			WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, level),
		)
	}

	ignoredResult, err := newPipeline(issue.LevelIgnored).ProcessEvent(newEvent())
	assert.NoError(err)
	assert.Empty(ignoredResult.Issues())

	warningResult, err := newPipeline(issue.LevelWarning).ProcessEvent(newEvent())
	assert.NoError(err)
	assert.Len(warningResult.Issues(), 1)
	assert.Equal(issue.EnrichmentEnumSiblingNotAdded, warningResult.Issues()[0].Code)

	errorEvent := newEvent()
	errorResult, err := newPipeline(issue.LevelError).ProcessEvent(errorEvent)
	var issueErr *ProcessingIssueError
	assert.ErrorAs(err, &issueErr)
	assert.Equal(issue.EnrichmentEnumSiblingNotAdded, issueErr.Issue().Code)
	assert.Equal(issue.SourceEnrichment, issueErr.Issue().Source)
	assert.Contains(err.Error(), issue.EnrichmentEnumSiblingNotAdded.String())
	assert.Empty(errorResult.Issues())
	assert.Empty(errorResult.Validation().Findings)
	assert.Zero(errorResult.Enrichment().EnumSiblingsAdded)
	assert.Zero(errorResult.Enrichment().ObservablesAdded)
	assert.Zero(errorResult.EnrichmentRemoval().EnumSiblingsRemoved)
	assert.Zero(errorResult.EnrichmentRemoval().ObservablesRemoved)
	assert.NotContains(errorEvent, "activity_name")
}

func TestExplicitLevelsOverrideAllCodeLevels(t *testing.T) {
	assert := require.New(t)
	loaded := makeValidationTestSchema(assert)
	event := validValidationEvent()
	delete(event, "name")
	event["activity_id"] = json.Number("1234")

	result, err := mustNewPipeline(assert, loaded,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.None),
		WithAllIssueLevels(issue.LevelIgnored),
		WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelWarning),
		WithValidation(
			WithAllValidationLevels(validation.LevelIgnored),
			WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelWarning),
		),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.Len(result.Issues(), 1)
	assert.Equal(issue.EnrichmentEnumSiblingNotAdded, result.Issues()[0].Code)
	assert.Equal(validation.LevelWarning, findingForCode(
		assert, result, validation.AttributeRequiredMissing,
	).Level)
}

func TestLevelOptionsRejectDuplicateCodesAndLateBaseline(t *testing.T) {
	loaded := makeValidationTestSchema(require.New(t))

	tests := []struct {
		name       string
		options    []PipelineOption
		checkError func(*require.Assertions, error)
	}{
		{
			name: "duplicate issue code",
			options: []PipelineOption{
				WithSchema(loaded),
				WithEnumSiblings(enrichment.Add),
				WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelWarning),
				WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelError),
			},
			checkError: func(assert *require.Assertions, err error) {
				var duplicate *PipelineOptionIssueLevelDuplicateCodeError
				assert.ErrorAs(err, &duplicate)
				assert.Equal(issue.EnrichmentEnumSiblingNotAdded, duplicate.Code())
				assert.Equal(PipelineOptionIssueLevel, duplicate.Option())
			},
		},
		{
			name: "duplicate validation code",
			options: []PipelineOption{WithSchema(loaded), WithValidation(
				WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelWarning),
				WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelError),
			)},
			checkError: func(assert *require.Assertions, err error) {
				var duplicate *PipelineOptionValidationLevelDuplicateCodeError
				assert.ErrorAs(err, &duplicate)
				assert.Equal(validation.AttributeRequiredMissing, duplicate.Code())
				assert.Equal(PipelineOptionValidationLevel, duplicate.Option())
			},
		},
		{
			name: "duplicate issue baseline",
			options: []PipelineOption{
				WithSchema(loaded),
				WithEnumSiblings(enrichment.Add),
				WithAllIssueLevels(issue.LevelWarning),
				WithAllIssueLevels(issue.LevelError),
			},
			checkError: func(assert *require.Assertions, err error) {
				var duplicate *PipelineOptionDuplicateError
				assert.ErrorAs(err, &duplicate)
				assert.Equal(PipelineOptionAllIssueLevels, duplicate.Option())
			},
		},
		{
			name: "issue baseline after code",
			options: []PipelineOption{
				WithSchema(loaded),
				WithEnumSiblings(enrichment.Add),
				WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelWarning),
				WithAllIssueLevels(issue.LevelError),
			},
			checkError: func(assert *require.Assertions, err error) {
				var afterCode *PipelineOptionIssueLevelAllAfterCodeError
				assert.ErrorAs(err, &afterCode)
				assert.Equal(PipelineOptionAllIssueLevels, afterCode.Option())
			},
		},
		{
			name: "validation baseline after code",
			options: []PipelineOption{WithSchema(loaded), WithValidation(
				WithValidationLevel(validation.AttributeRequiredMissing, validation.LevelWarning),
				WithAllValidationLevels(validation.LevelError),
			)},
			checkError: func(assert *require.Assertions, err error) {
				var afterCode *PipelineOptionValidationLevelAllAfterCodeError
				assert.ErrorAs(err, &afterCode)
				assert.Equal(PipelineOptionAllValidationLevels, afterCode.Option())
			},
		},
		{
			name: "duplicate validation baseline",
			options: []PipelineOption{WithSchema(loaded), WithValidation(
				WithAllValidationLevels(validation.LevelWarning),
				WithAllValidationLevels(validation.LevelError),
			)},
			checkError: func(assert *require.Assertions, err error) {
				var duplicate *PipelineOptionDuplicateError
				assert.ErrorAs(err, &duplicate)
				assert.Equal(PipelineOptionAllValidationLevels, duplicate.Option())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipeline, err := NewPipeline(test.options...)

			require.Nil(t, pipeline)
			assert := require.New(t)
			test.checkError(assert, err)

			var optionErr PipelineOptionError
			assert.ErrorAs(err, &optionErr)
		})
	}
}

func TestInvariantSelectiveValidationLevelReportsConfiguredFinding(t *testing.T) {
	// Invariant test: an explicit validation-code level must enable that finding even when every other code is ignored.
	assert := require.New(t)
	loaded := makeValidationTestSchema(assert)
	statusIDs := loaded.compiledForTest().Classes[int64(1)].Attributes["status_ids"]
	statusIDs.Enum["99"] = &enumDefinition{Caption: "Other"}
	loaded.compiledForTest().Classes[int64(1)].Attributes["mode_id"].Enum["1"].Deprecated = &deprecatedDefinition{
		Since: "1.0.0", Message: "enum deprecated",
	}

	tests := []struct {
		name  string
		code  validation.Code
		event func() jsonish.Map
	}{
		{
			name: "enum array sibling length mismatch",
			code: validation.AttributeEnumArraySiblingLengthMismatch,
			event: func() jsonish.Map {
				event := validValidationEvent()
				event["status_ids"] = []any{json.Number("1"), json.Number("2")}
				event["statuses"] = []any{"Open"}
				return event
			},
		},
		{
			name: "enum array sibling suspicious",
			code: validation.AttributeEnumSiblingSuspicious,
			event: func() jsonish.Map {
				event := validValidationEvent()
				event["status_ids"] = []any{json.Number("99")}
				event["statuses"] = []any{"Other"}
				return event
			},
		},
		{
			name: "enum array sibling missing",
			code: validation.AttributeEnumArraySiblingMissing,
			event: func() jsonish.Map {
				event := validValidationEvent()
				event["status_ids"] = []any{json.Number("99")}
				event["statuses"] = []any{nil}
				return event
			},
		},
		{
			name: "enum array sibling incorrect",
			code: validation.AttributeEnumArraySiblingIncorrect,
			event: func() jsonish.Map {
				event := validValidationEvent()
				event["status_ids"] = []any{json.Number("1")}
				event["statuses"] = []any{"Closed"}
				return event
			},
		},
		{
			name: "scalar enum value deprecated",
			code: validation.AttributeEnumValueDeprecated,
			event: func() jsonish.Map {
				event := validValidationEvent()
				event["mode_id"] = json.Number("1")
				return event
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			pipeline := mustNewPipeline(assert, loaded, WithValidation(
				WithAllValidationLevels(validation.LevelIgnored),
				WithValidationLevel(test.code, validation.LevelWarning),
			))

			result, err := pipeline.ProcessEvent(test.event())

			assert.NoError(err)
			assert.Equal(
				[]validation.Code{test.code},
				validationFindingCodes(result.Validation().Findings),
			)
			assert.Equal(validation.LevelWarning, result.Validation().Findings[0].Level)
		})
	}
}

func TestClassUIDValidationCanBeIgnoredWithoutIgnoringProcessingIssue(t *testing.T) {
	assert := require.New(t)
	loaded := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, loaded,
		WithAllIssueLevels(issue.LevelIgnored),
		WithValidation(
			WithAllValidationLevels(validation.LevelIgnored),
			WithValidationLevel(validation.AttributeDeprecated, validation.LevelWarning),
		),
	)

	result, err := pipeline.ProcessEvent(jsonish.Map{})

	assert.NoError(err)
	assert.Len(result.Issues(), 1)
	assert.Equal(issue.ClassUIDMissing, result.Issues()[0].Code)
	assert.Empty(result.Validation().Findings)
}

func TestIssueAndValidationLevelsRejectInvalidConfiguration(t *testing.T) {
	assert := require.New(t)
	loaded := makeValidationTestSchema(assert)

	tests := []struct {
		name    string
		options []PipelineOption
		wantErr string
	}{
		{
			name: "unknown issue code",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithIssueLevel(issue.Code(255), issue.LevelWarning),
			},
			wantErr: "issue policy has unknown issue code 255",
		},
		{
			name: "invalid issue level",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.Level(255)),
			},
			wantErr: "issue policy has invalid level 255 for issue_enrichment_enum_sibling_not_added",
		},
		{
			name: "mandatory issue ignored",
			options: []PipelineOption{
				WithEnumSiblings(enrichment.Add),
				WithIssueLevel(issue.ClassUIDMissing, issue.LevelIgnored),
			},
			wantErr: "issue policy cannot ignore mandatory issue code issue_class_uid_missing",
		},
		{
			name: "unknown validation code",
			options: []PipelineOption{WithValidation(
				WithValidationLevel(validation.Code(255), validation.LevelWarning),
			)},
			wantErr: "validation policy has unknown validation code 255",
		},
		{
			name: "invalid validation level",
			options: []PipelineOption{WithValidation(
				WithValidationLevel(validation.AttributeUnknown, validation.Level(255)),
			)},
			wantErr: "validation policy has invalid level 255 for validation_attribute_unknown",
		},
		{
			name: "all validations ignored",
			options: []PipelineOption{WithValidation(
				WithAllValidationLevels(validation.LevelIgnored),
			)},
			wantErr: "validation is enabled, but all validation codes are ignored",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			pipeline, err := newPipelineForSchema(loaded, test.options...)
			assert.Nil(pipeline)
			assert.EqualError(err, test.wantErr)
		})
	}
}

func hasFindingCode(result ProcessingResult, code validation.Code) bool {
	for _, finding := range result.Validation().Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func validationFindingCodes(findings []eventresult.ValidationFinding) []validation.Code {
	codes := make([]validation.Code, len(findings))
	for index, finding := range findings {
		codes[index] = finding.Code
	}
	return codes
}

func TestProcessingIssueErrorSupportsErrorsAs(t *testing.T) {
	found := eventresult.ProcessingIssue{
		Source:  issue.SourceProcessing,
		Code:    issue.ClassUIDMissing,
		Message: "processing stopped",
		Details: jsonish.Map{"attribute": "class_uid"},
	}
	err := newProcessingIssueError(found)

	var issueErr *ProcessingIssueError
	require.True(t, errors.As(err, &issueErr))
	require.Equal(t, found, issueErr.Issue())
}

func findingForCode(
	assert *require.Assertions,
	result ProcessingResult,
	code validation.Code,
) eventresult.ValidationFinding {
	for _, finding := range result.Validation().Findings {
		if finding.Code == code {
			return finding
		}
	}
	assert.FailNow("validation finding not found", "code: %s", code)
	return eventresult.ValidationFinding{}
}
