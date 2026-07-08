package eventschema

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/coerce"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// attributeValue implements OCSF's equivalence between a missing attribute and
// an attribute whose value is null.
func attributeValue(item jsonish.Map, attribute string) (any, bool) {
	value, present := item[attribute]
	return value, present && value != nil
}

func getInt64(value any) (int64, bool, bool) {
	if value == nil {
		return 0, false, false
	}
	i, ok := getInt64Value(value)
	return i, true, ok
}

func getInt64Value(value any) (int64, bool) {
	switch value := value.(type) {
	case json.Number:
		i, err := value.Int64()
		return i, err == nil
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
	default:
		return 0, false
	}
}

func getFloat64(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		if !strings.ContainsAny(value.String(), ".eE") {
			return 0, false
		}
		f, err := value.Float64()
		return f, err == nil
	case float32:
		return float64(value), true
	case float64:
		return value, true
	default:
		return 0, false
	}
}

func asSlice(value any) ([]any, bool) {
	switch value := value.(type) {
	case []any:
		return value, true
	case []jsonish.Map:
		result := make([]any, len(value))
		for i, element := range value {
			result[i] = element
		}
		return result, true
	case []string:
		result := make([]any, len(value))
		for i, element := range value {
			result[i] = element
		}
		return result, true
	default:
		reflected := reflect.ValueOf(value)
		if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
			return nil, false
		}
		result := make([]any, reflected.Len())
		for i := range reflected.Len() {
			result[i] = reflected.Index(i).Interface()
		}
		return result, true
	}
}

func makeAttributePath(parentAttributePath, attribute string) string {
	if parentAttributePath == "" {
		return attribute
	}
	return parentAttributePath + "." + attribute
}

func makeArrayElementPath(attributePath string, index int) string {
	return fmt.Sprintf("%s[%d]", attributePath, index)
}

func parentPath(attributePath string) string {
	lastDot := strings.LastIndex(attributePath, ".")
	lastBracket := strings.LastIndex(attributePath, "[")
	if lastBracket > lastDot {
		lastDot = strings.LastIndex(attributePath[:lastBracket], ".")
	}
	if lastDot < 0 {
		return ""
	}
	return attributePath[:lastDot]
}

func isOtherEnumValue(value any) bool {
	i, ok := getInt64Value(value)
	return ok && i == 99
}

func valuesEqual(left any, right any) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}
	leftNumber, leftOK := exactNumber(left)
	rightNumber, rightOK := exactNumber(right)
	if leftOK && rightOK {
		return leftNumber.Cmp(rightNumber) == 0
	}
	return coerce.StringLenient(left) == coerce.StringLenient(right)
}

func exactNumber(value any) (*big.Rat, bool) {
	var text string
	switch value := value.(type) {
	case json.Number:
		text = value.String()
	case int:
		return new(big.Rat).SetInt64(int64(value)), true
	case int8:
		return new(big.Rat).SetInt64(int64(value)), true
	case int16:
		return new(big.Rat).SetInt64(int64(value)), true
	case int32:
		return new(big.Rat).SetInt64(int64(value)), true
	case int64:
		return new(big.Rat).SetInt64(value), true
	case uint:
		return new(big.Rat).SetUint64(uint64(value)), true
	case uint8:
		return new(big.Rat).SetUint64(uint64(value)), true
	case uint16:
		return new(big.Rat).SetUint64(uint64(value)), true
	case uint32:
		return new(big.Rat).SetUint64(uint64(value)), true
	case uint64:
		return new(big.Rat).SetUint64(value), true
	case float32:
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, false
		}
		text = strconv.FormatFloat(float64(value), 'g', -1, 32)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, false
		}
		text = strconv.FormatFloat(value, 'g', -1, 64)
	default:
		return nil, false
	}

	number, ok := new(big.Rat).SetString(text)
	return number, ok
}

func typeOf(value any) (string, string) {
	switch value := value.(type) {
	case json.Number:
		if _, err := value.Int64(); err == nil {
			return "integer_t", " (integer in range of -2^63 to 2^63 - 1)"
		}
		if _, err := value.Float64(); err == nil {
			return "float_t", ""
		}
		return "big integer", " (outside of integer_t range of -2^63 to 2^63 - 1)"
	case int, int8, int16, int32, int64:
		return "integer_t", " (integer in range of -2^63 to 2^63 - 1)"
	case float32, float64:
		return "float_t", ""
	case bool:
		return "boolean_t", ""
	case string:
		return "string_t", ""
	case []any, []jsonish.Map:
		return "array", ""
	case jsonish.Map:
		return "object", ""
	case nil:
		return "null", ""
	default:
		kind := reflect.TypeOf(value).Kind()
		if kind == reflect.Slice || kind == reflect.Array {
			return "array", ""
		}
		return "unknown type", ""
	}
}
