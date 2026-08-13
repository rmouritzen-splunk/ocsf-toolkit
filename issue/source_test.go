package issue

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourcesHaveStableValuesInProcessingOrder(t *testing.T) {
	want := []Source{
		SourceProcessing,
		SourceEnrichment,
		SourceEnrichmentRemoval,
		SourceValidation,
	}
	require.Equal(t, want, Sources())
	require.Equal(t, []string{"processing", "enrichment", "enrichment_removal", "validation"}, []string{
		SourceProcessing.String(),
		SourceEnrichment.String(),
		SourceEnrichmentRemoval.String(),
		SourceValidation.String(),
	})

	for _, source := range want {
		require.True(t, source.Valid())
		parsed, ok := ParseSource(source.String())
		require.True(t, ok)
		require.Equal(t, source, parsed)

		encoded, err := json.Marshal(source)
		require.NoError(t, err)
		require.Equal(t, `"`+source.String()+`"`, string(encoded))

		var decoded Source
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.Equal(t, source, decoded)
	}
	require.False(t, invalidSource.Valid())
	require.False(t, sourceCount.Valid())
	_, ok := ParseSource("unknown")
	require.False(t, ok)
}

func TestSourceTextErrorsRemainStable(t *testing.T) {
	encoded, err := invalidSource.MarshalText()
	require.Nil(t, encoded)
	require.EqualError(t, err, "invalid issue source 0")

	source := SourceValidation
	require.EqualError(t, source.UnmarshalText([]byte("unknown")), `unknown issue source "unknown"`)
	require.Equal(t, SourceValidation, source)
}
