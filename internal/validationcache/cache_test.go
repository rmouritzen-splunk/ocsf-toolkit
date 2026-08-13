package validationcache

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/stretchr/testify/require"
)

func TestBuildCompilesAndResolvesSchemaConstraints(t *testing.T) {
	regex := `^[a-z]+$`
	maxLen := int64(12)
	compiled := &schema.Compiled{
		Version: "1.2.3",
		Dictionary: &schema.DictionaryDefinition{Types: &schema.TypesDefinition{
			Attributes: map[string]*schema.TypeDefinition{
				"integer_t": {Range: []int64{0, 100}},
				"level_t": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "integer_t"},
					Values:                    []any{json.Number("1"), json.Number("2")},
				},
				"alias_level_t": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "level_t"}},
				"string_t":      {RegEx: &regex, MaxLen: &maxLen},
				"name_t":        {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "string_t"}},
				"alias_t":       {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "name_t"}},
			},
		}},
	}

	cache, err := Build(compiled)
	require.NoError(t, err)
	require.True(t, cache.VersionOK)

	levelValidation := cache.Types["level_t"]
	require.NotNil(t, levelValidation)
	require.Equal(t, "integer_t", levelValidation.PrimitiveType)
	require.True(t, levelValidation.HasValue)
	values := levelValidation.Value
	require.True(t, values.Contains(int64(1)))
	require.False(t, values.Contains(int64(3)))

	aliasLevelValidation := cache.Types["alias_level_t"]
	require.NotNil(t, aliasLevelValidation)
	require.Equal(t, "integer_t", aliasLevelValidation.PrimitiveType)
	require.True(t, aliasLevelValidation.HasValue)
	require.Equal(t, "level_t", aliasLevelValidation.Value.TypeName)
	require.True(t, aliasLevelValidation.Value.ContainsInt64(2))
	require.True(t, aliasLevelValidation.HasRange)
	require.Equal(t, "integer_t", aliasLevelValidation.Range.TypeName)

	require.True(t, levelValidation.HasRange)
	rangeConstraint := levelValidation.Range
	require.Equal(t, "integer_t", rangeConstraint.TypeName)
	require.Equal(t, int64(0), rangeConstraint.Low)
	require.Equal(t, int64(100), rangeConstraint.High)

	nameValidation := cache.Types["name_t"]
	require.NotNil(t, nameValidation)
	require.True(t, nameValidation.HasRegex)
	require.Equal(t, "string_t", nameValidation.Regex.TypeName)
	require.True(t, nameValidation.Regex.Compiled.MatchString("valid"))
	require.True(t, nameValidation.HasMaxLen)
	require.Equal(t, int64(12), nameValidation.MaxLen.MaxLen)

	aliasValidation := cache.Types["alias_t"]
	require.NotNil(t, aliasValidation)
	require.Equal(t, "string_t", aliasValidation.PrimitiveType)
	require.True(t, aliasValidation.HasRegex)
	require.Equal(t, "string_t", aliasValidation.Regex.TypeName)
	require.True(t, aliasValidation.HasMaxLen)
	require.Equal(t, int64(12), aliasValidation.MaxLen.MaxLen)
}

func TestEngineeringInvariantBuildRejectsCyclicTypeDefinitions(t *testing.T) {
	// Engineering invariant test: cache construction must reject cyclic type inheritance
	// regardless of which type is used as the starting point for cycle detection.
	compiled := &schema.Compiled{
		Version: "1.2.3",
		Dictionary: &schema.DictionaryDefinition{Types: &schema.TypesDefinition{
			Attributes: map[string]*schema.TypeDefinition{
				"alias_t":  {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "first_t"}},
				"first_t":  {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "second_t"}},
				"second_t": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "first_t"}},
			},
		}},
	}

	_, err := Build(compiled)
	require.ErrorContains(t, err, "type inheritance for")
	require.ErrorContains(t, err, "contains a cycle at")
}

func TestEngineeringInvariantBuildMemoizesTypeInheritanceResolution(t *testing.T) {
	// Engineering invariant test: validation cache construction must reuse resolved type ancestry so allocation
	// growth remains subquadratic as a dictionary inheritance chain grows.
	allocations := func(typeCount int) float64 {
		compiled := compiledTypeChain(typeCount)
		return testing.AllocsPerRun(3, func() {
			_, err := Build(compiled)
			require.NoError(t, err)
		})
	}

	small := allocations(128)
	large := allocations(256)
	require.Less(t, large, small*2.25,
		"doubling type inheritance depth grew allocations from %.0f to %.0f", small, large)
}

func compiledTypeChain(typeCount int) *schema.Compiled {
	types := make(map[string]*schema.TypeDefinition, typeCount)
	types["integer_t"] = &schema.TypeDefinition{}
	parent := "integer_t"
	for index := 1; index < typeCount; index++ {
		name := "alias_" + strconv.Itoa(index) + "_t"
		types[name] = &schema.TypeDefinition{
			CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: parent},
		}
		parent = name
	}
	return &schema.Compiled{
		Version: "1.2.3",
		Dictionary: &schema.DictionaryDefinition{Types: &schema.TypesDefinition{
			Attributes: types,
		}},
	}
}

