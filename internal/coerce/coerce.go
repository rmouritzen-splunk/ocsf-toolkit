package coerce

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

var errCannotConvertNullToString = errors.New("cannot convert null to string")

// String converts common JSON-like scalar values to strings.
//
// Arrays, maps, and other structured values are converted with JSON encoding when possible.
func String(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case json.Number:
		return value.String(), nil
	case float64:
		return floatToString(value, 64), nil
	case float32:
		return floatToString(float64(value), 32), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case int32:
		return strconv.FormatInt(int64(value), 10), nil
	case int16:
		return strconv.FormatInt(int64(value), 10), nil
	case int8:
		return strconv.FormatInt(int64(value), 10), nil
	case int:
		return strconv.FormatInt(int64(value), 10), nil
	case bool:
		return strconv.FormatBool(value), nil
	case nil:
		return "", errCannotConvertNullToString
	default:
		// return "", fmt.Errorf("cannot convert value of type %T to string", value)
		// This handles arrays and maps. There is no universal string representation, so we'll use JSON.
		jsonString, err := json.Marshal(value)
		if err == nil {
			return string(jsonString), nil
		}
		// JSON marshalling failed, so we'll fall back to Go's string representation.
		return fmt.Sprintf("%v", value), nil
	}
}

// StringLenient converts value to a string and returns an empty string when conversion fails.
func StringLenient(value any) string {
	if s, err := String(value); err != nil {
		return ""
	} else {
		return s
	}
}

func floatToString(f float64, bits int) string {
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
