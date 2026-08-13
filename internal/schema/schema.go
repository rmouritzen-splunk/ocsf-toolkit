// Package schema loads and indexes compiled OCSF schemas for event processing.
package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"sync"

	"github.com/ocsf/ocsf-toolkit/internal/fserror"
	"github.com/ocsf/ocsf-toolkit/internal/semver"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/schemaresult"
)

const expectedCompileVersion = 1

// DeprecatedDefinition describes schema deprecation information.
type DeprecatedDefinition struct {
	Since   string `json:"since"`
	Message string `json:"message"`
}

// EnumDefinition describes one enum value.
type EnumDefinition struct {
	Deprecated *DeprecatedDefinition `json:"@deprecated,omitempty"`
	Caption    string                `json:"caption,omitempty"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Description string `json:"description,omitempty"`
	// References  []any  `json:"references,omitempty"`
	// Uid         int64  `json:"uid,omitempty"`

	captionValue any
}

// CaptionValue returns the caption preboxed for insertion into a JSON-like event map.
func (e *EnumDefinition) CaptionValue() any {
	if e == nil {
		return nil
	}
	if e.captionValue == nil {
		return e.Caption
	}
	return e.captionValue
}

// CommonAttributeDefinition contains fields shared by item attributes and dictionary types.
type CommonAttributeDefinition struct {
	Deprecated  *DeprecatedDefinition      `json:"@deprecated,omitempty"`
	Type        string                     `json:"type,omitempty"`
	Requirement string                     `json:"requirement,omitempty"`
	IsArray     *bool                      `json:"is_array,omitempty"`
	Enum        map[string]*EnumDefinition `json:"enum,omitempty"`
	Sibling     *string                    `json:"sibling,omitempty"`
	ObjectType  *string                    `json:"object_type,omitempty"`
	Observable  *int64                     `json:"observable,omitempty"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Caption        string   `json:"caption,omitempty"`
	// Description    string   `json:"description,omitempty"`
	// Extension      string   `json:"extension,omitempty"`
	// ExtensionID    int64    `json:"extension_id,omitempty"`
	// Group          *string  `json:"group,omitempty"`
	// ObjectName     string   `json:"object_name,omitempty"`
	// References     []any    `json:"references,omitempty"`
	// Source         string   `json:"source,omitempty"`
	// SuppressChecks []string `json:"suppress_checks,omitempty"`
	// TypeName       string   `json:"type_name,omitempty"`
}

// ItemAttributeDefinition describes an attribute on a class or object.
type ItemAttributeDefinition struct {
	CommonAttributeDefinition
	Profiles []string `json:"profiles,omitempty"`

	// PrimitiveType, ResolvedObject, ResolvedEnumSibling, and ResolvedEnumAttribute are initialized by
	// EnsureTraversalCache and remain immutable during event processing.
	PrimitiveType             string                   `json:"-"`
	ResolvedObject            *ObjectDefinition        `json:"-"`
	ResolvedEnumSibling       *ItemAttributeDefinition `json:"-"`
	ResolvedEnumAttribute     *ItemAttributeDefinition `json:"-"`
	ResolvedEnumAttributeName string                   `json:"-"`
	numericEnums              *numericEnumIndex
}

// OrderedAttribute pairs an item attribute name with its definition for deterministic traversal.
type OrderedAttribute struct {
	Name       string
	Definition *ItemAttributeDefinition
}

// ItemDefinition contains fields shared by class and object definitions.
type ItemDefinition struct {
	Deprecated  *DeprecatedDefinition               `json:"@deprecated,omitempty"`
	Name        string                              `json:"name"`
	Constraints map[string][]string                 `json:"constraints,omitempty"`
	Attributes  map[string]*ItemAttributeDefinition `json:"attributes,omitempty"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Caption     string   `json:"caption,omitempty"`
	// Description string   `json:"description,omitempty"`
	// Extends     string   `json:"extends,omitempty"`
	// Extension   string   `json:"extension,omitempty"`
	// ExtensionID int64    `json:"extension_id,omitempty"`
	// Profiles    []string `json:"profiles,omitempty"`
	// References  []any    `json:"references,omitempty"`

	OrderedAttributes     []OrderedAttribute `json:"-"`
	OrderedConstraintKeys []string           `json:"-"`
}

// ClassDefinition describes a compiled OCSF event class.
type ClassDefinition struct {
	ItemDefinition
	Uid         int64            `json:"uid"`
	Observables map[string]int64 `json:"observables,omitempty"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Associations map[string][]string `json:"associations,omitempty"`
	// Category     string              `json:"category"`
	// CategoryName string              `json:"category_name,omitempty"`
	// CategoryUID  int64               `json:"category_uid,omitempty"`
}

