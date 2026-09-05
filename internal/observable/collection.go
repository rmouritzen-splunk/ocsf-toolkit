package observable

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

type identity struct {
	name     string
	typeID   int64
	value    string
	hasValue bool
}

// GeneratedIdentitySet tracks semantic identities produced during one enrichment pass. Its zero value is ready for
// use and allocates storage only when the first identity is added.
type GeneratedIdentitySet struct {
	identities map[identity]struct{}
}

// Add records a generated identity. It returns true if the identity was newly added and false if it was already
// present.
func (s *GeneratedIdentitySet) Add(name string, typeID int64, value *string) bool {
	key := identity{name: name, typeID: typeID}
	if value != nil {
		key.value = *value
		key.hasValue = true
	}
	if _, present := s.identities[key]; present {
		return false
	}
	if s.identities == nil {
		s.identities = make(map[identity]struct{})
	}
	s.identities[key] = struct{}{}
	return true
}

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

// ObservableOrigin identifies whether an observable was present before enrichment or generated during it.
type ObservableOrigin string

const (
	// ObservableOriginExisting identifies an observable present before enrichment.
	ObservableOriginExisting ObservableOrigin = "existing"
	// ObservableOriginGenerated identifies an observable generated during enrichment.
	ObservableOriginGenerated ObservableOrigin = "generated"
)

// DuplicateOccurrence identifies one observable within its origin collection.
type DuplicateOccurrence struct {
	Origin ObservableOrigin
	Index  int
}

// Duplicate describes a semantic duplicate and the first occurrence of its identity.
type Duplicate struct {
	Name       string
	Occurrence DuplicateOccurrence
	First      DuplicateOccurrence
}

// DuplicateAnalysis contains all duplicate relationships and the generated candidates retained after optional
// generated-to-generated deduplication.
type DuplicateAnalysis struct {
	AcceptedGenerated []jsonish.Map
	Duplicates        []Duplicate
}

type duplicateIdentityState struct {
	first        DuplicateOccurrence
	hasGenerated bool
}

// AnalyzeDuplicates detects duplicate identities across existing and generated observables. When
// deduplicateGenerated is true, only a generated candidate that duplicates an earlier generated candidate is omitted;
// existing observables and generated candidates that duplicate only existing observables are retained. Malformed
// observables without a complete semantic identity do not participate.
func AnalyzeDuplicates(
	existing *Collection,
	generated []jsonish.Map,
	deduplicateGenerated bool,
) DuplicateAnalysis {
	capacity := len(generated)
	if existing != nil {
		capacity += existing.Len()
	}
	identities := make(map[identity]duplicateIdentityState, capacity)
	result := DuplicateAnalysis{AcceptedGenerated: generated}
	if existing != nil {
		for index := range existing.Len() {
			key, ok := identify(existing.at(index))
			if !ok {
				continue
			}
			occurrence := DuplicateOccurrence{Origin: ObservableOriginExisting, Index: index}
			state, present := identities[key]
			if present {
				result.Duplicates = append(result.Duplicates, Duplicate{
					Name: key.name, Occurrence: occurrence, First: state.first,
				})
				continue
			}
			identities[key] = duplicateIdentityState{first: occurrence}
		}
	}

	var accepted []jsonish.Map
	for index, candidate := range generated {
		key, ok := identify(candidate)
		if !ok {
			if accepted != nil {
				accepted = append(accepted, candidate)
			}
			continue
		}
		occurrence := DuplicateOccurrence{Origin: ObservableOriginGenerated, Index: index}
		state, present := identities[key]
		if present {
			result.Duplicates = append(result.Duplicates, Duplicate{
				Name: key.name, Occurrence: occurrence, First: state.first,
			})
		}
		remove := deduplicateGenerated && state.hasGenerated
		if !state.hasGenerated {
			state.hasGenerated = true
			if !present {
				state.first = occurrence
			}
			identities[key] = state
		}
		if remove {
			if accepted == nil {
				accepted = make([]jsonish.Map, index, len(generated)-1)
				copy(accepted, generated[:index])
			}
			continue
		}
		if accepted != nil {
			accepted = append(accepted, candidate)
		}
	}
	if accepted != nil {
		result.AcceptedGenerated = accepted
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
	result := identity{name: name, typeID: typeID}
	observableValue := observable["value"]
	if observableValue == nil {
		return result, true
	}
	valueString, ok := eventvalue.AsString(observableValue)
	if !ok {
		return identity{}, false
	}
	result.value = valueString
	result.hasValue = true
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
