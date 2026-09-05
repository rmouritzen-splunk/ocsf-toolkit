package eventpipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/internal/observablepath"
	"github.com/ocsf/ocsf-toolkit/internal/semver"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func BenchmarkProcessEventValidation(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventValidationSelection(b *testing.B) {
	suites := []struct {
		name     string
		baseline PipelineOption
	}{
		{name: "no_observables_force_remove", baseline: WithObservables(enrichment.ForceRemove)},
		{name: "no_enum_siblings_safe_remove", baseline: WithEnumSiblings(enrichment.Remove)},
	}
	benchmarks := []struct {
		name       string
		validation PipelineOption
	}{
		{name: "default", validation: WithValidation()},
		{
			name: "one_nontriggering",
			validation: WithValidation(
				WithAllValidationLevels(validation.LevelIgnored),
				WithValidationLevel(validation.AttributeDeprecated, validation.LevelWarning),
			),
		},
		{name: "none"},
	}
	for _, suite := range suites {
		b.Run(suite.name, func(b *testing.B) {
			for _, benchmark := range benchmarks {
				b.Run(benchmark.name, func(b *testing.B) {
					assert := require.New(b)
					schema := makeValidationTestSchema(assert)
					options := []PipelineOption{suite.baseline}
					if benchmark.validation != nil {
						options = append(options, benchmark.validation)
					}
					pipeline := mustNewPipeline(assert, schema, options...)
					event := validValidationEvent()
					result, err := pipeline.ProcessEvent(event)
					assert.NoError(err)
					assert.Empty(result.Validation().Findings)
					b.ReportAllocs()
					b.ResetTimer()

					for b.Loop() {
						if _, err := pipeline.ProcessEvent(event); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func BenchmarkProcessEventValidationWrongTypeOnly(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation(
		WithAllValidationLevels(validation.LevelIgnored),
		WithValidationLevel(validation.AttributeWrongType, validation.LevelWarning),
	))
	event := validValidationEvent()
	event["short_text"] = "abcd"
	for index := range 64 {
		event[fmt.Sprintf("unknown_%02d", index)] = "value"
	}
	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Empty(result.Validation().Findings)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventInactiveProfileAttribute(b *testing.B) {
	benchmarks := []struct {
		name  string
		value any
	}{
		{name: "valid_primitive", value: "inactive"},
		{name: "invalid_primitive", value: json.Number("1")},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			assert := require.New(b)
			schema := makeValidationTestSchema(assert)
			pipeline := mustNewPipeline(assert, schema, WithValidation())
			event := validValidationEvent()
			event["profile_attr"] = benchmark.value
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				if _, err := pipeline.ProcessEvent(event); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProcessEventValidationPolicy(b *testing.B) {
	benchmarks := []struct {
		name      string
		policy    ValidationOption
		wantLevel validation.Level
	}{
		{name: "reported", wantLevel: validation.LevelError},
		{
			name:      "ignore_selected",
			wantLevel: validation.LevelIgnored,
			policy: WithValidationLevel(
				validation.AttributeWrongType, validation.LevelIgnored,
			),
		},
		{
			name:      "error_as_warning",
			policy:    WithValidationLevel(validation.AttributeWrongType, validation.LevelWarning),
			wantLevel: validation.LevelWarning,
		},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			assert := require.New(b)
			schema := makeValidationTestSchema(assert)
			options := make([]ValidationOption, 0, 1)
			if benchmark.policy != nil {
				options = append(options, benchmark.policy)
			}
			pipeline := mustNewPipeline(assert, schema, WithValidation(options...))
			event := validValidationEvent()
			event["port"] = "invalid"
			result, err := pipeline.ProcessEvent(event)
			assert.NoError(err)
			matching := issuesWithCode(result.Validation().Findings, validation.AttributeWrongType.String())
			if benchmark.wantLevel == validation.LevelIgnored {
				assert.Empty(matching)
			} else {
				assert.Len(matching, 1)
				assert.Equal(benchmark.wantLevel, matching[0].Level)
			}
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				if _, err := pipeline.ProcessEvent(event); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProcessEventEnrichment(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	event := validValidationEvent()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		delete(event, "class_name")
		delete(event, "activity_name")
		delete(event, "observables")
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventCombined(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
		WithValidation(),
	)
	event := validValidationEvent()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		delete(event, "class_name")
		delete(event, "activity_name")
		delete(event, "observables")
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventEnrichmentRemoval(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Remove),
		WithObservables(enrichment.None),
	)
	event := validValidationEvent()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		event["class_name"] = "Alpha"
		event["activity_name"] = "Do"
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventIssueReporting(b *testing.B) {
	benchmarks := []struct {
		name   string
		policy PipelineOption
	}{
		{name: "reported"},
		{name: "ignore_all", policy: WithAllIssueLevels(issue.LevelIgnored)},
		{
			name:   "ignore_selected",
			policy: WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelIgnored),
		},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			assert := require.New(b)
			schema := makeValidationTestSchema(assert)
			options := []PipelineOption{WithEnumSiblings(enrichment.Add), WithObservables(enrichment.None)}
			if benchmark.policy != nil {
				options = append(options, benchmark.policy)
			}
			pipeline := mustNewPipeline(assert, schema, options...)
			event := validValidationEvent()
			event["activity_id"] = json.Number("1234")
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				delete(event, "class_name")
				if _, err := pipeline.ProcessEvent(event); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProcessEventIgnoredIssueCleanPath(b *testing.B) {
	benchmarks := []struct {
		name    string
		options []PipelineOption
	}{
		{name: "collect_all"},
		{name: "ignore_all", options: []PipelineOption{WithAllIssueLevels(issue.LevelIgnored)}},
		{
			name:    "ignore_selected",
			options: []PipelineOption{WithIssueLevel(issue.EnrichmentEnumSiblingNotAdded, issue.LevelIgnored)},
		},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			assert := require.New(b)
			schema := makeValidationTestSchema(assert)
			options := append(
				[]PipelineOption{WithEnumSiblings(enrichment.Add), WithObservables(enrichment.None)},
				benchmark.options...,
			)
			pipeline := mustNewPipeline(assert, schema, options...)
			event := validValidationEvent()
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				delete(event, "class_name")
				delete(event, "activity_name")
				if _, err := pipeline.ProcessEvent(event); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProcessEventObservableRemoval(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.None), WithObservables(enrichment.Remove))
	event := validValidationEvent()
	observables := []any{jsonish.Map{"name": "name", "type_id": int64(1000), "value": "event name"}}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		event["observables"] = observables
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventNestedArray(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("2")}
	event["statuses"] = []any{"Open", "Closed"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventTypedSlices(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["status_ids"] = []int64{1, 2}
	event["statuses"] = []string{"Open", "Closed"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventObservableHeavy(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
		WithValidation(),
	)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		delete(event, "class_name")
		delete(event, "activity_name")
		delete(event, "observables")
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventAddObservables(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.None),
		WithObservables(enrichment.Add),
	)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		delete(event, "observables")
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventAddSelectedObservables(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	schema.compiledForTest().ObservableTypes[1000] = "Test observable"
	pipeline := mustNewPipeline(
		assert,
		schema,
		WithEnumSiblings(enrichment.None), WithObservables(enrichment.Add, 1000),
	)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		delete(event, "observables")
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventAddEnumSiblings(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.None),
	)
	event := validValidationEvent()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		delete(event, "class_name")
		delete(event, "activity_name")
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventAddEnumSiblingsAndObservables(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		delete(event, "class_name")
		delete(event, "activity_name")
		delete(event, "observables")
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventAddObservablesAndValidate(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(
		assert,
		schema,
		WithEnumSiblings(enrichment.None), WithObservables(enrichment.Add),
		WithValidation(),
	)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		delete(event, "observables")
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventMalformed(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["class_uid"] = json.Number("invalid")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventObservableStructuralFinding(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["observables"] = []any{"not an object"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventObservableSemanticFinding(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["observables"] = []any{jsonish.Map{"name": "missing", "value": "value"}}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventSuperTypeConstraintFinding(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	schema.compiledForTest().Dictionary.Types.Attributes["derived_bounded_int_t"] = &typeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "bounded_int_t"},
	}
	schema.compiledForTest().Classes[1].Attributes["derived_bounded_count"] = &itemAttributeDefinition{
		CommonAttributeDefinition: commonAttributeDefinition{Type: "derived_bounded_int_t"},
	}
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["derived_bounded_count"] = json.Number("11")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventEnumSiblingRemovalFinding(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Remove),
		WithObservables(enrichment.None),
	)
	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["mode"] = "Mismatch"
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventEnumSiblingRemovalFindingIgnored(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Remove),
		WithObservables(enrichment.None),
		WithIssueLevel(issue.EnrichmentRemovalEnumSiblingNotRemoved, issue.LevelIgnored),
	)
	event := validValidationEvent()
	event["mode_id"] = json.Number("1")
	event["mode"] = "Mismatch"
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventAllowedIntegerValue(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	schema.compiledForTest().Dictionary.Types.Attributes["level_t"].Values = []any{int64(9007199254740993)}
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["level"] = json.Number("9007199254740993")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventScalarConstraints(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["bounded_count"] = json.Number("5")
	event["short_text"] = "abc"
	event["code"] = "ABC"
	event["level"] = json.Number("1")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseVersionRepeated(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := semver.Parse("1.7.0-custom.1"); !ok {
			b.Fatal("version was not parsed")
		}
	}
}

func BenchmarkParseVersionRotating(b *testing.B) {
	versions := make([]string, 128)
	for index := range versions {
		versions[index] = fmt.Sprintf("1.7.%d-custom.%d", index, index)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; b.Loop(); index++ {
		if _, ok := semver.Parse(versions[index%len(versions)]); !ok {
			b.Fatal("version was not parsed")
		}
	}
}

func BenchmarkParseObservablePathRepeated(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := observablepath.Parse("actor.user.groups[].name"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseObservablePathRotating(b *testing.B) {
	paths := make([]string, 128)
	for index := range paths {
		paths[index] = fmt.Sprintf("objects[%d].items[].value", index)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; b.Loop(); index++ {
		if _, err := observablepath.Parse(paths[index%len(paths)]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventConstraintPath(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	schema.compiledForTest().Classes[1].Constraints = map[string][]string{"just_one": {"ball.green"}}
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadSchema(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		schema, _, err := NewSchema(testSchemaFilePath)
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(schema)
	}
}

func BenchmarkLoadSchemaFromReader(b *testing.B) {
	data, err := os.ReadFile(testSchemaFilePath)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		schema, _, err := NewSchemaFromReader(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(schema)
	}
}

func BenchmarkLoadSchemaFromBytes(b *testing.B) {
	data, err := os.ReadFile(testSchemaFilePath)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		schema, _, err := NewSchemaFromBytes(data)
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(schema)
	}
}

func BenchmarkLoadSchemaWithValidationPipeline(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		schema, _, err := NewSchema(testSchemaFilePath)
		if err != nil {
			b.Fatal(err)
		}
		pipeline, err := newPipelineForSchema(schema, WithValidation())
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(pipeline)
	}
}

func BenchmarkSchemaRetained(b *testing.B) {
	data, err := os.ReadFile(testSchemaFilePath)
	if err != nil {
		b.Fatal(err)
	}
	var retainedBytes int64
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)

		schema, _, err := NewSchemaFromBytes(data)
		if err != nil {
			b.Fatal(err)
		}
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		//nolint:gosec // heap byte counts never approach int64's range.
		retainedBytes += int64(after.HeapAlloc) - int64(before.HeapAlloc)
		runtime.KeepAlive(schema)
	}
	b.ReportMetric(float64(retainedBytes)/float64(b.N), "retained-B/op")
	runtime.KeepAlive(data)
}

func BenchmarkValidationMetadataRetained(b *testing.B) {
	var retainedBytes int64
	b.ReportAllocs()
	for b.Loop() {
		schema, _, err := NewSchema(testSchemaFilePath)
		if err != nil {
			b.Fatal(err)
		}
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)

		pipeline, err := newPipelineForSchema(schema, WithValidation())
		if err != nil {
			b.Fatal(err)
		}
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		//nolint:gosec // heap byte counts never approach int64's range.
		retainedBytes += int64(after.HeapAlloc) - int64(before.HeapAlloc)
		runtime.KeepAlive(pipeline)
	}
	b.ReportMetric(float64(retainedBytes)/float64(b.N), "retained-B/op")
}
