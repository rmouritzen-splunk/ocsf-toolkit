package eventschema

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/stretchr/testify/require"
)

func BenchmarkProcessEventNumericEnumEncodings(b *testing.B) {
	// This deterministic 100-event distribution models numeric-enum density observed across a diverse set of OCSF
	// event shapes: mean 19.98 numeric enums per event, p50 18, p75 24, p90 30, p95 36, and p99/max 38. It does not
	// claim that those event-shape frequencies represent a production event stream.
	targetCounts := make([]int, 0, 100)
	appendCount := func(count, repetitions int) {
		for range repetitions {
			targetCounts = append(targetCounts, count)
		}
	}
	appendCount(5, 24)
	appendCount(8, 1)
	appendCount(18, 25)
	appendCount(24, 25)
	appendCount(30, 15)
	appendCount(36, 5)
	appendCount(38, 5)

	benchmarks := []struct {
		name  string
		value any
	}{
		{name: "json_number_integer", value: json.Number("1")},
		{name: "json_number_integral_float", value: json.Number("1.0")},
		{name: "int64", value: int64(1)},
		{name: "float64", value: float64(1)},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			benchmarkNumericEnumEncoding(b, targetCounts, benchmark.value)
		})
	}
}

func benchmarkNumericEnumEncoding(b *testing.B, targetCounts []int, enumValue any) {
	assert := require.New(b)
	compiledSchema := makeValidationTestSchema(assert)
	class := compiledSchema.compiledForTest().Classes[1]
	definition := class.Attributes["activity_id"].Enum["1"]
	const builtInNumericEnumCount = 2
	for index := range targetCounts[len(targetCounts)-1] - builtInNumericEnumCount {
		attributeName := fmt.Sprintf("numeric_enum_%d_id", index)
		class.Attributes[attributeName] = &itemAttributeDefinition{
			CommonAttributeDefinition: commonAttributeDefinition{
				Type: "integer_t",
				Enum: map[string]*enumDefinition{"1": definition},
			},
		}
	}
	events := make([]jsonish.Map, len(targetCounts))
	for eventIndex, targetCount := range targetCounts {
		event := validValidationEvent()
		event["class_uid"] = enumValue
		event["activity_id"] = enumValue
		for index := range targetCount - builtInNumericEnumCount {
			attributeName := fmt.Sprintf("numeric_enum_%d_id", index)
			event[attributeName] = enumValue
		}
		events[eventIndex] = event
	}
	pipeline := mustNewPipeline(
		assert,
		compiledSchema,
		WithValidation(),
	)
	for _, event := range events {
		result, err := pipeline.ProcessEvent(event)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Validation().Findings) != 0 {
			b.Fatalf("unexpected validation findings: %+v", result.Validation().Findings)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()

	eventIndex := 0
	for b.Loop() {
		if _, err := pipeline.ProcessEvent(events[eventIndex]); err != nil {
			b.Fatal(err)
		}
		eventIndex++
		if eventIndex == len(events) {
			eventIndex = 0
		}
	}
}
