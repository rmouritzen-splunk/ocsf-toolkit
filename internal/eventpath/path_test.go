package eventpath

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/pathstyle"
)

func TestEventPathRendersSupportedNotationStyles(t *testing.T) {
	path := Path{}
	path.PushAttribute("resources")
	path.PushArrayIndex(12)
	path.PushAttribute("uid")

	assert := require.New(t)
	assert.Equal("resources.uid", path.String(pathstyle.Simple))
	assert.Equal("resources[].uid", path.String(pathstyle.ArrayBrackets))
	assert.Equal("resources[*].uid", path.String(pathstyle.ArrayWildcard))
	assert.Equal("resources[12].uid", path.String(pathstyle.ArrayIndexed))
	assert.Equal("$.resources[12].uid", path.String(pathstyle.JSONPath))
	assert.Equal("resources[12].type", path.SiblingString("type", pathstyle.ArrayIndexed))
	assert.Equal("resources[12].uid.value", path.ChildString("value", pathstyle.ArrayIndexed))
}

func TestAppendArrayNotation(t *testing.T) {
	tests := []struct {
		style pathstyle.Style
		want  string
	}{
		{style: pathstyle.Simple},
		{style: pathstyle.ArrayBrackets, want: "[]"},
		{style: pathstyle.ArrayWildcard, want: "[*]"},
		{style: pathstyle.ArrayIndexed, want: "[12]"},
		{style: pathstyle.JSONPath, want: "[12]"},
		{style: pathstyle.Style("invalid")},
	}
	for _, test := range tests {
		var builder strings.Builder
		appendArrayNotation(&builder, test.style, 12)
		require.Equal(t, test.want, builder.String(), test.style)
	}
}

func TestEventPathSupportsDepthBeyondInlineStorage(t *testing.T) {
	path := Path{}
	names := make([]string, 10)
	for index := range names {
		names[index] = fmt.Sprintf("level%d", index)
		path.PushAttribute(names[index])
	}

	require.Equal(t, strings.Join(names, "."), path.String(pathstyle.Simple))
	for range names {
		path.Pop()
	}
	require.Equal(t, "$.value", path.ChildString("value", pathstyle.JSONPath))
}

func TestEventPathFindsPriorAttributeIgnoringArrayIndexes(t *testing.T) {
	path := Path{}
	path.PushAttribute("people")
	path.PushArrayIndex(0)
	path.PushAttribute("ldap_person")
	path.PushAttribute("manager")
	path.PushAttribute("ldap_person")

	require.True(t, path.HasPriorAttribute("ldap_person"))
	require.False(t, path.HasPriorAttribute("missing"))
}

func TestEventPathExposesStructuralElements(t *testing.T) {
	path := Path{}
	path.PushAttribute("items")
	path.PushArrayIndex(2)
	path.PushAttribute("type_id")

	require.Equal(t, 3, path.Len())
	name, attribute := path.AttributeAt(0)
	require.True(t, attribute)
	require.Equal(t, "items", name)
	_, attribute = path.AttributeAt(1)
	require.False(t, attribute)
	name, attribute = path.AttributeAt(2)
	require.True(t, attribute)
	require.Equal(t, "type_id", name)
}
