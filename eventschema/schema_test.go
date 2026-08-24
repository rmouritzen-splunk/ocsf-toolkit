package eventschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
)

const testSchemaFilePath = "../test/schema_v1.9.0.json"
const testSchemaVersion = "1.9.0"

func newSchemaFromDefinition(definition *schema.Definition) (*Schema, error) {
	compiled, err := schema.New(definition)
	if err != nil {
		return nil, err
	}
	return newSchema(compiled), nil
}

func testPtrTo[T any](value T) *T {
	return &value
}

func mustNewPipeline(
	assert *require.Assertions,
	schema *Schema,
	options ...PipelineOption,
) *Pipeline {
	pipeline, err := schema.NewPipeline(options...)
	assert.NoError(err)
	return pipeline
}

func TestLoadSchemaFromFile(t *testing.T) {
	assert := require.New(t)
	schema, _, err := Load(testSchemaFilePath)
	assert.NoError(err)
	checkSchema(assert, schema)
}

func TestLoadSchemaFromReader(t *testing.T) {
	assert := require.New(t)
	testSchemaFile, err := os.Open(testSchemaFilePath)
	assert.NoError(err)
	defer func(f *os.File) { _ = f.Close() }(testSchemaFile)
	schema, _, err := LoadReader(testSchemaFile)
	assert.NoError(err)
	checkSchema(assert, schema)
}

func TestLoadSchemaFromBytes(t *testing.T) {
	assert := require.New(t)
	data, err := os.ReadFile(testSchemaFilePath)
	assert.NoError(err)

	schema, _, err := LoadBytes(data)
	assert.NoError(err)
	checkSchema(assert, schema)
}

func TestLoadSchemaFromFS(t *testing.T) {
	data, err := os.ReadFile(testSchemaFilePath)
	require.NoError(t, err)
	filesystem := fstest.MapFS{"schema.json": {Data: data}}

	schema, _, err := LoadFS(filesystem, "schema.json")
	require.NoError(t, err)
	checkSchema(require.New(t), schema)
}

func TestLoadSchemaFromFSRejectsMissingFile(t *testing.T) {
	schema, _, err := LoadFS(fstest.MapFS{}, "missing.json")

	require.Nil(t, schema)
	require.ErrorContains(t, err, `failed to open schema file "missing.json"`)
}

func TestLoadSchemaFromFSRejectsNilFilesystem(t *testing.T) {
	schema, _, err := LoadFS(nil, "schema.json")

	require.Nil(t, schema)
	require.EqualError(t, err, "failed to open schema file: filesystem is nil")
}

func TestLoadSchemaFromBytesDoesNotRetainInput(t *testing.T) {
	data := []byte(`{
		"compile_version": 1,
		"version": "1.2.3",
		"classes": {"alpha": {"name": "alpha", "uid": 1}}
	}`)
	schema, _, err := LoadBytes(data)
	require.NoError(t, err)

	clear(data)

	require.Equal(t, "1.2.3", schema.compiledForTest().Version)
	require.Equal(t, "alpha", schema.compiledForTest().Classes[1].Name)
}

func TestLoadSchemaFromReaderPreservesTypeValueNumbers(t *testing.T) {
	assert := require.New(t)
	schema, _, err := LoadReader(strings.NewReader(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {"base_event": {"name": "base_event", "uid": 0}},
		"dictionary": {
			"types": {"attributes": {"precise_t": {"values": [9007199254740993]}}}
		}
	}`))

	assert.NoError(err)
	assert.Equal(
		[]any{json.Number("9007199254740993")},
		schema.compiledForTest().Dictionary.Types.Attributes["precise_t"].Values,
	)
}

func TestLoadSchemaReportsUnsupportedEnumSiblingAtInitialization(t *testing.T) {
	loaded, issues, err := LoadReader(strings.NewReader(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {
			"alpha": {
				"name": "alpha",
				"uid": 1,
				"attributes": {
						"code": {"type": "integer_t", "enum": {"1": {"caption": "Jammed"}}, "sibling": "message"},
					"message": {"type": "string_t", "is_array": true}
				}
			}
		}
	}`))

	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Len(t, issues, 1)
	require.Equal(t, "issue_at_init_schema_enum_sibling_target_not_string", issues[0].Code.String())
	require.Equal(t, "class", issues[0].Details["item_type"])
	require.Equal(t, "alpha", issues[0].Details["item_name"])
	require.Equal(t, "code", issues[0].Details["attribute"])
	require.Equal(t, "message", issues[0].Details["sibling"])
}

func TestLoadSchemaFromReaderRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		errorContains string
	}{
		{name: "malformed JSON", input: `{`, errorContains: "failed to decode schema"},
		{
			name:          "trailing JSON value",
			input:         `{"compile_version":1,"version":"1.0.0","classes":{"base_event":{"name":"base_event","uid":0}}} {}`, //nolint:lll // A single unbreakable JSON test literal.
			errorContains: "failed to decode schema",
		},
		{name: "invalid schema", input: `{"compile_version":2}`, errorContains: "failed to load schema"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, _, err := LoadReader(strings.NewReader(test.input))

			require.Nil(t, schema)
			require.ErrorContains(t, err, test.errorContains)
		})
	}
}

func TestLoadSchemaFromReaderRejectsReadFailure(t *testing.T) {
	schema, _, err := LoadReader(iotest.ErrReader(errors.New("read failed")))

	require.Nil(t, schema)
	require.ErrorContains(t, err, "failed to decode schema from reader: read failed")
}

func TestLoadSchemaFromReaderRejectsNilReader(t *testing.T) {
	schema, _, err := LoadReader(nil)

	require.Nil(t, schema)
	require.EqualError(t, err, "failed to read schema from reader: reader is nil")
}

func TestLoadSchemaFromFilePreservesTypeValueNumbers(t *testing.T) {
	assert := require.New(t)
	path := filepath.Join(t.TempDir(), "schema.json")
	assert.NoError(os.WriteFile(path, []byte(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {
			"base_event": {"name": "base_event", "uid": 0}
		},
		"dictionary": {
			"types": {
				"attributes": {
					"precise_t": {"values": [9007199254740993]}
				}
			}
		}
	}`), 0o600))

	schema, _, err := Load(path)

	assert.NoError(err)
	assert.Equal(
		[]any{json.Number("9007199254740993")},
		schema.compiledForTest().Dictionary.Types.Attributes["precise_t"].Values,
	)
}

func TestLoadSchemaFromFileRejectsTrailingJSONValue(t *testing.T) {
	assert := require.New(t)
	path := filepath.Join(t.TempDir(), "schema.json")
	assert.NoError(os.WriteFile(path, []byte(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {"base_event": {"name": "base_event", "uid": 0}}
	} {}`), 0o600))

	schema, _, err := Load(path)

	assert.Nil(schema)
	assert.ErrorContains(err, "failed to decode schema file")
}

