package coerce

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "string", value: "text", want: "text"},
		{name: "JSON number", value: json.Number("9223372036854775807"), want: "9223372036854775807"},
		{name: "float64", value: float64(1.25), want: "1.25"},
		{name: "float32", value: float32(1.25), want: "1.25"},
		{name: "integer", value: int64(-7), want: "-7"},
		{name: "boolean", value: true, want: "true"},
		{name: "array", value: []any{"value", 2}, want: `["value",2]`},
		{name: "map", value: map[string]any{"key": "value"}, want: `{"key":"value"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := String(test.value)

			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestStringReportsDetailedConversionError(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		wantError string
	}{
		{name: "null", wantError: "cannot convert null to string"},
		{
			name:      "JSON encoding failure",
			value:     complex64(1 + 2i),
			wantError: "cannot convert value of type complex64 to string: json: unsupported type: complex64",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := String(test.value)

			require.Empty(t, value)
			require.EqualError(t, err, test.wantError)
		})
	}
}

func TestStringLenientSuppressesConversionError(t *testing.T) {
	require.Empty(t, StringLenient(nil))
	require.Empty(t, StringLenient(complex64(1+2i)))
}

func TestStringValueSkipsDetailedErrorConstruction(t *testing.T) {
	value, err := stringValue(complex64(1+2i), false)

	require.Empty(t, value)
	require.Same(t, errCannotConvertToString, err)
}

func BenchmarkStringScalar(b *testing.B) {
	for b.Loop() {
		_, _ = String(json.Number("9223372036854775807"))
	}
}

func BenchmarkStringLenientScalar(b *testing.B) {
	for b.Loop() {
		_ = StringLenient(json.Number("9223372036854775807"))
	}
}

func BenchmarkStringLenientMarshalFailure(b *testing.B) {
	for b.Loop() {
		_ = StringLenient(complex64(1 + 2i))
	}
}

func BenchmarkStringMarshalFailure(b *testing.B) {
	for b.Loop() {
		_, _ = String(complex64(1 + 2i))
	}
}
