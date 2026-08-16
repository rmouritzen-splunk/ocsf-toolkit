package processing

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

func TestPipelineInitializesSharedCachesLazily(t *testing.T) {
	assert := require.New(t)
	processingSchema := makeValidationTestSchema(assert)
	assert.Nil(processingSchema.compiled.Classes[1].OrderedAttributes)

	enrichmentPipeline := mustNewEventProcessorPipeline(assert, processingSchema, NewEnrichment())
	assert.Nil(enrichmentPipeline.validation)
	for _, class := range processingSchema.compiled.Classes {
		assert.True(slices.IsSortedFunc(class.OrderedAttributes, func(left, right schema.OrderedAttribute) int {
			return strings.Compare(left.Name, right.Name)
		}))
		assertOrderedAttributesResolved(assert, processingSchema.compiled, &class.ItemDefinition)
		assert.True(sort.StringsAreSorted(class.OrderedConstraintKeys))
	}
	for _, object := range processingSchema.compiled.Objects {
		assert.True(slices.IsSortedFunc(object.OrderedAttributes, func(left, right schema.OrderedAttribute) int {
			return strings.Compare(left.Name, right.Name)
		}))
		assertOrderedAttributesResolved(assert, processingSchema.compiled, &object.ItemDefinition)
		assert.True(sort.StringsAreSorted(object.OrderedConstraintKeys))
	}

	validationPipeline := mustNewEventProcessorPipeline(assert, processingSchema, NewValidation())
	assert.True(validationPipeline.validation.cache.VersionOK)
	assert.NotNil(validationPipeline.validation.cache.Types)
}

func TestPipelineTraversalCacheInitializesNumericEnumsForEveryPipeline(t *testing.T) {
	assert := require.New(t)

	observablesFactory := makeValidationTestSchema(assert)
	observablesAttribute := observablesFactory.compiled.Classes[1].Attributes["activity_id"]
	observablesDefinition := observablesAttribute.Enum["1"]
	observablesOnly := mustNewEventProcessorPipeline(
		assert,
		observablesFactory,
		NewEnrichment(WithAddEnumSiblings(false)),
	)
	assert.NotNil(observablesOnly.mutations[0].add)
	assert.Same(observablesDefinition, observablesAttribute.NumericEnumDefinition(1))

	forceFactory := makeValidationTestSchema(assert)
	forceAttribute := forceFactory.compiled.Classes[1].Attributes["activity_id"]
	forceDefinition := forceAttribute.Enum["1"]
	forceRemoval := mustNewEventProcessorPipeline(
		assert,
		forceFactory,
		NewEnrichmentRemoval(WithForceRemoveEnumSiblings(), WithRemoveObservables(false)),
	)
	assert.NotNil(forceRemoval.mutations[0].forceRemove)
	assert.Same(forceDefinition, forceAttribute.NumericEnumDefinition(1))
}

func TestInvariantNumericEnumLookupNormalizesEquivalentRepresentations(t *testing.T) {
	// Invariant test: numeric enum lookup must resolve every exact integral representation to the same enum key,
	// independent of which JSON number style a processing context observes first.
	assert := require.New(t)
	factory := makeValidationTestSchema(assert)
	mustNewEventProcessorPipeline(assert, factory, NewEnrichment(WithAddObservables(false)))
	attribute := factory.compiled.Classes[1].Attributes["activity_id"]
	want := attribute.Enum["1"]

	integerFirst := processContext{}
	for _, value := range []any{json.Number("1"), json.Number("1e0"), float64(1), int32(1)} {
		definition, status := lookupEnumDefinition(&integerFirst, attribute, value)
		assert.Equal(enumLookupFound, status)
		assert.Same(want, definition)
	}
	assert.True(integerFirst.jsonNumberStyleKnown)
	assert.False(integerFirst.jsonNumberUsesFloatSyntax)

	floatFirst := processContext{}
	for _, value := range []any{
		json.Number("1.0"), json.Number("1.000"), json.Number("1e0"), json.Number("1E+0"),
		json.Number("10e-1"), json.Number("1"),
	} {
		definition, status := lookupEnumDefinition(&floatFirst, attribute, value)
		assert.Equal(enumLookupFound, status)
		assert.Same(want, definition)
	}
	assert.True(floatFirst.jsonNumberStyleKnown)
	assert.True(floatFirst.jsonNumberUsesFloatSyntax)

	definition, status := lookupEnumDefinition(&processContext{}, attribute, "1")
	assert.Nil(definition)
	assert.Equal(enumLookupValueUnusable, status)

	definition, status = lookupEnumDefinition(&processContext{}, attribute, json.Number("2"))
	assert.Nil(definition)
	assert.Equal(enumLookupDefinitionMissing, status)

}

