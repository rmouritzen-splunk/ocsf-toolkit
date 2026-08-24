package eventvalue

import (
	"encoding/json"
	"reflect"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// DescribeType returns the OCSF-oriented type name and optional explanatory suffix for a value.
func DescribeType(value any) (string, string) {
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
