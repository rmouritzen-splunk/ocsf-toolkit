package observable

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

type identity struct {
	name      string
	typeID    int64
	valueKind uint8
	value     string
}

const (
	objectValue uint8 = iota
	nullValue
	stringValue
)

// Collection retains an observable array's source representation and indexed view so read analysis and
// representation-preserving mutation operate on one array classification.
type Collection struct {
	source    any
	reflected reflect.Value
	length    int
}

// NewCollection returns an observable collection when value is a slice or array.
func NewCollection(value any) (Collection, bool) {
	switch values := value.(type) {
	case []any:
		return Collection{source: value, length: len(values)}, true
	case []jsonish.Map:
		return Collection{source: value, length: len(values)}, true
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return Collection{source: value}, false
	}
	return Collection{source: value, reflected: reflected, length: reflected.Len()}, true
}

// Len returns the number of observable entries in the collection.
func (c *Collection) Len() int {
	return c.length
}

func (c *Collection) at(index int) any {
	switch values := c.source.(type) {
	case []any:
		return values[index]
	case []jsonish.Map:
		return values[index]
	default:
		return c.reflected.Index(index).Interface()
	}
}

// DuplicateSource identifies the collection containing an observable duplicated by generated enrichment.
type DuplicateSource string

const (
	// DuplicateExisting identifies an observable already present in the event.
	DuplicateExisting DuplicateSource = "existing"
	// DuplicateGenerated identifies an observable generated earlier in the same enrichment pass.
	DuplicateGenerated DuplicateSource = "generated"
)

// Duplicate describes one generated observable excluded as a semantic duplicate.
type Duplicate struct {
	Observable jsonish.Map
	Name       string
	Source     DuplicateSource
	// GeneratedIndex identifies the duplicate in the generated input slice.
	GeneratedIndex int
}

// Deduplication contains generated observables accepted for append and duplicates that were excluded.
type Deduplication struct {
	Accepted   []jsonish.Map
	Duplicates []Duplicate
}

// DeduplicateGenerated excludes generated observables that duplicate existing or earlier generated entries. Malformed
// observables without a complete semantic identity do not participate in deduplication; validation reports them, and
// they must not suppress a valid generated observable.
func DeduplicateGenerated(existing *Collection, generated []jsonish.Map) Deduplication {
	existingLength := 0
	if existing != nil {
		existingLength = existing.Len()
	}
	seen := make(map[identity]DuplicateSource, existingLength+len(generated))
	for index := range existingLength {
		if key, ok := identify(existing.at(index)); ok {
			seen[key] = DuplicateExisting
		}
	}

	result := Deduplication{Accepted: make([]jsonish.Map, 0, len(generated))}
	for generatedIndex, observable := range generated {
		key, ok := identify(observable)
		if ok {
			if source, duplicate := seen[key]; duplicate {
				result.Duplicates = append(result.Duplicates, Duplicate{
					Observable:     observable,
					Name:           key.name,
					Source:         source,
					GeneratedIndex: generatedIndex,
				})
				continue
			}
			seen[key] = DuplicateGenerated
		}
		result.Accepted = append(result.Accepted, observable)
	}
	return result
}

func identify(value any) (identity, bool) {
	observable, ok := value.(jsonish.Map)
	if !ok {
		return identity{}, false
	}
	name, ok := eventvalue.AsString(observable["name"])
	if !ok {
		return identity{}, false
	}
	typeID, ok := eventvalue.AsInteger(observable["type_id"])
	if !ok {
		return identity{}, false
	}
	result := identity{name: name, typeID: typeID, valueKind: objectValue}
	value, present := observable["value"]
	if !present {
		return result, true
	}
	if value == nil {
		result.valueKind = nullValue
		return result, true
	}
	valueString, ok := eventvalue.AsString(value)
	if !ok {
		return identity{}, false
	}
	result.valueKind = stringValue
	result.value = valueString
	return result, true
}