func TestLoadSchemaFromFileRejectsNonArrayTypeValues(t *testing.T) {
	assert := require.New(t)
	path := filepath.Join(t.TempDir(), "schema.json")
	assert.NoError(os.WriteFile(path, []byte(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {"base_event": {"name": "base_event", "uid": 0}},
		"dictionary": {
			"types": {"attributes": {"precise_t": {"values": {"unexpected": true}}}}
		}
	}`), 0o600))

	schema, _, err := Load(path)

	assert.Nil(schema)
	assert.ErrorContains(err, "failed to decode schema file")
	assert.ErrorContains(err, "cannot unmarshal")
}

func checkSchema(assert *require.Assertions, schema *Schema) {
	assert.NotNil(schema)
	si := schema

	assert.Equal(testSchemaVersion, si.compiledForTest().Version)
	assert.NotEmpty(si.compiledForTest().Classes, "classes")
	assert.NotEmpty(si.compiledForTest().Objects, "objects")
	assert.NotEmpty(si.compiledForTest().Dictionary, "dictionary")
	assert.NotEmpty(si.compiledForTest().Profiles, "profiles")

	var err error

	emptyEvent := jsonish.Map{}
	pipeline := mustNewPipeline(assert, schema, WithValidation(),
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	result, err := pipeline.ProcessEvent(emptyEvent)
	assert.NoError(err)
	assert.Len(
		findingsAtLevel(result.Validation().Findings, validation.LevelError),
		1,
		"missing uid should fail validation",
	)
	assert.Equal(jsonish.Map{}, emptyEvent, "empty event should remain empty after error")

	pipeline, err = schema.NewPipeline()
	assert.EqualError(err, "at least one event processing action is required")
	assert.Nil(pipeline)

	// negative numbered class uid values are never used
	undefinedClassEvent := jsonish.Map{"class_uid": json.Number("-1")}
	pipeline = mustNewPipeline(assert, schema, WithValidation(),
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	result, err = pipeline.ProcessEvent(undefinedClassEvent)
	assert.NoError(err, "undefined class uid should be a validation error")
	assert.Len(findingsAtLevel(result.Validation().Findings, validation.LevelError), 1)
	assert.Equal(
		jsonish.Map{"class_uid": json.Number("-1")},
		undefinedClassEvent,
		"undefined class event should remain unchanged",
	)

	badUidEvent := jsonish.Map{"class_uid": "bogus"}
	result, err = pipeline.ProcessEvent(badUidEvent)
	assert.NoError(err, "bad class uid should be a validation error")
	assert.Len(findingsAtLevel(result.Validation().Findings, validation.LevelError), 1)
	assert.Equal(jsonish.Map{"class_uid": "bogus"}, badUidEvent, "bad uid event should remain unchanged")
}

func TestClassUIDResolutionFailuresReportMandatoryDiagnostics(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	tests := []struct {
		name              string
		event             jsonish.Map
		code              validation.Code
		validationCode    string
		processingCode    string
		processingMessage string
	}{
		{
			name:              "missing",
			event:             jsonish.Map{},
			code:              validation.ClassUIDMissing,
			validationCode:    "validation_class_uid_missing",
			processingCode:    "issue_class_uid_missing",
			processingMessage: `The "class_uid" attribute is missing, preventing further event processing.`,
		},
		{
			name:              "wrong type",
			event:             jsonish.Map{"class_uid": "invalid"},
			code:              validation.ClassUIDWrongType,
			validationCode:    "validation_class_uid_wrong_type",
			processingCode:    "issue_class_uid_wrong_type",
			processingMessage: `The "class_uid" attribute has the wrong type, preventing further event processing.`,
		},
		{
			name:           "unknown",
			event:          jsonish.Map{"class_uid": json.Number("-1")},
			code:           validation.ClassUIDUnknown,
			validationCode: "validation_class_uid_unknown",
			processingCode: "issue_class_uid_unknown",
			processingMessage: `The "class_uid" value does not identify a class in the schema,` +
				` preventing further event processing.`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			result, err := mustNewPipeline(assert, schema, WithValidation(
				WithSuppressValidation(),
				WithValidationErrorsAsWarnings(test.code),
			)).ProcessEvent(test.event)
			require.NoError(t, err)
			require.Equal(t, []string{test.validationCode}, issueCodes(result.Validation().Findings))
			require.Equal(t, validation.LevelWarning, result.Validation().Findings[0].Level)
			require.Zero(t, result.Validation().SuppressedErrorCount)
			require.Zero(t, result.Validation().SuppressedWarningCount)
			require.Equal(t, []string{test.processingCode}, issueCodes(result.Issues()))
			require.Equal(t, test.processingMessage, result.Issues()[0].Message)

			result, err = mustNewPipeline(assert, schema,
				WithEnumSiblings(enrichment.Add),
				WithSuppressIssues(),
			).ProcessEvent(test.event)
			require.NoError(t, err)
			require.Empty(t, result.Validation().Findings)
			require.Equal(t, []string{test.processingCode}, issueCodes(result.Issues()))
		})
	}
}

func makeTestSchema(assert *require.Assertions) *Schema {
	classNameAttribute := commonAttributeDefinition{
		Type: "string_t",
	}
	nameAttribute := commonAttributeDefinition{
		Type: "string_t",
	}
	typeStr := "type"
	typeIDAttribute := commonAttributeDefinition{
		Type:    "integer_t",
		Sibling: &typeStr,
	}
	typeAttribute := commonAttributeDefinition{
		Type: "string_t",
	}
	valueAttribute := commonAttributeDefinition{
		Type: "string_t",
	}

	redAttribute := commonAttributeDefinition{
		Type: "string_t",
	}
	greenAttribute := commonAttributeDefinition{
		Type: "string_t",
	}

	ballStr := "ball"
	ballAttribute := commonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &ballStr,
	}
	observableStr := "observable"
	trueValue := true
	observablesAttribute := commonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &observableStr,
		IsArray:    &trueValue,
	}

	classNameStr := "class_name"
	classes := map[string]*classDefinition{
		"alpha": {
			Uid: int64(1),
			Observables: map[string]int64{
				"ball.green": 1000,
			},
			ItemDefinition: commonItemDefinition{
				Name: "Alpha",
				Attributes: map[string]*itemAttributeDefinition{
					"class_uid": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type: "integer_t",
							Enum: map[string]*enumDefinition{
								"1": {Caption: "Alpha"},
							},
							Sibling: &classNameStr,
						},
					},
					"class_name": {
						CommonAttributeDefinition: classNameAttribute,
					},
					"observables": {
						CommonAttributeDefinition: observablesAttribute,
					},
					"red": {
						CommonAttributeDefinition: redAttribute,
					},
					"ball": {
						CommonAttributeDefinition: ballAttribute,
					},
				},
			},
		},
	}
	objects := map[string]*objectDefinition{
		"ball": {
			ItemDefinition: commonItemDefinition{
				Name: "Ball",
				Attributes: map[string]*itemAttributeDefinition{
					"green": {
						CommonAttributeDefinition: greenAttribute,
					},
				},
			},
		},
		"observable": {
			ItemDefinition: commonItemDefinition{
				Name: "Observable",
				Attributes: map[string]*itemAttributeDefinition{
					"name": {
						CommonAttributeDefinition: nameAttribute,
					},
					"type_id": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type: "integer_t",
							Enum: map[string]*enumDefinition{
								"1000": {
									Caption: "Class path ball.green",
								},
							},
							Sibling: &typeStr,
						},
					},
					"type": {
						CommonAttributeDefinition: typeAttribute,
					},
					"value": {
						CommonAttributeDefinition: valueAttribute,
					},
				},
			},
		},
	}
	dictionaryTypes := &typesDefinition{
		Attributes: map[string]*typeDefinition{
			"integer_t": {},
			"string_t":  {},
		},
	}
	dictionaryAttributes := map[string]*commonAttributeDefinition{
		"class_uid": {
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

	si, err := newSchemaFromDefinition(sd)
	assert.NoError(err)
	assert.NotNil(si, "schema should not be nil")
	return si
}

func makeValidationTestSchema(assert *require.Assertions) *Schema {
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
			Uid: int64(1),
			Observables: map[string]int64{
				"ball.green": 1000,
			},
			ItemDefinition: commonItemDefinition{
				Name:        "alpha",
				Constraints: map[string][]string{"at_least_one": {"name", "ball.green"}},
				Attributes: map[string]*itemAttributeDefinition{
					"class_uid": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "integer_t",
							Requirement: "required",
							Sibling:     &classNameSibling,
							Enum: map[string]*enumDefinition{
								"1": {Caption: "Alpha"},
							},
						},
					},
					"class_name": {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"activity_id": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "integer_t",
							Requirement: "required",
							Sibling:     &activityNameSibling,
							Enum: map[string]*enumDefinition{
								"1":  {Caption: "Do"},
								"99": {Caption: "Other"},
							},
						},
					},
					"activity_name": {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"type_uid": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "long_t",
							Requirement: "required",
						},
					},
					"metadata": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "object_t",
							ObjectType:  &metadataObject,
							Requirement: "required",
						},
					},
					"name": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
					"red": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "recommended",
						},
					},
					"port": {CommonAttributeDefinition: commonAttributeDefinition{Type: "port_t"}},
					"count": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"},
					},
					"long_value": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "long_t"},
					},
					"bounded_count": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "bounded_int_t"},
					},
					"short_text": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "short_text_t"},
					},
					"code": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "upper_code_t"},
					},
					"level": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "level_t"},
					},
					"mode_id": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:    "integer_t",
							Sibling: &modeSibling,
							Enum: map[string]*enumDefinition{
								"1":  {Caption: "Known"},
								"99": {Caption: "Other"},
							},
						},
					},
					"mode": {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"status_ids": {
						CommonAttributeDefinition: commonAttributeDefinition{
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
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:    "string_t",
							IsArray: &trueValue,
						},
					},
					"ball": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:       "object_t",
							ObjectType: &ballObject,
						},
					},
					"profile_attr": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
						Profiles:                  []string{"p1"},
					},
					"observables": {
						CommonAttributeDefinition: commonAttributeDefinition{
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
			ItemDefinition: commonItemDefinition{
				Name: "metadata",
				Attributes: map[string]*itemAttributeDefinition{
					"version": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
					"profiles": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:    "string_t",
							IsArray: &trueValue,
						},
					},
				},
			},
		},
		"ball": {
			ItemDefinition: commonItemDefinition{
				Name:        "ball",
				Constraints: map[string][]string{"at_least_one": {"green"}},
				Attributes: map[string]*itemAttributeDefinition{
					"green": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
				},
			},
		},
		"observable": {
			ItemDefinition: commonItemDefinition{
				Name: "observable",
				Attributes: map[string]*itemAttributeDefinition{
					"name":    {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"type_id": {CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
					"type":    {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"value":   {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
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
				"integer_t": {},
				"long_t":    {},
				"string_t":  {},
				"port_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "integer_t",
					},
					Range: []int64{0, 65535},
				},
				"bounded_int_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "integer_t",
					},
					Range: []int64{-10, 10},
				},
				"short_text_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "string_t",
					},
					MaxLen: testPtrTo(int64(3)),
				},
				"upper_code_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "string_t",
					},
					RegEx: testPtrTo("^[A-Z]+$"),
				},
				"level_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "integer_t",
					},
					Values: []any{json.Number("1"), json.Number("2")},
				},
			},
		},
	}

	si, err := newSchemaFromDefinition(&schemaDefinition{
		CompileVersion: 1,
		Classes:        classes,
		Objects:        objects,
		Dictionary:     dictionary,
		Profiles:       map[string]profileDefinition{"p1": {}},
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

	assert.Equal("1.2.3", si.compiledForTest().Version)
	assert.NotNil(si.compiledForTest().Classes[int64(1)])
	assert.NotNil(si.compiledForTest().Objects["observable"])
	assert.Equal("Modern Observable", si.compiledForTest().ObservableTypes[int64(1000)])
}

func TestNewSchemaImplRejectsNilObservableTypeEnumDefinition(t *testing.T) {
	assert := require.New(t)
	var sd schemaDefinition
	err := json.Unmarshal([]byte(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {
			"alpha": {"name": "alpha", "uid": 1}
		},
		"objects": {
			"observable": {
				"attributes": {
					"type_id": {
						"enum": {"1000": null}
					}
				}
			}
		}
	}`), &sd)
	assert.NoError(err)

	si, err := newSchemaFromDefinition(&sd)

	assert.Nil(si)
	assert.ErrorContains(err, `observable type enum "1000" has a null definition`)
}