// ObjectDefinition describes a compiled OCSF object.
type ObjectDefinition struct {
	ItemDefinition
	Observable *int64 `json:"observable,omitempty"`
}

// TypeDefinition describes a compiled OCSF dictionary type.
type TypeDefinition struct {
	CommonAttributeDefinition
	MaxLen *int64  `json:"max_len,omitempty"`
	Range  []int64 `json:"range,omitempty"`
	RegEx  *string `json:"regex,omitempty"`
	Values []any   `json:"values,omitempty"`

	// compile_version 1 field intentionally not decoded because event processing does not use it:
	// TypeName *string `json:"type_name,omitempty"`
}

// UnmarshalJSON preserves exact number literals in values while the surrounding schema uses Unmarshal.
func (d *TypeDefinition) UnmarshalJSON(data []byte) error {
	type typeDefinitionWithoutValues TypeDefinition
	decoded := struct {
		typeDefinitionWithoutValues
		Values json.RawMessage `json:"values,omitempty"`
	}{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = TypeDefinition(decoded.typeDefinitionWithoutValues)
	if len(decoded.Values) == 0 {
		return nil
	}
	decoder := jsonio.NewDecoder(bytes.NewReader(decoded.Values))
	return decoder.Decode(&d.Values)
}

// TypesDefinition contains compiled OCSF dictionary types.
type TypesDefinition struct {
	Attributes map[string]*TypeDefinition `json:"attributes"`
}

// DictionaryDefinition contains compiled OCSF dictionary attributes and types.
type DictionaryDefinition struct {
	Attributes map[string]*CommonAttributeDefinition `json:"attributes"`
	Types      *TypesDefinition                      `json:"types,omitempty"`
}

// ProfileDefinition describes a compiled OCSF profile.
type ProfileDefinition struct {
	// compile_version 1 fields intentionally not decoded because event processing uses only profile map membership:
	// Annotations map[string]string      `json:"annotations,omitempty"`
	// Deprecated  *DeprecatedDefinition `json:"@deprecated,omitempty"`
	// Name        string                `json:"name"`
	// Caption     string                `json:"caption,omitempty"`
	// Description string                `json:"description,omitempty"`
	// Extension   string                `json:"extension,omitempty"`
	// ExtensionID int64                 `json:"extension_id,omitempty"`
	// Meta        string                `json:"meta,omitempty"`
	// References  []any                 `json:"references,omitempty"`
}

// Definition is the decoded union of supported compiled schema formats.
type Definition struct {
	CompileVersion int                          `json:"compile_version"`
	Classes        map[string]*ClassDefinition  `json:"classes"`
	Objects        map[string]*ObjectDefinition `json:"objects"`
	Dictionary     *DictionaryDefinition        `json:"dictionary"`
	Profiles       map[string]ProfileDefinition `json:"profiles"`
	Version        string                       `json:"version"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Categories map[string]any `json:"categories,omitempty"`
	// Extensions map[string]any `json:"extensions,omitempty"`
}

// Compiled is a normalized and indexed compiled OCSF schema.
type Compiled struct {
	Classes              map[int64]*ClassDefinition
	Objects              map[string]*ObjectDefinition
	Dictionary           *DictionaryDefinition
	Profiles             map[string]ProfileDefinition
	Version              string
	ObservableTypes      map[int64]string
	observableTypeValues map[int64]any
	observableTypeIDs    map[int64]any
	initializationIssues []schemaresult.InitializationIssue

	traversalCacheGuard sync.Once
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

// Load decodes and indexes a compiled OCSF schema file.
func Load(name string) (*Compiled, []schemaresult.InitializationIssue, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open schema file %q: %w", name, fserror.QuotePaths(err))
	}
	return load(data, name)
}

// LoadFS decodes and indexes a compiled OCSF schema file from fsys.
func LoadFS(fsys fs.FS, name string) (*Compiled, []schemaresult.InitializationIssue, error) {
	if fsys == nil {
		return nil, nil, errors.New("failed to open schema file: filesystem is nil")
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open schema file %q: %w", name, fserror.QuotePaths(err))
	}
	return load(data, name)
}

// load decodes and compiles data, describing the source as name in error messages, or generically if name is
// empty (LoadReader and LoadBytes have no file name to report).
func load(data []byte, name string) (*Compiled, []schemaresult.InitializationIssue, error) {
	definition, err := decode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode schema%s: %w", namedSuffix(name), err)
	}
	return loadDefinition(definition, name)
}

func loadDefinition(
	definition *Definition,
	name string,
) (*Compiled, []schemaresult.InitializationIssue, error) {
	compiled, err := New(definition)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load schema%s: %w", namedSuffix(name), err)
	}
	return compiled, compiled.InitializationIssues(), nil
}

func namedSuffix(name string) string {
	if name == "" {
		return ""
	}
	return fmt.Sprintf(" file %q", name)
}

// LoadReader decodes and indexes a compiled OCSF schema from reader.
func LoadReader(reader io.Reader) (*Compiled, []schemaresult.InitializationIssue, error) {
	if reader == nil {
		return nil, nil, errors.New("failed to read schema from reader: reader is nil")
	}
	definition, err := decodeReader(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode schema from reader: %w", fserror.QuotePaths(err))
	}
	return loadDefinition(definition, "")
}

// LoadBytes decodes and indexes a compiled OCSF schema from data.
func LoadBytes(data []byte) (*Compiled, []schemaresult.InitializationIssue, error) {
	return load(data, "")
}

func decode(data []byte) (*Definition, error) {
	var definition Definition
	if err := json.Unmarshal(data, &definition); err != nil {
		return nil, err
	}
	return &definition, nil
}

func decodeReader(reader io.Reader) (*Definition, error) {
	var definition Definition
	decoder := jsonio.NewDecoder(reader)
	if err := decoder.Decode(&definition); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return &definition, nil
	} else if err != nil {
		return nil, err
	}
	return nil, errors.New("unexpected trailing JSON value")
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

// EnsureTraversalCache initializes immutable schema data used by event traversal.
func (s *Compiled) EnsureTraversalCache() {
	s.traversalCacheGuard.Do(func() {
		classIDs := make([]int64, 0, len(s.Classes))
		for classID := range s.Classes {
			classIDs = append(classIDs, classID)
		}
		slices.Sort(classIDs)
		for _, classID := range classIDs {
			class := s.Classes[classID]
			if class != nil {
				setItemTraversalCache(s, "class", class.Name, &class.ItemDefinition)
				s.initializeItemNumericEnums(&class.ItemDefinition)
			}
		}
		for _, objectName := range sortedKeys(s.Objects) {
			object := s.Objects[objectName]
			if object != nil {
				setItemTraversalCache(s, "object", objectName, &object.ItemDefinition)
				s.initializeItemNumericEnums(&object.ItemDefinition)
			}
		}
	})
}

// InitializationIssues returns nonfatal schema conditions found while constructing event-processing caches.
func (s *Compiled) InitializationIssues() []schemaresult.InitializationIssue {
	s.EnsureTraversalCache()
	issues := make([]schemaresult.InitializationIssue, len(s.initializationIssues))
	for index, found := range s.initializationIssues {
		found.Details = maps.Clone(found.Details)
		issues[index] = found
	}
	return issues
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

func setItemTraversalCache(schema *Compiled, itemType, itemName string, item *ItemDefinition) {
	item.OrderedAttributes = make([]OrderedAttribute, 0, len(item.Attributes))
	for name, definition := range item.Attributes {
		if definition != nil {
			// Loaded schemas have already passed type-inheritance cycle validation. A validation-enabled pipeline
			// independently propagates this defensive error for internally constructed schemas.
			definition.PrimitiveType, _ = schema.ResolvePrimitiveType(definition.Type)
			if definition.ObjectType != nil {
				definition.ResolvedObject = schema.Objects[*definition.ObjectType]
			}
		}
		item.OrderedAttributes = append(item.OrderedAttributes, OrderedAttribute{Name: name, Definition: definition})
	}
	sort.Slice(item.OrderedAttributes, func(left, right int) bool {
		return item.OrderedAttributes[left].Name < item.OrderedAttributes[right].Name
	})
	for _, attribute := range item.OrderedAttributes {
		definition := attribute.Definition
		if definition == nil || definition.Enum == nil || definition.Sibling == nil {
			continue
		}
		sibling := item.Attributes[*definition.Sibling]
		found := enumSiblingInitializationIssue(itemType, itemName, attribute.Name, definition, sibling)
		if found != nil {
			schema.initializationIssues = append(schema.initializationIssues, *found)
			continue
		}
		definition.ResolvedEnumSibling = sibling
		sibling.ResolvedEnumAttribute = definition
		sibling.ResolvedEnumAttributeName = attribute.Name
	}
	item.OrderedConstraintKeys = sortedKeys(item.Constraints)
}

func enumSiblingInitializationIssue(
	itemType string,
	itemName string,
	attributeName string,
	attribute *ItemAttributeDefinition,
	sibling *ItemAttributeDefinition,
) *schemaresult.InitializationIssue {
	siblingName := *attribute.Sibling
	baseDetails := jsonish.Map{
		"item_type": itemType,
		"item_name": itemName,
		"attribute": attributeName,
		"sibling":   siblingName,
	}
	if attribute.Type != "integer_t" && attribute.Type != "long_t" {
		baseDetails["attribute_type"] = attribute.Type
		baseDetails["attribute_is_array"] = isArray(attribute)
		return &schemaresult.InitializationIssue{
			Code: issue.AtInitSchemaEnumSiblingSourceNotIntegral,
			Message: fmt.Sprintf(
				"Schema %s %q enum attribute %q with sibling %q must have direct type integer_t or long_t.",
				itemType, itemName, attributeName, siblingName,
			),
			Details: baseDetails,
		}
	}
	if sibling == nil {
		return &schemaresult.InitializationIssue{
			Code: issue.AtInitSchemaEnumSiblingTargetNotFound,
			Message: fmt.Sprintf(
				"Schema %s %q enum attribute %q names missing sibling %q.",
				itemType, itemName, attributeName, siblingName,
			),
			Details: baseDetails,
		}
	}
	if sibling.Enum != nil {
		baseDetails["sibling_type"] = sibling.Type
		baseDetails["sibling_is_array"] = isArray(sibling)
		return &schemaresult.InitializationIssue{
			Code: issue.AtInitSchemaEnumSiblingTargetIsEnum,
			Message: fmt.Sprintf(
				"Schema %s %q enum attribute %q names sibling %q, which is itself an enum.",
				itemType, itemName, attributeName, siblingName,
			),
			Details: baseDetails,
		}
	}
	if sibling.Type != "string_t" || isArray(attribute) != isArray(sibling) {
		baseDetails["expected_type"] = "string_t"
		baseDetails["expected_is_array"] = isArray(attribute)
		baseDetails["actual_type"] = sibling.Type
		baseDetails["actual_is_array"] = isArray(sibling)
		return &schemaresult.InitializationIssue{
			Code: issue.AtInitSchemaEnumSiblingTargetNotString,
			Message: fmt.Sprintf(
				"Schema %s %q enum attribute %q names sibling %q with an incompatible direct type or array shape.",
				itemType, itemName, attributeName, siblingName,
			),
			Details: baseDetails,
		}
	}
	return nil
}

func sortedKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
