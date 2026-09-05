// Package schema loads and indexes compiled OCSF schemas for event processing.
package schema

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/ocsf/ocsf-toolkit/internal/semver"
	"github.com/ocsf/ocsf-toolkit/schemaresult"
)

const expectedCompileVersion = 1

// Compiled is a normalized and indexed compiled OCSF schema.
type Compiled struct {
	Classes              map[int64]*ClassDefinition
	Objects              map[string]*ObjectDefinition
	Dictionary           *DictionaryDefinition
	Profiles             map[string]ProfileDefinition
	Extensions           []Extension
	Version              string
	ObservableTypes      map[int64]string
	observableTypeValues map[int64]any
	observableTypeIDs    map[int64]any
	initializationIssues []schemaresult.InitializationIssue

	// traversalCacheOnce ensures traversal metadata throughout the compiled schema is initialized once.
	traversalCacheOnce  sync.Once
	validationCacheOnce sync.Once
	validationCache     ValidationCache
	validationCacheErr  error
}

// TypeDerivedFrom reports whether typeName is baseType or directly derives from it through the compiled type
// dictionary.
func (s *Compiled) TypeDerivedFrom(typeName, baseType string) bool {
	if s == nil || s.Dictionary == nil || s.Dictionary.Types == nil {
		return typeName == baseType
	}
	types := s.Dictionary.Types.Attributes
	for remaining := len(types) + 1; typeName != "" && remaining > 0; remaining-- {
		if typeName == baseType {
			return true
		}
		typeDef := types[typeName]
		if typeDef == nil {
			return false
		}
		typeName = typeDef.Type
	}
	return false
}

// ResolvePrimitiveType follows dictionary type inheritance to its terminal primitive or unknown type. It reports
// cyclic inheritance for internally constructed compiled schemas that bypass schema loading validation.
func (s *Compiled) ResolvePrimitiveType(typeName string) (string, error) {
	if s == nil || s.Dictionary == nil || s.Dictionary.Types == nil {
		return typeName, nil
	}
	types := s.Dictionary.Types.Attributes
	currentName := typeName
	seen := make(map[string]struct{})
	for {
		if isPrimitiveTypeName(currentName) {
			return currentName, nil
		}
		if _, repeated := seen[currentName]; repeated {
			return "", fmt.Errorf("type inheritance for %q contains a cycle at %q", typeName, currentName)
		}
		seen[currentName] = struct{}{}
		typeDef := types[currentName]
		if typeDef == nil || typeDef.Type == "" {
			return currentName, nil
		}
		currentName = typeDef.Type
	}
}

// DictionaryType returns the definition of a named dictionary type, or nil when it is unavailable.
func (s *Compiled) DictionaryType(typeName string) *TypeDefinition {
	if s == nil || s.Dictionary == nil || s.Dictionary.Types == nil {
		return nil
	}
	return s.Dictionary.Types.Attributes[typeName]
}

// New normalizes and indexes a decoded compiled OCSF schema definition.
func New(definition *Definition) (*Compiled, error) {
	if definition == nil {
		return nil, errors.New("compiled schema definition is nil")
	}
	if definition.CompileVersion != expectedCompileVersion {
		return nil, fmt.Errorf("unsupported compile_version: %d", definition.CompileVersion)
	}
	normalize(definition)
	if err := validateDefinitions(definition); err != nil {
		return nil, err
	}
	if len(definition.Classes) == 0 {
		return nil, errors.New("compiled schema is missing classes")
	}
	if _, ok := semver.Parse(definition.Version); !ok {
		return nil, fmt.Errorf("compiled schema version %q has invalid format", definition.Version)
	}

	classes := make(map[int64]*ClassDefinition, len(definition.Classes))
	for _, class := range definition.Classes {
		if existing, present := classes[class.Uid]; present {
			return nil, fmt.Errorf(
				"compiled schema has duplicate class uid %d for classes %q and %q",
				class.Uid,
				existing.Name,
				class.Name,
			)
		}
		classes[class.Uid] = class
	}
	boxEnumCaptions(definition)
	observableTypes, observableTypeValues, observableTypeIDs, err := makeObservableTypes(definition.Objects)
	if err != nil {
		return nil, err
	}
	compiled := &Compiled{
		Classes:              classes,
		Objects:              definition.Objects,
		Dictionary:           definition.Dictionary,
		Profiles:             definition.Profiles,
		Extensions:           makeExtensions(definition.Extensions),
		Version:              definition.Version,
		ObservableTypes:      observableTypes,
		observableTypeValues: observableTypeValues,
		observableTypeIDs:    observableTypeIDs,
	}
	if err := validateEnumSiblingRelationships(compiled, definition); err != nil {
		return nil, err
	}
	return compiled, nil
}

