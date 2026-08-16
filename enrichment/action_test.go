package enrichment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActionValid(t *testing.T) {
	for _, action := range []Action{None, Add, Remove, ForceRemove} {
		require.True(t, action.Valid(), action)
	}
	require.False(t, Action("invalid").Valid())
	require.False(t, Action("").Valid())
}
