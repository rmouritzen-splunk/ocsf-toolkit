package observable

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/stretchr/testify/require"
)

func TestEntryToDiagnosticIdentifiesObservableAttributeWithoutItsValue(t *testing.T) {
	var path eventpath.Path
	path.PushAttribute("observables")
	path.PushArrayIndex(2)

	diagnostic, present := EntryToDiagnostic(Entry{Problem: ProblemNameInvalidReference, Name: "file.name"}, 2, &path)

	require.True(t, present)
	require.Equal(t, "observables[2].name", diagnostic.Details["attribute_path"])
	require.NotContains(t, diagnostic.Details, "name")
	require.NotContains(t, diagnostic.Message, "file.name")
}

// TestEntryToDiagnosticCoversEveryReportableFailure walks every defined Problem (via ProblemCount, not a hand-kept
// list) so a newly added Problem is caught here if EntryToDiagnostic isn't updated to report it. ProblemNone has no
// failure to report, and ProblemTraversalLimited is handled by callers directly rather than through this diagnostic.
func TestEntryToDiagnosticCoversEveryReportableFailure(t *testing.T) {
	var path eventpath.Path
	path.PushAttribute("observables")
	path.PushArrayIndex(0)

	for problem := ProblemNone + 1; problem < ProblemCount; problem++ {
		if problem == ProblemTraversalLimited {
			continue
		}
		diagnostic, present := EntryToDiagnostic(Entry{Problem: problem}, 0, &path)
		require.True(t, present, "problem %d should produce diagnostic content", problem)
		require.NotEmpty(t, diagnostic.Message)
		require.NotEmpty(t, diagnostic.Details)
	}
}

func TestValidObservableEntryToDiagnosticDoesNotAllocate(t *testing.T) {
	var path eventpath.Path
	path.PushAttribute("observables")
	path.PushArrayIndex(0)

	require.Zero(t, testing.AllocsPerRun(1000, func() {
		if _, present := EntryToDiagnostic(Entry{}, 0, &path); present {
			panic("valid entry produced a diagnostic")
		}
	}))
}
