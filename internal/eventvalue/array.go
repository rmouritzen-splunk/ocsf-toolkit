package eventvalue

import (
	"encoding/json"
	"reflect"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// ArrayView provides allocation-free indexed access to supported slices and arrays. Common []any and homogeneous
// slices use non-reflective accessors; fixed arrays and defined container types use reflection. At may box a
// homogeneous scalar element, so prefer an AsTAt accessor when the schema expects OCSF type T. The zero value is an
// empty view; callers must check Len before accessing an element because only Len accepts an invalid reflect.Value.
type ArrayView struct {
	value     any
	reflected reflect.Value
	length    int
}

// ArrayLen returns the length of a slice or array without retaining an indexed view.
func ArrayLen(value any) (int, bool) {
	if values, ok := value.([]any); ok {
		return len(values), true
	}
	return arrayLenSlow(value)
}

func arrayLenSlow(value any) (int, bool) {
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return 0, false
	}
	return reflected.Len(), true
}

// NewArrayView returns an indexed view when value is a slice or array.
func NewArrayView(value any) (ArrayView, bool) {
	if values, ok := value.([]any); ok {
		return ArrayView{value: value, reflected: reflect.ValueOf(value), length: len(values)}, true
	}
	return newArrayViewSlow(value)
}

func newArrayViewSlow(value any) (ArrayView, bool) {
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return ArrayView{}, false
	}
	return ArrayView{value: value, reflected: reflected, length: reflected.Len()}, true
}

// Len returns the number of elements in the viewed array.
func (a ArrayView) Len() int {
	return a.length
}

// At returns the original array element as any. Use it when the caller needs the element's dynamic value, such as
// for object or json_t processing; prefer a type-specific accessor when the schema determines the expected scalar
// type. At assumes the view and index are valid.
func (a ArrayView) At(index int) any {
	if values, ok := a.value.([]any); ok {
		return values[index]
	}
	return a.atSlow(index)
}

func (a ArrayView) atSlow(index int) any {
	switch values := a.value.(type) {
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
	case []float32:
		return values[index]
	case []float64:
		return values[index]
	case []json.Number:
		return values[index]
	default:
		// Reflection is necessary for defined container types and arrays because type switches cannot match a defined
		// type as its underlying type or every possible array length with one case.
		return a.reflected.Index(index).Interface()
	}
}

// AsStringAt interprets an element as an OCSF string_t value without boxing it.
func (a ArrayView) AsStringAt(index int) (string, bool) {
	switch values := a.value.(type) {
	case []string:
		return values[index], true
	case []any:
		value, valid := values[index].(string)
		return value, valid
	default:
		return a.stringAtReflected(index)
	}
}

func (a ArrayView) stringAtReflected(index int) (string, bool) {
	value := a.reflected.Index(index)
	if value.Type() != reflect.TypeFor[string]() {
		return "", false
	}
	return value.String(), true
}

// AsBooleanAt interprets an element as an OCSF boolean_t value without boxing it.
func (a ArrayView) AsBooleanAt(index int) (bool, bool) {
	switch values := a.value.(type) {
	case []bool:
		return values[index], true
	case []any:
		value, valid := values[index].(bool)
		return value, valid
	default:
		return a.boolAtReflected(index)
	}
}

func (a ArrayView) boolAtReflected(index int) (bool, bool) {
	value := a.reflected.Index(index)
	if value.Type() != reflect.TypeFor[bool]() {
		return false, false
	}
	return value.Bool(), true
}

// AsIntegerAt interprets an element as an OCSF integer_t value using the toolkit's signed 64-bit representation.
func (a ArrayView) AsIntegerAt(index int) (int64, bool) {
	return a.asSignedIntegerAt(index)
}

// AsLongAt interprets an element as an OCSF long_t value using the toolkit's signed 64-bit representation. It
// intentionally shares its implementation with AsIntegerAt because the toolkit currently gives both OCSF types the
// same semantics.
func (a ArrayView) AsLongAt(index int) (int64, bool) {
	return a.asSignedIntegerAt(index)
}

func (a ArrayView) asSignedIntegerAt(index int) (int64, bool) {
	switch values := a.value.(type) {
	case []any:
		return AsInteger(values[index])
	case []int:
		return int64(values[index]), true
	case []int8:
		return int64(values[index]), true
	case []int16:
		return int64(values[index]), true
	case []int32:
		return int64(values[index]), true
	case []int64:
		return values[index], true
	case []float32:
		return integralFloat64(float64(values[index]))
	case []float64:
		return integralFloat64(values[index])
	case []json.Number:
		return jsonNumberInt64(values[index])
	default:
		return a.signedIntegerAtReflected(index)
	}
}

func (a ArrayView) signedIntegerAtReflected(index int) (int64, bool) {
	value := a.reflected.Index(index)
	switch value.Type() {
	case reflect.TypeFor[json.Number]():
		return jsonNumberInt64(json.Number(value.String()))
	case reflect.TypeFor[int](), reflect.TypeFor[int8](), reflect.TypeFor[int16](),
		reflect.TypeFor[int32](), reflect.TypeFor[int64]():
		return value.Int(), true
	case reflect.TypeFor[float32](), reflect.TypeFor[float64]():
		return integralFloat64(value.Float())
	default:
		return 0, false
	}
}

// AsFloatAt interprets an element as an OCSF float_t value using the toolkit's float64 representation without boxing
// homogeneous slice elements.
func (a ArrayView) AsFloatAt(index int) (float64, bool) {
	switch values := a.value.(type) {
	case []any:
		return AsFloat(values[index])
	case []int:
		return float64(values[index]), true
	case []int8:
		return float64(values[index]), true
	case []int16:
		return float64(values[index]), true
	case []int32:
		return float64(values[index]), true
	case []int64:
		return float64(values[index]), true
	case []float32:
		return float64(values[index]), true
	case []float64:
		return values[index], true
	case []json.Number:
		return AsFloat(values[index])
	default:
		return a.float64AtReflected(index)
	}
}

func (a ArrayView) float64AtReflected(index int) (float64, bool) {
	value := a.reflected.Index(index)
	switch value.Type() {
	case reflect.TypeFor[json.Number]():
		return AsFloat(json.Number(value.String()))
	case reflect.TypeFor[int](), reflect.TypeFor[int8](), reflect.TypeFor[int16](),
		reflect.TypeFor[int32](), reflect.TypeFor[int64]():
		return float64(value.Int()), true
	case reflect.TypeFor[float32](), reflect.TypeFor[float64]():
		return value.Float(), true
	default:
		return 0, false
	}
}
