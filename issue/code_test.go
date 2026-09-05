package issue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodeValidityExcludesSentinels(t *testing.T) {
	require.False(t, Code(0).Valid())
	require.False(t, issueCodeCount.Valid())
	require.False(t, Code(0).DefaultLevel().Valid())
	require.False(t, issueCodeCount.DefaultLevel().Valid())
	require.False(t, Code(0).Ignorable())
	require.False(t, issueCodeCount.Ignorable())
	require.Empty(t, Code(0).Description())
	require.Empty(t, issueCodeCount.Description())
}

func TestParseIssueCodeRejectsUnknownString(t *testing.T) {
	code, ok := ParseCode("issue_unknown")
	require.False(t, ok)
	require.False(t, code.Valid())
}
