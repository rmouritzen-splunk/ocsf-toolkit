package eventschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersionMatchesDocumentedPattern(t *testing.T) {
	versions := []string{
		"0.0.0",
		"1.8.0",
		"1.8.0-rc.1",
		"1.8.0-release-with-dashes",
		"1.8.0-prerelease+metadata",
		"",
		"v1.8.0",
		"1",
		"1.8",
		"1.8.0.1",
		"01.8.0",
		"1.08.0",
		"1.8.00",
		"1.8.0-",
		"1.8.0\nprerelease",
	}
	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			_, ok := parseVersion(version)
			assert.Equal(t, versionPattern.MatchString(version), ok)
		})
	}
}

func TestValidationReportsCachedInvalidTypeRegex(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	invalidRegex := "["
	schema.dictionary.Types.Attributes["upper_code_t"].RegEx = &invalidRegex
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
	event := validValidationEvent()
	event["code"] = "VALUE"

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	issues := issuesWithCode(result.Validation.Errors, "schema_bug_type_regex_invalid")
	assert.Len(issues, 1)
	assert.Equal("code", issues[0].AttributePath)
	assert.Equal("upper_code_t", issues[0].Details["type"])
	assert.Equal(invalidRegex, issues[0].Details["regex"])
	assert.NotEmpty(issues[0].Details["regex_error_message"])
	assert.Contains(issues[0].Details, "regex_error_position")
}

func TestValidationUsesCachedSuperTypeRegex(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	regex := "^[A-Z]+$"
	schema.dictionary.Types.Attributes["string_t"].RegEx = &regex
	schema.dictionary.Types.Attributes["child_code_t"] = &typeDefinition{
		commonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
	}
	schema.classes[1].Attributes["code"].Type = "child_code_t"
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
	event := validValidationEvent()
	event["code"] = "lowercase"

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	issues := issuesWithCode(result.Validation.Warnings, "attribute_value_super_type_regex_not_matched")
	assert.Len(issues, 1)
	assert.Equal("string_t", issues[0].Details["super_type"])
}
