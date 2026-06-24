package eventschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-processor/jsonish"
)

const testSchemaFilePath = "../test/schema_v1.8.0.json"
const testSchemaVersion = "1.8.0"

func TestLoadSchemaFromFile(t *testing.T) {
	assert := require.New(t)
	schema, err := New(testSchemaFilePath)
	assert.NoError(err)
	checkSchema(assert, schema)
}

func checkSchema(assert *require.Assertions, schema Schema) {
	assert.NotNil(schema)
	si := schema.(*schemaImpl) // cast back to *schemaImpl to interrogate

	assert.Equal(testSchemaVersion, si.version)
	assert.NotEmpty(si.classes, "classes")
	assert.NotEmpty(si.objects, "objects")
	assert.NotEmpty(si.dictionary, "dictionary")
	assert.NotEmpty(si.profiles, "profiles")

	var err error

	emptyEvent := jsonish.Map{}
	err = schema.Enrich(emptyEvent, true, true)
	assert.Error(err, "missing uid should fail")
	assert.Equal(jsonish.Map{}, emptyEvent, "empty event should remain empty after error")

	err = schema.Enrich(emptyEvent, false, false)
	assert.NoError(err, "should not fail when no enrichment asked for")
	assert.Equal(jsonish.Map{}, emptyEvent, "empty event should remain empty after no enrichment")

	// negative numbered class uid values are never used
	undefinedClassEvent := jsonish.Map{"class_uid": -1}
	err = schema.Enrich(undefinedClassEvent, true, true)
	assert.Error(err, "undefined class uid")
	assert.Equal(jsonish.Map{"class_uid": -1}, undefinedClassEvent, "undefined class event should remain unchanged")

	badUidEvent := jsonish.Map{"class_uid": "bogus"}
	err = schema.Enrich(badUidEvent, true, true)
	assert.Error(err, "bad class uid")
	assert.Equal(jsonish.Map{"class_uid": "bogus"}, badUidEvent, "bad uid event should remain unchanged")
}

func makeTestSchema(assert *require.Assertions) *schemaImpl {
	classNameAttribute := commonAttributeDefinition{
		Caption: "Class Name",
		Type:    "string_t",
	}
	nameAttribute := commonAttributeDefinition{
		Caption: "Name",
		Type:    "string_t",
	}
	typeStr := "type"
	typeIDAttribute := commonAttributeDefinition{
		Caption: "Type ID",
		Type:    "integer_t",
		Sibling: &typeStr,
	}
	typeAttribute := commonAttributeDefinition{
		Caption: "Type",
		Type:    "string_t",
	}
	valueAttribute := commonAttributeDefinition{
		Caption: "Value",
		Type:    "string_t",
	}

	redAttribute := commonAttributeDefinition{
		Caption: "Red",
		Type:    "string_t",
	}
	greenAttribute := commonAttributeDefinition{
		Caption: "Green",
		Type:    "string_t",
	}

	ballStr := "ball"
	ballAttribute := commonAttributeDefinition{
		Caption:    "Ball",
		Type:       "object_t",
		ObjectType: &ballStr,
	}
	observableStr := "observable"
	trueValue := true
	observablesAttribute := commonAttributeDefinition{
		Caption:    "Observables",
		Type:       "object_t",
		ObjectType: &observableStr,
		IsArray:    &trueValue,
	}

	classNameStr := "class_name"
	classes := map[string]*classDefinition{
		"alpha": {
			Uid:      int32(1),
			Category: "Greek",
			Observables: map[string]int32{
				"ball.green": 1000,
			},
			commonItemDefinition: commonItemDefinition{
				Name: "Alpha",
				Attributes: map[string]*itemAttributeDefinition{
					"class_uid": {
						commonAttributeDefinition: commonAttributeDefinition{
							Caption: "Class UID",
							Type:    "integer_t",
							Enum: map[string]*enumDefinition{
								"1": {Caption: "Alpha"},
							},
							Sibling: &classNameStr,
						},
					},
					"class_name": {
						commonAttributeDefinition: classNameAttribute,
					},
					"observables": {
						commonAttributeDefinition: observablesAttribute,
					},
					"red": {
						commonAttributeDefinition: redAttribute,
					},
					"ball": {
						commonAttributeDefinition: ballAttribute,
					},
				},
			},
		},
	}
	objects := map[string]*objectDefinition{
		"ball": {
			commonItemDefinition: commonItemDefinition{
				Name: "Ball",
				Attributes: map[string]*itemAttributeDefinition{
					"green": {
						commonAttributeDefinition: greenAttribute,
					},
				},
			},
		},
		"observable": {
			commonItemDefinition: commonItemDefinition{
				Name: "Observable",
				Attributes: map[string]*itemAttributeDefinition{
					"name": {
						commonAttributeDefinition: nameAttribute,
					},
					"type_id": {
						commonAttributeDefinition: commonAttributeDefinition{
							Caption: "Type ID",
							Type:    "integer_t",
							Enum: map[string]*enumDefinition{
								"1000": {
									Caption: "Class path ball.green",
								},
							},
							Sibling: &typeStr,
						},
					},
					"type": {
						commonAttributeDefinition: typeAttribute,
					},
					"value": {
						commonAttributeDefinition: valueAttribute,
					},
				},
			},
		},
	}
	dictionaryTypes := &typesDefinition{
		Attributes: map[string]*typeDefinition{
			"integer_t": {
				commonAttributeDefinition: commonAttributeDefinition{
					Caption: "Integer",
				},
			},
			"string_t": {
				commonAttributeDefinition: commonAttributeDefinition{
					Caption: "String",
				},
			},
		},
	}
	dictionaryAttributes := map[string]*commonAttributeDefinition{
		"class_uid": {
			Caption: "Class UID",
			Type:    "integer_t",
			Sibling: &classNameStr,
		},
		"class_name":  &classNameAttribute,
		"name":        &nameAttribute,
		"type_id":     &typeIDAttribute,
		"type":        &typeAttribute,
		"value":       &valueAttribute,
		"observables": &observablesAttribute,
		"red":         &redAttribute,
		"green":       &greenAttribute,
		"ball":        &ballAttribute,
	}
	dictionary := &dictionaryDefinition{
		Attributes: dictionaryAttributes,
		Types:      dictionaryTypes,
	}
	sd := &schemaDefinition{
		CompileVersion: 1,
		Classes:        classes,
		Objects:        objects,
		Dictionary:     dictionary,
		Version:        "0.1.0",
	}

	si, err := newSchemaImpl(sd)
	assert.NoError(err)
	assert.NotNil(si, "schemaImpl should not be nil")
	return si
}

