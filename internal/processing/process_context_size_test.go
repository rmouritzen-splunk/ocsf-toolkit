package processing

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestEngineeringInvariantProcessContextSize(t *testing.T) {
	// Engineering invariant test: per-event context growth must not restore duplicate-diagnostic state to the common
	// hot-loop frame. The pre-cleanup processContext occupied 552 bytes on darwin/arm64 with Go 1.27.1.
	size := unsafe.Sizeof(processContext{})
	t.Logf("processContext size: %d bytes", size)
	require.LessOrEqual(t, size, uintptr(536))
}
