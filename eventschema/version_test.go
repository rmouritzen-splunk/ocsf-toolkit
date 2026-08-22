package eventschema

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestValidationReportsCachedInvalidTypeRegex(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	invalidRegex := "["
	schema.compiledForTest().Dictionary.Types.Attributes["upper_code_t"].RegEx = &invalidRegex
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["code"] = "VALUE"

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	findings := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	issues := issuesWithCode(findings, "validation_schema_bug_type_regex_invalid")
	assert.Len(issues, 1)
	assert.Equal("code", issues[0].Details["attribute_path"])
	assert.Equal("upper_code_t", issues[0].Details["type"])
	assert.Equal(invalidRegex, issues[0].Details["regex"])
	assert.NotEmpty(issues[0].Details["regex_error_message"])
}

func TestValidationUsesCachedSuperTypeRegex(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	regex := "^[A-Z]+$"
	schema.compiledForTest().Dictionary.Types.Attributes["string_t"].RegEx = &regex
	schema.compiledForTest().Dictionary.Types.Attributes["child_code_t"] = &typeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
	}
	schema.compiledForTest().Classes[1].Attributes["code"].Type = "child_code_t"
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["code"] = "lowercase"

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	findings := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
	issues := issuesWithCode(findings, "validation_attribute_value_super_type_regex_not_matched")
	assert.Len(issues, 1)
	assert.Equal("string_t", issues[0].Details["super_type"])
}
