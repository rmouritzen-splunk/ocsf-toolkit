package pathstyle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStyleValid(t *testing.T) {
	for _, style := range []Style{Simple, ArrayBrackets, ArrayWildcard, ArrayIndexed, JSONPath} {
		require.True(t, style.Valid(), style)
	}
	require.False(t, Style("invalid").Valid())
}
