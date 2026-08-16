package processing

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

func TestResultMessagesEscapeUnsafeText(t *testing.T) {
	assert := require.New(t)
	context := processContext{}
	validator := validationProcessor{}
	message := "printable café\nnext\t\x1b[31m\u202eraw\xff"
	expected := `printable café\nnext\t\x1b[31m\u202eraw\xff`

	validator.addFinding(&context, validation.AttributeWrongType, nil, message)
	validator.addFinding(&context, validation.AttributeDeprecated, nil, message)
	context.addProcessorIssue(issue.SourceProcessing, issue.EnrichmentEnumSiblingNotAdded, nil, message)
	context.addProcessorIssue(issue.SourceProcessing, issue.EnrichmentEnumSiblingNotAdded, nil, "formatted: "+message)

	assert.Equal(expected, findingsAtLevel(context.result.Validation.Findings, validation.LevelError)[0].Message)
	assert.Equal(expected, findingsAtLevel(context.result.Validation.Findings, validation.LevelWarning)[0].Message)
	assert.Equal(expected, context.result.Issues[0].Message)
	assert.Equal("formatted: "+expected, context.result.Issues[1].Message)
}
