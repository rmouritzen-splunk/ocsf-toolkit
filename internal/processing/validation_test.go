package processing

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

// TestValidationSkipsTypesDerivedFromJSONType exercises an attribute whose type derives from json_t (rather than
// being literally named "json_t") to confirm validation classifies it as json_t via the same schema-derivation-aware
// PrimitiveType the schema validation cache already computes for every other primitive kind, not via a separate
// check tied to the attribute's own unresolved type name.
func TestValidationSkipsTypesDerivedFromJSONType(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	schema.Dictionary.Types.Attributes["custom_json_t"] = &typeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "json_t"},
	}
	schema.Classes[int64(1)].Attributes["count"].Type = "custom_json_t"

	event := validValidationEvent()
	event["count"] = jsonish.Map{"key": "value"}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewValidation()).ProcessEvent(event)

	assert.NoError(err)
	errors := findingsAtLevel(result.Validation.Findings, validation.LevelError)
	assert.NotContains(issueCodes(errors), "validation_schema_bug_type_missing")
	assert.NotContains(issueCodes(errors), "validation_schema_bug_primitive_type_unknown")
	assert.NotContains(issueCodes(errors), "validation_attribute_wrong_type")
}

func TestFloatOutsideRangeComparesInt64BoundsExactly(t *testing.T) {
	require.False(t, floatOutsideRange(float64(math.MaxInt64-1023), math.MinInt64, math.MaxInt64))
	require.True(t, floatOutsideRange(float64(uint64(1)<<63), math.MinInt64, math.MaxInt64))
	require.False(t, floatOutsideRange(float64(math.MinInt64), math.MinInt64, math.MaxInt64))
	require.True(
		t, floatOutsideRange(math.Nextafter(float64(math.MinInt64), math.Inf(-1)), math.MinInt64, math.MaxInt64),
	)
	require.True(t, floatOutsideRange(9.5, -9, 9))
	require.True(t, floatOutsideRange(-9.5, -9, 9))
}

func TestInvariantUnsupportedHomogeneousArrayElementsAreValidationFindings(t *testing.T) {
	// Invariant test: an unsupported element has the same validation outcome in a homogeneous container as it does
	// in []any; the container representation does not turn an invalid event value into a processing failure.
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["status_ids"] = []uint64{1}

	result, err := mustNewEventProcessorPipeline(assert, schema, NewValidation()).ProcessEvent(event)

	assert.NoError(err)
	errors := findingsAtLevel(result.Validation.Findings, validation.LevelError)
	wrongTypes := issuesWithCode(errors, "validation_attribute_wrong_type")
	assert.Len(wrongTypes, 1)
	assert.Equal("status_ids[0]", wrongTypes[0].Details["attribute_path"])
}

func TestEngineeringInvariantIgnoredEnumSiblingTargetRetainsOrdinaryEnumValidation(t *testing.T) {
	// Engineering invariant test: ignoring an enum-to-enum sibling declaration must not suppress the target enum's
	// ordinary validation.
	siblingName := "code"
	compiled, err := schema.New(&schemaDefinition{
		CompileVersion: 1,
		Version:        "1.0.0",
		Classes: map[string]*classDefinition{
			"test": {
				Uid: 1,
				ItemDefinition: commonItemDefinition{Name: "test", Attributes: map[string]*itemAttributeDefinition{
					"class_uid": {CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
					"code_id": {CommonAttributeDefinition: commonAttributeDefinition{
						Type: "integer_t", Enum: map[string]*enumDefinition{"1": {Caption: "One"}},
						Sibling: &siblingName,
					}},
					"code": {CommonAttributeDefinition: commonAttributeDefinition{
						Type: "string_t", Enum: map[string]*enumDefinition{"ok": {Caption: "Okay"}},
					}},
				}},
			},
		},
		Dictionary: &dictionaryDefinition{Types: &typesDefinition{Attributes: map[string]*typeDefinition{
			"integer_t": {},
			"string_t":  {},
		}}},
	})
	require.NoError(t, err)
	require.Equal(t,
		"issue_at_init_schema_enum_sibling_target_is_enum",
		compiled.InitializationIssues()[0].Code.String(),
	)
	pipeline := mustNewEventProcessorPipeline(require.New(t), compiled, NewValidation())

	result, err := pipeline.ProcessEvent(jsonish.Map{
		"class_uid": json.Number("1"),
		"code_id":   json.Number("1"),
		"code":      "One",
	})

	require.NoError(t, err)
	errors := findingsAtLevel(result.Validation.Findings, validation.LevelError)
	require.Contains(t, issueCodes(errors), "validation_attribute_enum_value_unknown")
}
