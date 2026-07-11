package eventschema

import (
	"encoding/json"
	"fmt"
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
