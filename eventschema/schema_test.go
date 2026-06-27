package eventschema

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

const testSchemaFilePath = "../test/schema_v1.8.0.json"
const testSchemaVersion = "1.8.0"

func testPtrTo[T any](value T) *T {
	return &value
}

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
	processor := schema.NewEventProcessor(NewValidation(), NewEnrichment())
	result, err := processor.ProcessEvent(emptyEvent)
	assert.NoError(err)
	assert.Len(result.Validation.Errors, 1, "missing uid should fail validation")
	assert.Equal(jsonish.Map{}, emptyEvent, "empty event should remain empty after error")

	processor = schema.NewEventProcessor()
	result, err = processor.ProcessEvent(emptyEvent)
	assert.NoError(err, "should not fail when no enrichment asked for")
	assert.Empty(result.Validation.Errors)
	assert.Equal(jsonish.Map{}, emptyEvent, "empty event should remain empty after no enrichment")

	// negative numbered class uid values are never used
	undefinedClassEvent := jsonish.Map{"class_uid": json.Number("-1")}
	processor = schema.NewEventProcessor(NewValidation(), NewEnrichment())
	result, err = processor.ProcessEvent(undefinedClassEvent)
	assert.NoError(err, "undefined class uid should be a validation error")
	assert.Len(result.Validation.Errors, 1)
	assert.Equal(jsonish.Map{"class_uid": json.Number("-1")}, undefinedClassEvent, "undefined class event should remain unchanged")

	badUidEvent := jsonish.Map{"class_uid": "bogus"}
	result, err = processor.ProcessEvent(badUidEvent)
	assert.NoError(err, "bad class uid should be a validation error")
	assert.Len(result.Validation.Errors, 1)
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
			Uid:      int64(1),
			Category: "Greek",
			Observables: map[string]int64{
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

func makeValidationTestSchema(assert *require.Assertions) *schemaImpl {
	classNameSibling := "class_name"
	activityNameSibling := "activity_name"
	modeSibling := "mode"
	statusesSibling := "statuses"
	ballObject := "ball"
	metadataObject := "metadata"
	observableObject := "observable"
	trueValue := true

	classes := map[string]*classDefinition{
		"alpha": {
			Uid:      int64(1),
			Category: "test",
			Observables: map[string]int64{
				"ball.green": 1000,
			},
			commonItemDefinition: commonItemDefinition{
				Name:        "alpha",
				Constraints: map[string][]string{"at_least_one": {"name", "ball.green"}},
				Attributes: map[string]*itemAttributeDefinition{
					"class_uid": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:        "integer_t",
							Requirement: "required",
							Sibling:     &classNameSibling,
							Enum: map[string]*enumDefinition{
								"1": {Caption: "Alpha"},
							},
						},
					},
					"class_name": {commonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"activity_id": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:        "integer_t",
							Requirement: "required",
							Sibling:     &activityNameSibling,
							Enum: map[string]*enumDefinition{
								"1":  {Caption: "Do"},
								"99": {Caption: "Other"},
							},
						},
					},
					"activity_name": {commonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"type_uid": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:        "long_t",
							Requirement: "required",
						},
					},
					"metadata": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:        "object_t",
							ObjectType:  &metadataObject,
							Requirement: "required",
						},
					},
					"name": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
					"red": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "recommended",
						},
					},
					"port": {commonAttributeDefinition: commonAttributeDefinition{Type: "port_t"}},
					"count": {
						commonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"},
					},
					"long_value": {
						commonAttributeDefinition: commonAttributeDefinition{Type: "long_t"},
					},
					"bounded_count": {
						commonAttributeDefinition: commonAttributeDefinition{Type: "bounded_int_t"},
					},
					"short_text": {
						commonAttributeDefinition: commonAttributeDefinition{Type: "short_text_t"},
					},
					"code": {
						commonAttributeDefinition: commonAttributeDefinition{Type: "upper_code_t"},
					},
					"level": {
						commonAttributeDefinition: commonAttributeDefinition{Type: "level_t"},
					},
					"mode_id": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:    "integer_t",
							Sibling: &modeSibling,
							Enum: map[string]*enumDefinition{
								"1":  {Caption: "Known"},
								"99": {Caption: "Other"},
							},
						},
					},
					"mode": {commonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"status_ids": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:    "integer_t",
							IsArray: &trueValue,
							Sibling: &statusesSibling,
							Enum: map[string]*enumDefinition{
								"1": {Caption: "Open"},
								"2": {Caption: "Closed"},
							},
						},
					},
					"statuses": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:    "string_t",
							IsArray: &trueValue,
						},
					},
					"ball": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:       "object_t",
							ObjectType: &ballObject,
						},
					},
					"profile_attr": {
						commonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
						Profiles:                  []string{"p1"},
					},
					"observables": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:       "object_t",
							ObjectType: &observableObject,
							IsArray:    &trueValue,
						},
					},
				},
			},
		},
	}
	objects := map[string]*objectDefinition{
		"metadata": {
			commonItemDefinition: commonItemDefinition{
				Name: "metadata",
				Attributes: map[string]*itemAttributeDefinition{
					"version": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
					"profiles": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:    "string_t",
							IsArray: &trueValue,
						},
					},
				},
			},
		},
		"ball": {
			commonItemDefinition: commonItemDefinition{
				Name:        "ball",
				Constraints: map[string][]string{"at_least_one": {"green"}},
				Attributes: map[string]*itemAttributeDefinition{
					"green": {
						commonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
				},
			},
		},
		"observable": {
			commonItemDefinition: commonItemDefinition{
				Name: "observable",
				Attributes: map[string]*itemAttributeDefinition{
					"name":    {commonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"type_id": {commonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
					"type":    {commonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"value":   {commonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
				},
			},
		},
	}
	dictionary := &dictionaryDefinition{
		Attributes: map[string]*commonAttributeDefinition{
			"green": {Type: "string_t", Observable: testPtrTo(int64(1000))},
		},
		Types: &typesDefinition{
			Attributes: map[string]*typeDefinition{
				"integer_t": {commonAttributeDefinition: commonAttributeDefinition{Caption: "Integer"}},
				"long_t":    {commonAttributeDefinition: commonAttributeDefinition{Caption: "Long"}},
				"string_t":  {commonAttributeDefinition: commonAttributeDefinition{Caption: "String"}},
				"port_t": {
					commonAttributeDefinition: commonAttributeDefinition{
						Caption: "Port",
						Type:    "integer_t",
					},
					Range: []int64{0, 65535},
				},
				"bounded_int_t": {
					commonAttributeDefinition: commonAttributeDefinition{
						Caption: "Bounded Integer",
						Type:    "integer_t",
					},
					Range: []int64{-10, 10},
				},
				"short_text_t": {
					commonAttributeDefinition: commonAttributeDefinition{
						Caption: "Short Text",
						Type:    "string_t",
					},
					MaxLen: testPtrTo(int64(3)),
				},
				"upper_code_t": {
					commonAttributeDefinition: commonAttributeDefinition{
						Caption: "Uppercase Code",
						Type:    "string_t",
					},
					RegEx: testPtrTo("^[A-Z]+$"),
				},
				"level_t": {
					commonAttributeDefinition: commonAttributeDefinition{
						Caption: "Level",
						Type:    "integer_t",
					},
					Values: []any{json.Number("1"), json.Number("2")},
				},
			},
		},
	}

	si, err := newSchemaImpl(&schemaDefinition{
		CompileVersion: 1,
		Classes:        classes,
		Objects:        objects,
		Dictionary:     dictionary,
		Profiles:       map[string]*profileDefinition{"p1": {Name: "p1"}},
		Version:        "1.0.0",
	})
	assert.NoError(err)
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
	assert.NotNil(si.classes[int64(1)])
	assert.NotNil(si.objects["observable"])
	assert.Equal("Integer", si.dictionary.Types.Attributes["integer_t"].Caption)
	assert.Equal("Class UID", si.dictionary.Attributes["class_uid"].Caption)
	assert.Equal("Modern Observable", si.observableTypes[int64(1000)])
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

func TestNewSchemaImplRejectsDuplicateClassUIDs(t *testing.T) {
	assert := require.New(t)
	var sd schemaDefinition
	err := json.Unmarshal([]byte(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {
			"alpha": {"name": "alpha", "uid": 1},
			"beta": {"name": "beta", "uid": 1}
		},
		"objects": {},
		"dictionary": {"attributes": {}, "types": {"attributes": {}}}
	}`), &sd)
	assert.NoError(err)

	si, err := newSchemaImpl(&sd)

	assert.Nil(si)
	assert.ErrorContains(err, "compiled schema has duplicate class uid 1")
}

func TestNewSchemaImplNormalizesMissingOptionalSections(t *testing.T) {
	assert := require.New(t)
	var sd schemaDefinition
	err := json.Unmarshal([]byte(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {
			"alpha": {"name": "alpha", "uid": 1}
		}
	}`), &sd)
	assert.NoError(err)

	si, err := newSchemaImpl(&sd)

	assert.NoError(err)
	assert.NotNil(si)
	assert.NotNil(si.dictionary)
	assert.NotNil(si.dictionary.Attributes)
	assert.NotNil(si.dictionary.Types)
	assert.NotNil(si.dictionary.Types.Attributes)
	assert.NotNil(si.objects)
	assert.NotNil(si.profiles)
}

func TestNewSchemaImplRejectsMissingClasses(t *testing.T) {
	assert := require.New(t)
	var sd schemaDefinition
	err := json.Unmarshal([]byte(`{"compile_version":1}`), &sd)
	assert.NoError(err)

	si, err := newSchemaImpl(&sd)

	assert.Nil(si)
	assert.EqualError(err, "compiled schema is missing classes")
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
		"class_uid": json.Number("1"),
		"red":       "A red thing",
		"ball": jsonish.Map{
			"green": "A green thing",
		},
	}

	processor := si.NewEventProcessor(NewEnrichment())
	result, err := processor.ProcessEvent(event)
	assert.NoError(err)
	assert.Empty(result.Validation.Errors)
	assert.Equal(1, result.Enrichment.EnumSiblingsAdded)
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	assert.Equal("Alpha", event["class_name"])
	assert.NotNil(event["observables"])
	observables, ok := event["observables"].([]jsonish.Map)
	assert.True(ok)
	assert.Len(observables, 1)
	observable := observables[0]
	assert.Equal("ball.green", observable["name"])
	assert.Equal(int64(1000), observable["type_id"])
	assert.Equal("Class path ball.green", observable["type"])
	assert.Equal("A green thing", observable["value"])
}

func TestClassObservablesWithoutSiblings(t *testing.T) {
	assert := require.New(t)
	si := makeTestSchema(assert)

	// same as above but without add enum siblings
	event := jsonish.Map{
		"class_uid": json.Number("1"),
		"red":       "A red thing",
		"ball": jsonish.Map{
			"green": "A green thing",
		},
	}

	processor := si.NewEventProcessor(NewEnrichment(WithAddEnumSiblings(false)))
	result, err := processor.ProcessEvent(event)
	assert.NoError(err)
	assert.Empty(result.Validation.Errors)
	assert.Equal(0, result.Enrichment.EnumSiblingsAdded)
	assert.Equal(1, result.Enrichment.ObservablesAdded)
	assert.NotContains(event, "class_name")
	assert.NotNil(event["observables"])
	observables, ok := event["observables"].([]jsonish.Map)
	assert.True(ok)
	assert.Len(observables, 1)
	observable := observables[0]
	assert.Equal("ball.green", observable["name"])
	assert.Equal(int64(1000), observable["type_id"])
	assert.NotContains(observable, "type")
	assert.Equal("A green thing", observable["value"])
}

func TestProcessEventValidationValidEvent(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)

	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("101"),
		"metadata": jsonish.Map{
			"version":  "1.0.0",
			"profiles": []any{"p1"},
		},
		"name":         "event name",
		"red":          "recommended present",
		"port":         json.Number("443"),
		"mode_id":      json.Number("1"),
		"ball":         jsonish.Map{"green": "go"},
		"profile_attr": "active",
		"observables": []any{
			jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
		},
	}

	processor := si.NewEventProcessor(
		NewValidation(WithWarnOnMissingRecommended()),
		NewEnrichment(),
	)
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(result.Validation.Errors)
	assert.Empty(result.Validation.Warnings)
	assert.Equal("Alpha", event["class_name"])
	assert.Equal("Do", event["activity_name"])
	assert.Equal("Known", event["mode"])
}

