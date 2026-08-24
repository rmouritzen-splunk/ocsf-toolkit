package pathstyle

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStyleValid(t *testing.T) {
	for _, style := range []Style{Simple, ArrayBrackets, ArrayWildcard, ArrayIndexed, JSONPath} {
		require.True(t, style.Valid(), style)
	}
	require.False(t, Style("invalid").Valid())
}

func TestAppendArrayNotation(t *testing.T) {
	tests := []struct {
		style Style
		want  string
	}{
		{style: Simple},
		{style: ArrayBrackets, want: "[]"},
		{style: ArrayWildcard, want: "[*]"},
		{style: ArrayIndexed, want: "[12]"},
		{style: JSONPath, want: "[12]"},
		{style: Style("invalid")},
	}
	for _, test := range tests {
		var builder strings.Builder
		test.style.AppendArrayNotation(&builder, 12)
		require.Equal(t, test.want, builder.String(), test.style)
	}
}