func TestValidationDoesNotReportUnknownEnumForUnusableValue(t *testing.T) {
	assert := require.New(t)
	factory := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, factory, NewValidation())
	event := validValidationEvent()
	event["activity_id"] = "1"

	result, err := pipeline.ProcessEvent(event)

	assert.NoError(err)
	errors := findingsAtLevel(result.Validation.Findings, validation.LevelError)
	assert.Contains(issueCodes(errors), validation.AttributeWrongType.String())
	assert.NotContains(issueCodes(errors), validation.AttributeEnumValueUnknown.String())
}

func TestVisitAttributeRejectsUnexpectedState(t *testing.T) {
	err := (&processContext{}).visitAttribute(nil, nil, "test", nil, -1, attributeState(255))
	require.ErrorContains(t, err, "unexpected attribute state 255")
}

func TestVisitAttributeRejectsPresentAttributeWithoutDefinition(t *testing.T) {
	err := (&processContext{}).visitAttribute(nil, nil, "test", nil, -1, attributePresent)
	require.ErrorContains(t, err, `present attribute "test" has no definition`)
}

func TestMutationDispatcherRejectsInvalidUnion(t *testing.T) {
	tests := []struct {
		name       string
		dispatcher mutationDispatcher
	}{
		{name: "empty"},
		{
			name: "multiple processors",
			dispatcher: mutationDispatcher{
				add:        &enrichmentProcessor{},
				safeRemove: &enrichmentSafeRemovalProcessor{},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, test.dispatcher.validate(), "exactly one processor")
		})
	}
}

func assertOrderedAttributesResolved(
	assert *require.Assertions,
	compiled *schema.Compiled,
	item *schema.ItemDefinition,
) {
	for _, attribute := range item.OrderedAttributes {
		assert.Same(item.Attributes[attribute.Name], attribute.Definition)
		if attribute.Definition != nil && attribute.Definition.ObjectType != nil {
			assert.Same(compiled.Objects[*attribute.Definition.ObjectType], attribute.Definition.ResolvedObject)
		}
	}
}

func TestPipelineCompilesClassObservableTriesOnlyWhenAddingObservables(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)

	withObservables := mustNewEventProcessorPipeline(assert, schema, NewEnrichment())
	withObservablesAdd := withObservables.mutations[0].add
	assert.NotNil(withObservablesAdd)
	assert.NotNil(withObservablesAdd.classObservableTries)

	withoutObservables := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddObservables(false)))
	withoutObservablesAdd := withoutObservables.mutations[0].add
	assert.NotNil(withoutObservablesAdd)
	assert.Nil(withoutObservablesAdd.classObservableTries)

	schema.compiled.Classes[1].Observables = nil
	withoutDeclarations := mustNewEventProcessorPipeline(assert, schema, NewEnrichment())
	withoutDeclarationsAdd := withoutDeclarations.mutations[0].add
	assert.NotNil(withoutDeclarationsAdd)
	assert.Nil(withoutDeclarationsAdd.classObservableTries)
}

func TestObservableEnrichmentWithoutClassDeclarationsDoesNotAllocatePerEvent(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	schema.compiled.Classes[1].Observables = nil
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichment(WithAddEnumSiblings(false)))
	event := validValidationEvent()
	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Empty(result)

	assert.Equal(float64(0), testing.AllocsPerRun(1000, func() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			panic(err)
		}
	}))
}

func TestValidationDoesNotAllocatePathsWithoutFindings(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
	event := validValidationEvent()

	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Empty(findingsAtLevel(result.Validation.Findings, validation.LevelError))
	assert.Empty(findingsAtLevel(result.Validation.Findings, validation.LevelWarning))
	assert.Equal(float64(0), testing.AllocsPerRun(1000, func() {
		if _, err := pipeline.ProcessEvent(event); err != nil {
			panic(err)
		}
	}))
}

