package observable

import (
	"encoding/json"
	"testing"

	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeDuplicatesSeparatesDetectionFromGeneratedDeduplication(t *testing.T) {
	existingValue := []jsonish.Map{
		{"name": "red", "type_id": json.Number("1"), "value": "go"},
		{"name": "ball", "type_id": json.Number("2")},
		{"name": "nullable", "type_id": json.Number("3"), "value": nil},
	}
	existing, ok := NewCollection(existingValue)
	require.True(t, ok)
	generated := []jsonish.Map{
		{"name": "red", "type_id": int64(1), "value": "go"},
		{"name": "red", "type_id": int64(1), "value": "other"},
		{"name": "red", "type_id": int64(1), "value": "other"},
		{"name": "nullable", "type_id": int64(3)},
	}

	result := AnalyzeDuplicates(&existing, generated, true)
	require.Equal(t, []jsonish.Map{generated[0], generated[1], generated[3]}, result.AcceptedGenerated)
	require.Equal(t, []Duplicate{
		{
			Name:       "red",
			Occurrence: DuplicateOccurrence{Origin: ObservableOriginGenerated, Index: 0},
			First:      DuplicateOccurrence{Origin: ObservableOriginExisting, Index: 0},
		},
		{
			Name:       "red",
			Occurrence: DuplicateOccurrence{Origin: ObservableOriginGenerated, Index: 2},
			First:      DuplicateOccurrence{Origin: ObservableOriginGenerated, Index: 1},
		},
		{
			Name:       "nullable",
			Occurrence: DuplicateOccurrence{Origin: ObservableOriginGenerated, Index: 3},
			First:      DuplicateOccurrence{Origin: ObservableOriginExisting, Index: 2},
		},
	}, result.Duplicates)

	withoutGeneratedDeduplication := AnalyzeDuplicates(&existing, generated, false)
	require.Equal(t, generated, withoutGeneratedDeduplication.AcceptedGenerated)
	require.Equal(t, result.Duplicates, withoutGeneratedDeduplication.Duplicates)
}

func TestAnalyzeDuplicatesIgnoresMalformedObservableIdentities(t *testing.T) {
	existing, ok := NewCollection([]jsonish.Map{
		{"name": "red", "value": "go"},
		{"name": "red", "type_id": "malformed", "value": "go"},
	})
	require.True(t, ok)
	generated := []jsonish.Map{{"name": "red", "type_id": int64(1), "value": "go"}}

	result := AnalyzeDuplicates(&existing, generated, true)
	require.Equal(t, generated, result.AcceptedGenerated)
	require.Empty(t, result.Duplicates)
}

func TestAppendAndFilterPreserveTypedArrayRepresentations(t *testing.T) {
	type observableMap jsonish.Map
	existingValue := [1]observableMap{{"name": "existing"}}
	existing, ok := NewCollection(existingValue)
	require.True(t, ok)

	appended, err := existing.Append([]jsonish.Map{{"name": "generated"}})
	require.NoError(t, err)
	require.Equal(t, []observableMap{{"name": "existing"}, {"name": "generated"}}, appended)

	entries := []Entry{{Removable: true}, {Removable: false}}
	appendedCollection, ok := NewCollection(appended)
	require.True(t, ok)
	filtered, err := appendedCollection.FilterRemovable(entries, 1)
	require.NoError(t, err)
	require.Equal(t, []observableMap{{"name": "generated"}}, filtered)
}

func TestAppendRejectsNonArrayExistingValue(t *testing.T) {
	var existing Collection
	appended, err := existing.Append([]jsonish.Map{{"name": "generated"}})
	require.Nil(t, appended)
	require.EqualError(t, err, "existing observables value is not an array")
}

func TestFilterRemovableRejectsInconsistentAnalysis(t *testing.T) {
	tests := []struct {
		name        string
		value       any
		entries     []Entry
		removeCount int
		errorText   string
	}{
		{name: "non-array", value: "not an array", errorText: "observable value is not an array"},
		{
			name:      "entry count",
			value:     []any{"one"},
			errorText: "observable analysis has 0 entries for an array of length 1",
		},
		{
			name:  "remove count",
			value: []any{"one"}, entries: []Entry{{Removable: true}},
			errorText: "observable analysis remove count is 0; expected 1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values, _ := NewCollection(test.value)
			filtered, err := values.FilterRemovable(test.entries, test.removeCount)
			require.Nil(t, filtered)
			require.EqualError(t, err, test.errorText)
		})
	}
}
