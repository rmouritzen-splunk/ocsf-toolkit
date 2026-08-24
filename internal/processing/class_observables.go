package processing

import (
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
)

type classObservableTrie struct {
	root classObservableTrieNode
}

type classObservableTrieNode struct {
	children map[string]*classObservableTrieNode
	typeID   int64
	defined  bool
}

func compileClassObservableTries(
	classes map[int64]*schema.ClassDefinition,
	observableTypes observableTypeSelector,
) map[int64]*classObservableTrie {
	var tries map[int64]*classObservableTrie
	for classUID, class := range classes {
		if class == nil {
			continue
		}
		trie := compileClassObservableTrie(class.Observables, observableTypes)
		if trie == nil {
			continue
		}
		if tries == nil {
			tries = make(map[int64]*classObservableTrie)
		}
		tries[classUID] = trie
	}
	return tries
}

func compileClassObservableTrie(
	declarations map[string]int64,
	observableTypes observableTypeSelector,
) *classObservableTrie {
	if len(declarations) == 0 {
		return nil
	}
	trie := &classObservableTrie{}
	for path, typeID := range declarations {
		if !observableTypes.allows(typeID) {
			continue
		}
		node := &trie.root
		for attribute := range strings.SplitSeq(path, ".") {
			if node.children == nil {
				node.children = make(map[string]*classObservableTrieNode)
			}
			child := node.children[attribute]
			if child == nil {
				child = &classObservableTrieNode{}
				node.children[attribute] = child
			}
			node = child
		}
		node.typeID = typeID
		node.defined = true
	}
	if len(trie.root.children) == 0 {
		return nil
	}
	return trie
}

func (t *classObservableTrie) TypeID(path *eventpath.Path) (int64, bool) {
	node := t.lookup(path)
	if node == nil || !node.defined {
		return 0, false
	}
	return node.typeID, true
}

func (t *classObservableTrie) HasDeclarationAtOrBelow(path *eventpath.Path) bool {
	node := t.lookup(path)
	return node != nil && (node.defined || len(node.children) > 0)
}

func (t *classObservableTrie) lookup(path *eventpath.Path) *classObservableTrieNode {
	if t == nil || path == nil {
		return nil
	}
	node := &t.root
	for index := 0; index < path.Len(); index++ {
		attribute, ok := path.AttributeAt(index)
		if !ok {
			continue
		}
		node = node.children[attribute]
		if node == nil {
			return nil
		}
	}
	return node
}
