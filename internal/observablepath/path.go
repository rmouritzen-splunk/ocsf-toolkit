package observablepath

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/pathseq"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
)

// Path is a parsed OCSF observable name path. It uses inline storage for ordinary path depths so parsing
// does not add a per-path allocation.
type Path struct {
	segments pathseq.Sequence[segment]
	rooted   bool
}

type segment struct {
	attribute string
	selectors []arraySelector
}

type arraySelector int

// DefinitionStatus describes whether a path is defined within the toolkit's supported schema traversal depth.
type DefinitionStatus uint8

const (
	DefinitionUndefined DefinitionStatus = iota
	DefinitionDefined
	DefinitionTraversalLimited
)

const (
	arraySelectorBrackets arraySelector = -1
	arraySelectorWildcard arraySelector = -2
)

// Resolution summarizes whether a path selected a value matching the requested kind. Found distinguishes an
// existing nonmatching value from a path that selected no value. Missing records a branch that ended at an absent
// or null intermediate attribute or an empty or out-of-range explicitly traversed array.
type Resolution struct {
	Found   bool
	Matched bool
	Missing bool
}

type valueMatcher func(any) bool

// Parse parses all supported OCSF observable path notation variations.
func Parse(name string) (Path, error) {
	rooted := false
	if strings.HasPrefix(name, "$") {
		if name == "$" {
			return Path{}, errors.New("the root marker must be followed by an attribute")
		}
		if !strings.HasPrefix(name, "$.") {
			return Path{}, errors.New("the root marker must be followed by a dot")
		}
		name = name[2:]
		rooted = true
	}
	if name == "" {
		return Path{}, errors.New("the path is empty")
	}

	path := Path{rooted: rooted}
	for {
		part, remainder, more := strings.Cut(name, ".")
		segment, err := parseSegment(part)
		if err != nil {
			return Path{}, err
		}
		path.segments.Push(segment)
		if !more {
			break
		}
		name = remainder
	}
	return path, nil
}

func parseSegment(part string) (segment, error) {
	if part == "" {
		return segment{}, errors.New("the path contains an empty attribute")
	}
	open := strings.IndexByte(part, '[')
	if open < 0 {
		return segment{attribute: part}, nil
	}
	if open == 0 {
		return segment{}, errors.New("an array selector has no attribute")
	}
	result := segment{attribute: part[:open]}
	remainder := part[open:]
	for remainder != "" {
		if remainder[0] != '[' {
			return segment{}, errors.New("unexpected text after an array selector")
		}
		close := strings.IndexByte(remainder, ']')
		if close < 0 {
			return segment{}, errors.New("an array selector is not closed")
		}
		selectorText := remainder[1:close]
		var selector arraySelector
		switch selectorText {
		case "":
			selector = arraySelectorBrackets
		case "*":
			selector = arraySelectorWildcard
		default:
			for _, character := range selectorText {
				if character < '0' || character > '9' {
					return segment{}, fmt.Errorf("array selector %q is not a non-negative index", selectorText)
				}
			}
			index, err := strconv.Atoi(selectorText)
			if err != nil {
				return segment{}, fmt.Errorf("array index %q is too large", selectorText)
			}
			selector = arraySelector(index)
		}
		result.selectors = append(result.selectors, selector)
		remainder = remainder[close+1:]
	}
	return result, nil
}

// FirstAttribute returns the first attribute in the path.
func (p *Path) FirstAttribute() string {
	if p.segments.Len() == 0 {
		return ""
	}
	return p.segments.At(0).attribute
}

// Definition reports whether the path is defined and whether checking it would cross the shared recursive-object
// traversal boundary.
func (p *Path) Definition(
	class *schema.ClassDefinition,
	objects map[string]*schema.ObjectDefinition,
	activeProfiles schema.ProfileSet,
) DefinitionStatus {
	if class == nil {
		return DefinitionUndefined
	}
	itemDefinition := &class.ItemDefinition
	var definitionPath eventpath.Path
	for index := 0; index < p.segments.Len(); index++ {
		segment := p.segments.At(index)
		attrDef := itemDefinition.Attributes[segment.attribute]
		if !schema.AttributeActive(attrDef, activeProfiles) {
			return DefinitionUndefined
		}
		selectorCount := len(segment.selectors)
		if selectorCount > 0 && (attrDef.IsArray == nil || !*attrDef.IsArray) {
			return DefinitionUndefined
		}
		if selectorCount > 1 {
			return DefinitionUndefined
		}
		if index == p.segments.Len()-1 {
			return DefinitionDefined
		}
		definitionPath.PushAttribute(segment.attribute)
		if definitionPath.HasPriorAttribute(segment.attribute) {
			return DefinitionTraversalLimited
		}
		if attrDef.Type != "object_t" || attrDef.ObjectType == nil {
			return DefinitionUndefined
		}
		objectDef := objects[*attrDef.ObjectType]
		if objectDef == nil {
			return DefinitionUndefined
		}
		itemDefinition = &objectDef.ItemDefinition
	}
	return DefinitionUndefined
}