func TestProcessEventValidationReportsExpectedIssues(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)

	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": "bad",
		"type_uid":    json.Number("102"),
		"metadata": jsonish.Map{
			"version":  "1.0.1",
			"profiles": []any{"unknown"},
		},
		"port":     json.Number("70000"),
		"mode_id":  json.Number("2"),
		"ball":     jsonish.Map{"blue": "nope"},
		"surprise": true,
	}

	processor := si.NewEventProcessor(NewValidation(WithWarnOnMissingRecommended()))
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	errorCodes := issueCodes(result.Validation.Errors)
	warningCodes := issueCodes(result.Validation.Warnings)

	assert.Contains(errorCodes, "attribute_required_missing")
	assert.Contains(errorCodes, "attribute_wrong_type")
	assert.Contains(errorCodes, "attribute_value_exceeds_range")
	assert.Contains(errorCodes, "attribute_enum_value_unknown")
	assert.Contains(errorCodes, "attribute_unknown")
	assert.Contains(errorCodes, "constraint_failed")
	assert.Contains(errorCodes, "profile_unknown")
	assert.Contains(errorCodes, "version_incompatible_later")
	assert.Contains(warningCodes, "attribute_recommended_missing")
}

func TestProcessEventValidationLongTIsInt64(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)

	validMaxInt64Event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number(strconv.FormatInt(math.MaxInt64, 10)),
		"metadata":    jsonish.Map{"version": "1.0.0"},
		"name":        "event name",
		"red":         "recommended present",
	}
	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(validMaxInt64Event)
	assert.NoError(err)
	assert.NotContains(issueCodes(result.Validation.Errors), "attribute_wrong_type")

	tooLargeEvent := jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("9223372036854775808"),
		"metadata":    jsonish.Map{"version": "1.0.0"},
		"name":        "event name",
		"red":         "recommended present",
	}
	result, err = processor.ProcessEvent(tooLargeEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_wrong_type")
}

