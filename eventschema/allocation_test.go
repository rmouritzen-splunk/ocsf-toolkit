package eventschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func TestProcessEventAllocationCeilings(t *testing.T) {
	// These ceilings are regression budgets. Do not loosen them to make another change pass;
	// changing a ceiling requires explicit human agreement supported by benchmark evidence.
	tests := []struct {
		name    string
		ceiling float64
		setup   func(*require.Assertions) (EventProcessorPipeline, jsonish.Map, func(jsonish.Map))
	}{
		{name: "validation", ceiling: 4, setup: allocationValidationCase},
		{name: "enrichment", ceiling: 8, setup: allocationEnrichmentCase},
		{name: "enrichment removal", ceiling: 1, setup: allocationRemovalCase},
		{name: "combined", ceiling: 8, setup: allocationCombinedCase},
		{name: "typed slices", ceiling: 28, setup: allocationTypedSlicesCase},
		{name: "nested arrays", ceiling: 20, setup: allocationNestedArrayCase},
		{name: "observable heavy", ceiling: 28, setup: allocationObservableCase},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			pipeline, event, reset := test.setup(assert)
			allocations := testing.AllocsPerRun(1000, func() {
				reset(event)
				if _, err := pipeline.ProcessEvent(event); err != nil {
					panic(err)
				}
			})
			assert.LessOrEqual(allocations, test.ceiling,
				"allocations per event exceeded the regression ceiling")
		})
	}
}

func allocationValidationCase(assert *require.Assertions) (EventProcessorPipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	return mustNewEventProcessorPipeline(assert, schema, NewValidation()), validValidationEvent(), func(jsonish.Map) {}
}

func allocationEnrichmentCase(assert *require.Assertions) (EventProcessorPipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	return mustNewEventProcessorPipeline(assert, schema, NewEnrichment()), validValidationEvent(), resetEnrichedEvent
}

func allocationRemovalCase(assert *require.Assertions) (EventProcessorPipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichmentRemoval(WithRemoveObservables(false)))
	return pipeline, validValidationEvent(), func(event jsonish.Map) {
		event["class_name"] = "Alpha"
		event["activity_name"] = "Do"
	}
}

func allocationCombinedCase(assert *require.Assertions) (EventProcessorPipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(), NewValidation())
	return pipeline, validValidationEvent(), resetEnrichedEvent
}

func allocationTypedSlicesCase(assert *require.Assertions) (EventProcessorPipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["status_ids"] = []int64{1, 2}
	event["statuses"] = []string{"Open", "Closed"}
	return mustNewEventProcessorPipeline(assert, schema, NewValidation()), event, func(jsonish.Map) {}
}

func allocationNestedArrayCase(assert *require.Assertions) (EventProcessorPipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("2")}
	event["statuses"] = []any{"Open", "Closed"}
	return mustNewEventProcessorPipeline(assert, schema, NewValidation()), event, func(jsonish.Map) {}
}

func allocationObservableCase(assert *require.Assertions) (EventProcessorPipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(), NewValidation())
	return pipeline, event, resetEnrichedEvent
}

func resetEnrichedEvent(event jsonish.Map) {
	delete(event, "class_name")
	delete(event, "activity_name")
	delete(event, "observables")
}