// UsesNotation reports whether the path uses style for every schema-defined array segment and root marker.
func (p *Path) UsesNotation(
	style pathstyle.Style,
	class *schema.ClassDefinition,
	objects map[string]*schema.ObjectDefinition,
) bool {
	if class == nil || p.segments.Len() == 0 || p.rooted != (style == pathstyle.JSONPath) {
		return false
	}
	itemDefinition := &class.ItemDefinition
	var definitionPath eventpath.Path
	for index := 0; index < p.segments.Len(); index++ {
		segment := p.segments.At(index)
		attrDef := itemDefinition.Attributes[segment.attribute]
		if attrDef == nil {
			return false
		}
		isArray := attrDef.IsArray != nil && *attrDef.IsArray
		if isArray {
			if !selectorsUseNotation(segment.selectors, style) {
				return false
			}
		} else if len(segment.selectors) != 0 {
			return false
		}
		if index == p.segments.Len()-1 {
			return true
		}
		definitionPath.PushAttribute(segment.attribute)
		if definitionPath.HasPriorAttribute(segment.attribute) {
			return false
		}
		if attrDef.ObjectType == nil {
			return false
		}
		objectDef := objects[*attrDef.ObjectType]
		if objectDef == nil {
			return false
		}
		itemDefinition = &objectDef.ItemDefinition
	}
	return true
}

func selectorsUseNotation(selectors []arraySelector, style pathstyle.Style) bool {
	switch style {
	case pathstyle.Simple:
		return len(selectors) == 0
	case pathstyle.ArrayBrackets:
		return len(selectors) == 1 && selectors[0] == arraySelectorBrackets
	case pathstyle.ArrayWildcard:
		return len(selectors) == 1 && selectors[0] == arraySelectorWildcard
	case pathstyle.ArrayIndexed, pathstyle.JSONPath:
		return len(selectors) == 1 && selectors[0] >= 0
	default:
		return false
	}
}

// ResolveObject reports whether the path selects an event object.
func (p *Path) ResolveObject(event jsonish.Map) Resolution {
	return p.resolve(event, objectValueMatches)
}

// ResolveString reports whether the path selects a scalar with expected's JSON-compatible string representation.
func (p *Path) ResolveString(event jsonish.Map, expected string) Resolution {
	return p.resolve(event, func(value any) bool {
		return scalarStringMatches(value, expected)
	})
}

func (p *Path) resolve(event jsonish.Map, matches valueMatcher) Resolution {
	if value, resolution, handled := p.resolveSingle(event); handled {
		if resolution.Found {
			resolution.Matched = matches(value)
		}
		return resolution
	}

	current := []any{event}
	resolution := Resolution{}
	for index := 0; index < p.segments.Len(); index++ {
		current = resolveSegmentCandidates(current, p.segments.At(index), &resolution.Missing)
		if len(current) == 0 {
			return resolution
		}
	}
	return matchCandidates(current, matches, resolution)
}

// resolveSingle answers a resolve call directly when every segment addresses at most one attribute deep
// with no array selector, without falling back to resolve's general array-candidate walk. handled is
// false when the path cannot be answered this way (an array was encountered), and the caller must fall
// back to resolve.
func (p *Path) resolveSingle(event jsonish.Map) (value any, resolution Resolution, handled bool) {
	var current any = event
	for index := 0; index < p.segments.Len(); index++ {
		segment := p.segments.At(index)
		if len(segment.selectors) != 0 {
			return nil, Resolution{}, false
		}
		if _, array := eventvalue.NewArrayView(current); array {
			return nil, Resolution{}, false
		}
		if current == nil {
			return nil, Resolution{Missing: true}, true
		}
		item, ok := current.(jsonish.Map)
		if !ok {
			return nil, Resolution{}, true
		}
		next := item[segment.attribute]
		if next == nil {
			return nil, Resolution{Missing: true}, true
		}
		current = next
	}
	if _, array := eventvalue.NewArrayView(current); array {
		return nil, Resolution{}, false
	}
	return current, Resolution{Found: true}, true
}

