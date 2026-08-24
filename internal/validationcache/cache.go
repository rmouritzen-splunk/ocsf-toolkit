package validationcache

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sync"

	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	internalversion "github.com/ocsf/ocsf-toolkit/internal/semver"
)

// Lazy owns a validation cache that is initialized at most once. Get is safe for concurrent use.
// Cache construction occurs while pipelines are built, never while an event is processed.
type Lazy struct {
	cacheGuard sync.Once
	cache      Cache
	err        error
}

// Get returns the immutable cache derived from compiled, building it on the first call.
// Subsequent calls return the same cache or construction error.
func (l *Lazy) Get(compiled *schema.Compiled) (*Cache, error) {
	l.cacheGuard.Do(func() {
		l.cache, l.err = Build(compiled)
	})
	if l.err != nil {
		return nil, l.err
	}
	return &l.cache, nil
}

// Cache contains immutable schema-derived constraints shared by validation pipelines.
type Cache struct {
	VersionOK bool
	Version   internalversion.Version
	Types     map[string]*TypeValidation
}

// TypeValidation contains the resolved definition and constraints for one schema type.
type TypeValidation struct {
	Definition    *schema.TypeDefinition
	PrimitiveType string
	Regex         RegexConstraint
	Value         ValueConstraint
	Range         RangeConstraint
	MaxLen        MaxLenConstraint
	HasRegex      bool
	HasValue      bool
	HasRange      bool
	HasMaxLen     bool
}

// RangeConstraint is the resolved numeric range for a type.
type RangeConstraint struct {
	TypeName string
	Low      int64
	High     int64
}

// MaxLenConstraint is the resolved maximum string length for a type.
type MaxLenConstraint struct {
	TypeName string
	MaxLen   int64
}

// ValueConstraint is the resolved allowed-value set for a type.
type ValueConstraint struct {
	TypeName     string
	intValues    valueSet[int64]
	floatValues  valueSet[float64]
	stringValues valueSet[string]
	boolValues   valueSet[bool]
}

const valueIndexThreshold = 16

// valueSet keeps small constraints compact and cache-friendly, while indexing larger constraints once during
// pipeline construction so per-event membership checks do not scan the complete allowed-value list.
type valueSet[T comparable] struct {
	values []T
	index  map[T]struct{}
}

// RegexConstraint is the resolved and compiled regular expression for a type.
type RegexConstraint struct {
	TypeName string
	Regex    string
	Compiled *regexp.Regexp
	Err      error
}

// Build compiles validation constraints from a normalized schema.
func Build(compiled *schema.Compiled) (Cache, error) {
	version, versionOK := internalversion.Parse(compiled.Version)
	cache := Cache{
		Version:   version,
		VersionOK: versionOK,
		Types:     make(map[string]*TypeValidation, len(compiled.Dictionary.Types.Attributes)),
	}
	resolver := typeResolver{
		types:    compiled.Dictionary.Types.Attributes,
		resolved: cache.Types,
		visiting: make(map[string]struct{}, len(compiled.Dictionary.Types.Attributes)),
	}
	for typeName := range compiled.Dictionary.Types.Attributes {
		if _, err := resolver.resolve(typeName); err != nil {
			return Cache{}, err
		}
	}
	return cache, nil
}

// typeResolver builds final TypeValidation values while using completed ancestors as temporary memoized results.
// visiting exists only during Build; resolved becomes the immutable Cache.Types map returned to the caller.
type typeResolver struct {
	types    map[string]*schema.TypeDefinition
	resolved map[string]*TypeValidation
	visiting map[string]struct{}
}

func (r *typeResolver) resolve(typeName string) (*TypeValidation, error) {
	if resolved := r.resolved[typeName]; resolved != nil {
		return resolved, nil
	}

	currentName := typeName
	path := make([]string, 0)
	var inherited *TypeValidation
	for {
		if resolved := r.resolved[currentName]; resolved != nil {
			inherited = resolved
			break
		}
		if _, present := r.visiting[currentName]; present {
			return nil, fmt.Errorf("type inheritance for %q contains a cycle at %q", typeName, currentName)
		}

		definition := r.types[currentName]
		if definition == nil {
			inherited = &TypeValidation{PrimitiveType: currentName}
			break
		}
		r.visiting[currentName] = struct{}{}
		path = append(path, currentName)
		if isPrimitiveTypeName(currentName) || definition.Type == "" {
			inherited = &TypeValidation{PrimitiveType: currentName}
			break
		}
		currentName = definition.Type
	}

	for _, name := range slices.Backward(path) {
		resolved, err := resolveTypeValidation(name, r.types[name], inherited)
		if err != nil {
			return nil, err
		}
		r.resolved[name] = resolved
		delete(r.visiting, name)
		inherited = resolved
	}
	return r.resolved[typeName], nil
}