func TestValidationDoesNotAllocateTypedScalarSlices(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())
	tests := []struct {
		name       string
		statusIDs  any
		statusText any
	}{
		{name: "slices", statusIDs: []int64{1, 2}, statusText: []string{"Open", "Closed"}},
		{name: "slice and array", statusIDs: []int64{1, 2}, statusText: [2]string{"Open", "Closed"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := validValidationEvent()
			event["status_ids"] = test.statusIDs
			event["statuses"] = test.statusText
			require.Equal(t, float64(0), testing.AllocsPerRun(1000, func() {
				result, err := pipeline.ProcessEvent(event)
				errorCount := len(findingsAtLevel(result.Validation.Findings, validation.LevelError))
				warningCount := len(findingsAtLevel(result.Validation.Findings, validation.LevelWarning))
				if err != nil || errorCount != 0 || warningCount != 0 {
					panic("valid typed slices produced a validation finding")
				}
			}))
		})
	}
}

func TestTypedScalarSliceValidationDetailsOmitEventValues(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewValidation())

	event := validValidationEvent()
	event["status_ids"] = []int64{1, 1234}
	result, err := pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Contains(
		issueCodes(findingsAtLevel(result.Validation.Findings, validation.LevelError)),
		"validation_attribute_enum_array_value_unknown",
	)
	assert.NotContains(findingsAtLevel(result.Validation.Findings, validation.LevelError)[0].Details, "value")

	event = validValidationEvent()
	event["statuses"] = []int64{7}
	result, err = pipeline.ProcessEvent(event)
	assert.NoError(err)
	assert.Contains(
		issueCodes(findingsAtLevel(result.Validation.Findings, validation.LevelError)),
		"validation_attribute_wrong_type",
	)
	assert.NotContains(findingsAtLevel(result.Validation.Findings, validation.LevelError)[0].Details, "value")
}

func TestEnrichmentWithoutGeneratedObservablesDoesNotAllocate(t *testing.T) {
	assert := require.New(t)
	schema := makeValidationTestSchema(assert)
	pipeline := mustNewEventProcessorPipeline(assert, schema, NewEnrichment())
	event := validValidationEvent()

	assert.Equal(float64(0), testing.AllocsPerRun(1000, func() {
		delete(event, "class_name")
		delete(event, "activity_name")
		result, err := pipeline.ProcessEvent(event)
		if err != nil || result.Enrichment.EnumSiblingsAdded != 2 {
			panic("enrichment did not add the expected enum siblings")
		}
	}))
}

func TestNewPipelineOrdersEnumSiblingsBeforeObservablesRegardlessOfConfigurationOrder(t *testing.T) {
	tests := []struct {
		name    string
		reverse bool
	}{
		{name: "observables-enabling configuration passed first"},
		{name: "observables-enabling configuration passed second", reverse: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			schema := makeValidationTestSchema(assert)
			addsObservables := NewEnrichment(WithAddEnumSiblings(false), WithAddObservables(true))
			removesEnumSiblings := NewEnrichmentRemoval(WithRemoveEnumSiblings(true), WithRemoveObservables(false))

			configurations := []EventProcessor{addsObservables, removesEnumSiblings}
			if test.reverse {
				configurations = []EventProcessor{removesEnumSiblings, addsObservables}
			}
			pipeline := mustNewEventProcessorPipeline(assert, schema, configurations...)

			assert.Len(pipeline.mutations, 2)
			removalProcessor := pipeline.mutations[0].safeRemove
			assert.NotNil(removalProcessor)
			assert.True(removalProcessor.enumSiblingsEnabled)
			assert.False(removalProcessor.observablesEnabled)

			addProcessor := pipeline.mutations[1].add
			assert.NotNil(addProcessor)
			assert.True(addProcessor.observablesEnabled)
			assert.False(addProcessor.enumSiblingsEnabled)
		})
	}
}

func TestPipelineZeroValueCannotProcessEvent(t *testing.T) {
	var pipeline Pipeline

	result, err := pipeline.ProcessEvent(jsonish.Map{})

	require.ErrorIs(t, err, errUninitializedPipeline)
	require.Empty(t, result)
}

func TestPipelineRejectsNilEvent(t *testing.T) {
	pipeline := &Pipeline{compiled: &schema.Compiled{}}

	result, err := pipeline.ProcessEvent(nil)

	require.ErrorIs(t, err, errNilEvent)
	require.Empty(t, result)
}