func TestProcessEventValidationObservableReference(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)

	event := jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("101"),
		"metadata":    jsonish.Map{"version": "1.0.0"},
		"name":        "event name",
		"red":         "recommended present",
		"ball":        jsonish.Map{"green": "go"},
		"observables": []any{
			jsonish.Map{"name": "ball.blue", "type_id": json.Number("1000")},
		},
	}

	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "observable_name_invalid_reference")
}

func TestProcessEventValidationDoesNotEnrich(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["ball"] = jsonish.Map{"green": "go"}

	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(result.Validation.Errors)
	assert.Zero(result.Enrichment.EnumSiblingsAdded)
	assert.Zero(result.Enrichment.ObservablesAdded)
	assert.NotContains(event, "class_name")
	assert.NotContains(event, "activity_name")
	assert.NotContains(event, "mode")
	assert.NotContains(event, "observables")
}

func TestProcessEventValidationRunsAfterEnrichment(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["ball"] = jsonish.Map{"green": "go"}

	processor := si.NewEventProcessor(NewValidation(), NewEnrichment())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(result.Validation.Errors)
	assert.Empty(result.Validation.Warnings)
	assert.Equal("Alpha", event["class_name"])
	assert.Equal("Do", event["activity_name"])
	assert.Equal("Known", event["mode"])
	assert.Contains(event, "observables")
}

