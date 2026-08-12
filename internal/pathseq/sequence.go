// Package pathseq provides the ordered-sequence storage shared by this toolkit's path representations
// (eventpath.Path, observablepath.Path), which must avoid a heap allocation for ordinary traversal
// depths.
package pathseq

// InlineCapacity is the number of elements a Sequence stores without allocating. Sequences deeper than
// this spill into an overflow slice.
const InlineCapacity = 8

// Sequence is an ordered, appendable sequence of T. The zero value is an empty, ready-to-use sequence.
// It stores its first InlineCapacity elements inline, so ordinary traversal depths add no heap
// allocation; deeper sequences spill into an overflow slice.
type Sequence[T any] struct {
	inline   [InlineCapacity]T
	overflow []T
	length   int
}

// Push appends value to the end of the sequence.
func (s *Sequence[T]) Push(value T) {
	if s.length < len(s.inline) {
		s.inline[s.length] = value
	} else {
		overflowIndex := s.length - len(s.inline)
		if overflowIndex < len(s.overflow) {
			s.overflow[overflowIndex] = value
		} else {
			s.overflow = append(s.overflow, value)
		}
	}
	s.length++
}

// Pop removes the final element. It has no effect on an empty sequence.
func (s *Sequence[T]) Pop() {
	if s.length == 0 {
		return
	}
	s.length--
	var zero T
	if s.length < len(s.inline) {
		s.inline[s.length] = zero
	} else {
		s.overflow[s.length-len(s.inline)] = zero
	}
}

// At returns the element at index. The caller must ensure 0 <= index < Len().
func (s *Sequence[T]) At(index int) T {
	if index < len(s.inline) {
		return s.inline[index]
	}
	return s.overflow[index-len(s.inline)]
}

// Len returns the number of elements in the sequence.
func (s *Sequence[T]) Len() int {
	return s.length
}
