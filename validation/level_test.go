package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLevelStableRepresentations(t *testing.T) {
	tests := []struct {
		level Level
		name  string
	}{
		{level: LevelIgnored, name: "ignored"},
		{level: LevelWarning, name: "warning"},
		{level: LevelError, name: "error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.True(t, test.level.Valid())
			require.Equal(t, test.name, test.level.String())
			parsed, ok := ParseLevel(test.name)
			require.True(t, ok)
			require.Equal(t, test.level, parsed)
		})
	}
}