func TestProcessEventEnrichmentDoesNotValidate(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := jsonish.Map{
		"class_uid": json.Number("1"),
		"mode_id":   json.Number("1"),
	}

	processor := si.NewEventProcessor(NewEnrichment())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(result.Validation.Errors)
	assert.Empty(result.Validation.Warnings)
	assert.Equal("Alpha", event["class_name"])
	assert.Equal("Known", event["mode"])
}

func TestProcessEventNilEventReturnsError(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	processor := si.NewEventProcessor(NewValidation(), NewEnrichment())

	result, err := processor.ProcessEvent(nil)

	assert.ErrorIs(err, errNilEvent)
	assert.Empty(result.Validation.Errors)
	assert.Empty(result.Validation.Warnings)
}

func TestProcessEventValidationUnknownClassUIDUsesInt64(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := jsonish.Map{"class_uid": json.Number("2147483648")}

	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "class_uid_unknown")
	assert.NotContains(issueCodes(result.Validation.Errors), "attribute_wrong_type")
}

func TestProcessEventValidationRecommendedWarningsAreOptional(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)

	event := validValidationEvent()
	delete(event, "red")
	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(event)
	assert.NoError(err)
	assert.NotContains(issueCodes(result.Validation.Warnings), "attribute_recommended_missing")

	event = validValidationEvent()
	delete(event, "red")
	processor = si.NewEventProcessor(NewValidation(WithWarnOnMissingRecommended()))
	result, err = processor.ProcessEvent(event)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Warnings), "attribute_recommended_missing")
}

