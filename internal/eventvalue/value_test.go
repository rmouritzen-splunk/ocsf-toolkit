package eventvalue

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/stretchr/testify/require"
)

func TestAttributeTreatsNullAsAbsent(t *testing.T) {
	item := jsonish.Map{"present": "value", "null": nil}

	value, present := Attribute(item, "present")
	require.True(t, present)
	require.Equal(t, "value", value)

	_, present = Attribute(item, "null")
	require.False(t, present)
	_, present = Attribute(item, "missing")
	require.False(t, present)
}

func TestHasPathOrKeySupportsLiteralAndNestedKeys(t *testing.T) {
	item := jsonish.Map{
		"literal.value": "present",
		"nested":        jsonish.Map{"value": true, "null": nil},
	}

	require.True(t, HasPathOrKey(item, "literal.value"))
	require.True(t, HasPathOrKey(item, "nested.value"))
	require.False(t, HasPathOrKey(item, "nested.null"))
	require.False(t, HasPathOrKey(item, "nested.missing"))
	require.False(t, HasPathOrKey(item, "literal.value.child"))
}

func TestAsBooleanAndStringRequireCompatibleOCSFValues(t *testing.T) {
	boolean, ok := AsBoolean(true)
	require.True(t, ok)
	require.True(t, boolean)
	_, ok = AsBoolean("true")
	require.False(t, ok)

	text, ok := AsString("value")
	require.True(t, ok)
	require.Equal(t, "value", text)
	_, ok = AsString(json.Number("1"))
	require.False(t, ok)
}

func TestAsIntegerAndLongAcceptOnlyExactSignedIntegralNumericRepresentations(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int64
		ok    bool
	}{
		{name: "JSON integer", value: json.Number("-7"), want: -7, ok: true},
		{name: "JSON maximum integer", value: json.Number("9223372036854775807"), want: math.MaxInt64, ok: true},
		{name: "JSON minimum integer", value: json.Number("-9223372036854775808"), want: math.MinInt64, ok: true},
		{name: "Go integer", value: int32(42), want: 42, ok: true},
		{name: "JSON integral decimal", value: json.Number("1.0"), want: 1, ok: true},
		{name: "JSON positive exponent", value: json.Number("1e3"), want: 1000, ok: true},
		{name: "JSON negative exponent with integral value", value: json.Number("1000e-3"), want: 1, ok: true},
		{name: "JSON negative exponent with fractional value", value: json.Number("15e-1")},
		{name: "JSON negative value with exponent", value: json.Number("-1e3"), want: -1000, ok: true},
		{
			name:  "JSON exponent within upper bound",
			value: json.Number("9.223372036854775e18"),
			want:  9223372036854775000,
			ok:    true,
		},
		{name: "JSON exponent at exclusive upper bound", value: json.Number("9.223372036854776e18")},
		{name: "JSON exponent below lower bound", value: json.Number("-9.223372036854776e18")},
		{name: "JSON fractional decimal beyond float precision", value: json.Number("1.00000000000000000001")},
		{
			name:  "JSON long integral decimal",
			value: json.Number("1.000000000000000000000000000000000"),
			want:  1,
			ok:    true,
		},
		{
			name:  "JSON minimum with exponent",
			value: json.Number("-9223372036854775808e0"),
			want:  math.MinInt64,
			ok:    true,
		},
		{name: "float64 integral", value: float64(-7), want: -7, ok: true},
		{name: "float32 integral", value: float32(42), want: 42, ok: true},
		{name: "negative zero", value: math.Copysign(0, -1), want: 0, ok: true},
		{name: "JSON fraction", value: json.Number("1.5")},
		{
			name:  "JSON maximum integral decimal",
			value: json.Number("9223372036854775807.0"),
			want:  math.MaxInt64,
			ok:    true,
		},
		{name: "JSON decimal below minimum", value: json.Number("-9223372036854775809.0")},
		{name: "JSON fractional exponent below float range", value: json.Number("1e-4000")},
		{name: "JSON float overflow", value: json.Number("1e309")},
		{name: "JSON malformed", value: json.Number("not-a-number")},
		{name: "JSON integer below minimum", value: json.Number("-9223372036854775809")},
		{name: "JSON NaN", value: json.Number("NaN")},
		{name: "JSON positive infinity", value: json.Number("+Inf")},
		{name: "JSON negative infinity", value: json.Number("-Inf")},
		{name: "float fraction", value: 1.5},
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "JSON above maximum", value: json.Number("9223372036854775808")},
		{name: "float at exclusive upper bound", value: float64(1 << 63)},
		{name: "unsigned integer", value: uint64(1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, ocsfType := range []struct {
				name    string
				convert func(any) (int64, bool)
			}{
				{name: "integer_t", convert: AsInteger},
				{name: "long_t", convert: AsLong},
			} {
				t.Run(ocsfType.name, func(t *testing.T) {
					got, ok := ocsfType.convert(test.value)
					require.Equal(t, test.ok, ok)
					if ok {
						require.Equal(t, test.want, got)
					}
				})
			}
		})
	}
}

