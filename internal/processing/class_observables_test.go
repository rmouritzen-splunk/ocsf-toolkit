package processing

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/stretchr/testify/require"
)

func TestClassObservableTrieMatchesStructuralPaths(t *testing.T) {
	trie := compileClassObservableTrie(map[string]int64{
		"items.type_id": 20,
		"ball.green":    1000,
	}, observableTypeSelector{})

	var path eventpath.Path
	path.PushAttribute("items")
	require.True(t, trie.HasDeclarationAtOrBelow(&path))
	_, present := trie.TypeID(&path)
	require.False(t, present)

	path.PushArrayIndex(3)
	path.PushAttribute("type_id")
	typeID, present := trie.TypeID(&path)
	require.True(t, present)
	require.Equal(t, int64(20), typeID)
	require.True(t, trie.HasDeclarationAtOrBelow(&path))

	path.Pop()
	path.PushAttribute("other")
	_, present = trie.TypeID(&path)
	require.False(t, present)
	require.False(t, trie.HasDeclarationAtOrBelow(&path))
}

func TestClassObservableTrieIsAbsentWithoutDeclarations(t *testing.T) {
	require.Nil(t, compileClassObservableTrie(nil, observableTypeSelector{}))
	require.Nil(t, compileClassObservableTrie(map[string]int64{}, observableTypeSelector{}))
}
