package eventschema

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/ocsf/ocsf-processor/jsonish"
)

const expectedCompileVersion = 1

// Schema is the top level interface for functions related to a specific OCSF schema version.
type Schema interface {
	// Enrich event in-place, adding OCSF enum siblings and/or OCSF observables based on flags.
	// If neither flag is true, the event is unmodified.
	// If event is malformed, it is ignored. A malformed event is one where class_uid is missing, invalid, or unknown.
	Enrich(event jsonish.Map, addEnumSiblings, addObservables bool) error
}

func New(name string) (Schema, error) {
	var f *os.File
	var err error
	if f, err = os.Open(name); err != nil {
		return nil, fmt.Errorf("failed to open schema file %q: %w", name, err)
	}
	defer func(f *os.File) { _ = f.Close() }(f)
	var sd schemaDefinition
	decoder := json.NewDecoder(f)
	if err = decoder.Decode(&sd); err != nil {
		return nil, fmt.Errorf("failed to decode schema file %q: %w", name, err)
	}
	schema, err := newSchemaImpl(&sd)
	if err != nil {
		return nil, fmt.Errorf("failed to load schema file %q: %w", name, err)
	}
	return schema, nil
}

func newSchemaImpl(sd *schemaDefinition) (*schemaImpl, error) {
	if sd.CompileVersion != expectedCompileVersion {
		return nil, fmt.Errorf("unsupported compile_version: %d", sd.CompileVersion)
	}

	// transform classes map of class names (like "base_event") to class definitions, to class uid to class definition
	classes := make(map[int32]*classDefinition, len(sd.Classes))
	for _, definition := range sd.Classes {
		classes[definition.Uid] = definition
	}
	return &schemaImpl{
		classes:         classes,
		objects:         sd.Objects,
		dictionary:      sd.Dictionary,
		profiles:        sd.Profiles,
		version:         sd.Version,
		observableTypes: makeObservableTypes(sd.Objects),
	}, nil
}

func makeObservableTypes(objects map[string]*objectDefinition) map[int32]string {
	observableObjectDef, objectDefPresent := objects["observable"]
	if objectDefPresent && observableObjectDef != nil {
		typeIDDef, typeIDDefPresent := observableObjectDef.Attributes["type_id"]
		if typeIDDefPresent && typeIDDef != nil && typeIDDef.Enum != nil {
			observableTypes := make(map[int32]string, len(typeIDDef.Enum))
			for typeIDStr, enumDef := range typeIDDef.Enum {
				i, err := strconv.ParseInt(typeIDStr, 10, 32)
				if err == nil { // add if successfully parsed int, otherwise ignore err
					observableTypes[int32(i)] = enumDef.Caption
				}
			}
			return observableTypes
		}
	}
	return make(map[int32]string)
}

type deprecatedDefinition struct {
	Since   string `json:"since"`
	Message string `json:"message"`
}

type enumDefinition struct {
	Caption     string `json:"caption,omitempty"`
	Description string `json:"description,omitempty"`
}

type commonAttributeDefinition struct {
	Deprecated  *deprecatedDefinition      `json:"@deprecated,omitempty"`
	Caption     string                     `json:"caption,omitempty"`
	Description string                     `json:"description,omitempty"`
	Type        string                     `json:"type,omitempty"`
	Requirement string                     `json:"requirement,omitempty"`
	IsArray     *bool                      `json:"is_array,omitempty"`
	Group       *string                    `json:"group,omitempty"`
	Enum        map[string]*enumDefinition `json:"enum,omitempty"`
	Sibling     *string                    `json:"sibling,omitempty"`
	ObjectType  *string                    `json:"object_type,omitempty"`
	Observable  *int32                     `json:"observable,omitempty"`
}

type itemAttributeDefinition struct {
	commonAttributeDefinition
	Profiles []string `json:"profiles,omitempty"`
}

// commonItemDefinition is the common structure shared by classes and objects.
// (The term "item" is used as the generic name an object or class).
type commonItemDefinition struct {
	Deprecated  *deprecatedDefinition               `json:"@deprecated,omitempty"`
	Name        string                              `json:"name"`
	Caption     string                              `json:"caption,omitempty"`
	Description string                              `json:"description,omitempty"`
	Profiles    []string                            `json:"profiles,omitempty"`
	Attributes  map[string]*itemAttributeDefinition `json:"attributes,omitempty"`
}

type classDefinition struct {
	commonItemDefinition
	Uid         int32            `json:"uid"`
	Category    string           `json:"category"`
	Observables map[string]int32 `json:"observables,omitempty"`
}

type objectDefinition struct {
	commonItemDefinition
	Observable *int32 `json:"observable,omitempty"`
}

type typeDefinition struct {
	commonAttributeDefinition
	TypeName *string `json:"type_name,omitempty"`
	MaxLen   *int32  `json:"max_len,omitempty"`
	Range    []int32 `json:"range,omitempty"`
	RegEx    *string `json:"regex,omitempty"`
	Values   []any   `json:"values,omitempty"`
}

type typesDefinition struct {
	Attributes map[string]*typeDefinition `json:"attributes"`
}
type dictionaryDefinition struct {
	Attributes map[string]*commonAttributeDefinition `json:"attributes"`
	Types      *typesDefinition                      `json:"types,omitempty"`
}

type profileDefinition struct {
	Deprecated  *deprecatedDefinition `json:"@deprecated,omitempty"`
	Name        string                `json:"name"`
	Caption     string                `json:"caption,omitempty"`
	Description string                `json:"description,omitempty"`
}

// schemaDefinition is the union of supported compiled schema formats.
type schemaDefinition struct {
	CompileVersion int                           `json:"compile_version"`
	Classes        map[string]*classDefinition   `json:"classes"`
	Objects        map[string]*objectDefinition  `json:"objects"`
	Dictionary     *dictionaryDefinition         `json:"dictionary"`
	Profiles       map[string]*profileDefinition `json:"profiles"`
	Version        string                        `json:"version"`
}

// schemaImpl is a lightly transformed variation of schemaDefinition that is more useful for enrichment and validation.
type schemaImpl struct {
	schemaDefinition
	classes         map[int32]*classDefinition
	objects         map[string]*objectDefinition
	dictionary      *dictionaryDefinition
	profiles        map[string]*profileDefinition
	version         string
	observableTypes map[int32]string
}
