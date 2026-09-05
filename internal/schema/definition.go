package schema

import (
	"bytes"
	"encoding/json"

	"github.com/ocsf/ocsf-toolkit/jsonio"
)

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
	Profiles    []string `json:"profiles,omitempty"`
	Extension   string   `json:"extension,omitempty"`
	ExtensionID int64    `json:"extension_id,omitempty"`

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
	Extension   string                              `json:"extension,omitempty"`
	ExtensionID int64                               `json:"extension_id,omitempty"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Caption     string   `json:"caption,omitempty"`
	// Description string   `json:"description,omitempty"`
	// Extends     string   `json:"extends,omitempty"`
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
	Extension   string `json:"extension,omitempty"`
	ExtensionID int64  `json:"extension_id,omitempty"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Annotations map[string]string      `json:"annotations,omitempty"`
	// Deprecated  *DeprecatedDefinition `json:"@deprecated,omitempty"`
	// Name        string                `json:"name"`
	// Caption     string                `json:"caption,omitempty"`
	// Description string                `json:"description,omitempty"`
	// Meta        string                `json:"meta,omitempty"`
	// References  []any                 `json:"references,omitempty"`
}

// Extension describes a schema extension included in a compiled OCSF schema.
type Extension struct {
	UID               int64  `json:"uid"`
	Name              string `json:"name"`
	Version           string `json:"version"`
	PlatformExtension bool   `json:"platform_extension?"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Caption     string `json:"caption,omitempty"`
	// Description string `json:"description,omitempty"`
}

// Definition is the decoded union of supported compiled schema formats.
type Definition struct {
	CompileVersion int                          `json:"compile_version"`
	Classes        map[string]*ClassDefinition  `json:"classes"`
	Objects        map[string]*ObjectDefinition `json:"objects"`
	Dictionary     *DictionaryDefinition        `json:"dictionary"`
	Profiles       map[string]ProfileDefinition `json:"profiles"`
	Extensions     map[string]*Extension        `json:"extensions,omitempty"`
	Version        string                       `json:"version"`

	// compile_version 1 fields intentionally not decoded because event processing does not use them:
	// Categories map[string]any `json:"categories,omitempty"`
}