func TestNewSchemaImplWithUnsupportedCompiledSchemaVersion(t *testing.T) {
	assert := require.New(t)
	var sd schemaDefinition
	err := json.Unmarshal([]byte(`{"compile_version":2}`), &sd)
	assert.NoError(err)

	si, err := newSchemaFromDefinition(&sd)
	assert.Nil(si)
	assert.EqualError(err, "unsupported compile_version: 2")
}

func TestNewSchemaImplRejectsInvalidSchemaVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{name: "missing"},
		{name: "malformed", version: "not-a-version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			sd := schemaDefinition{
				CompileVersion: 1,
				Version:        test.version,
				Classes: map[string]*classDefinition{
					"alpha": {ItemDefinition: commonItemDefinition{Name: "alpha"}, Uid: 1},
				},
			}

			si, err := newSchemaFromDefinition(&sd)

			assert.Nil(si)
			assert.EqualError(err, fmt.Sprintf("compiled schema version %q has invalid format", test.version))
		})
	}
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

	si, err := newSchemaFromDefinition(&sd)

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

	si, err := newSchemaFromDefinition(&sd)

	assert.NoError(err)
	assert.NotNil(si)
	assert.NotNil(si.compiledForTest().Dictionary)
	assert.NotNil(si.compiledForTest().Dictionary.Attributes)
	assert.NotNil(si.compiledForTest().Dictionary.Types)
	assert.NotNil(si.compiledForTest().Dictionary.Types.Attributes)
	assert.NotNil(si.compiledForTest().Objects)
	assert.NotNil(si.compiledForTest().Profiles)
}

func TestNewSchemaImplRejectsMissingClasses(t *testing.T) {
	assert := require.New(t)
	var sd schemaDefinition
	err := json.Unmarshal([]byte(`{"compile_version":1}`), &sd)
	assert.NoError(err)

	si, err := newSchemaFromDefinition(&sd)

	assert.Nil(si)
	assert.EqualError(err, "compiled schema is missing classes")
}

func newTestSchemaFromJSON(assert *require.Assertions, data string) *Schema {
	var sd schemaDefinition
	err := json.Unmarshal([]byte(data), &sd)
	assert.NoError(err)

	si, err := newSchemaFromDefinition(&sd)
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

	pipeline := mustNewPipeline(assert, si,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	assert.Equal(1, result.Enrichment().EnumSiblingsAdded)
	assert.Equal(1, result.Enrichment().ObservablesAdded)
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

	pipeline := mustNewPipeline(assert, si,
		WithEnumSiblings(enrichment.None),
		WithObservables(enrichment.Add),
	)
	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	assert.Equal(0, result.Enrichment().EnumSiblingsAdded)
	assert.Equal(1, result.Enrichment().ObservablesAdded)
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

	pipeline := mustNewPipeline(assert, si,
		WithValidation(WithWarnOnMissingRecommended()),
		WithEnumSiblings(enrichment.Add), WithObservables(enrichment.Add),
	)
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))
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

	pipeline := mustNewPipeline(assert, si, WithValidation(WithWarnOnMissingRecommended()))
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	errorCodes := issueCodes(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	warningCodes := issueCodes(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))

	assert.Contains(errorCodes, "validation_attribute_required_missing")
	assert.Contains(errorCodes, "validation_attribute_wrong_type")
	assert.Contains(errorCodes, "validation_attribute_value_exceeds_range")
	assert.Contains(errorCodes, "validation_attribute_enum_value_unknown")
	assert.Contains(errorCodes, "validation_attribute_unknown")
	assert.Contains(errorCodes, "validation_constraint_failed")
	assert.Contains(errorCodes, "validation_profile_unknown")
	assert.Contains(errorCodes, "validation_version_incompatible_later")
	assert.Contains(warningCodes, "validation_attribute_recommended_missing")
	assert.Empty(result.Issues())
}

func TestProcessEventValidationReportsInvalidVersionFormat(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	metadata, ok := event["metadata"].(jsonish.Map)
	assert.True(ok)
	metadata["version"] = "not-a-version"

	result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	issues := issuesWithCode(resultErrorFindings, "validation_version_invalid_format")
	assert.Len(issues, 1)
	assert.Equal("metadata.version", issues[0].Details["attribute_path"])
	assert.NotContains(issues[0].Details, "value")
	assert.Contains(issues[0].Details, "expected_regex")
	resultWarningFindings := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
	assert.NotContains(issueCodes(resultWarningFindings), "validation_version_earlier")
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
	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(validMaxInt64Event)
	assert.NoError(err)
	resultErrorFindings2 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.NotContains(issueCodes(resultErrorFindings2), "validation_attribute_wrong_type")

	tooLargeEvent := jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("9223372036854775808"),
		"metadata":    jsonish.Map{"version": "1.0.0"},
		"name":        "event name",
		"red":         "recommended present",
	}
	result, err = pipeline.ProcessEvent(tooLargeEvent)
	assert.NoError(err)
	resultErrorFindings3 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings3), "validation_attribute_wrong_type")
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

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings4 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings4), "validation_observable_name_invalid_reference")
}

func TestProcessEventValidationDoesNotEnrich(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["ball"] = jsonish.Map{"green": "go"}

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	assert.Zero(result.Enrichment().EnumSiblingsAdded)
	assert.Zero(result.Enrichment().ObservablesAdded)
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

	pipeline := mustNewPipeline(assert, si, WithValidation(),
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))
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

	pipeline := mustNewPipeline(assert, si,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))
	assert.Equal("Alpha", event["class_name"])
	assert.Equal("Known", event["mode"])
}

func TestProcessEventNilEventReturnsError(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, si, WithValidation(),
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)

	result, err := pipeline.ProcessEvent(nil)

	assert.EqualError(err, "event is nil")
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))
}

func TestPipelineZeroValueCannotProcessEvent(t *testing.T) {
	var pipeline Pipeline
	result, err := pipeline.ProcessEvent(jsonish.Map{})

	require.EqualError(t, err, "event processor pipeline is not initialized; create it with Schema.NewPipeline")
	require.Empty(t, result)
}

