package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func runCLI(args ...string) (int, string, string) {
	return runCLIWithInput("", args...)
}

func runCLIWithInput(input string, args ...string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithIO(args, strings.NewReader(input), &stdout, &stderr)
	return exitCode, stdout.String(), stderr.String()
}

func summaryText(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func writeTestSchema(assert *require.Assertions, dir string) string {
	schemaPath := filepath.Join(dir, "schema.json")
	writeJSONFile(assert, schemaPath, jsonish.Map{
		"compile_version": 1,
		"version":         "1.0.0",
		"classes": jsonish.Map{
			"alpha": jsonish.Map{
				"name":     "alpha",
				"uid":      1,
				"category": "test",
				"attributes": jsonish.Map{
					"class_uid": jsonish.Map{
						"type":        "integer_t",
						"requirement": "required",
						"sibling":     "class_name",
						"enum": jsonish.Map{
							"1": jsonish.Map{"caption": "Alpha"},
						},
					},
					"class_name": jsonish.Map{"type": "string_t"},
					"activity_id": jsonish.Map{
						"type":        "integer_t",
						"requirement": "required",
						"sibling":     "activity_name",
						"enum": jsonish.Map{
							"1": jsonish.Map{"caption": "Do"},
						},
					},
					"activity_name": jsonish.Map{"type": "string_t"},
					"message": jsonish.Map{
						"type":        "string_t",
						"requirement": "recommended",
					},
					"type_uid": jsonish.Map{
						"type":        "long_t",
						"requirement": "required",
					},
					"metadata": jsonish.Map{
						"type":        "object_t",
						"object_type": "metadata",
						"requirement": "required",
					},
					"ball": jsonish.Map{
						"type":        "object_t",
						"object_type": "ball",
					},
					"observables": jsonish.Map{
						"type":        "object_t",
						"object_type": "observable",
						"is_array":    true,
					},
				},
			},
		},
		"objects": jsonish.Map{
			"metadata": jsonish.Map{
				"name": "metadata",
				"attributes": jsonish.Map{
					"version": jsonish.Map{
						"type":        "string_t",
						"requirement": "required",
					},
				},
			},
			"ball": jsonish.Map{
				"name": "ball",
				"attributes": jsonish.Map{
					"green": jsonish.Map{"type": "string_t"},
				},
			},
			"observable": jsonish.Map{
				"name": "observable",
				"attributes": jsonish.Map{
					"name":  jsonish.Map{"type": "string_t"},
					"value": jsonish.Map{"type": "string_t"},
					"type_id": jsonish.Map{
						"type": "integer_t",
						"enum": jsonish.Map{
							"0":    jsonish.Map{"caption": "Unknown"},
							"1000": jsonish.Map{"caption": "Selected type"},
							"2000": jsonish.Map{"caption": "Other type"},
						},
					},
				},
			},
		},
		"dictionary": jsonish.Map{
			"attributes": jsonish.Map{},
			"types": jsonish.Map{
				"attributes": jsonish.Map{
					"integer_t": jsonish.Map{"caption": "Integer"},
					"long_t":    jsonish.Map{"caption": "Long"},
					"string_t":  jsonish.Map{"caption": "String"},
					"object_t":  jsonish.Map{"caption": "Object"},
				},
			},
		},
		"profiles": jsonish.Map{},
	})
	return schemaPath
}

func writeInitializationIssueTestSchema(assert *require.Assertions, dir string) string {
	path := writeTestSchema(assert, dir)
	data, err := os.ReadFile(path)
	assert.NoError(err)
	var definition jsonish.Map
	assert.NoError(json.Unmarshal(data, &definition))
	classes, ok := definition["classes"].(map[string]any)
	assert.True(ok)
	alpha, ok := classes["alpha"].(map[string]any)
	assert.True(ok)
	attributes, ok := alpha["attributes"].(map[string]any)
	assert.True(ok)
	attributes["status_code"] = map[string]any{
		"type":    "integer_t",
		"sibling": "status_message",
		"enum": map[string]any{
			"1": map[string]any{"caption": "Entabulator has jammed"},
		},
	}
	attributes["status_message"] = map[string]any{"type": "string_t", "is_array": true}
	writeJSONFile(assert, path, definition)
	return path
}

// writeIgnoredIssueTestSchema writes a schema like writeTestSchema, but additionally maps "ball.green" to
// observable type ID 1000 at the class level, so a pre-existing duplicate observable entry can be detected
// during enrichment.
func writeIgnoredIssueTestSchema(assert *require.Assertions, dir string) string {
	schemaPath := filepath.Join(dir, "schema.json")
	writeJSONFile(assert, schemaPath, jsonish.Map{
		"compile_version": 1,
		"version":         "1.0.0",
		"classes": jsonish.Map{
			"alpha": jsonish.Map{
				"name":        "alpha",
				"uid":         1,
				"category":    "test",
				"observables": jsonish.Map{"ball.green": 1000, "balls.green": 1000},
				"attributes": jsonish.Map{
					"class_uid": jsonish.Map{
						"type":        "integer_t",
						"requirement": "required",
						"sibling":     "class_name",
						"enum": jsonish.Map{
							"1": jsonish.Map{"caption": "Alpha"},
						},
					},
					"class_name": jsonish.Map{"type": "string_t"},
					"activity_id": jsonish.Map{
						"type":        "integer_t",
						"requirement": "required",
						"sibling":     "activity_name",
						"enum": jsonish.Map{
							"1": jsonish.Map{"caption": "Do"},
						},
					},
					"activity_name": jsonish.Map{"type": "string_t"},
					"type_uid": jsonish.Map{
						"type":        "long_t",
						"requirement": "required",
					},
					"metadata": jsonish.Map{
						"type":        "object_t",
						"object_type": "metadata",
						"requirement": "required",
					},
					"ball": jsonish.Map{
						"type":        "object_t",
						"object_type": "ball",
					},
					"balls": jsonish.Map{
						"type":        "object_t",
						"object_type": "ball",
						"is_array":    true,
					},
					"observables": jsonish.Map{
						"type":        "object_t",
						"object_type": "observable",
						"is_array":    true,
					},
				},
			},
		},
		"objects": jsonish.Map{
			"metadata": jsonish.Map{
				"name": "metadata",
				"attributes": jsonish.Map{
					"version": jsonish.Map{
						"type":        "string_t",
						"requirement": "required",
					},
				},
			},
			"ball": jsonish.Map{
				"name": "ball",
				"attributes": jsonish.Map{
					"green": jsonish.Map{"type": "string_t"},
				},
			},
			"observable": jsonish.Map{
				"name": "observable",
				"attributes": jsonish.Map{
					"name":  jsonish.Map{"type": "string_t"},
					"value": jsonish.Map{"type": "string_t"},
					"type_id": jsonish.Map{
						"type": "integer_t",
						"enum": jsonish.Map{
							"0":    jsonish.Map{"caption": "Unknown"},
							"1000": jsonish.Map{"caption": "Selected type"},
							"2000": jsonish.Map{"caption": "Other type"},
						},
					},
				},
			},
		},
		"dictionary": jsonish.Map{
			"attributes": jsonish.Map{},
			"types": jsonish.Map{
				"attributes": jsonish.Map{
					"integer_t": jsonish.Map{"caption": "Integer"},
					"long_t":    jsonish.Map{"caption": "Long"},
					"string_t":  jsonish.Map{"caption": "String"},
					"object_t":  jsonish.Map{"caption": "Object"},
				},
			},
		},
		"profiles": jsonish.Map{},
	})
	return schemaPath
}

func validCLIEvent() jsonish.Map {
	return jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("101"),
		"metadata": jsonish.Map{
			"version": "1.0.0",
		},
	}
}

