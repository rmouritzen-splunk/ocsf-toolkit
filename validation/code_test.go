package validation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodesHaveStableLevelNeutralStringsAndDefaultLevels(t *testing.T) {
	codes := Codes()
	require.Len(t, codes, int(codeCount-1))
	for _, code := range codes {
		name := code.String()
		require.True(t, code.Valid(), "code %d should be valid", code)
		require.True(t, strings.HasPrefix(name, "validation_"), "code %d has incorrect prefix in %q", code, name)
		require.NotContains(t, name, "validation_error_")
		require.NotContains(t, name, "validation_warning_")
		require.True(t, code.DefaultLevel().Valid())
		require.NotEmpty(t, code.Description())

		parsed, ok := ParseCode(name)
		require.True(t, ok)
		require.Equal(t, code, parsed)

		encoded, err := json.Marshal(code)
		require.NoError(t, err)
		require.Equal(t, `"`+name+`"`, string(encoded))
		var decoded Code
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.Equal(t, code, decoded)
	}
	require.Equal(t, LevelError, AttributeRequiredMissing.DefaultLevel())
	require.Equal(t, LevelWarning, AttributeDeprecated.DefaultLevel())
}

func TestCodeValidityExcludesSentinels(t *testing.T) {
	require.False(t, None.Valid())
	require.False(t, codeCount.Valid())
	require.False(t, None.DefaultLevel().Valid())
	require.False(t, codeCount.DefaultLevel().Valid())
	require.Empty(t, None.Description())
	require.Empty(t, codeCount.Description())
}

func TestClassUIDResolutionCodesAreNotSuppressible(t *testing.T) {
	mandatory := map[Code]struct{}{
		ClassUIDMissing:   {},
		ClassUIDWrongType: {},
		ClassUIDUnknown:   {},
	}
	for _, code := range Codes() {
		_, isMandatory := mandatory[code]
		require.Equal(t, !isMandatory, code.Suppressible(), "unexpected suppressibility for %q", code.String())
	}
	require.False(t, None.Suppressible())
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
