package eventvalue

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// The AsT functions interpret event values according to the corresponding OCSF primitive type T. They return the
// toolkit's canonical Go representation and report whether the source value is compatible with that OCSF type.

// AsBoolean interprets value as an OCSF boolean_t value.
func AsBoolean(value any) (bool, bool) {
	boolean, ok := value.(bool)
	return boolean, ok
}

// AsString interprets value as an OCSF string_t value.
func AsString(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}

// AsInteger interprets value as an OCSF integer_t value using the toolkit's signed 64-bit representation.
func AsInteger(value any) (int64, bool) {
	return asSignedInteger(value)
}

// AsLong interprets value as an OCSF long_t value using the toolkit's signed 64-bit representation. It intentionally
// shares its implementation with AsInteger because the toolkit currently gives both OCSF types the same semantics.
func AsLong(value any) (int64, bool) {
	return asSignedInteger(value)
}

// AsFloat interprets value as an OCSF float_t value using the toolkit's float64 representation.
func AsFloat(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		floatingPoint, err := value.Float64()
		return floatingPoint, err == nil
	case float32:
		return float64(value), true
	case float64:
		return value, true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	default:
		return 0, false
	}
}

// FormatScalar converts common JSON-like scalar values to strings. The boolean reports whether the conversion is
// supported; arrays, maps, null, and other structured or unsupported values return false.
func FormatScalar(value any) (string, bool) {
	switch value := value.(type) {
	case string:
		return value, true
	case json.Number:
		return value.String(), true
	case float64:
		return formatFloat(value, 64), true
	case float32:
		return formatFloat(float64(value), 32), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case int32:
		return strconv.FormatInt(int64(value), 10), true
	case int16:
		return strconv.FormatInt(int64(value), 10), true
	case int8:
		return strconv.FormatInt(int64(value), 10), true
	case int:
		return strconv.FormatInt(int64(value), 10), true
	case bool:
		return strconv.FormatBool(value), true
	default:
		return "", false
	}
}

func formatFloat(f float64, bits int) string {
	// Limits for use of 'e' taken from encoding/json encode.go func (bits floatEncoder) encode(...)
	abs := math.Abs(f)
	format := byte('f')
	// Note: Must use float32 comparisons for underlying float32 value to get precise cutoffs right.
	if abs != 0 {
		if bits == 64 && (abs < 1e-6 || abs >= 1e21) || bits == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			format = 'e'
		}
	}
	return strconv.FormatFloat(f, format, -1, bits)
}

func asSignedInteger(value any) (int64, bool) {
	switch value := value.(type) {
	case json.Number:
		return jsonNumberInt64(value)
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case float32:
		return integralFloat64(float64(value))
	case float64:
		return integralFloat64(value)
	default:
		return 0, false
	}
}

func jsonNumberInt64(value json.Number) (int64, bool) {
	if integer, err := value.Int64(); err == nil {
		return integer, true
	}
	// An integer spelling that failed Int64 is out of range. Decimal and exponent spellings need exact
	// normalization so float64 rounding cannot turn a fraction or out-of-range value into a valid integer.
	if !strings.ContainsAny(value.String(), ".eE") {
		return 0, false
	}
	// Validate the json.Number spelling before using big.Rat, whose accepted syntax is broader than JSON numbers.
	if _, err := value.Float64(); err != nil {
		return 0, false
	}
	var rational big.Rat
	if _, ok := rational.SetString(value.String()); !ok || !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, false
	}
	return rational.Num().Int64(), true
}

func integralFloat64(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < -1<<63 || value >= 1<<63 {
		return 0, false
	}
	return int64(value), true
}

// IsOtherEnumValue reports whether value is the OCSF source-specific enum identifier 99.
func IsOtherEnumValue(value any) bool {
	integer, ok := AsInteger(value)
	return ok && integer == 99
}