func TestNewSchemaImplWithModernCompiledSchema(t *testing.T) {
	assert := require.New(t)
	si := newTestSchemaFromJSON(assert, `{
		"compile_version": 1,
		"version": "1.2.3",
		"classes": {
			"alpha": {
				"name": "Alpha",
				"uid": 1,
				"category": "Greek",
				"attributes": {
					"class_uid": {
						"caption": "Class UID",
						"type": "integer_t",
						"enum": {
							"1": {
								"caption": "Alpha"
							}
						},
						"sibling": "class_name"
					},
					"profiles_attr": {
						"caption": "Profiles Attr",
						"type": "string_t",
						"profiles": [
							"alpha_profile",
							"beta_profile"
						]
					}
				}
			}
		},
		"objects": {
			"observable": {
				"name": "Observable",
				"attributes": {
					"type_id": {
						"caption": "Type ID",
						"type": "integer_t",
						"enum": {
							"1000": {
								"caption": "Modern Observable"
							}
						}
					}
				}
			}
		},
		"dictionary": {
			"attributes": {
				"class_uid": {
					"caption": "Class UID",
					"type": "integer_t"
				}
			},
			"types": {
				"attributes": {
					"integer_t": {
						"caption": "Integer",
						"observable": 1000
					}
				}
			}
		}
	}`)

	assert.Equal("1.2.3", si.version)
	assert.NotNil(si.classes[int32(1)])
	assert.NotNil(si.objects["observable"])
	assert.Equal("Integer", si.dictionary.Types.Attributes["integer_t"].Caption)
	assert.Equal("Class UID", si.dictionary.Attributes["class_uid"].Caption)
	assert.Equal("Modern Observable", si.observableTypes[int32(1000)])
}

func TestNewSchemaImplWithUnsupportedCompiledSchemaVersion(t *testing.T) {
	assert := require.New(t)
	var sd schemaDefinition
	err := json.Unmarshal([]byte(`{"compile_version":2}`), &sd)
	assert.NoError(err)

	si, err := newSchemaImpl(&sd)
	assert.Nil(si)
	assert.EqualError(err, "unsupported compile_version: 2")
}

func newTestSchemaFromJSON(assert *require.Assertions, data string) *schemaImpl {
	var sd schemaDefinition
	err := json.Unmarshal([]byte(data), &sd)
	assert.NoError(err)

	si, err := newSchemaImpl(&sd)
	assert.NoError(err)
	assert.NotNil(si)
	return si
}

func TestClassObservablesWithSiblings(t *testing.T) {
	assert := require.New(t)
	si := makeTestSchema(assert)

	event := jsonish.Map{
		"class_uid": int32(1),
		"red":       "A red thing",
		"ball": jsonish.Map{
			"green": "A green thing",
		},
	}

	err := si.Enrich(event, true, true)
	assert.NoError(err)
	assert.Equal("Alpha", event["class_name"])
	assert.NotNil(event["observables"])
	observables, err := jsonish.GetSliceOfMaps(event, "observables")
	assert.NoError(err)
	assert.Len(observables, 1)
	observable := observables[0]
	assert.Equal("ball.green", observable["name"])
	assert.Equal(int32(1000), observable["type_id"])
	assert.Equal("Class path ball.green", observable["type"])
	assert.Equal("A green thing", observable["value"])
}

func TestClassObservablesWithoutSiblings(t *testing.T) {
	assert := require.New(t)
	si := makeTestSchema(assert)

	// same as above but without add enum siblings
	event := jsonish.Map{
		"class_uid": int32(1),
		"red":       "A red thing",
		"ball": jsonish.Map{
			"green": "A green thing",
		},
	}

	err := si.Enrich(event, false, true)
	assert.NoError(err)
	assert.NotContains(event, "class_name")
	assert.NotNil(event["observables"])
	observables, err := jsonish.GetSliceOfMaps(event, "observables")
	assert.NoError(err)
	assert.Len(observables, 1)
	observable := observables[0]
	assert.Equal("ball.green", observable["name"])
	assert.Equal(int32(1000), observable["type_id"])
	assert.NotContains(observable, "type")
	assert.Equal("A green thing", observable["value"])
}