func TestProcessEventValidationTypeUIDIncorrect(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["type_uid"] = json.Number("102")

	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "type_uid_incorrect")
}

func TestProcessEventValidationTypeUIDOverflow(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	si.classes[math.MaxInt64/100+1] = &classDefinition{
		Uid:      math.MaxInt64/100 + 1,
		Category: "test",
		commonItemDefinition: commonItemDefinition{
			Name: "overflow",
			Attributes: map[string]*itemAttributeDefinition{
				"class_uid":   {commonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
				"activity_id": {commonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
				"type_uid":    {commonAttributeDefinition: commonAttributeDefinition{Type: "long_t"}},
			},
		},
	}
	event := jsonish.Map{
		"class_uid":   json.Number(strconv.FormatInt(math.MaxInt64/100+1, 10)),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("1"),
	}

	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "type_uid_expected_value_overflow")
}

func TestProcessEventValidationInactiveProfileAttributeIsUnknown(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["profile_attr"] = "inactive"

	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_unknown")
	assert.Contains(issueAttributePaths(result.Validation.Errors), "profile_attr")
}

func TestProcessEventValidationEnumSiblingWarnings(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	processor := si.NewEventProcessor(NewValidation())

	incorrectSiblingEvent := validValidationEvent()
	incorrectSiblingEvent["mode_id"] = json.Number("1")
	incorrectSiblingEvent["mode"] = "Wrong"
	result, err := processor.ProcessEvent(incorrectSiblingEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Warnings), "attribute_enum_sibling_incorrect")

	suspiciousOtherEvent := validValidationEvent()
	suspiciousOtherEvent["mode_id"] = json.Number("99")
	suspiciousOtherEvent["mode"] = "Other"
	result, err = processor.ProcessEvent(suspiciousOtherEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Warnings), "attribute_enum_sibling_suspicious_other")
}

