package eventpipeline

import (
	"fmt"

	"github.com/ocsf/ocsf-toolkit/eventresult"
)

// ProcessingIssueError reports an event-processing issue promoted to an error by pipeline issue-level policy.
// ProcessEvent returns an empty ProcessingResult with this error. Event mutation is not transactional and may have
// partially completed before the issue occurred.
type ProcessingIssueError struct {
	issue eventresult.ProcessingIssue
}

func newProcessingIssueError(issue eventresult.ProcessingIssue) *ProcessingIssueError {
	return &ProcessingIssueError{issue: issue}
}

// Error returns the issue's human-readable error representation.
func (e *ProcessingIssueError) Error() string {
	return fmt.Sprintf("processing issue %s from %s: %s", e.issue.Code, e.issue.Source, e.issue.Message)
}

// Issue returns the structured processing issue promoted to an error.
func (e *ProcessingIssueError) Issue() eventresult.ProcessingIssue {
	return e.issue
}