func TestPipelineNilReceiverCannotProcessEvent(t *testing.T) {
	var pipeline *Pipeline

	result, err := pipeline.ProcessEvent(jsonish.Map{})

	require.EqualError(t, err, "event processor pipeline is not initialized; create it with Schema.NewPipeline")
	require.Empty(t, result.Validation().Findings)
}

func TestProcessEventValidationUnknownClassUIDUsesInt64(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := jsonish.Map{"class_uid": json.Number("2147483648")}

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings5 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings5), "validation_class_uid_unknown")
	resultErrorFindings6 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.NotContains(issueCodes(resultErrorFindings6), "validation_attribute_wrong_type")
}

func TestProcessEventValidationRecommendedWarningsAreOptional(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)

	event := validValidationEvent()
	delete(event, "red")
	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	resultWarningFindings2 := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
	assert.NotContains(issueCodes(resultWarningFindings2), "validation_attribute_recommended_missing")

	event = validValidationEvent()
	delete(event, "red")
	pipeline = mustNewPipeline(assert, si, WithValidation(WithWarnOnMissingRecommended()))
	result, err = pipeline.ProcessEvent(event)
	assert.NoError(err)
	resultWarningFindings3 := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
	assert.Contains(issueCodes(resultWarningFindings3), "validation_attribute_recommended_missing")
}

func TestProcessEventValidationTypeUIDIncorrect(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["type_uid"] = json.Number("102")

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings7 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings7), "validation_type_uid_incorrect")
}

func TestProcessEventValidationTypeUIDOverflow(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	si.compiledForTest().Classes[math.MaxInt64/100+1] = &classDefinition{
		Uid: math.MaxInt64/100 + 1,
		ItemDefinition: commonItemDefinition{
			Name: "overflow",
			Attributes: map[string]*itemAttributeDefinition{
				"class_uid":   {CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
				"activity_id": {CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
				"type_uid":    {CommonAttributeDefinition: commonAttributeDefinition{Type: "long_t"}},
			},
		},
	}
	event := jsonish.Map{
		"class_uid":   json.Number(strconv.FormatInt(math.MaxInt64/100+1, 10)),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("1"),
	}

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings8 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings8), "validation_type_uid_expected_value_overflow")
}

func TestProcessEventValidationInactiveProfileAttributeRequiresProfile(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["profile_attr"] = "inactive"

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	findings := issuesWithCode(result.Validation().Findings, "validation_attribute_requires_profile")
	assert.Len(findings, 1)
	if len(findings) == 1 {
		assert.Equal(
			"Attribute at \"profile_attr\" requires profile \"p1\", which is not listed in metadata.profiles. "+
				"The value is valid.",
			findings[0].Message,
		)
		assert.Equal("profile_attr", findings[0].Details["attribute_path"])
		assert.Equal("profile_attr", findings[0].Details["attribute"])
		assert.Equal([]string{"p1"}, findings[0].Details["required_profiles"])
		assert.Equal("valid", findings[0].Details["value_validation"])
		assert.NotContains(findings[0].Details, "invalid_value")
	}
	assert.NotContains(issueCodes(result.Validation().Findings), "validation_attribute_unknown")
}

func TestProcessEventValidationInactiveProfileAttributeIncludesInvalidValueFindings(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["profile_attr"] = json.Number("1")

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	findings := issuesWithCode(result.Validation().Findings, "validation_attribute_requires_profile")
	assert.Len(findings, 1)
	if len(findings) == 1 {
		assert.Equal(
			"Attribute at \"profile_attr\" requires profile \"p1\", which is not listed in metadata.profiles. "+
				"The value would be invalid if enabled by a profile.",
			findings[0].Message,
		)
		assert.Equal(
			jsonish.Map{
				"findings": []any{
					jsonish.Map{
						"level": "error",
						"code":  "validation_attribute_wrong_type",
						"message": "Attribute \"profile_attr\" value has wrong type;" +
							" expected string_t, got integer_t (integer in range of -2^63 to 2^63 - 1).",
						"details": jsonish.Map{
							"attribute_path": "profile_attr",
							"attribute":      "profile_attr",
							"value_type":     "integer_t",
							"expected_type":  "string_t",
						},
					},
				},
			},
			findings[0].Details["invalid_value"],
		)
		assert.Equal("invalid", findings[0].Details["value_validation"])
	}
	assert.NotContains(issueCodes(result.Validation().Findings), "validation_attribute_wrong_type")
}

func TestProcessEventValidationInactiveProfileObjectIsOnlyShallowlyValidated(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	ballObjectType := "ball"
	si.compiledForTest().Classes[int64(1)].Attributes["profile_ball"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "object_t", ObjectType: &ballObjectType},
		Profiles:                  []string{"p1"},
	}
	event := validValidationEvent()
	event["profile_ball"] = jsonish.Map{"green": json.Number("1")}

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	findings := issuesWithCode(result.Validation().Findings, "validation_attribute_requires_profile")
	assert.Len(findings, 1)
	if len(findings) == 1 {
		assert.Equal("shallowly_valid", findings[0].Details["value_validation"])
		assert.NotContains(findings[0].Details, "invalid_value")
		assert.Contains(
			findings[0].Message,
			"The value passes a shallow validation; full validation requires enabling a profile.",
		)
	}
	assert.NotContains(issueCodes(result.Validation().Findings), "validation_attribute_wrong_type")
}

func TestProcessEventValidationInactiveProfileObjectRejectsWrongShape(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	ballObjectType := "ball"
	si.compiledForTest().Classes[int64(1)].Attributes["profile_ball"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "object_t", ObjectType: &ballObjectType},
		Profiles:                  []string{"p1"},
	}
	event := validValidationEvent()
	event["profile_ball"] = "not an object"

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	findings := issuesWithCode(result.Validation().Findings, "validation_attribute_requires_profile")
	assert.Len(findings, 1)
	if len(findings) == 1 {
		assert.Equal("invalid", findings[0].Details["value_validation"])
		assert.Contains(findings[0].Details, "invalid_value")
	}
}

func TestProcessEventValidationInactiveEnumAndArrayAreShallowlyValidated(t *testing.T) {
	tests := []struct {
		name       string
		attribute  string
		definition *itemAttributeDefinition
		value      any
	}{
		{
			name:      "enum membership is deferred",
			attribute: "profile_enum",
			definition: &itemAttributeDefinition{
				CommonAttributeDefinition: commonAttributeDefinition{
					Type: "integer_t",
					Enum: map[string]*enumDefinition{"1": {Caption: "Known"}},
				},
				Profiles: []string{"p1"},
			},
			value: json.Number("2"),
		},
		{
			name:      "array elements are deferred",
			attribute: "profile_values",
			definition: &itemAttributeDefinition{
				CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t", IsArray: testPtrTo(true)},
				Profiles:                  []string{"p1"},
			},
			value: []any{json.Number("1")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			si := makeValidationTestSchema(assert)
			si.compiledForTest().Classes[int64(1)].Attributes[test.attribute] = test.definition
			event := validValidationEvent()
			event[test.attribute] = test.value

			pipeline := mustNewPipeline(assert, si, WithValidation())
			result, err := pipeline.ProcessEvent(event)

			assert.NoError(err)
			findings := issuesWithCode(result.Validation().Findings, "validation_attribute_requires_profile")
			assert.Len(findings, 1)
			if len(findings) == 1 {
				assert.Equal("shallowly_valid", findings[0].Details["value_validation"])
				assert.NotContains(findings[0].Details, "invalid_value")
			}
		})
	}
}

func TestProcessEventValidationInactiveAttributeReportsSortedEnablingProfiles(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	si.compiledForTest().Classes[int64(1)].Attributes["profile_attr"].Profiles = []string{"p2", "p1"}
	event := validValidationEvent()
	event["profile_attr"] = "inactive"

	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	findings := issuesWithCode(result.Validation().Findings, "validation_attribute_requires_profile")
	assert.Len(findings, 1)
	if len(findings) == 1 {
		assert.Equal([]string{"p1", "p2"}, findings[0].Details["required_profiles"])
		assert.Equal(
			"Attribute at \"profile_attr\" requires one of the profiles \"p1\" or \"p2\"; "+
				"none is listed in metadata.profiles. The value is valid.",
			findings[0].Message,
		)
	}
}