func TestIntegralFloat64Boundaries(t *testing.T) {
	upperBound := float64(1 << 63)
	lowerBound := -upperBound
	largestInRange := math.Nextafter(upperBound, 0)
	belowLowerBound := math.Nextafter(lowerBound, math.Inf(-1))

	tests := []struct {
		name  string
		value float64
		want  int64
		ok    bool
	}{
		{name: "positive zero", value: 0, want: 0, ok: true},
		{name: "negative zero", value: math.Copysign(0, -1), want: 0, ok: true},
		{name: "positive integral", value: 42, want: 42, ok: true},
		{name: "negative integral", value: -42, want: -42, ok: true},
		{name: "positive fraction", value: 42.5},
		{name: "negative fraction", value: -42.5},
		{name: "smallest positive subnormal", value: math.SmallestNonzeroFloat64},
		{name: "largest in range", value: largestInRange, want: int64(largestInRange), ok: true},
		{name: "lower bound", value: lowerBound, want: math.MinInt64, ok: true},
		{name: "exclusive upper bound", value: upperBound},
		{name: "below lower bound", value: belowLowerBound},
		{name: "maximum float", value: math.MaxFloat64},
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := integralFloat64(test.value)
			require.Equal(t, test.ok, ok)
			if ok {
				require.Equal(t, test.want, got)
			}
		})
	}
}

func TestAsFloatAcceptsAllParseableJSONNumbers(t *testing.T) {
	tests := []struct {
		name  string
		value json.Number
		want  float64
		ok    bool
	}{
		{name: "integer", value: json.Number("1"), want: 1, ok: true},
		{name: "decimal", value: json.Number("1.25"), want: 1.25, ok: true},
		{name: "exponent", value: json.Number("1e2"), want: 100, ok: true},
		{name: "NaN", value: json.Number("NaN"), want: math.NaN(), ok: true},
		{name: "positive infinity", value: json.Number("+Inf"), want: math.Inf(1), ok: true},
		{name: "negative infinity", value: json.Number("-Inf"), want: math.Inf(-1), ok: true},
		{name: "malformed", value: json.Number("not-a-number")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := AsFloat(test.value)
			require.Equal(t, test.ok, ok)
			if math.IsNaN(test.want) {
				require.True(t, math.IsNaN(got))
			} else if ok {
				require.Equal(t, test.want, got)
			}
		})
	}
}

func TestAsFloatAcceptsSignedIntegerRepresentations(t *testing.T) {
	for _, value := range []any{int(-1), int8(2), int16(3), int32(4), int64(5)} {
		got, ok := AsFloat(value)

		require.True(t, ok)
		require.Equal(t, float64(reflect.ValueOf(value).Int()), got)
	}

	_, ok := AsFloat(uint64(1))
	require.False(t, ok)
}

