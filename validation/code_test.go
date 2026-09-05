package validation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodeValidityExcludesSentinels(t *testing.T) {
	require.False(t, None.Valid())
	require.False(t, codeCount.Valid())
	require.False(t, None.DefaultLevel().Valid())
	require.False(t, codeCount.DefaultLevel().Valid())
	require.False(t, None.Ignorable())
	require.False(t, codeCount.Ignorable())
	require.Empty(t, None.Description())
	require.Empty(t, codeCount.Description())
}

func TestParseCodeRejectsUnknownString(t *testing.T) {
	code, ok := ParseCode("validation_unknown")
	require.False(t, ok)
	require.False(t, code.Valid())
}

func TestLevelsHaveStableStrings(t *testing.T) {
	for _, level := range []Level{LevelWarning, LevelError} {
		require.True(t, level.Valid())
		parsed, ok := ParseLevel(level.String())
		require.True(t, ok)
		require.Equal(t, level, parsed)

		encoded, err := json.Marshal(level)
		require.NoError(t, err)
		require.Equal(t, `"`+level.String()+`"`, string(encoded))
		var decoded Level
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.Equal(t, level, decoded)
	}
	require.False(t, invalidLevel.Valid())
	require.False(t, levelCount.Valid())
	_, ok := ParseLevel("unknown")
	require.False(t, ok)
}

func TestLevelTextErrorsRemainStable(t *testing.T) {
	encoded, err := invalidLevel.MarshalText()
	require.Nil(t, encoded)
	require.EqualError(t, err, "invalid validation level 0")

	level := LevelError
	require.EqualError(t, level.UnmarshalText([]byte("unknown")), `unknown validation level "unknown"`)
	require.Equal(t, LevelError, level)
}