func TestProcessEventValidationInactiveValueCheckIgnoresFindingSuppression(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["profile_attr"] = json.Number("1")

	pipeline := mustNewPipeline(assert, si, WithValidation(WithSuppressValidation(validation.AttributeWrongType)))
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	findings := issuesWithCode(result.Validation().Findings, "validation_attribute_requires_profile")
	assert.Len(findings, 1)
	if len(findings) == 1 {
		assert.Equal("invalid", findings[0].Details["value_validation"])
		assert.Contains(findings[0].Details, "invalid_value")
	}
	assert.Zero(result.Validation().SuppressedErrorCount)
}

func TestProcessEventValidationGenericAndProfileFilteredObjects(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	genericObjectType := "object"
	emptyObjectType := "empty_object"
	derivedObjectType := "derived_object"
	schema.compiledForTest().Objects[genericObjectType] = &objectDefinition{
		ItemDefinition: commonItemDefinition{
			Name: genericObjectType, Attributes: map[string]*itemAttributeDefinition{},
		},
	}
	schema.compiledForTest().Objects[emptyObjectType] = &objectDefinition{
		ItemDefinition: commonItemDefinition{Name: emptyObjectType, Attributes: map[string]*itemAttributeDefinition{}},
	}
	schema.compiledForTest().Objects[derivedObjectType] = &objectDefinition{
		ItemDefinition: commonItemDefinition{
			Name: derivedObjectType,
			Attributes: map[string]*itemAttributeDefinition{
				// Compiled schemas flatten inherited profile attributes onto the derived object.
				"parent_profile_attr": {
					CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
					Profiles:                  []string{"p1"},
				},
			},
		},
	}
	schema.compiledForTest().Classes[int64(1)].Attributes["unmapped"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "object_t", ObjectType: &genericObjectType},
	}
	schema.compiledForTest().Classes[int64(1)].Attributes["empty"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "object_t", ObjectType: &emptyObjectType},
	}
	schema.compiledForTest().Classes[int64(1)].Attributes["derived"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "object_t", ObjectType: &derivedObjectType},
	}
	pipeline := mustNewPipeline(assert, schema, WithValidation())

	t.Run("direct generic object is open", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["unmapped"] = jsonish.Map{"anything": jsonish.Map{"nested": true}}

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		resultErrorFindings11 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
		assert.NotContains(issueAttributePaths(resultErrorFindings11), "unmapped.anything")
	})

	t.Run("other empty object is closed", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["empty"] = jsonish.Map{"anything": true}

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		resultErrorFindings12 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
		assert.Contains(issueAttributePaths(resultErrorFindings12), "empty.anything")
	})

	t.Run("inherited profile attribute is inactive", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["derived"] = jsonish.Map{"parent_profile_attr": "inactive", "typo": true}

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		paths := issueAttributePaths(findingsAtLevel(result.Validation().Findings, validation.LevelError))
		assert.Contains(paths, "derived.parent_profile_attr")
		assert.Contains(paths, "derived.typo")
	})

	t.Run("inherited profile attribute is active", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["metadata"] = jsonish.Map{"version": "1.0.0", "profiles": []any{"p1"}}
		event["derived"] = jsonish.Map{"parent_profile_attr": "active"}

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		resultErrorFindings13 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
		assert.NotContains(issueAttributePaths(resultErrorFindings13), "derived.parent_profile_attr")
	})

	t.Run("inactive null profile attribute is missing", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["derived"] = jsonish.Map{"parent_profile_attr": nil}

		result, err := pipeline.ProcessEvent(event)

		assert.NoError(err)
		resultErrorFindings14 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
		assert.NotContains(issueAttributePaths(resultErrorFindings14), "derived.parent_profile_attr")
	})
}

func TestProcessEventValidationEnumSiblingWarnings(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, si, WithValidation())

	incorrectSiblingEvent := validValidationEvent()
	incorrectSiblingEvent["mode_id"] = json.Number("1")
	incorrectSiblingEvent["mode"] = "Wrong"
	result, err := pipeline.ProcessEvent(incorrectSiblingEvent)
	assert.NoError(err)
	resultWarningFindings4 := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
	assert.Contains(issueCodes(resultWarningFindings4), "validation_attribute_enum_sibling_incorrect")

	suspiciousOtherEvent := validValidationEvent()
	suspiciousOtherEvent["mode_id"] = json.Number("99")
	suspiciousOtherEvent["mode"] = "Other"
	result, err = pipeline.ProcessEvent(suspiciousOtherEvent)
	assert.NoError(err)
	resultWarningFindings5 := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
	assert.Contains(issueCodes(resultWarningFindings5), "validation_attribute_enum_sibling_suspicious_other")
}

func TestProcessEventValidationEnumArraySiblingErrors(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, si, WithValidation())

	missingSiblingElementEvent := validValidationEvent()
	missingSiblingElementEvent["status_ids"] = []any{json.Number("1"), json.Number("2")}
	missingSiblingElementEvent["statuses"] = []any{"Open"}
	result, err := pipeline.ProcessEvent(missingSiblingElementEvent)
	assert.NoError(err)
	resultErrorFindings15 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings15), "validation_attribute_enum_array_sibling_length_mismatch")

	incorrectSiblingElementEvent := validValidationEvent()
	incorrectSiblingElementEvent["status_ids"] = []any{json.Number("1")}
	incorrectSiblingElementEvent["statuses"] = []any{"Closed"}
	result, err = pipeline.ProcessEvent(incorrectSiblingElementEvent)
	assert.NoError(err)
	resultErrorFindings16 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings16), "validation_attribute_enum_array_sibling_incorrect")

	unknownArrayValueEvent := validValidationEvent()
	unknownArrayValueEvent["status_ids"] = []any{json.Number("3")}
	result, err = pipeline.ProcessEvent(unknownArrayValueEvent)
	assert.NoError(err)
	resultErrorFindings17 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings17), "validation_attribute_enum_array_value_unknown")
}

func TestProcessEventValidationEnumArraySiblingPairing(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	si.compiledForTest().Classes[int64(1)].Attributes["status_ids"].Enum["99"] = &enumDefinition{Caption: "Other"}
	pipeline := mustNewPipeline(assert, si, WithValidation())

	tests := []struct {
		name           string
		statusIDs      []any
		statuses       []any
		wantCode       string
		wantCodeAbsent string
	}{
		{
			name:      "shorter sibling",
			statusIDs: []any{json.Number("1"), json.Number("2")},
			statuses:  []any{"Open"},
			wantCode:  "validation_attribute_enum_array_sibling_length_mismatch",
		},
		{
			name:      "longer sibling",
			statusIDs: []any{json.Number("1")},
			statuses:  []any{"Open", "Closed"},
			wantCode:  "validation_attribute_enum_array_sibling_length_mismatch",
		},
		{
			name:      "sibling paired with empty enum",
			statusIDs: []any{},
			statuses:  []any{"Open"},
			wantCode:  "validation_attribute_enum_array_sibling_length_mismatch",
		},
		{
			name:           "other source specific",
			statusIDs:      []any{json.Number("99")},
			statuses:       []any{"Vendor-specific"},
			wantCodeAbsent: "validation_attribute_enum_sibling_suspicious_other",
		},
		{
			name:      "other schema caption",
			statusIDs: []any{json.Number("99")},
			statuses:  []any{"Other"},
			wantCode:  "validation_attribute_enum_sibling_suspicious_other",
		},
		{
			name:      "other null sibling",
			statusIDs: []any{json.Number("99")},
			statuses:  []any{nil},
			wantCode:  "validation_attribute_enum_array_sibling_missing",
		},
		{
			name:      "other sibling array absent",
			statusIDs: []any{json.Number("99")},
			wantCode:  "validation_attribute_enum_array_sibling_missing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			event := validValidationEvent()
			event["status_ids"] = test.statusIDs
			if test.statuses != nil {
				event["statuses"] = test.statuses
			}

			result, err := pipeline.ProcessEvent(event)

			assert.NoError(err)
			codes := issueCodes(result.Validation().Findings)
			if test.wantCode != "" {
				assert.Contains(codes, test.wantCode)
			}
			if test.wantCodeAbsent != "" {
				assert.NotContains(codes, test.wantCodeAbsent)
			}
		})
	}
}

func TestProcessEventValidationChecksAvailablePairsWhenArrayLengthsDiffer(t *testing.T) {
	assert := require.New(t)
	pipeline := mustNewPipeline(assert, makeValidationTestSchema(assert), WithValidation())
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("2")}
	event["statuses"] = []any{"Wrong"}

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	codes := issueCodes(result.Validation().Findings)
	assert.Contains(codes, "validation_attribute_enum_array_sibling_length_mismatch")
	assert.Contains(codes, "validation_attribute_enum_array_sibling_incorrect")
}