// Append preserves the collection's array representation when it can hold generated observable maps. A fixed-size
// Go array is necessarily normalized to a slice because appending cannot preserve its length or concrete array type.
func (c *Collection) Append(generated []jsonish.Map) (any, error) {
	existing := c.source
	switch values := existing.(type) {
	case []jsonish.Map:
		result := make([]jsonish.Map, 0, len(values)+len(generated))
		result = append(result, values...)
		return append(result, generated...), nil
	case []any:
		result := make([]any, 0, len(values)+len(generated))
		result = append(result, values...)
		for _, observable := range generated {
			result = append(result, observable)
		}
		return result, nil
	}

	reflected := reflect.ValueOf(existing)
	if reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array {
		resultType := reflected.Type()
		if reflected.Kind() == reflect.Array {
			resultType = reflect.SliceOf(reflected.Type().Elem())
		}
		result := reflect.MakeSlice(resultType, 0, reflected.Len()+len(generated))
		for index := range reflected.Len() {
			result = reflect.Append(result, reflected.Index(index))
		}
		for _, observable := range generated {
			value := reflect.ValueOf(observable)
			if value.Type().AssignableTo(resultType.Elem()) {
				result = reflect.Append(result, value)
				continue
			}
			if value.Type().ConvertibleTo(resultType.Elem()) {
				result = reflect.Append(result, value.Convert(resultType.Elem()))
				continue
			}
			return appendAsAny(c, generated), nil
		}
		return result.Interface(), nil
	}
	return nil, errors.New("existing observables value is not an array")
}

func appendAsAny(existing *Collection, generated []jsonish.Map) []any {
	result := make([]any, 0, existing.Len()+len(generated))
	for index := range existing.Len() {
		result = append(result, existing.at(index))
	}
	for _, observable := range generated {
		result = append(result, observable)
	}
	return result
}

// FilterRemovable preserves the source slice element type while removing entries marked removable. Its concrete
// fast paths and reflective fallbacks intentionally remain together because observable removal is an event-processing
// hot path and the branches describe one type-preservation operation.
func (c *Collection) FilterRemovable(entries []Entry, removeCount int) (any, error) {
	value := c.source
	if value == nil {
		return nil, errors.New("observable value is not an array")
	}
	if len(entries) != c.Len() {
		return nil, fmt.Errorf(
			"observable analysis has %d entries for an array of length %d", len(entries), c.Len(),
		)
	}
	if removeCount < 0 || removeCount > c.Len() {
		return nil, fmt.Errorf(
			"observable analysis remove count %d is outside array length %d", removeCount, c.Len(),
		)
	}
	switch values := value.(type) {
	case []jsonish.Map:
		filtered := make([]jsonish.Map, 0, len(values)-removeCount)
		for index, element := range values {
			if !entries[index].Removable { //nolint:gosec // entries is always sized to match values.
				filtered = append(filtered, element)
			}
		}
		return validateFilteredResult(filtered, len(values), len(filtered), removeCount)
	case []any:
		filtered := make([]any, 0, len(values)-removeCount)
		for index, element := range values {
			if !entries[index].Removable { //nolint:gosec // entries is always sized to match values.
				filtered = append(filtered, element)
			}
		}
		return validateFilteredResult(filtered, len(values), len(filtered), removeCount)
	default:
		reflected := reflect.ValueOf(value)
		if reflected.Kind() == reflect.Slice {
			filtered := reflect.MakeSlice(reflected.Type(), 0, reflected.Len()-removeCount)
			for index := range reflected.Len() {
				//nolint:gosec // entries is always sized to match the reflected slice callers pass here.
				if !entries[index].Removable {
					filtered = reflect.Append(filtered, reflected.Index(index))
				}
			}
			return validateFilteredResult(filtered.Interface(), reflected.Len(), filtered.Len(), removeCount)
		}
		if reflected.Kind() == reflect.Array {
			filtered := reflect.MakeSlice(reflect.SliceOf(reflected.Type().Elem()), 0, reflected.Len()-removeCount)
			for index := range reflected.Len() {
				//nolint:gosec // entries is always sized to match the reflected array callers pass here.
				if !entries[index].Removable {
					filtered = reflect.Append(filtered, reflected.Index(index))
				}
			}
			return validateFilteredResult(filtered.Interface(), reflected.Len(), filtered.Len(), removeCount)
		}
		return nil, errors.New("observable value is not an array")
	}
}

func validateFilteredResult(result any, originalLength, filteredLength, removeCount int) (any, error) {
	actualRemoveCount := originalLength - filteredLength
	if removeCount != actualRemoveCount {
		return nil, fmt.Errorf(
			"observable analysis remove count is %d; expected %d", removeCount, actualRemoveCount,
		)
	}
	return result, nil
}