func writeJSONFile(assert *require.Assertions, path string, value any) {
	assert.NoError(os.MkdirAll(filepath.Dir(path), 0o750))
	data, err := json.MarshalIndent(value, "", "  ")
	assert.NoError(err)
	data = append(data, '\n')
	assert.NoError(os.WriteFile(path, data, 0o600))
}

func makeTestSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symbolic links are unavailable on this Windows host: %s", err)
		}
		require.NoError(t, err)
	}
}

func caseAliasPath(t *testing.T, path string) string {
	t.Helper()
	base := filepath.Base(path)
	aliasBase := strings.ToUpper(base[:1]) + base[1:]
	if aliasBase == base {
		aliasBase = strings.ToLower(base[:1]) + base[1:]
	}
	alias := filepath.Join(filepath.Dir(path), aliasBase)
	originalInfo, err := os.Stat(path)
	require.NoError(t, err)
	aliasInfo, err := os.Stat(alias)
	if err != nil || !os.SameFile(originalInfo, aliasInfo) {
		t.Skip("filesystem is case-sensitive")
	}
	return alias
}

func readEventReport(assert *require.Assertions, path string) eventReport {
	var output eventReport
	readJSONFile(assert, path, &output)
	return output
}

func readJSONFile(assert *require.Assertions, path string, target any) {
	data, err := os.ReadFile(path)
	assert.NoError(err)
	assert.NoError(json.Unmarshal(data, target))
}