func TestProcessEventValidationTreatsStringEnumArrayValue99Normally(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	statusIDs := si.compiledForTest().Classes[int64(1)].Attributes["status_ids"]
	statusIDs.Type = "string_t"
	statusIDs.Enum = map[string]*enumDefinition{"99": {Caption: "Ninety-nine"}}
	pipeline := mustNewPipeline(assert, si, WithValidation())
	event := validValidationEvent()
	event["status_ids"] = []any{"99"}
	event["statuses"] = []any{"Ninety-nine"}

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.NotContains(
		issueCodes(result.Validation().Findings),
		"validation_attribute_enum_sibling_suspicious_other",
	)
}

func TestProcessEventValidationRequiresScalarSiblingForEnumID99(t *testing.T) {
	assert := require.New(t)
	pipeline := mustNewPipeline(assert, makeValidationTestSchema(assert), WithValidation())
	event := validValidationEvent()
	event["mode_id"] = json.Number("99")

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Contains(issueCodes(result.Validation().Findings), "validation_attribute_required_missing")
}

func TestProcessEventValidationConstraintEdgeCases(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, si, WithValidation())

	dottedPathEvent := validValidationEvent()
	delete(dottedPathEvent, "name")
	dottedPathEvent["ball"] = jsonish.Map{"green": "go"}
	result, err := pipeline.ProcessEvent(dottedPathEvent)
	assert.NoError(err)
	resultErrorFindings18 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.NotContains(issueCodes(resultErrorFindings18), "validation_constraint_failed")

	si = makeValidationTestSchema(assert)
	si.compiledForTest().Classes[int64(1)].Constraints = map[string][]string{"just_one": {"name", "ball.green"}}
	pipeline = mustNewPipeline(assert, si, WithValidation())

	onePresentEvent := validValidationEvent()
	result, err = pipeline.ProcessEvent(onePresentEvent)
	assert.NoError(err)
	resultErrorFindings19 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.NotContains(issueCodes(resultErrorFindings19), "validation_constraint_failed")

	nonePresentEvent := validValidationEvent()
	delete(nonePresentEvent, "name")
	result, err = pipeline.ProcessEvent(nonePresentEvent)
	assert.NoError(err)
	resultErrorFindings20 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings20), "validation_constraint_failed")

	twoPresentEvent := validValidationEvent()
	twoPresentEvent["ball"] = jsonish.Map{"green": "go"}
	result, err = pipeline.ProcessEvent(twoPresentEvent)
	assert.NoError(err)
	resultErrorFindings21 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings21), "validation_constraint_failed")

	nullPresentEvent := validValidationEvent()
	nullPresentEvent["name"] = nil
	result, err = pipeline.ProcessEvent(nullPresentEvent)
	assert.NoError(err)
	resultErrorFindings22 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings22), "validation_constraint_failed")

	nullDottedPathEvent := validValidationEvent()
	delete(nullDottedPathEvent, "name")
	nullDottedPathEvent["ball"] = jsonish.Map{"green": nil}
	result, err = pipeline.ProcessEvent(nullDottedPathEvent)
	assert.NoError(err)
	resultErrorFindings23 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings23), "validation_constraint_failed")
}

func TestProcessEventTreatsNullAttributesAsMissing(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)

	t.Run("unknown attribute", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["unknown"] = nil

		result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(event)

		assert.NoError(err)
		resultErrorFindings24 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
		assert.NotContains(issueCodes(resultErrorFindings24), "validation_attribute_unknown")
	})

	t.Run("enum sibling validation", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["mode_id"] = json.Number("1")
		event["mode"] = nil

		result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(event)

		assert.NoError(err)
		resultWarningFindings6 := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
		assert.NotContains(issueCodes(resultWarningFindings6), "validation_attribute_enum_sibling_incorrect")
	})

	t.Run("enum sibling enrichment", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["mode_id"] = json.Number("1")
		event["mode"] = nil

		result, err := mustNewPipeline(
			assert,
			schema,
			WithEnumSiblings(enrichment.Add), WithObservables(enrichment.None),
		).ProcessEvent(event)

		assert.NoError(err)
		assert.Equal("Known", event["mode"])
		assert.Equal(3, result.Enrichment().EnumSiblingsAdded)
	})

	t.Run("observables validation", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["observables"] = nil

		result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(event)

		assert.NoError(err)
		assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	})

	t.Run("null array element", func(t *testing.T) {
		assert := require.New(t)
		event := validValidationEvent()
		event["statuses"] = []any{nil}

		result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(event)

		assert.NoError(err)
		resultErrorFindings25 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
		assert.Contains(
			issueAttributePaths(issuesWithCode(resultErrorFindings25, "validation_attribute_wrong_type")),
			"statuses[0]",
		)
	})
}

func TestProcessEventSupportsTypedSlicesAndArrays(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	addObservableArrayTestAttributes(schema)
	trueValue := true
	schema.compiledForTest().Dictionary.Types.Attributes["float_t"] = &typeDefinition{}
	schema.compiledForTest().Classes[int64(1)].Attributes["scores"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "float_t", IsArray: &trueValue},
	}
	event := validValidationEvent()
	event["status_ids"] = []int64{1, 2}
	event["statuses"] = [2]string{"Open", "Closed"}
	event["scores"] = []float64{1.5, 2.5}
	event["metadata"] = jsonish.Map{"version": "1.0.0", "profiles": []string{"p1"}}
	event["profile_attr"] = "active"
	event["balls"] = []jsonish.Map{{"green": "first"}, {"green": "second"}}

	result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
}

func TestInvariantProcessEventAcceptsDefinedSliceAndArrayContainers(t *testing.T) {
	// Invariant test: event processing depends on the logical values in an array, not whether the Go slice or array
	// container has a defined type.
	type integerList []int64
	type statusArray [2]string
	tests := []struct {
		name      string
		attribute string
		value     any
	}{
		{name: "slice", attribute: "status_ids", value: integerList{1}},
		{name: "array", attribute: "statuses", value: statusArray{"Open", "Closed"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			event := validValidationEvent()
			event[test.attribute] = test.value

			result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(event)

			assert.NoError(err)
			assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
		})
	}
}

func addObservableArrayTestAttributes(schema *Schema) {
	trueValue := true
	ballType := "ball"
	schema.compiledForTest().Classes[int64(1)].Attributes["balls"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{
			Type:       "object_t",
			ObjectType: &ballType,
			IsArray:    &trueValue,
		},
	}
	schema.compiledForTest().Objects["ball"].Attributes["children"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{
			Type:       "object_t",
			ObjectType: &ballType,
			IsArray:    &trueValue,
		},
	}
}

func TestProcessEventValidationTypeConstraintChecks(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, si, WithValidation())

	inclusiveBoundsEvent := validValidationEvent()
	inclusiveBoundsEvent["bounded_count"] = json.Number("-10")
	result, err := pipeline.ProcessEvent(inclusiveBoundsEvent)
	assert.NoError(err)
	resultErrorFindings26 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.NotContains(issueCodes(resultErrorFindings26), "validation_attribute_value_exceeds_range")

	inclusiveBoundsEvent = validValidationEvent()
	inclusiveBoundsEvent["bounded_count"] = json.Number("10")
	result, err = pipeline.ProcessEvent(inclusiveBoundsEvent)
	assert.NoError(err)
	resultErrorFindings27 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.NotContains(issueCodes(resultErrorFindings27), "validation_attribute_value_exceeds_range")

	outOfRangeEvent := validValidationEvent()
	outOfRangeEvent["bounded_count"] = json.Number("-11")
	result, err = pipeline.ProcessEvent(outOfRangeEvent)
	assert.NoError(err)
	resultErrorFindings28 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings28), "validation_attribute_value_exceeds_range")

	outOfRangeEvent = validValidationEvent()
	outOfRangeEvent["bounded_count"] = json.Number("11")
	result, err = pipeline.ProcessEvent(outOfRangeEvent)
	assert.NoError(err)
	resultErrorFindings29 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings29), "validation_attribute_value_exceeds_range")

	maxLenEvent := validValidationEvent()
	maxLenEvent["short_text"] = "abcd"
	result, err = pipeline.ProcessEvent(maxLenEvent)
	assert.NoError(err)
	resultErrorFindings30 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings30), "validation_attribute_value_exceeds_max_len")

	regexEvent := validValidationEvent()
	regexEvent["code"] = "abc"
	result, err = pipeline.ProcessEvent(regexEvent)
	assert.NoError(err)
	resultWarningFindings7 := findingsAtLevel(result.Validation().Findings, validation.LevelWarning)
	assert.Contains(issueCodes(resultWarningFindings7), "validation_attribute_value_regex_not_matched")

	valuesEvent := validValidationEvent()
	valuesEvent["level"] = json.Number("3")
	result, err = pipeline.ProcessEvent(valuesEvent)
	assert.NoError(err)
	resultErrorFindings31 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.Contains(issueCodes(resultErrorFindings31), "validation_attribute_value_not_in_type_values")
}

