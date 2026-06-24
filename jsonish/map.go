package jsonish

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Map is a type alias for map[string]any. It is a general map container type useful as in memory representation of a
// JSON Object.
type Map = map[string]any

var ErrNilMapParameter = errors.New("map parameter is nil")
var ErrWrongType = errors.New("wrong type")
var ErrExistingAssociation = errors.New("existing association")

// GetMap returns the value of key in Map m if the result is a Map (a map[string]any),
// and otherwise returns an error.
func GetMap(m Map, key string) (Map, error) {
	if m == nil {
		return nil, ErrNilMapParameter
	}
	v := m[key]
	switch v := v.(type) {
	case Map:
		return v, nil
	default:
		return nil, fmt.Errorf("type of key %q is not a map (not a JSON object): %T: %w", key, v, ErrWrongType)
	}
}

// GetOptionalMap returns the value of key in Map m if the result is a Map (a map[string]any),
// or nil if key has no mapping or maps to nil, and otherwise returns an error.
func GetOptionalMap(m Map, key string) (Map, error) {
	if m == nil {
		return nil, ErrNilMapParameter
	}
	v := m[key]
	switch v := v.(type) {
	case Map:
		return v, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("type of key %q is not a map (not a JSON object): %T: %w", key, v, ErrWrongType)
	}
}

func sliceOfAnyToSliceOfMaps(key string, v []any) ([]Map, error) {
	array := make([]Map, len(v))
	for i, item := range v {
		switch item := item.(type) {
		case Map:
			array[i] = item
		default:
			return nil, fmt.Errorf(
				"item %d of key %q is not a map (not a JSON object): %T: %w", i+1, key, item, ErrWrongType)
		}
	}
	return array, nil
}

// GetSliceOfMaps returns the value of key in Map m if the result is or can be converted to a slice of maps,
// and otherwise returns an error.
func GetSliceOfMaps(m Map, key string) ([]Map, error) {
	if m == nil {
		return nil, ErrNilMapParameter
	}
	v := m[key]
	switch v := v.(type) {
	case []any: // This type used by the JSON parsing (ReadJSONObjectFromFile) and mapping code
		return sliceOfAnyToSliceOfMaps(key, v)
	case []Map: // This type is created in unit tests (map_test.go)
		return v, nil
	default:
		return nil, fmt.Errorf("value of key %q is not a slice (not a JSON array): %T: %w", key, v, ErrWrongType)
	}
}

// GetOptionalSliceOfMaps returns the value of key in Map m if the result is or can be converted to a slice of maps,
// or nil if key has no mapping or maps to nil, and otherwise returns an error.
func GetOptionalSliceOfMaps(m Map, key string) ([]Map, error) {
	if m == nil {
		return nil, ErrNilMapParameter
	}
	v := m[key]
	switch v := v.(type) {
	case []any: // This type used by the JSON parsing (ReadJSONObjectFromFile) and mapping code
		return sliceOfAnyToSliceOfMaps(key, v)
	case []Map: // This type is created in unit tests (map_test.go)
		return v, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("value of key %q is not a slice (not a JSON array): %T: %w", key, v, ErrWrongType)
	}
}

// GetString returns the value of key in Map m if the result is a string,
// and otherwise returns an error (and a blank string).
func GetString(m Map, key string) (string, error) {
	if m == nil {
		return "", ErrNilMapParameter
	}
	v := m[key]
	switch v := v.(type) {
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("value of key %q is not a string: %T: %w", key, v, ErrWrongType)
	}
}

// GetOptionalString returns the value and present boolean of key in Map m, and returns an error when key is
// associated with a value that is not a string, and also returns an error when the map passed in is nil.
func GetOptionalString(m Map, key string) (string, bool, error) {
	if m == nil {
		return "", false, ErrNilMapParameter
	}
	v, present := m[key]
	if present {
		switch v := v.(type) {
		case string:
			return v, true, nil
		default:
			return "", false, fmt.Errorf("value of key %q is not a string: %T: %w", key, v, ErrWrongType)
		}
	}
	return "", false, nil
}

// GetStringOrDefault returns the value of key in Map m if the result is a string, returns defaultValue if m is nil
// or key has no association, and returns false and an error if key is associated with a non-string value.
func GetStringOrDefault(m Map, key string, defaultValue string) (string, error) {
	if m == nil {
		return defaultValue, nil
	}
	v, present := m[key]
	if present {
		switch v := v.(type) {
		case string:
			return v, nil
		default:
			return "", fmt.Errorf("value of key %q is not a string: %T: %w", key, v, ErrWrongType)
		}
	} else {
		return defaultValue, nil
	}
}