func makeExtensions(definitions map[string]*Extension) []Extension {
	extensions := make([]Extension, 0, len(definitions))
	for _, name := range sortedKeys(definitions) {
		definition := definitions[name]
		if definition != nil {
			extensions = append(extensions, *definition)
		}
	}
	return extensions
}

func validateDefinitions(definition *Definition) error {
	for _, className := range sortedKeys(definition.Classes) {
		class := definition.Classes[className]
		if class == nil {
			return fmt.Errorf("compiled schema class %q is null", className)
		}
		if err := validateItemAttributeDefinitions("class", className, class.Attributes); err != nil {
			return err
		}
	}
	for _, objectName := range sortedKeys(definition.Objects) {
		object := definition.Objects[objectName]
		if object == nil {
			return fmt.Errorf("compiled schema object %q is null", objectName)
		}
		if err := validateItemAttributeDefinitions("object", objectName, object.Attributes); err != nil {
			return err
		}
	}
	for _, attributeName := range sortedKeys(definition.Dictionary.Attributes) {
		if definition.Dictionary.Attributes[attributeName] == nil {
			return fmt.Errorf("compiled schema dictionary attribute %q is null", attributeName)
		}
	}
	for _, typeName := range sortedKeys(definition.Dictionary.Types.Attributes) {
		if definition.Dictionary.Types.Attributes[typeName] == nil {
			return fmt.Errorf("compiled schema dictionary type %q is null", typeName)
		}
	}
	if err := validateTypeInheritance(definition.Dictionary.Types.Attributes); err != nil {
		return err
	}
	return nil
}

func validateTypeInheritance(types map[string]*TypeDefinition) error {
	const (
		typeUnvisited uint8 = iota
		typeVisiting
		typeVisited
	)
	states := make(map[string]uint8, len(types))
	for _, typeName := range sortedKeys(types) {
		if states[typeName] == typeVisited {
			continue
		}
		currentName := typeName
		path := make([]string, 0)
	walk:
		for !isPrimitiveTypeName(currentName) {
			switch states[currentName] {
			case typeVisiting:
				return fmt.Errorf(
					"compiled schema dictionary type inheritance for %q contains a cycle at %q",
					typeName,
					currentName,
				)
			case typeVisited:
				break walk
			}
			states[currentName] = typeVisiting
			path = append(path, currentName)
			typeDef := types[currentName]
			if typeDef == nil || typeDef.Type == "" {
				break
			}
			currentName = typeDef.Type
		}
		for _, visitedName := range path {
			states[visitedName] = typeVisited
		}
	}
	return nil
}

func isPrimitiveTypeName(typeName string) bool {
	switch typeName {
	case "boolean_t", "float_t", "integer_t", "long_t", "string_t", "json_t":
		return true
	default:
		return false
	}
}

func validateItemAttributeDefinitions(
	itemKind string,
	itemName string,
	attributes map[string]*ItemAttributeDefinition,
) error {
	for _, attributeName := range sortedKeys(attributes) {
		if attributes[attributeName] == nil {
			return fmt.Errorf(
				"compiled schema %s %q attribute %q is null", itemKind, itemName, attributeName,
			)
		}
	}
	return nil
}