func TestInvariantScalarConversionsPreserveEquivalentValues(t *testing.T) {
	// Invariant test: supported representations of the same scalar value must normalize equally without making
	// adjacent signed 64-bit integers equal through floating-point rounding.
	t.Run("signed 64-bit integer", func(t *testing.T) {
		const want = int64(9007199254740993)
		for _, value := range []any{
			json.Number("9007199254740993"),
			json.Number("9007199254740993.0"),
			json.Number("90071992547409930e-1"),
			want,
		} {
			got, ok := AsInteger(value)
			require.True(t, ok)
			require.Equal(t, want, got)
		}

		adjacent, ok := AsInteger(json.Number("9007199254740992"))
		require.True(t, ok)
		require.NotEqual(t, want, adjacent)
	})

	t.Run("floating point", func(t *testing.T) {
		for _, value := range []any{
			json.Number("1.25"),
			json.Number("125e-2"),
			float32(1.25),
			float64(1.25),
		} {
			got, ok := AsFloat(value)
			require.True(t, ok)
			require.Equal(t, 1.25, got)
		}
	})

	t.Run("string", func(t *testing.T) {
		for _, value := range []any{
			"9007199254740993",
			json.Number("9007199254740993"),
			int64(9007199254740993),
		} {
			got, ok := FormatScalar(value)
			require.True(t, ok)
			require.Equal(t, "9007199254740993", got)
		}
	})
}

func TestArrayViewSupportsSlicesAndArrays(t *testing.T) {
	t.Run("slice fast path", func(t *testing.T) {
		slice, ok := NewArrayView([]int64{1, 2})
		require.True(t, ok)
		require.Equal(t, 2, slice.Len())
		require.Equal(t, int64(2), slice.At(1))
	})

	t.Run("array reflection fallback", func(t *testing.T) {
		array, ok := NewArrayView([2]string{"one", "two"})
		require.True(t, ok)
		require.Equal(t, "one", array.At(0))
	})

	_, ok := NewArrayView("not an array")
	require.False(t, ok)
}

func TestArrayLenSupportsSlicesAndArrays(t *testing.T) {
	length, ok := ArrayLen([]int64{1, 2})
	require.True(t, ok)
	require.Equal(t, 2, length)

	length, ok = ArrayLen([1]string{"one"})
	require.True(t, ok)
	require.Equal(t, 1, length)

	_, ok = ArrayLen("not an array")
	require.False(t, ok)
}

func TestArrayViewAcceptsDefinedContainerTypes(t *testing.T) {
	type integerList []int64
	type integerArray [2]int64

	for _, value := range []any{integerList{1}, integerArray{1}} {
		view, ok := NewArrayView(value)
		require.True(t, ok)
		integer, valid := view.AsIntegerAt(0)
		require.True(t, valid)
		require.Equal(t, int64(1), integer)
	}
}

func TestArrayViewAcceptsContainerTypeAliases(t *testing.T) {
	type integerList = []int64

	view, ok := NewArrayView(integerList{1})
	require.True(t, ok)
	require.Equal(t, int64(1), view.At(0))
}

func TestArrayViewAsFloatPreservesValueSemantics(t *testing.T) {
	numbers, ok := NewArrayView([1]json.Number{"1.25"})
	require.True(t, ok)
	value, valid := numbers.AsFloatAt(0)
	require.True(t, valid)
	require.Equal(t, 1.25, value)
}

func TestEngineeringInvariantArrayAccessorsIgnoreContainerRepresentation(t *testing.T) {
	// Engineering invariant test: array accessors depend on the element's logical value, not whether its container
	// is a dynamic slice, a homogeneous slice, or a fixed-length array.
	for _, value := range []any{[]any{"value"}, []string{"value"}, [1]string{"value"}} {
		view, ok := NewArrayView(value)
		require.True(t, ok)
		actual, valid := view.AsStringAt(0)
		require.True(t, valid)
		require.Equal(t, "value", actual)
	}

	for _, value := range []any{[]any{true}, []bool{true}, [1]bool{true}} {
		view, ok := NewArrayView(value)
		require.True(t, ok)
		actual, valid := view.AsBooleanAt(0)
		require.True(t, valid)
		require.True(t, actual)
	}

	for _, value := range []any{[]any{json.Number("2.0")}, []json.Number{"2.0"}, [1]json.Number{"2.0"}} {
		view, ok := NewArrayView(value)
		require.True(t, ok)
		actual, valid := view.AsIntegerAt(0)
		require.True(t, valid)
		require.Equal(t, int64(2), actual)
	}

	for _, value := range []any{[]any{float32(1.25)}, []float32{1.25}, [1]float32{1.25}} {
		view, ok := NewArrayView(value)
		require.True(t, ok)
		actual, valid := view.AsFloatAt(0)
		require.True(t, valid)
		require.Equal(t, 1.25, actual)
	}

}