// GetOptionalBoolean returns the value and present boolean of key in Map m, and returns an error when key is
// associated with a value that is not a bool, and also returns an error when the map passed in is nil.
func GetOptionalBoolean(m Map, key string) (bool, bool, error) {
	if m == nil {
		return false, false, ErrNilMapParameter
	}
	v, present := m[key]
	if present {
		switch v := v.(type) {
		case bool:
			return v, true, nil
		default:
			return false, false, fmt.Errorf("value of key %q is not a boolean: %T: %w", key, v, ErrWrongType)
		}
	}
	return false, false, nil
}

// GetBooleanOrDefault returns the value of key in Map m if the result is a boolean, returns defaultValue if m is nil
// or key has no association, and returns false and an error if key is associated with a non-boolean value.
func GetBooleanOrDefault(m Map, key string, defaultValue bool) (bool, error) {
	if m == nil {
		return defaultValue, nil
	}
	v, present := m[key]
	if present {
		switch v := v.(type) {
		case bool:
			return v, nil
		default:
			return false, fmt.Errorf("value of key %q is not a boolean: %T: %w", key, v, ErrWrongType)
		}
	} else {
		return defaultValue, nil
	}
}

// GetIn returns the value of keys in a nested structure, or nil if keys does not map to a value.
func GetIn(m Map, keys ...string) any {
	if m == nil || len(keys) == 0 {
		return nil
	}
	last := len(keys) - 1
	for i := range last {
		value := m[keys[i]]
		switch value := value.(type) {
		case Map:
			m = value
		default:
			return nil
		}
	}
	return m[keys[last]]
}

func SplitMapPath(path string) []string {
	// strings.Split returns single element array with a blank string: [""]
	// this is not what we want
	if len(path) == 0 {
		return nil
	}
	return strings.Split(path, ".")
}

// GetInPath returns the value associated with a path of period separated keys, or nil.
func GetInPath(m Map, path string) any {
	// Try map key with dots first
	v, present := m[path]
	if present {
		return v
	}
	return GetIn(m, SplitMapPath(path)...)
}

// RemoveIn returns and removes the value of keys in a nested structure, or returns nil if keys does not map to a value.
func RemoveIn(m Map, keys ...string) any {
	if m == nil || len(keys) == 0 {
		return nil
	}
	last := len(keys) - 1
	for i := range last {
		value := m[keys[i]]
		switch value := value.(type) {
		case Map:
			m = value
		default:
			return nil
		}
	}
	k := keys[last]
	v := m[k]
	delete(m, k)
	return v
}

// RemoveInPath returns and removes the value associated with a path of period separated keys,
// or returns nil if keys does not map to a value.
func RemoveInPath(m Map, path string) any {
	// Try map key with dots first
	v, present := m[path]
	if present {
		delete(m, path)
		return v
	}
	return RemoveIn(m, SplitMapPath(path)...)
}

func formatStringArray(ss []string) string {
	if ss == nil {
		return "nil"
	}
	if len(ss) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, s := range ss {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(s))
	}
	b.WriteString("]")
	return b.String()
}

func Put(m Map, key string, value any, overwrite bool) (Map, error) {
	if m == nil {
		m = make(Map)
	}
	_, present := m[key]
	if present {
		if overwrite {
			m[key] = value
		} else {
			return nil, fmt.Errorf("not overwriting existing key %q: %w", key, ErrExistingAssociation)
		}
	} else {
		m[key] = value
	}
	return m, nil
}

func PutIn(m Map, keys []string, value any, overwrite bool) (Map, error) {
	keyCount := len(keys)
	if keyCount == 0 {
		// nothing to do; avoid doing any work, like creating map for nil m
		return m, nil
	}

	if m == nil {
		m = make(Map)
	}
	topMap := m

	// Iterate over branch (non-leaf) associations
	last := keyCount - 1
	for i := range last {
		key := keys[i]
		v, present := m[key]
		if present {
			switch v := v.(type) {
			case Map:
				m = v
			default:
				if overwrite {
					// Overwrite v with a map
					v := make(Map)
					m[key] = v
					m = v
				} else {
					return nil, fmt.Errorf("PutIn(m, %s, value, %t): key %d %q associated to non-map of type %T: %w",
						formatStringArray(keys), overwrite, i+1, key, v, ErrExistingAssociation)
				}
			}
		} else {
			// No association, so create a map at this level regardless of overwrite flag
			v := make(Map)
			m[key] = v
			m = v
		}
	}

	// Now handle leaf association
	lastKey := keys[last]
	_, present := m[lastKey]
	if present {
		if overwrite {
			m[lastKey] = value
		} else {
			return nil, fmt.Errorf("PutIn(m, %s, value, %t): key %d %q: %w",
				formatStringArray(keys), overwrite, last+1, lastKey, ErrExistingAssociation)
		}
	} else {
		m[lastKey] = value
	}

	return topMap, nil
}

func PutInPath(m Map, path string, value any, overwrite bool) (Map, error) {
	return PutIn(m, SplitMapPath(path), value, overwrite)
}

func KeysToString(m Map) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, strconv.Quote(k))
	}
	return strings.Join(keys, ", ")
}
