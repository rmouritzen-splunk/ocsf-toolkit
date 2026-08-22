package eventschema_test

import (
	"encoding/json"
	"testing"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventschema"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

// Invariant test: a class_uid that is not exactly integral must not resolve a class or permit event mutation.
func TestInvariantNonIntegralClassUIDCannotResolveOrMutateEvent(t *testing.T) {
	schema, initializationIssues, err := eventschema.Load("../test/schema_v1.9.0.json")
	require.NoError(t, err)
	require.Empty(t, initializationIssues)
	pipeline, err := schema.NewPipeline(
		eventschema.WithEnumSiblings(enrichment.Add),
		eventschema.WithObservables(enrichment.Add),
		eventschema.WithValidation(),
	)
	require.NoError(t, err)
	event := jsonish.Map{
		"class_uid":   json.Number("1e-4000"),
		"activity_id": json.Number("1"),
	}

	result, err := pipeline.ProcessEvent(event)

	require.NoError(t, err)
	require.Equal(t, jsonish.Map{
		"class_uid":   json.Number("1e-4000"),
		"activity_id": json.Number("1"),
	}, event)
	require.Len(t, result.Issues(), 1)
	require.Equal(t, issue.ClassUIDWrongType, result.Issues()[0].Code)
	require.Len(t, result.Validation().Findings, 1)
	require.Equal(t, validation.ClassUIDWrongType, result.Validation().Findings[0].Code)
	require.Zero(t, result.Enrichment().EnumSiblingsAdded)
	require.Zero(t, result.Enrichment().ObservablesAdded)
}

// Invariant test: profile activation applies independently to each enum and sibling attribute, and pair-specific
// processing occurs only while both attributes are active.
func TestInvariantEnumSiblingProfilesActivateEachAttributeIndependently(t *testing.T) {
	loaded, initializationIssues, err := eventschema.Load("testdata/enum_sibling_profiles.json")
	require.NoError(t, err)
	require.Empty(t, initializationIssues)
	pipeline, err := loaded.NewPipeline(
		eventschema.WithEnumSiblings(enrichment.Add),
		eventschema.WithValidation(),
	)
	require.NoError(t, err)

	t.Run("active enum and inactive sibling", func(t *testing.T) {
		event := jsonish.Map{"class_uid": json.Number("1"), "action_id": json.Number("1")}

		result, processErr := pipeline.ProcessEvent(event)

		require.NoError(t, processErr)
		require.NotContains(t, event, "action")
		require.Zero(t, result.Enrichment().EnumSiblingsAdded)
		require.Empty(t, result.Validation().Findings)
	})

	t.Run("present inactive sibling", func(t *testing.T) {
		event := jsonish.Map{
			"class_uid": json.Number("1"),
			"action_id": json.Number("1"),
			"action":    "Provided",
		}

		result, processErr := pipeline.ProcessEvent(event)

		require.NoError(t, processErr)
		require.Equal(t, "Provided", event["action"])
		require.Zero(t, result.Enrichment().EnumSiblingsAdded)
		require.Len(t, result.Validation().Findings, 1)
		require.Equal(t, validation.AttributeRequiresProfile, result.Validation().Findings[0].Code)
	})

	t.Run("inactive enum and active sibling", func(t *testing.T) {
		event := jsonish.Map{"class_uid": json.Number("1"), "status": json.Number("1")}

		result, processErr := pipeline.ProcessEvent(event)

		require.NoError(t, processErr)
		require.Zero(t, result.Enrichment().EnumSiblingsAdded)
		require.Len(t, result.Validation().Findings, 1)
		require.Equal(t, validation.AttributeWrongType, result.Validation().Findings[0].Code)
	})

	t.Run("both attributes active", func(t *testing.T) {
		event := jsonish.Map{
			"class_uid": json.Number("1"),
			"action_id": json.Number("1"),
			"metadata":  jsonish.Map{"profiles": []any{"p1"}},
		}

		result, processErr := pipeline.ProcessEvent(event)

		require.NoError(t, processErr)
		require.Equal(t, "Allowed", event["action"])
		require.Equal(t, 1, result.Enrichment().EnumSiblingsAdded)
		require.Empty(t, result.Validation().Findings)
	})
}
