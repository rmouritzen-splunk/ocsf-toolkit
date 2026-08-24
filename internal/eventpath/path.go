// Package eventpath maintains and renders structural paths while walking an event.
package eventpath

import (
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/pathseq"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
)

type elementKind uint8

const (
	elementAttribute elementKind = iota
	elementArrayIndex
)

type element struct {
	kind  elementKind
	name  string
	index int
}

// Path is a mutable structural event path. It uses inline storage for ordinary event depths so maintaining the
// traversal path does not add a per-event allocation. Callers must not retain a pointer to a traversal-owned Path
// beyond a visitor callback.
type Path struct {
	elements pathseq.Sequence[element]
}

// PushAttribute appends an attribute to the path.
func (p *Path) PushAttribute(name string) {
	p.elements.Push(element{kind: elementAttribute, name: name})
}

// PushArrayIndex appends an array index to the path.
func (p *Path) PushArrayIndex(index int) {
	p.elements.Push(element{kind: elementArrayIndex, index: index})
}

// Pop removes the final path element. It has no effect on an empty path.
func (p *Path) Pop() {
	p.elements.Pop()
}

func (p *Path) element(index int) element {
	return p.elements.At(index)
}

// Len returns the number of structural elements in the path, including array indexes.
func (p *Path) Len() int {
	return p.elements.Len()
}

// AttributeAt returns the attribute name at structural element index. The boolean is false for an array-index element.
func (p *Path) AttributeAt(index int) (string, bool) {
	if index < 0 || index >= p.Len() {
		return "", false
	}
	element := p.element(index)
	return element.name, element.kind == elementAttribute
}

// HasArrayIndex reports whether the path contains an array element.
func (p *Path) HasArrayIndex() bool {
	for index := 0; index < p.Len(); index++ {
		if p.element(index).kind == elementArrayIndex {
			return true
		}
	}
	return false
}

// HasPriorAttribute reports whether name occurs before the current attribute in the active path. Array indexes do
// not participate in recursive object-path detection.
func (p *Path) HasPriorAttribute(name string) bool {
	currentAttributeIndex := -1
	for index := p.Len() - 1; index >= 0; index-- {
		if p.element(index).kind == elementAttribute {
			currentAttributeIndex = index
			break
		}
	}
	for index := 0; index < currentAttributeIndex; index++ {
		element := p.element(index)
		if element.kind == elementAttribute && element.name == name {
			return true
		}
	}
	return false
}

// String renders the path using style.
func (p *Path) String(style pathstyle.Style) string {
	return p.render(style, -1, "", false)
}

// ChildString renders the path with attribute appended without mutating the path.
func (p *Path) ChildString(attribute string, style pathstyle.Style) string {
	return p.render(style, -1, attribute, true)
}

// SiblingString renders the path with its final attribute replaced without mutating the path.
func (p *Path) SiblingString(attribute string, style pathstyle.Style) string {
	lastAttribute := -1
	for index := p.Len() - 1; index >= 0; index-- {
		if p.element(index).kind == elementAttribute {
			lastAttribute = index
			break
		}
	}
	return p.render(style, lastAttribute, attribute, false)
}

// render keeps path traversal and rendering together because it is an event-processing hot path; splitting its
// branches into helpers would add calls without making the formatting rules easier to follow.
func (p *Path) render(
	style pathstyle.Style,
	replaceAttribute int,
	attribute string,
	appendAttribute bool,
) string {
	if p.Len() == 0 {
		if appendAttribute {
			if style == pathstyle.JSONPath {
				return "$." + attribute
			}
			return attribute
		}
		if style == pathstyle.JSONPath {
			return "$"
		}
		return ""
	}
	if !appendAttribute && style != pathstyle.JSONPath {
		attributeIndex := -1
		arrayNotationPresent := false
		for index := 0; index < p.Len(); index++ {
			element := p.element(index)
			if element.kind == elementAttribute {
				if attributeIndex >= 0 {
					attributeIndex = -2
					break
				}
				attributeIndex = index
			} else if style != pathstyle.Simple {
				arrayNotationPresent = true
			}
		}
		if attributeIndex >= 0 && !arrayNotationPresent {
			if attributeIndex == replaceAttribute {
				return attribute
			}
			return p.element(attributeIndex).name
		}
	}
	var builder strings.Builder
	builder.Grow(p.estimatedRenderedLength(attribute))
	if style == pathstyle.JSONPath {
		builder.WriteString("$.")
	}
	attributeWritten := false
	for index := 0; index < p.Len(); index++ {
		element := p.element(index)
		switch element.kind {
		case elementAttribute:
			if attributeWritten {
				builder.WriteByte('.')
			}
			if index == replaceAttribute {
				builder.WriteString(attribute)
			} else {
				builder.WriteString(element.name)
			}
			attributeWritten = true
		case elementArrayIndex:
			style.AppendArrayNotation(&builder, element.index)
		}
	}
	if appendAttribute {
		if attributeWritten {
			builder.WriteByte('.')
		}
		builder.WriteString(attribute)
	}
	return builder.String()
}

// maxArrayNotationLength upper-bounds Style.AppendArrayNotation's output across every style, including
// the widest indexed/JSONPath case ("[" + a 64-bit index's sign and digits + "]").
const maxArrayNotationLength = 22

// estimatedRenderedLength upper-bounds render's output to pre-size its builder, trading exactness
// (which would require mirroring render's per-style branching) for a single flat estimate; an
// occasional overestimate wastes a little capacity, and strings.Builder still grows on demand if
// this ever falls short.
func (p *Path) estimatedRenderedLength(attribute string) int {
	length := len(attribute) + len("$.")
	for index := 0; index < p.Len(); index++ {
		if element := p.element(index); element.kind == elementAttribute {
			length += len(element.name) + len(".")
		} else {
			length += maxArrayNotationLength
		}
	}
	return length
}
