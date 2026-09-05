package schema

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureTraversalCacheIndexesCanonicalIntegralEnums(t *testing.T) {
	want := &EnumDefinition{Caption: "One"}
	wantTwo := &EnumDefinition{Caption: "Two"}
	wantOther := &EnumDefinition{Caption: "Other"}
	alias := &EnumDefinition{Caption: "Alias"}
	stringEnum := &EnumDefinition{Caption: "String"}
	integerAttribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "integer_t",
		Enum: map[string]*EnumDefinition{
			"1": want, "2": wantTwo, "99": wantOther, "01": alias, "invalid": alias,
		},
	}}
	sparseAttribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "integer_t",
		Enum: map[string]*EnumDefinition{"0": want, "100": wantTwo},
	}}
	mapDefinitions := make(map[string]*EnumDefinition, numericEnumScanThreshold+1)
	for value := range numericEnumScanThreshold + 1 {
		mapDefinitions[strconv.Itoa(value*2)] = want
	}
	mapAttribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "integer_t",
		Enum: mapDefinitions,
	}}
	stringAttribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "string_t",
		Enum: map[string]*EnumDefinition{"1": stringEnum},
	}}
	compiled := &Compiled{Classes: map[int64]*ClassDefinition{
		1: {ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{
			"integer": integerAttribute,
			"map":     mapAttribute,
			"sparse":  sparseAttribute,
			"string":  stringAttribute,
		}}},
	}}
	require.Nil(t, integerAttribute.NumericEnumDefinition(1))

	compiled.EnsureTraversalCache()

	require.Same(t, want, integerAttribute.NumericEnumDefinition(1))
	require.Same(t, wantTwo, integerAttribute.NumericEnumDefinition(2))
	require.Same(t, wantOther, integerAttribute.NumericEnumDefinition(99))
	require.Nil(t, integerAttribute.NumericEnumDefinition(3))
	require.NotNil(t, integerAttribute.numericEnums.dense)
	require.Nil(t, integerAttribute.numericEnums.sparse)
	require.False(t, integerAttribute.NumericEnumKeysCanonical())
	require.Same(t, want, sparseAttribute.NumericEnumDefinition(0))
	require.Same(t, wantTwo, sparseAttribute.NumericEnumDefinition(100))
	require.Nil(t, sparseAttribute.numericEnums.dense)
	require.NotNil(t, sparseAttribute.numericEnums.scan)
	require.Nil(t, sparseAttribute.numericEnums.sparse)
	require.True(t, sparseAttribute.NumericEnumKeysCanonical())
	require.Same(t, want, mapAttribute.NumericEnumDefinition(2))
	require.Nil(t, mapAttribute.numericEnums.dense)
	require.Nil(t, mapAttribute.numericEnums.scan)
	require.NotNil(t, mapAttribute.numericEnums.sparse)
	require.True(t, mapAttribute.NumericEnumKeysCanonical())
	require.Nil(t, stringAttribute.NumericEnumDefinition(1))
}

func TestEnsureTraversalCacheInitializesNumericEnumsConcurrently(t *testing.T) {
	want := &EnumDefinition{Caption: "One"}
	attribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "integer_t",
		Enum: map[string]*EnumDefinition{"1": want},
	}}
	compiled := &Compiled{Classes: map[int64]*ClassDefinition{
		1: {ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{
			"integer": attribute,
		}}},
	}}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			compiled.EnsureTraversalCache()
		})
	}
	workers.Wait()

	require.Same(t, want, attribute.NumericEnumDefinition(1))
}

func TestEnsureTraversalCacheResolvesNestedPrimitiveType(t *testing.T) {
	attribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "outer_t"}}
	compiled := &Compiled{
		Classes: map[int64]*ClassDefinition{
			1: {ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{"value": attribute}}},
		},
		Dictionary: &DictionaryDefinition{Types: &TypesDefinition{Attributes: map[string]*TypeDefinition{
			"outer_t": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "inner_t"}},
			"inner_t": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t"}},
		}}},
	}

	compiled.EnsureTraversalCache()

	require.Equal(t, "integer_t", attribute.PrimitiveType)
}

func BenchmarkNumericEnumDefinition(b *testing.B) {
	definition := &EnumDefinition{Caption: "Value"}
	dense := make([]*EnumDefinition, 16)
	for index := range dense {
		dense[index] = definition
	}
	scan := make([]numericEnumEntry, 16)
	sparse := make(map[int64]*EnumDefinition, 16)
	for index := range scan {
		value := int64(index * 100)
		scan[index] = numericEnumEntry{value: value, definition: definition}
		sparse[value] = definition
	}
	benchmarks := []struct {
		name      string
		attribute *ItemAttributeDefinition
		value     int64
	}{
		{
			name: "dense", value: 15,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				minimum: 0, maximum: 15, dense: dense, other: definition,
			}},
		},
		{
			name: "scan_first", value: 0,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				scan: scan, other: definition,
			}},
		},
		{
			name: "scan_middle", value: 800,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				scan: scan, other: definition,
			}},
		},
		{
			name: "scan_last", value: 1500,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				scan: scan, other: definition,
			}},
		},
		{
			name: "map_first", value: 0,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				sparse: sparse, other: definition,
			}},
		},
		{
			name: "map_middle", value: 800,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				sparse: sparse, other: definition,
			}},
		},
		{
			name: "map_last", value: 1500,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				sparse: sparse, other: definition,
			}},
		},
		{
			name: "other", value: 99,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				minimum: 0, maximum: 15, dense: dense, other: definition,
			}},
		},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			var got *EnumDefinition
			b.ReportAllocs()
			for b.Loop() {
				got = benchmark.attribute.NumericEnumDefinition(benchmark.value)
			}
			if got != definition {
				b.Fatal("numeric enum definition was not found")
			}
		})
	}
}
