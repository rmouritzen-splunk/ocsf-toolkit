package schemaresult

import (
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// InitializationIssue describes a nonfatal condition found while preparing a schema for event processing.
type InitializationIssue struct {
	// Prevent external positional literals so fields can be added compatibly; keyed literals remain supported.
	_ struct{}

	// Code is a short machine-readable issue identifier suitable for searching, grouping, metrics, and structured logs.
	Code issue.IssueCode `json:"code"`

	// Message is a human-readable issue description.
	Message string `json:"message"`

	// Details contains issue-specific structured schema context.
	Details jsonish.Map `json:"details,omitempty"`
}
