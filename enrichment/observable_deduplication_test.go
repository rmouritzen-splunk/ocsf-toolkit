package enrichment_test

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/stretchr/testify/require"
)

func TestObservableDeduplicationModes(t *testing.T) {
	require.True(t, enrichment.ObservableDeduplicationIgnored.Valid())
	require.True(t, enrichment.ObservableDeduplicationGenerated.Valid())
	require.False(t, enrichment.ObservableDeduplication("all").Valid())
	require.False(t, enrichment.ObservableDeduplication("unknown").Valid())
}
