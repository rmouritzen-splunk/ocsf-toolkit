package processing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestValidationPolicyExplicitLevelOverridesAllLevel(t *testing.T) {
	policy, err := compileValidationPolicy([]ValidationPolicyRule{
		{Level: validation.LevelIgnored, All: true},
		{
			Level: validation.LevelWarning,
			Code:  validation.AttributeRequiredMissing,
		},
	})
	require.NoError(t, err)
	processor := validationProcessor{policy: policy}
	context := processContext{}

	level, reported := processor.findingLevel(&context, validation.AttributeRequiredMissing)
	require.True(t, reported)
	require.Equal(t, validation.LevelWarning, level)
	_, reported = processor.findingLevel(&context, validation.AttributeDeprecated)
	require.False(t, reported)
}

func TestEngineeringInvariantIgnoredRecommendedMissingSkipsDiagnosticConstruction(t *testing.T) {
	// Engineering invariant test: a default-ignored recommended requirement must not allocate diagnostic state.
	processor := validationProcessor{policy: defaultValidationPolicy()}
	context := processContext{}
	path := eventpath.Path{}
	path.PushAttribute("recommended")
	definition := schema.ItemAttributeDefinition{
		CommonAttributeDefinition: schema.CommonAttributeDefinition{Requirement: "recommended"},
	}

	allocations := testing.AllocsPerRun(1000, func() {
		processor.validateRequirement(&context, &path, "recommended", &definition)
	})

	require.Zero(t, allocations)
	require.Empty(t, context.result.Validation.Findings)
}

func TestEngineeringInvariantIgnoredUnknownAttributesSkipTraversalWork(t *testing.T) {
	// Engineering invariant test: an ignored unknown-attribute check must return before scanning or allocating.
	policy, err := compileValidationPolicy([]ValidationPolicyRule{
		{Level: validation.LevelIgnored, All: true},
		{Level: validation.LevelWarning, Code: validation.AttributeWrongType},
	})
	require.NoError(t, err)
	processor := validationProcessor{policy: policy}
	context := processContext{pipelineImpl: &PipelineImpl{validation: &processor}}
	item := jsonish.Map{"unknown": "value"}
	definition := schema.ItemDefinition{Attributes: map[string]*schema.ItemAttributeDefinition{}}

	allocations := testing.AllocsPerRun(1000, func() {
		context.visitUnknownAttributes(item, &definition)
	})

	require.Zero(t, allocations)
	require.Empty(t, context.result.Validation.Findings)
}

func TestEngineeringInvariantIgnoredEnumArrayDeprecationSkipsDiagnosticConstruction(t *testing.T) {
	// Engineering invariant test: enum-sibling-only validation must not allocate diagnostics for ignored enum checks.
	policy, err := compileValidationPolicy([]ValidationPolicyRule{
		{Level: validation.LevelIgnored, All: true},
		{Level: validation.LevelWarning, Code: validation.AttributeEnumSiblingIncorrect},
	})
	require.NoError(t, err)
	processor := validationProcessor{policy: policy}
	require.True(t, processor.policy.isIgnored(enumValueValidationMask))
	require.False(t, processor.policy.isIgnored(enumSiblingValidationMask))
	context := processContext{}
	path := eventpath.Path{}
	path.PushAttribute("status_ids")
	path.PushArrayIndex(0)
	definition := schema.EnumDefinition{
		Deprecated: &schema.DeprecatedDefinition{Since: "1.0.0", Message: "enum deprecated"},
	}

	allocations := testing.AllocsPerRun(1000, func() {
		processor.validateEnumArrayValueDeprecated(&context, "status_ids", &path, &definition)
	})

	require.Zero(t, allocations)
	require.Empty(t, context.result.Validation.Findings)
}

func TestEngineeringInvariantWrongTypeOnlySkipsPrimitiveConstraints(t *testing.T) {
	// Engineering invariant test: wrong-type-only validation must return after a successful representation check.
	policy, err := compileValidationPolicy([]ValidationPolicyRule{
		{Level: validation.LevelIgnored, All: true},
		{Level: validation.LevelWarning, Code: validation.AttributeWrongType},
	})
	require.NoError(t, err)
	typeValidation := &schema.TypeValidation{
		Definition:    &schema.TypeDefinition{},
		PrimitiveType: "string_t",
		MaxLen:        schema.MaxLenConstraint{TypeName: "short_text_t", MaxLen: 1},
		HasMaxLen:     true,
	}
	processor := validationProcessor{
		cache:  &schema.ValidationCache{Types: map[string]*schema.TypeValidation{"short_text_t": typeValidation}},
		policy: policy,
	}
	context := processContext{}
	path := eventpath.Path{}
	path.PushAttribute("short_text")
	definition := schema.ItemAttributeDefinition{
		CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "short_text_t"},
	}

	t.Run("scalar", func(t *testing.T) {
		allocations := testing.AllocsPerRun(1000, func() {
			processor.validatePrimitiveValue(&context, "too long", &path, "short_text", &definition)
		})

		require.Zero(t, allocations)
		require.Empty(t, context.result.Validation.Findings)
	})

	t.Run("array", func(t *testing.T) {
		values, ok := eventvalue.NewArrayView([]string{"too long"})
		require.True(t, ok)
		allocations := testing.AllocsPerRun(1000, func() {
			processor.validateArrayPrimitiveValue(&context, values, 0, "short_text", &definition, &path)
		})

		require.Zero(t, allocations)
		require.Empty(t, context.result.Validation.Findings)
	})
}