func TestValidationConstraintFindingDetailsDoNotExposeSchemaSlices(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation())

	constraintEvent := validValidationEvent()
	delete(constraintEvent, "name")
	result, err := pipeline.ProcessEvent(constraintEvent)
	assert.NoError(err)
	resultErrorFindings32 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	constraintFinding := issuesWithCode(resultErrorFindings32, "validation_constraint_failed")
	assert.Len(constraintFinding, 1)
	constraint, ok := constraintFinding[0].Details["constraint"].(jsonish.Map)
	assert.True(ok)
	paths, ok := constraint["at_least_one"].([]string)
	assert.True(ok)
	paths[0] = "changed"
	assert.Equal("name", schema.compiledForTest().Classes[1].Constraints["at_least_one"][0])
}

func TestProcessEventValidationNonFiniteFloatRanges(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	trueValue := true
	schema.compiledForTest().Dictionary.Types.Attributes["float_t"] = &typeDefinition{}
	schema.compiledForTest().Dictionary.Types.Attributes["bounded_float_t"] = &typeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "float_t"},
		Range:                     []int64{-10, 10},
	}
	schema.compiledForTest().Dictionary.Types.Attributes["wide_bounded_float_t"] = &typeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "float_t"},
		Range:                     []int64{math.MinInt64, math.MaxInt64},
	}
	schema.compiledForTest().Classes[int64(1)].Attributes["float_value"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "float_t"},
	}
	schema.compiledForTest().Classes[int64(1)].Attributes["bounded_float"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "bounded_float_t"},
	}
	schema.compiledForTest().Classes[int64(1)].Attributes["bounded_floats"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "bounded_float_t", IsArray: &trueValue},
	}
	schema.compiledForTest().Classes[int64(1)].Attributes["wide_bounded_float"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "wide_bounded_float_t"},
	}
	schema.compiledForTest().Classes[int64(1)].Attributes["wide_bounded_floats"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "wide_bounded_float_t", IsArray: &trueValue},
	}
	pipeline := mustNewPipeline(assert, schema, WithValidation())

	integralEvent := validValidationEvent()
	integralEvent["float_value"] = int64(7)
	integralEvent["bounded_float"] = int32(-7)
	integralEvent["bounded_floats"] = []int16{-10, 0, 10}
	integralResult, err := pipeline.ProcessEvent(integralEvent)
	assert.NoError(err)
	integralResultErrorFindings := findingsAtLevel(integralResult.Validation().Findings, validation.LevelError)
	assert.NotContains(
		issueAttributePaths(issuesWithCode(integralResultErrorFindings, "validation_attribute_wrong_type")),
		"float_value",
	)
	integralResultErrorFindings2 := findingsAtLevel(integralResult.Validation().Findings, validation.LevelError)
	assert.Empty(issuesWithCode(integralResultErrorFindings2, "validation_attribute_value_exceeds_range"))

	boundaryEvent := validValidationEvent()
	boundaryEvent["wide_bounded_float"] = json.Number("9.223372036854775e18")
	boundaryEvent["wide_bounded_floats"] = []int64{math.MinInt64, math.MaxInt64}
	boundaryResult, err := pipeline.ProcessEvent(boundaryEvent)
	assert.NoError(err)
	boundaryResultErrorFindings := findingsAtLevel(boundaryResult.Validation().Findings, validation.LevelError)
	assert.Empty(issuesWithCode(boundaryResultErrorFindings, "validation_attribute_value_exceeds_range"))

	outOfBoundaryEvent := validValidationEvent()
	outOfBoundaryEvent["wide_bounded_float"] = float64(uint64(1) << 63)
	outOfBoundaryResult, err := pipeline.ProcessEvent(outOfBoundaryEvent)
	assert.NoError(err)
	outOfBoundaryResultErrorFindings := findingsAtLevel(
		outOfBoundaryResult.Validation().Findings, validation.LevelError,
	)
	assert.Contains(
		issueAttributePaths(
			issuesWithCode(outOfBoundaryResultErrorFindings, "validation_attribute_value_exceeds_range"),
		),
		"wide_bounded_float",
	)

	for _, testCase := range []struct {
		name  string
		value any
	}{
		{name: "native NaN", value: math.NaN()},
		{name: "native positive infinity", value: math.Inf(1)},
		{name: "native negative infinity", value: math.Inf(-1)},
		{name: "JSON Number NaN", value: json.Number("NaN")},
		{name: "JSON Number positive infinity", value: json.Number("+Inf")},
		{name: "JSON Number negative infinity", value: json.Number("-Inf")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert := require.New(t)
			event := validValidationEvent()
			event["float_value"] = testCase.value
			event["bounded_float"] = testCase.value

			result, err := pipeline.ProcessEvent(event)

			assert.NoError(err)
			resultErrorFindings34 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
			assert.NotContains(
				issueAttributePaths(issuesWithCode(resultErrorFindings34, "validation_attribute_wrong_type")),
				"float_value",
			)
			resultErrorFindings35 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
			assert.Contains(
				issueAttributePaths(issuesWithCode(resultErrorFindings35, "validation_attribute_value_exceeds_range")),
				"bounded_float",
			)
		})
	}

	for _, testCase := range []struct {
		name  string
		value any
	}{
		{name: "native typed slice", value: []float64{math.NaN(), math.Inf(1), math.Inf(-1)}},
		{name: "JSON Number typed slice", value: []json.Number{"NaN", "+Inf", "-Inf"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert := require.New(t)
			event := validValidationEvent()
			event["bounded_floats"] = testCase.value

			result, err := pipeline.ProcessEvent(event)

			assert.NoError(err)
			resultErrorFindings36 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
			assert.ElementsMatch(
				[]string{"bounded_floats[0]", "bounded_floats[1]", "bounded_floats[2]"},
				issueAttributePaths(issuesWithCode(resultErrorFindings36, "validation_attribute_value_exceeds_range")),
			)
		})
	}
}

func TestProcessEventValidationMaxLenCountsUnicodeCodePoints(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	// The combining acute accent is a distinct Unicode code point, so this
	// value contains four code points even though it has three graphemes.
	event["short_text"] = "e\u0301ab"

	result, err := mustNewPipeline(assert, si, WithValidation()).ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings37 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	issues := issuesWithCode(resultErrorFindings37, "validation_attribute_value_exceeds_max_len")
	assert.Len(issues, 1)
	assert.Equal(4, issues[0].Details["length"])
}

func TestProcessEventValidationTypeValuesDoNotRequireJSONNumbers(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	si.compiledForTest().Dictionary.Types.Attributes["level_t"].Values = []any{int64(9007199254740993)}
	event := validValidationEvent()
	event["level"] = json.Number("9007199254740993")

	result, err := mustNewPipeline(assert, si, WithValidation()).ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings38 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.NotContains(issueCodes(resultErrorFindings38), "validation_attribute_value_not_in_type_values")
}

func TestProcessEventValidationTypeValuesCompareEquivalentJSONNumbersNumerically(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	si.compiledForTest().Dictionary.Types.Attributes["float_t"] = &typeDefinition{}
	si.compiledForTest().Dictionary.Types.Attributes["decimal_level_t"] = &typeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "float_t"},
		Values:                    []any{json.Number("1.0")},
	}
	si.compiledForTest().Classes[int64(1)].Attributes["decimal_level"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "decimal_level_t"},
	}
	event := validValidationEvent()
	event["decimal_level"] = json.Number("1.00")

	result, err := mustNewPipeline(assert, si, WithValidation()).ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings39 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	assert.NotContains(issueCodes(resultErrorFindings39), "validation_attribute_value_not_in_type_values")
}

