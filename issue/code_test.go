package issue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIssueCodesHaveStablePrefixedStrings(t *testing.T) {
	for code := IssueCode(1); code < issueCodeCount; code++ {
		name := code.String()
		require.True(t, code.Valid(), "code %d should be valid", code)
		require.True(t, strings.HasPrefix(name, "issue_"), "code %d has unprefixed name %q", code, name)
		parsed, ok := ParseCode(name)
		require.True(t, ok)
		require.Equal(t, code, parsed)

		encoded, err := json.Marshal(code)
		require.NoError(t, err)
		require.Equal(t, `"`+name+`"`, string(encoded))
		var decoded IssueCode
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.Equal(t, code, decoded)
	}
}

func TestIssueCodeValidityAndSuppressibility(t *testing.T) {
	require.False(t, IssueCode(0).Valid())
	require.False(t, issueCodeCount.Valid())
	mandatory := map[IssueCode]struct{}{
		EventTraversalLimited: {},
		ClassUIDMissing:       {},
		ClassUIDWrongType:     {},
		ClassUIDUnknown:       {},
	}
	for code := IssueCode(1); code < issueCodeCount; code++ {
		_, isMandatory := mandatory[code]
		require.Equal(t, !isMandatory, code.Suppressible(), "unexpected suppressibility for %q", code.String())
	}
}

func TestParseIssueCodeRejectsUnknownString(t *testing.T) {
	code, ok := ParseCode("issue_unknown")
	require.False(t, ok)
	require.False(t, code.Valid())
}

func TestIssueCodesHaveDescriptions(t *testing.T) {
	require.Empty(t, None.Description())
	require.Empty(t, issueCodeCount.Description())
	for code := IssueCode(1); code < issueCodeCount; code++ {
		require.NotEmpty(t, code.Description(), "code %q should have a description", code.String())
	}
}
