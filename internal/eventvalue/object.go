package eventvalue

import (
	"strings"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// HasPathOrKey reports whether a non-nil value exists at a literal key or dotted object path.
// A literal key containing dots takes precedence over interpreting the value as a nested path.
func HasPathOrKey(item jsonish.Map, path string) bool {
	if item[path] != nil {
		return true
	}
	current := item
	for {
		part, remainder, nested := strings.Cut(path, ".")
		value := current[part]
		if value == nil {
			return false
		}
		if !nested {
			return true
		}
		next, ok := value.(jsonish.Map)
		if !ok {
			return false
		}
		current = next
		path = remainder
	}
}