func TestProcessEventValidationIntegerAndLongBounds(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, si, WithValidation())

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
		{name: "integer integral decimal", attribute: "count", value: json.Number("1.0")},
		{name: "integer fractional decimal", attribute: "count", value: json.Number("1.5"), wantWrong: true},
		{name: "long min", attribute: "long_value", value: json.Number(strconv.FormatInt(math.MinInt64, 10))},
		{name: "long max", attribute: "long_value", value: json.Number(strconv.FormatInt(math.MaxInt64, 10))},
		{name: "long below min", attribute: "long_value", value: json.Number("-9223372036854775809"), wantWrong: true},
		{name: "long above max", attribute: "long_value", value: json.Number("9223372036854775808"), wantWrong: true},
		{name: "long integral decimal", attribute: "long_value", value: json.Number("1.0")},
		{name: "long fractional decimal", attribute: "long_value", value: json.Number("1.5"), wantWrong: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := require.New(t)
			event := validValidationEvent()
			event[testCase.attribute] = testCase.value

			result, err := pipeline.ProcessEvent(event)

			assert.NoError(err)
			resultErrorFindings40 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
			wrongTypePaths := issueAttributePaths(
				issuesWithCode(resultErrorFindings40, "validation_attribute_wrong_type"),
			)
			if testCase.wantWrong {
				assert.Contains(wrongTypePaths, testCase.attribute)
			} else {
				assert.NotContains(wrongTypePaths, testCase.attribute)
			}
		})
	}
}

func TestProcessEventValidationDetailsOmitEventValues(t *testing.T) {
	assert := require.New(t)
	pipeline := mustNewPipeline(assert, makeValidationTestSchema(assert), WithValidation())

	for _, testCase := range []struct {
		name  string
		value any
	}{
		{name: "object", value: jsonish.Map{"nested": "value"}},
		{name: "array", value: []any{"value"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert := require.New(t)
			event := validValidationEvent()
			event["port"] = testCase.value

			result, err := pipeline.ProcessEvent(event)

			assert.NoError(err)
			resultErrorFindings41 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
			errors := issuesWithCode(resultErrorFindings41, "validation_attribute_wrong_type")
			assert.Len(errors, 1)
			assert.Equal("port", errors[0].Details["attribute_path"])
			assert.NotContains(errors[0].Details, "value")
		})
	}

	event := validValidationEvent()
	event["port"] = "not an integer"
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	resultErrorFindings42 := findingsAtLevel(result.Validation().Findings, validation.LevelError)
	errors := issuesWithCode(resultErrorFindings42, "validation_attribute_wrong_type")
	assert.Len(errors, 1)
	assert.NotContains(errors[0].Details, "value")
}

func TestProcessEventValidationResultDoesNotRepeatEventValues(t *testing.T) {
	assert := require.New(t)
	const sensitiveValue = "sensitive-event-value-7f4c2a"
	event := validValidationEvent()
	metadata, ok := event["metadata"].(jsonish.Map)
	assert.True(ok)
	metadata["version"] = sensitiveValue
	metadata["profiles"] = []any{sensitiveValue}
	event["mode_id"] = json.Number("1")
	event["mode"] = sensitiveValue
	event["code"] = sensitiveValue
	event["observables"] = []any{jsonish.Map{
		"name":    sensitiveValue,
		"type_id": json.Number("1000"),
		"value":   sensitiveValue,
	}}

	result, err := mustNewPipeline(
		assert,
		makeValidationTestSchema(assert),
		WithValidation(),
	).ProcessEvent(event)

	assert.NoError(err)
	assert.NotEmpty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
	assert.NotEmpty(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))
	encoded, err := json.Marshal(result.Validation())
	assert.NoError(err)
	assert.NotContains(string(encoded), sensitiveValue)
}

func TestProcessEventAcceptsIntegralFloatingPointRepresentationsForIntegerTypes(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["class_uid"] = float64(1)
	event["activity_id"] = float32(1)
	event["type_uid"] = json.Number("101.0")
	event["count"] = float64(7)
	event["long_value"] = json.Number("8e0")
	event["float_value"] = json.Number("9")
	schema.compiledForTest().Dictionary.Types.Attributes["float_t"] = &typeDefinition{}
	schema.compiledForTest().Classes[int64(1)].Attributes["float_value"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "float_t"},
	}

	result, err := mustNewPipeline(assert, schema, WithValidation()).ProcessEvent(event)

	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation().Findings, validation.LevelError))
}

func TestProcessEventValidationDeprecationWarnings(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	si.compiledForTest().Classes[int64(1)].Deprecated = &deprecatedDefinition{
		Since: "1.0.0", Message: "class deprecated",
	}
	si.compiledForTest().Classes[int64(1)].Attributes["red"].Deprecated = &deprecatedDefinition{
		Since:   "1.0.0",
		Message: "attribute deprecated",
	}
	si.compiledForTest().Classes[int64(1)].Attributes["mode_id"].Enum["1"].Deprecated = &deprecatedDefinition{
		Since:   "1.0.0",
		Message: "enum deprecated",
	}
	si.compiledForTest().Objects["ball"].Deprecated = &deprecatedDefinition{
		Since: "1.0.0", Message: "object deprecated",
	}

	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["ball"] = jsonish.Map{"green": "go"}
	pipeline := mustNewPipeline(assert, si, WithValidation())
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	warningCodes := issueCodes(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))
	assert.Contains(warningCodes, "validation_class_deprecated")
	assert.Contains(warningCodes, "validation_attribute_deprecated")
	assert.Contains(warningCodes, "validation_attribute_enum_value_deprecated")
	assert.Contains(warningCodes, "validation_object_deprecated")
}

func TestProcessEventValidationDeprecatedAttributeTypeWarning(t *testing.T) {
	for _, test := range []struct {
		name                string
		attributeDeprecated bool
		wantTypeWarning     bool
	}{
		{name: "active attribute", wantTypeWarning: true},
		{name: "deprecated attribute", attributeDeprecated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			si := makeValidationTestSchema(assert)
			si.compiledForTest().Dictionary.Types.Attributes["port_t"].Deprecated = &deprecatedDefinition{
				Since: "1.1.0", Message: "port type deprecated",
			}
			if test.attributeDeprecated {
				si.compiledForTest().Classes[int64(1)].Attributes["port"].Deprecated = &deprecatedDefinition{
					Since: "1.2.0", Message: "port attribute deprecated",
				}
			}

			event := validValidationEvent()
			event["port"] = json.Number("443")
			result, err := mustNewPipeline(assert, si, WithValidation()).ProcessEvent(event)

			assert.NoError(err)
			warningCodes := issueCodes(findingsAtLevel(result.Validation().Findings, validation.LevelWarning))
			if test.attributeDeprecated {
				assert.Contains(warningCodes, "validation_attribute_deprecated")
			}
			if test.wantTypeWarning {
				assert.Contains(warningCodes, "validation_attribute_type_deprecated")
			} else {
				assert.NotContains(warningCodes, "validation_attribute_type_deprecated")
			}
		})
	}
}

func TestProcessEventPipelineOptions(t *testing.T) {
	assert := require.New(t)
	si := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["ball"] = jsonish.Map{"green": "go"}

	pipeline := mustNewPipeline(assert, si,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.None),
	)
	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	assert.Equal(3, result.Enrichment().EnumSiblingsAdded)
	assert.Zero(result.Enrichment().ObservablesAdded)
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

type resultFinding interface {
	eventresult.ProcessingIssue | eventresult.ValidationFinding
}

func findingsAtLevel(findings []eventresult.ValidationFinding, level validation.Level) []eventresult.ValidationFinding {
	selected := make([]eventresult.ValidationFinding, 0)
	for _, finding := range findings {
		if finding.Level == level {
			selected = append(selected, finding)
		}
	}
	return selected
}

func issueCodes[T resultFinding](issues []T) []string {
	codes := make([]string, len(issues))
	for i, issue := range issues {
		codes[i] = resultFindingCode(issue)
	}
	return codes
}

func resultFindingCode[T resultFinding](finding T) string {
	switch finding := any(finding).(type) {
	case eventresult.ProcessingIssue:
		return finding.Code.String()
	case eventresult.ValidationFinding:
		return finding.Code.String()
	default:
		return ""
	}
}

func issueAttributePaths[T resultFinding](issues []T) []string {
	paths := make([]string, len(issues))
	for i, issue := range issues {
		switch issue := any(issue).(type) {
		case eventresult.ProcessingIssue:
			paths[i], _ = issue.Details["attribute_path"].(string)
		case eventresult.ValidationFinding:
			paths[i], _ = issue.Details["attribute_path"].(string)
		}
	}
	return paths
}

func issuesWithCode[T resultFinding](issues []T, code string) []T {
	result := make([]T, 0)
	for _, issue := range issues {
		if resultFindingCode(issue) == code {
			result = append(result, issue)
		}
	}
	return result
}
