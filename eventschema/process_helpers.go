package eventschema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

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

type arrayView struct {
	value     any
	reflected reflect.Value
}

func newArrayView(value any) (arrayView, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return arrayView{}, false
	}
	return arrayView{value: value, reflected: reflected}, true
}

func (a arrayView) Len() int {
	if !a.reflected.IsValid() {
		return 0
	}
	return a.reflected.Len()
}

func (a arrayView) At(index int) any {
	switch values := a.value.(type) {
	case []any:
		return values[index]
	case []jsonish.Map:
		return values[index]
	case []string:
		return values[index]
	case []bool:
		return values[index]
	case []int:
		return values[index]
	case []int8:
		return values[index]
	case []int16:
		return values[index]
	case []int32:
		return values[index]
	case []int64:
		return values[index]
	case []uint:
		return values[index]
	case []uint8:
		return values[index]
	case []uint16:
		return values[index]
	case []uint32:
		return values[index]
	case []uint64:
		return values[index]
	case []float32:
		return values[index]
	case []float64:
		return values[index]
	case []json.Number:
		return values[index]
	default:
		return a.reflected.Index(index).Interface()
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