func resolveTypeValidation(
	typeName string,
	definition *schema.TypeDefinition,
	inherited *TypeValidation,
) (*TypeValidation, error) {
	resolved := &TypeValidation{Definition: definition, PrimitiveType: inherited.PrimitiveType}
	resolved.Regex, resolved.HasRegex = inherited.Regex, inherited.HasRegex
	resolved.Value, resolved.HasValue = inherited.Value, inherited.HasValue
	resolved.Range, resolved.HasRange = inherited.Range, inherited.HasRange
	resolved.MaxLen, resolved.HasMaxLen = inherited.MaxLen, inherited.HasMaxLen

	if definition.RegEx != nil {
		regex := *definition.RegEx
		compiledRegex, err := regexp.Compile(regex)
		resolved.Regex = RegexConstraint{TypeName: typeName, Regex: regex, Compiled: compiledRegex, Err: err}
		resolved.HasRegex = true
	}
	if len(definition.Values) > 0 {
		constraint, err := compileValueConstraint(typeName, resolved.PrimitiveType, definition)
		if err != nil {
			return nil, err
		}
		resolved.Value = constraint
		resolved.HasValue = true
	}
	if len(definition.Range) == 2 {
		resolved.Range = RangeConstraint{TypeName: typeName, Low: definition.Range[0], High: definition.Range[1]}
		resolved.HasRange = true
	}
	if definition.MaxLen != nil {
		resolved.MaxLen = MaxLenConstraint{TypeName: typeName, MaxLen: *definition.MaxLen}
		resolved.HasMaxLen = true
	}
	return resolved, nil
}

func isPrimitiveTypeName(typeName string) bool {
	switch typeName {
	case "boolean_t", "float_t", "integer_t", "long_t", "string_t", "json_t":
		return true
	default:
		return false
	}
}

func compileValueConstraint(typeName, primitiveType string, typeDef *schema.TypeDefinition) (ValueConstraint, error) {
	constraint := ValueConstraint{TypeName: typeName}
	for index, value := range typeDef.Values {
		var err error
		switch primitiveType {
		case "integer_t":
			constraint.intValues, err = appendConstraintValue(
				constraint.intValues, typeName, index, value, eventvalue.AsInteger, "signed 64-bit integer",
			)
		case "long_t":
			constraint.intValues, err = appendConstraintValue(
				constraint.intValues, typeName, index, value, eventvalue.AsLong, "signed 64-bit integer",
			)
		case "float_t":
			constraint.floatValues, err = appendConstraintValue(
				constraint.floatValues, typeName, index, value, schemaFloat64, "finite float64",
			)
		case "string_t":
			constraint.stringValues, err = appendConstraintValue(
				constraint.stringValues, typeName, index, value, eventvalue.AsString, "string",
			)
		case "boolean_t":
			constraint.boolValues, err = appendConstraintValue(
				constraint.boolValues, typeName, index, value, eventvalue.AsBoolean, "boolean",
			)
		default:
			err = fmt.Errorf("type %q has allowed values but unsupported primitive type %q", typeName, primitiveType)
		}
		if err != nil {
			return ValueConstraint{}, err
		}
	}
	constraint.intValues.buildIndex()
	constraint.floatValues.buildIndex()
	constraint.stringValues.buildIndex()
	constraint.boolValues.buildIndex()
	return constraint, nil
}

// appendConstraintValue converts value via convert and appends it to values, or reports a uniform error naming
// typeName, index, and kindDescription when convert rejects it.
func appendConstraintValue[T comparable](
	values valueSet[T], typeName string, index int, value any, convert func(any) (T, bool), kindDescription string,
) (valueSet[T], error) {
	normalized, ok := convert(value)
	if !ok {
		return valueSet[T]{}, fmt.Errorf(
			"type %q allowed value at index %d is not a %s", typeName, index, kindDescription,
		)
	}
	values.values = append(values.values, normalized)
	return values, nil
}

func (s *valueSet[T]) buildIndex() {
	if len(s.values) < valueIndexThreshold {
		return
	}
	s.index = make(map[T]struct{}, len(s.values))
	for _, value := range s.values {
		s.index[value] = struct{}{}
	}
}

func (s valueSet[T]) configured() bool {
	return s.values != nil
}

func (s valueSet[T]) contains(value T) bool {
	if s.index != nil {
		_, present := s.index[value]
		return present
	}
	return slices.Contains(s.values, value)
}

func schemaFloat64(value any) (float64, bool) {
	normalized, ok := eventvalue.AsFloat(value)
	return normalized, ok && !math.IsNaN(normalized) && !math.IsInf(normalized, 0)
}

// Contains reports whether value is in the normalized allowed-value set.
func (c ValueConstraint) Contains(value any) bool {
	if c.intValues.configured() {
		candidate, ok := eventvalue.AsInteger(value)
		return ok && c.ContainsInt64(candidate)
	}
	if c.floatValues.configured() {
		candidate, ok := eventvalue.AsFloat(value)
		return ok && c.ContainsFloat64(candidate)
	}
	if c.stringValues.configured() {
		candidate, ok := eventvalue.AsString(value)
		return ok && c.ContainsString(candidate)
	}
	candidate, ok := eventvalue.AsBoolean(value)
	return ok && c.ContainsBool(candidate)
}

// ContainsInt64 reports whether value is in an integer constraint without boxing it.
func (c ValueConstraint) ContainsInt64(value int64) bool {
	return c.intValues.contains(value)
}

// ContainsFloat64 reports whether value is in a floating-point constraint without boxing it.
func (c ValueConstraint) ContainsFloat64(value float64) bool {
	return c.floatValues.contains(value)
}

// ContainsString reports whether value is in a string constraint without boxing it.
func (c ValueConstraint) ContainsString(value string) bool {
	return c.stringValues.contains(value)
}

// ContainsBool reports whether value is in a boolean constraint without boxing it.
func (c ValueConstraint) ContainsBool(value bool) bool {
	return c.boolValues.contains(value)
}
