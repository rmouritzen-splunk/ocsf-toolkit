package eventpipeline

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func TestProcessEventAllocationCeilings(t *testing.T) {
	// These ceilings are regression budgets. Do not loosen them to make another change pass;
	// changing a ceiling requires explicit human agreement supported by benchmark evidence.
	tests := []struct {
		name    string
		ceiling float64
		setup   func(*require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map))
	}{
		{name: "validation", ceiling: 4, setup: allocationValidationCase},
		{name: "enrichment", ceiling: 8, setup: allocationEnrichmentCase},
		{name: "selected observable enrichment", ceiling: 8, setup: allocationSelectedObservableCase},
		{name: "enrichment removal", ceiling: 1, setup: allocationRemovalCase},
		{name: "combined", ceiling: 8, setup: allocationCombinedCase},
		{name: "typed slices", ceiling: 28, setup: allocationTypedSlicesCase},
		{name: "nested arrays", ceiling: 20, setup: allocationNestedArrayCase},
		{name: "observable heavy", ceiling: 28, setup: allocationObservableCase},
		{name: "enrichment detection finding", ceiling: 6600, setup: allocationDetectionFindingCase},
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

func TestEngineeringInvariantValidationDoesNotAllocateUnusedEnumSiblingPaths(t *testing.T) {
	// Engineering invariant test: validating valid enum-sibling pairs does not render diagnostic paths unless a
	// finding needs them.
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	objectDefinition := schema.compiledForTest().Objects["ball"]
	event := validValidationEvent()
	object := jsonish.Map{"green": "go"}
	event["ball"] = object
	for index := range 100 {
		enumAttributeName := fmt.Sprintf("extra_enum_%d_id", index)
		siblingAttributeName := fmt.Sprintf("extra_enum_%d", index)
		objectDefinition.Attributes[enumAttributeName] = &itemAttributeDefinition{
			CommonAttributeDefinition: commonAttributeDefinition{
				Type:    "integer_t",
				Sibling: &siblingAttributeName,
				Enum:    map[string]*enumDefinition{"1": {Caption: "Known"}},
			},
		}
		objectDefinition.Attributes[siblingAttributeName] = &itemAttributeDefinition{
			CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
		}
		object[enumAttributeName] = int64(1)
		object[siblingAttributeName] = "Known"
	}
	pipeline := mustNewPipeline(assert, schema, WithValidation())
	allocations := testing.AllocsPerRun(100, func() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			panic(err)
		}
	})
	assert.LessOrEqual(allocations, float64(10), "unused enum-sibling paths exceeded the allocation budget")
}

func allocationSelectedObservableCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	schema.compiledForTest().ObservableTypes[1000] = "Test observable"
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	pipeline := mustNewPipeline(
		assert,
		schema,
		WithEnumSiblings(enrichment.None), WithObservables(enrichment.Add, 1000),
	)
	return pipeline, event, func(event jsonish.Map) { delete(event, "observables") }
}

func allocationValidationCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	return mustNewPipeline(assert, schema, WithValidation()), validValidationEvent(), func(jsonish.Map) {}
}

func allocationEnrichmentCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	return pipeline, validValidationEvent(), resetEnrichedEvent
}

func allocationRemovalCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Remove),
		WithObservables(enrichment.None),
	)
	return pipeline, validValidationEvent(), func(event jsonish.Map) {
		event["class_name"] = "Alpha"
		event["activity_name"] = "Do"
	}
}

func allocationCombinedCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
		WithValidation(),
	)
	return pipeline, validValidationEvent(), resetEnrichedEvent
}

func allocationTypedSlicesCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["status_ids"] = []int64{1, 2}
	event["statuses"] = []string{"Open", "Closed"}
	return mustNewPipeline(assert, schema, WithValidation()), event, func(jsonish.Map) {}
}

func allocationNestedArrayCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["status_ids"] = []any{json.Number("1"), json.Number("2")}
	event["statuses"] = []any{"Open", "Closed"}
	return mustNewPipeline(assert, schema, WithValidation()), event, func(jsonish.Map) {}
}

func allocationObservableCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeValidationTestSchema(assert)
	event := validValidationEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
		WithValidation(),
	)
	return pipeline, event, resetEnrichedEvent
}

// allocationDetectionFindingCase budgets enrichment of a representative event
// against the released schema fixture. The small cases above cannot detect
// regressions in paths that only a real schema and a deeply nested event reach.
func allocationDetectionFindingCase(assert *require.Assertions) (*Pipeline, jsonish.Map, func(jsonish.Map)) {
	schema := makeRealSchema(assert)
	pipeline := mustNewPipeline(assert, schema,
		WithEnumSiblings(enrichment.Add),
		WithObservables(enrichment.Add),
	)
	return pipeline, benchmarkDetectionFinding(), resetBenchmarkEvent
}

func resetEnrichedEvent(event jsonish.Map) {
	delete(event, "class_name")
	delete(event, "activity_name")
	delete(event, "observables")
}
