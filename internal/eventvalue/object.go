package eventvalue

import (
	"strings"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// Attribute returns a non-null attribute value. OCSF processing treats a null attribute as absent.
func Attribute(item jsonish.Map, attribute string) (any, bool) {
	value, present := item[attribute]
	return value, present && value != nil
}

// HasPathOrKey reports whether a non-null value exists at a literal key or dotted object path.
// A literal key containing dots takes precedence over interpreting the value as a nested path.
func HasPathOrKey(item jsonish.Map, path string) bool {
	if _, present := Attribute(item, path); present {
		return true
	}
	current := item
	for {
		part, remainder, nested := strings.Cut(path, ".")
		value, present := Attribute(current, part)
		if !present {
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