func TestArrayViewTypedAccessorsRejectDefinedElementTypes(t *testing.T) {
	type text string
	type boolean bool
	type integer int64
	type floatingPoint float64

	strings, ok := NewArrayView([]text{"value"})
	require.True(t, ok)
	_, valid := strings.AsStringAt(0)
	require.False(t, valid)

	booleans, ok := NewArrayView([]boolean{true})
	require.True(t, ok)
	_, valid = booleans.AsBooleanAt(0)
	require.False(t, valid)

	integers, ok := NewArrayView([]integer{1})
	require.True(t, ok)
	_, valid = integers.AsIntegerAt(0)
	require.False(t, valid)

	floats, ok := NewArrayView([]floatingPoint{1})
	require.True(t, ok)
	_, valid = floats.AsFloatAt(0)
	require.False(t, valid)
}

func TestArrayViewAllowsDynamicElementsForVisitorInspection(t *testing.T) {
	view, ok := NewArrayView([]any{uint64(1)})
	require.True(t, ok)
	require.Equal(t, uint64(1), view.At(0))
}

func TestArrayViewNumericAccessAcceptsCompatibleRepresentations(t *testing.T) {
	numbers, ok := NewArrayView([]json.Number{"1", "2.0", "NaN"})
	require.True(t, ok)

	integer, valid := numbers.AsIntegerAt(1)
	require.True(t, valid)
	require.Equal(t, int64(2), integer)
	floatingPoint, valid := numbers.AsFloatAt(0)
	require.True(t, valid)
	require.Equal(t, float64(1), floatingPoint)
	floatingPoint, valid = numbers.AsFloatAt(2)
	require.True(t, valid)
	require.True(t, math.IsNaN(floatingPoint))

	typedFloats, ok := NewArrayView([]float64{3, 3.5})
	require.True(t, ok)
	integer, valid = typedFloats.AsIntegerAt(0)
	require.True(t, valid)
	require.Equal(t, int64(3), integer)
	_, valid = typedFloats.AsIntegerAt(1)
	require.False(t, valid)

	typedIntegers, ok := NewArrayView([]int32{-2, 7})
	require.True(t, ok)
	floatingPoint, valid = typedIntegers.AsFloatAt(1)
	require.True(t, valid)
	require.Equal(t, float64(7), floatingPoint)
}

func TestDescribeType(t *testing.T) {
	name, suffix := DescribeType(json.Number("9223372036854775808"))
	require.Equal(t, "float_t", name)
	require.Empty(t, suffix)

	name, suffix = DescribeType(json.Number("1e10000"))
	require.Equal(t, "big integer", name)
	require.Contains(t, suffix, "outside")

	name, suffix = DescribeType(math.MaxInt64)
	require.Equal(t, "integer_t", name)
	require.Contains(t, suffix, "range")
}

func TestFormatScalar(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "string", value: "text", want: "text"},
		{name: "JSON number", value: json.Number("9223372036854775807"), want: "9223372036854775807"},
		{name: "float64", value: float64(1.25), want: "1.25"},
		{name: "float32", value: float32(1.25), want: "1.25"},
		{name: "float32 precision", value: float32(1.2345678), want: "1.2345678"},
		{name: "NaN", value: math.NaN(), want: "NaN"},
		{name: "positive infinity", value: math.Inf(1), want: "+Inf"},
		{name: "negative infinity", value: math.Inf(-1), want: "-Inf"},
		{name: "integer", value: int64(-7), want: "-7"},
		{name: "boolean", value: true, want: "true"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := FormatScalar(test.value)

			require.True(t, ok)
			require.Equal(t, test.want, got)
		})
	}
}

func TestFormatScalarRejectsUnsupportedValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "null"},
		{name: "unsupported scalar type", value: complex64(1 + 2i)},
		{name: "array", value: []any{"value"}},
		{name: "map", value: jsonish.Map{"key": "value"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, ok := FormatScalar(test.value)

			require.Empty(t, value)
			require.False(t, ok)
		})
	}
}

func TestFormatScalarUnsupportedValueDoesNotAllocate(t *testing.T) {
	var value string
	var ok bool
	allocations := testing.AllocsPerRun(1000, func() {
		value, ok = FormatScalar(complex64(1 + 2i))
	})

	require.Empty(t, value)
	require.False(t, ok)
	require.Zero(t, allocations)
}

func BenchmarkFormatScalar(b *testing.B) {
	for b.Loop() {
		_, _ = FormatScalar(json.Number("9223372036854775807"))
	}
}

func BenchmarkAsIntegerNumericRepresentations(b *testing.B) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "JSON integer", value: json.Number("123")},
		{name: "JSON integral decimal", value: json.Number("123.0")},
		{name: "native float", value: float64(123)},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_, _ = AsInteger(test.value)
			}
		})
	}
}

func BenchmarkFormatScalarUnsupportedType(b *testing.B) {
	for b.Loop() {
		_, _ = FormatScalar(complex64(1 + 2i))
	}
}

func BenchmarkArrayViewCommonSlices(b *testing.B) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "dynamic", value: []any{"one", "two", "three"}},
		{name: "typed scalar", value: []int64{1, 2, 3}},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				view, _ := NewArrayView(test.value)
				_ = view.Len()
			}
		})
	}
}

func BenchmarkArrayLenCommonSlices(b *testing.B) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "dynamic", value: []any{"one", "two", "three"}},
		{name: "typed scalar", value: []int64{1, 2, 3}},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = ArrayLen(test.value)
			}
		})
	}
}

func BenchmarkArrayViewTraversal(b *testing.B) {
	b.Run("dynamic", func(b *testing.B) {
		value := []any{"one", "two", "three", "four", "five", "six", "seven", "eight"}
		b.ReportAllocs()
		for b.Loop() {
			view, _ := NewArrayView(value)
			for index := 0; index < view.Len(); index++ {
				_ = view.At(index)
			}
		}
	})

	b.Run("typed scalar conversion", func(b *testing.B) {
		value := []int64{1, 2, 3, 4, 5, 6, 7, 8}
		b.ReportAllocs()
		for b.Loop() {
			view, _ := NewArrayView(value)
			for index := 0; index < view.Len(); index++ {
				_, _ = view.AsIntegerAt(index)
			}
		}
	})
}

func BenchmarkArrayViewTypedScalarAccessors(b *testing.B) {
	strings, _ := NewArrayView([]string{"value"})
	booleans, _ := NewArrayView([]bool{true})
	integers, _ := NewArrayView([]int64{42})
	floats, _ := NewArrayView([]float64{1.25})

	b.Run("AsStringAt", func(b *testing.B) {
		for b.Loop() {
			_, _ = strings.AsStringAt(0)
		}
	})
	b.Run("AsBooleanAt", func(b *testing.B) {
		for b.Loop() {
			_, _ = booleans.AsBooleanAt(0)
		}
	})
	b.Run("AsIntegerAt", func(b *testing.B) {
		for b.Loop() {
			_, _ = integers.AsIntegerAt(0)
		}
	})
	b.Run("AsLongAt", func(b *testing.B) {
		for b.Loop() {
			_, _ = integers.AsLongAt(0)
		}
	})
	b.Run("AsFloatAt", func(b *testing.B) {
		for b.Loop() {
			_, _ = floats.AsFloatAt(0)
		}
	})
}
