package eventschema

import (
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func BenchmarkProcessEventValidation(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
	event := validValidationEvent()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventEnrichment(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichment())
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
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(), NewValidation())
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
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false)))
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

func BenchmarkProcessEventNestedArray(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
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
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
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
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(), NewValidation())
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

func BenchmarkProcessEventMalformed(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
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

func BenchmarkProcessEventAllowedIntegerValue(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	schema.dictionary.Types.Attributes["level_t"].Values = []any{int64(9007199254740993)}
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
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
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
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
		if _, ok := parseVersion("1.7.0-custom.1"); !ok {
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
		if _, ok := parseVersion(versions[index%len(versions)]); !ok {
			b.Fatal("version was not parsed")
		}
	}
}

func BenchmarkParseObservablePathRepeated(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseObservablePath("actor.user.groups[].name"); err != nil {
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
		if _, err := parseObservablePath(paths[index%len(paths)]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessEventConstraintPath(b *testing.B) {
	assert := require.New(b)
	schema := makeValidationTestSchema(assert)
	schema.classes[1].Constraints = map[string][]string{"just_one": {"ball.green"}}
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
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
		schema, err := New(testSchemaFilePath)
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(schema)
	}
}

func BenchmarkLoadSchemaWithValidationPipeline(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		schema, err := New(testSchemaFilePath)
		if err != nil {
			b.Fatal(err)
		}
		pipeline, err := schema.NewEventProcessorPipeline(NewValidation())
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(pipeline)
	}
}

func BenchmarkValidationMetadataRetained(b *testing.B) {
	var retainedBytes int64
	b.ReportAllocs()
	for b.Loop() {
		schema, err := New(testSchemaFilePath)
		if err != nil {
			b.Fatal(err)
		}
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)

		pipeline, err := schema.NewEventProcessorPipeline(NewValidation())
		if err != nil {
			b.Fatal(err)
		}
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		retainedBytes += int64(after.HeapAlloc) - int64(before.HeapAlloc)
		runtime.KeepAlive(pipeline)
	}
	b.ReportMetric(float64(retainedBytes)/float64(b.N), "retained-B/op")
}
