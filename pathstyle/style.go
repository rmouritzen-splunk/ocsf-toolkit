package pathstyle

import (
	"strconv"
	"strings"
)

// Style selects how an OCSF event attribute path represents array traversal and the event root.
type Style string

const (
	// Simple omits array selectors, for example resources.uid.
	Simple Style = "simple"
	// ArrayBrackets uses empty array selectors, for example resources[].uid.
	ArrayBrackets Style = "brackets"
	// ArrayWildcard uses wildcard array selectors, for example resources[*].uid.
	ArrayWildcard Style = "wildcard"
	// ArrayIndexed uses concrete array indexes, for example resources[1].uid.
	ArrayIndexed Style = "indexed"
	// JSONPath uses root-relative JSONPath with concrete indexes, for example $.resources[1].uid.
	JSONPath Style = "jsonpath"
)

// Valid reports whether the style is supported.
func (s Style) Valid() bool {
	switch s {
	case Simple, ArrayBrackets, ArrayWildcard, ArrayIndexed, JSONPath:
		return true
	default:
		return false
	}
}

// AppendArrayNotation appends the style's representation of an array traversal at index.
// Simple and invalid styles append nothing.
func (s Style) AppendArrayNotation(builder *strings.Builder, index int) {
	switch s {
	case ArrayBrackets:
		builder.WriteString("[]")
	case ArrayWildcard:
		builder.WriteString("[*]")
	case ArrayIndexed, JSONPath:
		builder.WriteByte('[')
		var buffer [20]byte
		builder.Write(strconv.AppendInt(buffer[:0], int64(index), 10))
		builder.WriteByte(']')
	default:
		// Simple, and defensively other cases, do not add any array notation.
		return
	}
}