// resolveSegmentCandidates keeps array expansion, cycle detection, and selector application in one event-processing
// hot-path loop so candidate traversal does not bounce through additional helpers.
func resolveSegmentCandidates(values []any, segment segment, missing *bool) []any {
	result := make([]any, 0)
	stack := values
	var seenArrays arrayIdentitySet
	for len(stack) > 0 {
		last := len(stack) - 1
		value := stack[last]
		stack = stack[:last]
		if array, ok := eventvalue.NewArrayView(value); ok {
			if array.Len() == 0 {
				*missing = true
				continue
			}
			if identity, track := identityOfArray(value); track {
				if seenArrays.add(identity) {
					continue
				}
			}
			for index := array.Len() - 1; index >= 0; index-- {
				stack = append(stack, array.At(index))
			}
			continue
		}
		if value == nil {
			*missing = true
			continue
		}
		item, ok := value.(jsonish.Map)
		if !ok {
			continue
		}
		value = item[segment.attribute]
		if value == nil {
			*missing = true
			continue
		}
		switch len(segment.selectors) {
		case 0:
			result = append(result, value)
		case 1:
			selected, selectionMissing := applySelectorToValue(result, value, segment.selectors[0])
			result = selected
			*missing = *missing || selectionMissing
		default:
			selected := []any{value}
			for _, selector := range segment.selectors {
				var selectionMissing bool
				selected, selectionMissing = applySelector(selected, selector)
				*missing = *missing || selectionMissing
				if len(selected) == 0 {
					break
				}
			}
			result = append(result, selected...)
		}
	}
	return result
}

func applySelectorToValue(result []any, value any, selector arraySelector) ([]any, bool) {
	array, ok := eventvalue.NewArrayView(value)
	if !ok {
		return result, false
	}
	if selector < 0 {
		if array.Len() == 0 {
			return result, true
		}
		for index := range array.Len() {
			result = append(result, array.At(index))
		}
		return result, false
	}
	index := int(selector)
	if index >= array.Len() {
		return result, true
	}
	return append(result, array.At(index)), false
}

func applySelector(values []any, selector arraySelector) ([]any, bool) {
	result := make([]any, 0)
	missing := false
	for _, value := range values {
		array, ok := eventvalue.NewArrayView(value)
		if !ok {
			continue
		}
		if selector < 0 {
			if array.Len() == 0 {
				missing = true
			}
			for index := range array.Len() {
				result = append(result, array.At(index))
			}
			continue
		}
		index := int(selector)
		if index < array.Len() {
			result = append(result, array.At(index))
		} else {
			missing = true
		}
	}
	return result, missing
}

func matchCandidates(values []any, matches valueMatcher, resolution Resolution) Resolution {
	stack := values
	var seenArrays arrayIdentitySet
	for len(stack) > 0 {
		last := len(stack) - 1
		value := stack[last]
		stack = stack[:last]
		if array, ok := eventvalue.NewArrayView(value); ok {
			if identity, track := identityOfArray(value); track {
				if seenArrays.add(identity) {
					continue
				}
			}
			for index := array.Len() - 1; index >= 0; index-- {
				stack = append(stack, array.At(index))
			}
			continue
		}
		resolution.Found = true
		if matches(value) {
			resolution.Matched = true
			return resolution
		}
	}
	return resolution
}

type arrayIdentity struct {
	typeOf   reflect.Type
	pointer  uintptr
	length   int
	capacity int
}

type arrayIdentitySet struct {
	inline   [8]arrayIdentity
	length   int
	overflow map[arrayIdentity]struct{}
}

// add records identity and reports whether it was already present. Ordinary event depths remain allocation-free;
// the overflow map prevents pathological array graphs from turning the linear inline scan into quadratic work.
func (s *arrayIdentitySet) add(identity arrayIdentity) bool {
	inlineLength := min(s.length, len(s.inline))
	for index := range inlineLength {
		if s.inline[index] == identity {
			return true
		}
	}
	if s.length < len(s.inline) {
		s.inline[s.length] = identity
		s.length++
		return false
	}
	if _, present := s.overflow[identity]; present {
		return true
	}
	if s.overflow == nil {
		s.overflow = make(map[arrayIdentity]struct{})
	}
	s.overflow[identity] = struct{}{}
	s.length++
	return false
}

func identityOfArray(value any) (arrayIdentity, bool) {
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice || reflected.Len() == 0 {
		return arrayIdentity{}, false
	}
	return arrayIdentity{
		typeOf: reflected.Type(), pointer: reflected.Pointer(), length: reflected.Len(), capacity: reflected.Cap(),
	}, true
}

func objectValueMatches(value any) bool {
	_, ok := value.(jsonish.Map)
	return ok
}

func scalarStringMatches(value any, expected string) bool {
	switch value.(type) {
	case jsonish.Map, []any, []jsonish.Map, []string, nil:
		return false
	}
	actual, ok := eventvalue.FormatScalar(value)
	return ok && actual == expected
}