func TestBuildReportsTerminalUnknownPrimitiveType(t *testing.T) {
	compiled := &schema.Compiled{
		Version: "1.2.3",
		Dictionary: &schema.DictionaryDefinition{Types: &schema.TypesDefinition{
			Attributes: map[string]*schema.TypeDefinition{
				"alias_t": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "custom_t"},
				},
				"custom_t": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "missing_t"},
				},
			},
		}},
	}

	cache, err := Build(compiled)
	require.NoError(t, err)
	require.Equal(t, "missing_t", cache.Types["alias_t"].PrimitiveType)
	require.Equal(t, "missing_t", cache.Types["custom_t"].PrimitiveType)
}

func TestBuildRejectsInvalidAllowedValues(t *testing.T) {
	compiled := &schema.Compiled{
		Version: "1.2.3",
		Dictionary: &schema.DictionaryDefinition{Types: &schema.TypesDefinition{
			Attributes: map[string]*schema.TypeDefinition{
				"bad_t": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "integer_t"},
					Values:                    []any{"not an integer"},
				},
			},
		}},
	}

	_, err := Build(compiled)
	require.EqualError(t, err, `type "bad_t" allowed value at index 0 is not a signed 64-bit integer`)
}

func TestInvariantValueConstraintsUseTypedEquality(t *testing.T) {
	// Invariant test: allowed-value constraints must compare values using the equality semantics of their resolved
	// primitive type, including exact signed 64-bit normalization and exact string and boolean types.
	tests := []struct {
		name          string
		primitiveType string
		allowed       any
		equivalent    []any
		different     []any
	}{
		{
			name:          "signed 64-bit integer",
			primitiveType: "integer_t",
			allowed:       json.Number("9007199254740993"),
			equivalent: []any{
				json.Number("9007199254740993.0"),
				json.Number("90071992547409930e-1"),
				int64(9007199254740993),
			},
			different: []any{json.Number("9007199254740992"), int64(9007199254740992)},
		},
		{
			name:          "floating point",
			primitiveType: "float_t",
			allowed:       json.Number("1.25"),
			equivalent:    []any{json.Number("125e-2"), float32(1.25), float64(1.25)},
			different:     []any{json.Number("1.2500000000000002"), float64(1.5)},
		},
		{
			name:          "string",
			primitiveType: "string_t",
			allowed:       "1",
			equivalent:    []any{"1"},
			different:     []any{json.Number("1"), int64(1), true},
		},
		{
			name:          "boolean",
			primitiveType: "boolean_t",
			allowed:       true,
			equivalent:    []any{true},
			different:     []any{false, "true", int64(1)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			constraint, err := compileValueConstraint(
				"allowed_t",
				test.primitiveType,
				&schema.TypeDefinition{Values: []any{test.allowed}},
			)
			require.NoError(t, err)
			for _, value := range test.equivalent {
				require.True(t, constraint.Contains(value), "expected %#v to compare equal", value)
			}
			for _, value := range test.different {
				require.False(t, constraint.Contains(value), "expected %#v to compare different", value)
			}
		})
	}
}

func TestCompileValueConstraintIndexesOnlyLargerSets(t *testing.T) {
	compile := func(size int) ValueConstraint {
		values := make([]any, size)
		for index := range values {
			values[index] = json.Number(strconv.Itoa(index))
		}
		constraint, err := compileValueConstraint(
			"indexed_t",
			"integer_t",
			&schema.TypeDefinition{Values: values},
		)
		require.NoError(t, err)
		return constraint
	}

	small := compile(valueIndexThreshold - 1)
	require.Nil(t, small.intValues.index)
	require.True(t, small.ContainsInt64(valueIndexThreshold-2))

	large := compile(valueIndexThreshold)
	require.Len(t, large.intValues.index, valueIndexThreshold)
	require.True(t, large.ContainsInt64(valueIndexThreshold-1))
}

func TestLazyGetReusesBuiltCache(t *testing.T) {
	compiled := &schema.Compiled{
		Version: "1.2.3",
		Dictionary: &schema.DictionaryDefinition{Types: &schema.TypesDefinition{
			Attributes: map[string]*schema.TypeDefinition{},
		}},
	}
	var lazy Lazy

	first, err := lazy.Get(compiled)
	require.NoError(t, err)
	second, err := lazy.Get(compiled)
	require.NoError(t, err)

	require.Same(t, first, second)
}

func BenchmarkValueConstraintContainsInt64(b *testing.B) {
	for _, size := range []int{4, 64} {
		values := make([]any, size)
		for index := range values {
			values[index] = json.Number(strconv.Itoa(index))
		}
		constraint, err := compileValueConstraint(
			"benchmark_t",
			"integer_t",
			&schema.TypeDefinition{Values: values},
		)
		require.NoError(b, err)
		candidate := int64(size - 1)
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			for b.Loop() {
				if !constraint.ContainsInt64(candidate) {
					b.Fatal("expected allowed value")
				}
			}
		})
	}
}
