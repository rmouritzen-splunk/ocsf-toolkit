package processing

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

func TestResultMessagesEscapeUnsafeText(t *testing.T) {
	assert := require.New(t)
	context := processContext{pipelineImpl: &PipelineImpl{issuePolicy: defaultIssuePolicy()}}
	validator := validationProcessor{policy: defaultValidationPolicy()}
	message := "printable café\nnext\t\x1b[31m\u202eraw\xff"
	expected := `printable café\nnext\t\x1b[31m\u202eraw\xff`

	validator.addFinding(&context, validation.AttributeWrongType, nil, message)
	validator.addFinding(&context, validation.AttributeDeprecated, nil, message)
	require.NoError(t, context.addProcessorIssue(
		issue.SourceProcessing, issue.EnrichmentEnumSiblingNotAdded, nil, message,
	))
	require.NoError(t, context.addProcessorIssue(
		issue.SourceProcessing, issue.EnrichmentEnumSiblingNotAdded, nil, "formatted: "+message,
	))

	assert.Equal(expected, findingsAtLevel(context.result.Validation.Findings, validation.LevelError)[0].Message)
	assert.Equal(expected, findingsAtLevel(context.result.Validation.Findings, validation.LevelWarning)[0].Message)
	assert.Equal(expected, context.result.Issues[0].Message)
	assert.Equal("formatted: "+expected, context.result.Issues[1].Message)
}

func TestAddProcessorIssueReturnsFatalIssue(t *testing.T) {
	config := IssuePolicyConfig{LevelRules: []IssueLevelRule{
		{Code: issue.EnrichmentEnumSiblingNotAdded, Level: issue.LevelError},
	}}
	policy, err := compileIssuePolicy(config.LevelRules)
	require.NoError(t, err)
	context := processContext{pipelineImpl: &PipelineImpl{issuePolicy: policy}}

	err = context.addProcessorIssue(
		issue.SourceEnrichment,
		issue.EnrichmentEnumSiblingNotAdded,
		nil,
		"fatal issue",
	)

	var issueErr *processingIssueError
	require.ErrorAs(t, err, &issueErr)
	require.Equal(t, issue.EnrichmentEnumSiblingNotAdded, issueErr.ProcessingIssue().Code)
	require.Empty(t, context.result.Issues)
}
