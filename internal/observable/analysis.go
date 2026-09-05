package observable

import (
	"fmt"

	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/observablepath"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// Problem identifies why an observable entry could not be resolved safely.
type Problem uint8

const (
	// ProblemNone indicates successful observable resolution.
	ProblemNone Problem = iota
	// ProblemArrayWrongType indicates that the event observables attribute is not an array.
	ProblemArrayWrongType
	// ProblemElementWrongType indicates that an observable array element is not an object.
	ProblemElementWrongType
	// ProblemNameMissing indicates that an observable has no non-null name.
	ProblemNameMissing
	// ProblemNameWrongType indicates that an observable name is not a string.
	ProblemNameWrongType
	// ProblemNameInvalidSyntax indicates that an observable name is not a valid observable path.
	ProblemNameInvalidSyntax
	// ProblemNameInvalidReference indicates that an observable name is not defined by the active class schema.
	ProblemNameInvalidReference
	// ProblemTraversalLimited indicates that resolving a schema-valid name would cross the toolkit's
	// recursive-object boundary.
	ProblemTraversalLimited
	// ProblemPathNotFound indicates that an observable path resolves to no event value.
	ProblemPathNotFound
	// ProblemPathNotObject indicates that a valueless observable does not resolve to an object.
	ProblemPathNotObject
	// ProblemValueWrongType indicates that an observable value is neither a string nor null.
	ProblemValueWrongType
	// ProblemValueNotFound indicates that an observable value is not present at its path.
	ProblemValueNotFound
	// ProblemCount is the number of defined observable resolution problem values, including ProblemNone.
	ProblemCount
)

// Analysis contains the observable entries found in one event state.
type Analysis struct {
	Entries []Entry
}

// Entry contains the resolution state for one observable entry.
type Entry struct {
	Raw         any
	Observable  jsonish.Map
	Name        string
	Path        observablepath.Path
	PathDefined bool
	Removable   bool
	Problem     Problem
	Err         error
}

// Analyzer resolves observable entries from one event state without collecting them.
type Analyzer struct {
	event          jsonish.Map
	class          *schema.ClassDefinition
	objects        map[string]*schema.ObjectDefinition
	activeProfiles schema.ProfileSet
	observables    Collection
	index          int
	limit          int
	wrongType      bool
}

// NewAnalyzer prepares independent observable analysis for the supplied event state. The boolean is false when the
// event has no observables attribute.
func NewAnalyzer(
	event jsonish.Map,
	class *schema.ClassDefinition,
	objects map[string]*schema.ObjectDefinition,
	activeProfiles schema.ProfileSet,
) (Analyzer, bool) {
	value, present := eventvalue.Attribute(event, "observables")
	if !present {
		return Analyzer{}, false
	}
	observables, ok := NewCollection(value)
	return Analyzer{
		event:          event,
		class:          class,
		objects:        objects,
		activeProfiles: activeProfiles,
		observables:    observables,
		limit:          observables.Len(),
		wrongType:      !ok,
	}, true
}

// LimitEntries restricts analysis to the first count observable entries. It is used when a processor has appended a
// suffix whose semantic validity is already established by construction.
func (a *Analyzer) LimitEntries(count int) {
	if count < 0 || count >= a.limit {
		return
	}
	a.limit = count
}

// Next returns the next observable index and its independently resolved entry. It returns an error for an
// unexpected internal resolution state.
func (a *Analyzer) Next() (int, Entry, bool, error) {
	if a.wrongType {
		if a.index != 0 {
			return 0, Entry{}, false, nil
		}
		a.index++
		return 0, Entry{Raw: a.observables.source, Problem: ProblemArrayWrongType}, true, nil
	}
	if a.index >= a.limit {
		return 0, Entry{}, false, nil
	}
	index := a.index
	a.index++
	entry, err := analyzeEntry(a.event, a.observables.at(index), a.class, a.objects, a.activeProfiles)
	return index, entry, true, err
}

// Analyze resolves the event's observable entries against the active class schema and event values. It also returns
// the collection used during analysis for representation-preserving mutation. The boolean is false when the
// observables attribute is absent or is not an array. An error reports an unexpected internal resolution state.
func Analyze(
	event jsonish.Map,
	class *schema.ClassDefinition,
	objects map[string]*schema.ObjectDefinition,
	activeProfiles schema.ProfileSet,
) (*Analysis, Collection, bool, error) {
	analyzer, present := NewAnalyzer(event, class, objects, activeProfiles)
	if !present {
		return nil, Collection{}, false, nil
	}
	analysis := &Analysis{Entries: make([]Entry, 0, analyzer.observables.Len())}
	for {
		_, entry, ok, err := analyzer.Next()
		if err != nil {
			return nil, Collection{}, false, err
		}
		if !ok {
			break
		}
		analysis.Entries = append(analysis.Entries, entry)
	}
	return analysis, analyzer.observables, !analyzer.wrongType, nil
}

func analyzeEntry(
	event jsonish.Map,
	value any,
	class *schema.ClassDefinition,
	objects map[string]*schema.ObjectDefinition,
	activeProfiles schema.ProfileSet,
) (Entry, error) {
	result := Entry{Raw: value}
	observable, ok := value.(jsonish.Map)
	if !ok {
		result.Problem = ProblemElementWrongType
		return result, nil
	}
	result.Observable = observable

	nameValue, namePresent := eventvalue.Attribute(observable, "name")
	if !namePresent {
		result.Problem = ProblemNameMissing
		return result, nil
	}
	name, ok := eventvalue.AsString(nameValue)
	if !ok {
		result.Problem = ProblemNameWrongType
		return result, nil
	}
	result.Name = name

	path, err := observablepath.Parse(name)
	if err != nil {
		result.Problem = ProblemNameInvalidSyntax
		result.Err = err
		return result, nil //nolint:nilerr // Invalid event paths are diagnostics, not processing failures.
	}
	if path.FirstAttribute() == "observables" {
		result.Problem = ProblemNameInvalidReference
		return result, nil
	}
	definitionStatus := path.Definition(class, objects, activeProfiles)
	definitionProblem, err := problemForDefinitionStatus(definitionStatus)
	if err != nil {
		return Entry{}, err
	}
	if definitionProblem != ProblemNone {
		result.Problem = definitionProblem
		return result, nil
	}
	result.Path = path
	result.PathDefined = true

	observableValue, valuePresent := observable["value"]
	if !valuePresent {
		pathResolution := path.ResolveObject(event)
		if pathResolution.Matched {
			result.Removable = true
			return result, nil
		}
		if !pathResolution.Found {
			result.Problem = ProblemPathNotFound
			return result, nil
		}
		result.Problem = ProblemPathNotObject
		return result, nil
	}
	if observableValue == nil {
		pathResolution := path.ResolveNull(event)
		if pathResolution.Missing || pathResolution.Matched {
			result.Removable = true
			return result, nil
		}
		if !pathResolution.Found {
			result.Problem = ProblemPathNotFound
			return result, nil
		}
		result.Problem = ProblemValueNotFound
		return result, nil
	}
	valueString, ok := eventvalue.AsString(observableValue)
	if !ok {
		result.Problem = ProblemValueWrongType
		return result, nil
	}
	pathResolution := path.ResolveString(event, valueString)
	if pathResolution.Matched {
		result.Removable = true
		return result, nil
	}
	if !pathResolution.Found {
		result.Problem = ProblemPathNotFound
		return result, nil
	}
	result.Problem = ProblemValueNotFound
	return result, nil
}

func problemForDefinitionStatus(status observablepath.DefinitionStatus) (Problem, error) {
	switch status {
	case observablepath.DefinitionUndefined:
		return ProblemNameInvalidReference, nil
	case observablepath.DefinitionDefined:
		return ProblemNone, nil
	case observablepath.DefinitionTraversalLimited:
		return ProblemTraversalLimited, nil
	default:
		return ProblemNone, fmt.Errorf("unexpected observable path definition status %d", status)
	}
}

// RemovableCount returns the number of entries proven redundant.
func (a *Analysis) RemovableCount() int {
	count := 0
	for _, entry := range a.Entries {
		if entry.Removable {
			count++
		}
	}
	return count
}
