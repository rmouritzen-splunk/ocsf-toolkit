package pathseq

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSequencePreservesOrderAcrossInlineAndOverflow(t *testing.T) {
	var sequence Sequence[int]
	const count = InlineCapacity*2 + 3
	for i := range count {
		sequence.Push(i)
	}
	require.Equal(t, count, sequence.Len())
	for i := range count {
		require.Equal(t, i, sequence.At(i), "index %d", i)
	}
}

func TestSequencePopRemovesFinalElementAndShrinksLen(t *testing.T) {
	var sequence Sequence[string]
	sequence.Push("a")
	sequence.Push("b")
	sequence.Push("c")

	sequence.Pop()

	require.Equal(t, 2, sequence.Len())
	require.Equal(t, "a", sequence.At(0))
	require.Equal(t, "b", sequence.At(1))
}

func TestSequencePopClearsTheVacatedSlotSoItDoesNotRetainOldValues(t *testing.T) {
	var sequence Sequence[string]
	sequence.Push("first")
	sequence.Pop()
	sequence.Push("second")

	require.Equal(t, "second", sequence.At(0))
}

func TestSequencePopBeyondInlineCapacityShrinksBackIntoInlineStorage(t *testing.T) {
	var sequence Sequence[int]
	for i := range InlineCapacity + 2 {
		sequence.Push(i)
	}
	sequence.Pop()
	sequence.Pop()
	sequence.Pop()

	require.Equal(t, InlineCapacity-1, sequence.Len())
	for i := 0; i < sequence.Len(); i++ {
		require.Equal(t, i, sequence.At(i), "index %d", i)
	}
}

func TestSequencePopOnEmptySequenceHasNoEffect(t *testing.T) {
	var sequence Sequence[int]
	sequence.Pop()
	require.Equal(t, 0, sequence.Len())
}

func TestSequenceReusesOverflowCapacityAfterPushPopPush(t *testing.T) {
	var sequence Sequence[int]
	for i := range InlineCapacity + 2 {
		sequence.Push(i)
	}
	sequence.Pop()
	sequence.Pop()
	sequence.Push(100)
	sequence.Push(101)

	require.Equal(t, InlineCapacity+2, sequence.Len())
	require.Equal(t, 100, sequence.At(InlineCapacity))
	require.Equal(t, 101, sequence.At(InlineCapacity+1))
}
