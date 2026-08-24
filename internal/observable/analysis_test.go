package observable

import (
	"encoding/json"
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeClassifiesObservableEntries(t *testing.T) {
	class, objects := observableTestSchema()
	event := jsonish.Map{
		"red":  "go",
		"ball": jsonish.Map{"green": "stop"},
		"observables": []jsonish.Map{
			{"name": "red", "type_id": json.Number("1"), "value": "go"},
			{"name": "ball", "type_id": json.Number("2")},
			{"type_id": json.Number("3")},
			{"name": "red", "type_id": json.Number("1"), "value": "missing"},
			{"name": "red[", "type_id": json.Number("1"), "value": "go"},
			{"name": "unknown", "type_id": json.Number("1"), "value": "go"},
		},
	}

	analysis, observables, isArray, err := Analyze(event, class, objects, nil)
	require.NoError(t, err)
	require.True(t, isArray)
	require.Equal(t, 6, observables.Len())
	require.Len(t, analysis.Entries, 6)
	require.True(t, analysis.Entries[0].Removable)
	require.True(t, analysis.Entries[1].Removable)
	require.Equal(t, ProblemNameMissing, analysis.Entries[2].Problem)
	require.Equal(t, ProblemValueNotFound, analysis.Entries[3].Problem)
	require.Equal(t, ProblemNameInvalidSyntax, analysis.Entries[4].Problem)
	require.Error(t, analysis.Entries[4].Err)
	require.Equal(t, ProblemNameInvalidReference, analysis.Entries[5].Problem)
	require.Equal(t, 2, analysis.RemovableCount())
}

func TestAnalyzeDistinguishesAbsentAndMalformedObservables(t *testing.T) {
	class, objects := observableTestSchema()
	analysis, _, _, err := Analyze(jsonish.Map{}, class, objects, nil)
	require.NoError(t, err)
	require.Nil(t, analysis)

	analysis, _, isArray, err := Analyze(jsonish.Map{"observables": "not an array"}, class, objects, nil)
	require.NoError(t, err)
	require.False(t, isArray)
	require.Len(t, analysis.Entries, 1)
	require.Equal(t, ProblemArrayWrongType, analysis.Entries[0].Problem)
	require.Equal(t, "not an array", analysis.Entries[0].Raw)
}

func TestProblemForDefinitionStatusRejectsUnexpectedStatus(t *testing.T) {
	problem, err := problemForDefinitionStatus(255)
	require.ErrorContains(t, err, "unexpected observable path definition status 255")
	require.Equal(t, ProblemNone, problem)
}

func observableTestSchema() (*schema.ClassDefinition, map[string]*schema.ObjectDefinition) {
	ballType := "ball"
	class := &schema.ClassDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"red": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "string_t"}},
			"ball": {
				CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &ballType},
			},
		},
	}}
	objects := map[string]*schema.ObjectDefinition{
		"ball": {ItemDefinition: schema.ItemDefinition{
			Attributes: map[string]*schema.ItemAttributeDefinition{
				"green": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "string_t"}},
			},
		}},
	}
	return class, objects
}
