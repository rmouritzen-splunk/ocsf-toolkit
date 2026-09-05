package processing

import (
	"fmt"

	"github.com/ocsf/ocsf-toolkit/eventresult"
)

type processingIssueError struct {
	issue eventresult.ProcessingIssue
}

func (e *processingIssueError) Error() string {
	return fmt.Sprintf("processing issue %s from %s: %s", e.issue.Code, e.issue.Source, e.issue.Message)
}

func (e *processingIssueError) ProcessingIssue() eventresult.ProcessingIssue {
	return e.issue
}