func validateEnumSiblingRelationships(compiled *Compiled, definition *Definition) error {
	for _, className := range sortedKeys(definition.Classes) {
		class := definition.Classes[className]
		err := validateItemEnumSiblingRelationships(compiled, "class", className, &class.ItemDefinition)
		if err != nil {
			return err
		}
	}
	for _, objectName := range sortedKeys(definition.Objects) {
		object := definition.Objects[objectName]
		if object == nil {
			continue
		}
		err := validateItemEnumSiblingRelationships(compiled, "object", objectName, &object.ItemDefinition)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateItemEnumSiblingRelationships(
	compiled *Compiled,
	itemKind string,
	itemName string,
	item *ItemDefinition,
) error {
	siblingOf := make(map[string]string)
	for _, attributeName := range sortedKeys(item.Attributes) {
		attribute := item.Attributes[attributeName]
		if attribute == nil || attribute.Enum == nil || attribute.Sibling == nil {
			continue
		}
		siblingName := *attribute.Sibling
		sibling := item.Attributes[siblingName]
		if !compiled.EnumSiblingSupported(attribute, sibling) {
			continue
		}
		if owner, present := siblingOf[siblingName]; present {
			return fmt.Errorf(
				"compiled schema %s %q enum attributes %q and %q cannot share sibling %q",
				itemKind, itemName, owner, attributeName, siblingName,
			)
		}
		siblingOf[siblingName] = attributeName
	}
	return nil
}

func normalize(definition *Definition) {
	if definition.Dictionary == nil {
		definition.Dictionary = &DictionaryDefinition{}
	}
	if definition.Dictionary.Attributes == nil {
		definition.Dictionary.Attributes = make(map[string]*CommonAttributeDefinition)
	}
	if definition.Dictionary.Types == nil {
		definition.Dictionary.Types = &TypesDefinition{}
	}
	if definition.Dictionary.Types.Attributes == nil {
		definition.Dictionary.Types.Attributes = make(map[string]*TypeDefinition)
	}
	if definition.Classes == nil {
		definition.Classes = make(map[string]*ClassDefinition)
	}
	if definition.Objects == nil {
		definition.Objects = make(map[string]*ObjectDefinition)
	}
	if definition.Profiles == nil {
		definition.Profiles = make(map[string]ProfileDefinition)
	}
}

func makeObservableTypes(objects map[string]*ObjectDefinition) (map[int64]string, map[int64]any, map[int64]any, error) {
	observableObject := objects["observable"]
	if observableObject != nil {
		typeID := observableObject.Attributes["type_id"]
		if typeID != nil && typeID.Enum != nil {
			observableTypes := make(map[int64]string, len(typeID.Enum))
			observableTypeValues := make(map[int64]any, len(typeID.Enum))
			observableTypeIDs := make(map[int64]any, len(typeID.Enum))
			seenTypeIDs := make(map[int64]string, len(typeID.Enum))
			for _, typeIDText := range sortedKeys(typeID.Enum) {
				enum := typeID.Enum[typeIDText]
				if enum == nil {
					return nil, nil, nil, fmt.Errorf("observable type enum %q has a null definition", typeIDText)
				}
				value, err := strconv.ParseInt(typeIDText, 10, 64)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("observable type enum ID %q is invalid: %w", typeIDText, err)
				}
				if previous, duplicate := seenTypeIDs[value]; duplicate {
					return nil, nil, nil, fmt.Errorf(
						"observable type enum IDs %q and %q both normalize to %d",
						previous,
						typeIDText,
						value,
					)
				}
				seenTypeIDs[value] = typeIDText
				observableTypes[value] = enum.Caption
				observableTypeValues[value] = enum.CaptionValue()
				observableTypeIDs[value] = value
			}
			return observableTypes, observableTypeValues, observableTypeIDs, nil
		}
	}
	return make(map[int64]string), make(map[int64]any), make(map[int64]any), nil
}

// ObservableTypeIDValue returns an observable type ID preboxed for insertion into a JSON-like event map.
func (s *Compiled) ObservableTypeIDValue(typeID int64) (any, bool) {
	if s == nil {
		return nil, false
	}
	value, present := s.observableTypeIDs[typeID]
	return value, present
}

// ObservableTypeValue returns an observable type caption preboxed for insertion into a JSON-like event map.
func (s *Compiled) ObservableTypeValue(typeID int64) (any, bool) {
	if s == nil {
		return nil, false
	}
	value, present := s.observableTypeValues[typeID]
	return value, present
}

func boxEnumCaptions(definition *Definition) {
	boxItemEnumCaptions := func(item *ItemDefinition) {
		for _, attribute := range item.Attributes {
			if attribute == nil {
				continue
			}
			for _, enum := range attribute.Enum {
				if enum != nil {
					enum.captionValue = enum.Caption
				}
			}
		}
	}
	for _, class := range definition.Classes {
		if class != nil {
			boxItemEnumCaptions(&class.ItemDefinition)
		}
	}
	for _, object := range definition.Objects {
		if object != nil {
			boxItemEnumCaptions(&object.ItemDefinition)
		}
	}
}

func sortedKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