func TestProcessEventValidationEnumArraySiblingErrors(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	processor := si.NewEventProcessor(NewValidation())

	missingSiblingElementEvent := validValidationEvent()
	missingSiblingElementEvent["status_ids"] = []any{json.Number("1"), json.Number("2")}
	missingSiblingElementEvent["statuses"] = []any{"Open"}
	result, err := processor.ProcessEvent(missingSiblingElementEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_enum_array_sibling_missing")

	incorrectSiblingElementEvent := validValidationEvent()
	incorrectSiblingElementEvent["status_ids"] = []any{json.Number("1")}
	incorrectSiblingElementEvent["statuses"] = []any{"Closed"}
	result, err = processor.ProcessEvent(incorrectSiblingElementEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_enum_array_sibling_incorrect")

	unknownArrayValueEvent := validValidationEvent()
	unknownArrayValueEvent["status_ids"] = []any{json.Number("3")}
	result, err = processor.ProcessEvent(unknownArrayValueEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_enum_array_value_unknown")
}

func TestProcessEventValidationConstraintEdgeCases(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	processor := si.NewEventProcessor(NewValidation())

	dottedPathEvent := validValidationEvent()
	delete(dottedPathEvent, "name")
	dottedPathEvent["ball"] = jsonish.Map{"green": "go"}
	result, err := processor.ProcessEvent(dottedPathEvent)
	assert.NoError(err)
	assert.NotContains(issueCodes(result.Validation.Errors), "constraint_failed")

	si = makeValidationTestSchema(assert)
	si.classes[int64(1)].Constraints = map[string][]string{"just_one": {"name", "ball.green"}}
	processor = si.NewEventProcessor(NewValidation())

	onePresentEvent := validValidationEvent()
	result, err = processor.ProcessEvent(onePresentEvent)
	assert.NoError(err)
	assert.NotContains(issueCodes(result.Validation.Errors), "constraint_failed")

	nonePresentEvent := validValidationEvent()
	delete(nonePresentEvent, "name")
	result, err = processor.ProcessEvent(nonePresentEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "constraint_failed")

	twoPresentEvent := validValidationEvent()
	twoPresentEvent["ball"] = jsonish.Map{"green": "go"}
	result, err = processor.ProcessEvent(twoPresentEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "constraint_failed")
}

func TestProcessEventValidationTypeConstraintChecks(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	processor := si.NewEventProcessor(NewValidation())

	inclusiveBoundsEvent := validValidationEvent()
	inclusiveBoundsEvent["bounded_count"] = json.Number("-10")
	result, err := processor.ProcessEvent(inclusiveBoundsEvent)
	assert.NoError(err)
	assert.NotContains(issueCodes(result.Validation.Errors), "attribute_value_exceeds_range")

	inclusiveBoundsEvent = validValidationEvent()
	inclusiveBoundsEvent["bounded_count"] = json.Number("10")
	result, err = processor.ProcessEvent(inclusiveBoundsEvent)
	assert.NoError(err)
	assert.NotContains(issueCodes(result.Validation.Errors), "attribute_value_exceeds_range")

	outOfRangeEvent := validValidationEvent()
	outOfRangeEvent["bounded_count"] = json.Number("-11")
	result, err = processor.ProcessEvent(outOfRangeEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_value_exceeds_range")

	outOfRangeEvent = validValidationEvent()
	outOfRangeEvent["bounded_count"] = json.Number("11")
	result, err = processor.ProcessEvent(outOfRangeEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_value_exceeds_range")

	maxLenEvent := validValidationEvent()
	maxLenEvent["short_text"] = "abcd"
	result, err = processor.ProcessEvent(maxLenEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_value_exceeds_max_len")

	regexEvent := validValidationEvent()
	regexEvent["code"] = "abc"
	result, err = processor.ProcessEvent(regexEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Warnings), "attribute_value_regex_not_matched")

	valuesEvent := validValidationEvent()
	valuesEvent["level"] = json.Number("3")
	result, err = processor.ProcessEvent(valuesEvent)
	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation.Errors), "attribute_value_not_in_type_values")
}

func TestProcessEventValidationIntegerAndLongBounds(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	processor := si.NewEventProcessor(NewValidation())

	testCases := []struct {
		name      string
		attribute string
		value     json.Number
		wantWrong bool
	}{
		{name: "integer min", attribute: "count", value: json.Number(strconv.FormatInt(math.MinInt64, 10))},
		{name: "integer max", attribute: "count", value: json.Number(strconv.FormatInt(math.MaxInt64, 10))},
		{name: "integer below min", attribute: "count", value: json.Number("-9223372036854775809"), wantWrong: true},
		{name: "integer above max", attribute: "count", value: json.Number("9223372036854775808"), wantWrong: true},
		{name: "integer decimal", attribute: "count", value: json.Number("1.0"), wantWrong: true},
		{name: "long min", attribute: "long_value", value: json.Number(strconv.FormatInt(math.MinInt64, 10))},
		{name: "long max", attribute: "long_value", value: json.Number(strconv.FormatInt(math.MaxInt64, 10))},
		{name: "long below min", attribute: "long_value", value: json.Number("-9223372036854775809"), wantWrong: true},
		{name: "long above max", attribute: "long_value", value: json.Number("9223372036854775808"), wantWrong: true},
		{name: "long decimal", attribute: "long_value", value: json.Number("1.0"), wantWrong: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := require.New(t)
			event := validValidationEvent()
			event[testCase.attribute] = testCase.value

			result, err := processor.ProcessEvent(event)

			assert.NoError(err)
			wrongTypePaths := issueAttributePaths(issuesWithCode(result.Validation.Errors, "attribute_wrong_type"))
			if testCase.wantWrong {
				assert.Contains(wrongTypePaths, testCase.attribute)
			} else {
				assert.NotContains(wrongTypePaths, testCase.attribute)
			}
		})
	}
}

func TestProcessEventValidationDeprecationWarnings(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	si.classes[int64(1)].Deprecated = &deprecatedDefinition{Since: "1.0.0", Message: "class deprecated"}
	si.classes[int64(1)].Attributes["red"].Deprecated = &deprecatedDefinition{Since: "1.0.0", Message: "attribute deprecated"}
	si.classes[int64(1)].Attributes["mode_id"].Enum["1"].Deprecated = &deprecatedDefinition{Since: "1.0.0", Message: "enum deprecated"}
	si.objects["ball"].Deprecated = &deprecatedDefinition{Since: "1.0.0", Message: "object deprecated"}

	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["ball"] = jsonish.Map{"green": "go"}
	processor := si.NewEventProcessor(NewValidation())
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	warningCodes := issueCodes(result.Validation.Warnings)
	assert.Contains(warningCodes, "class_deprecated")
	assert.Contains(warningCodes, "attribute_deprecated")
	assert.Contains(warningCodes, "attribute_enum_value_deprecated")
	assert.Contains(warningCodes, "object_deprecated")
}

func TestProcessEventEnrichmentOptions(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["ball"] = jsonish.Map{"green": "go"}

	processor := si.NewEventProcessor(NewEnrichment(WithAddObservables(false)))
	result, err := processor.ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(3, result.Enrichment.EnumSiblingsAdded)
	assert.Zero(result.Enrichment.ObservablesAdded)
	assert.Equal("Alpha", event["class_name"])
	assert.Equal("Do", event["activity_name"])
	assert.Equal("Known", event["mode"])
	assert.NotContains(event, "observables")
}

func validValidationEvent() jsonish.Map {
	return jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("101"),
		"metadata":    jsonish.Map{"version": "1.0.0"},
		"name":        "event name",
		"red":         "recommended present",
	}
}

func issueCodes(issues []ProcessingIssue) []string {
	codes := make([]string, len(issues))
	for i, issue := range issues {
		codes[i] = issue.Code
	}
	return codes
}

func issueAttributePaths(issues []ProcessingIssue) []string {
	paths := make([]string, len(issues))
	for i, issue := range issues {
		paths[i] = issue.AttributePath
	}
	return paths
}

func issuesWithCode(issues []ProcessingIssue, code string) []ProcessingIssue {
	result := make([]ProcessingIssue, 0)
	for _, issue := range issues {
		if issue.Code == code {
			result = append(result, issue)
		}
	}
	return result
}
