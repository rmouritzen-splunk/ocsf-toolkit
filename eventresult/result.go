package eventresult

import (
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
)

// ValidationResult contains OCSF validation findings in reporting order.
type ValidationResult struct {
	// Prevent external positional literals so fields can be added compatibly; keyed literals remain supported.
	_ struct{}

	// Findings contains validation findings with their effective reporting levels.
	Findings []ValidationFinding `json:"findings,omitempty"`

	// SuppressedErrorCount is the number of suppressed findings whose toolkit default level is error.
	SuppressedErrorCount int `json:"suppressed_error_count,omitempty"`

	// SuppressedWarningCount is the number of suppressed findings whose toolkit default level is warning.
	SuppressedWarningCount int `json:"suppressed_warning_count,omitempty"`
}

// Count returns the number of findings at level.
func (r ValidationResult) Count(level validation.Level) int {
	count := 0
	for _, finding := range r.Findings {
		if finding.Level == level {
			count++
		}
	}
	return count
}

// ValidationFinding describes an OCSF validation condition. Messages and details identify the affected
// location and schema condition without repeating event values so findings can be logged more safely.
type ValidationFinding struct {
	// Prevent external positional literals so fields can be added compatibly; keyed literals remain supported.
	_ struct{}

	// Level is the effective reporting level under the pipeline's validation policy.
	Level validation.Level `json:"level"`

	// Code is a short machine-readable finding identifier suitable for searching, grouping,
	// metrics, and structured logs.
	Code validation.Code `json:"code"`

	// Message is a human-readable finding description.
	Message string `json:"message"`

	// Details contains finding-specific structured context.
	Details jsonish.Map `json:"details,omitempty"`
}

// EnrichmentResult reports what enrichment added to the processed event.
type EnrichmentResult struct {
	// Prevent external positional literals so fields can be added compatibly; keyed literals remain supported.
	_ struct{}

	// EnumSiblingsAdded is the number of enum sibling fields added to the event.
	EnumSiblingsAdded int `json:"enum_siblings_added"`

	// ObservablesAdded is the number of observable entries added to the event.
	ObservablesAdded int `json:"observables_added"`
}

// EnrichmentRemovalResult reports what enrichment removal changed or retained in the processed event.
type EnrichmentRemovalResult struct {
	// Prevent external positional literals so fields can be added compatibly; keyed literals remain supported.
	_ struct{}

	// EnumSiblingsRemoved is the number of enum sibling fields removed from the event.
	EnumSiblingsRemoved int `json:"enum_siblings_removed"`

	// EnumSiblingsRetained is the number of enum sibling fields retained because removal was unsafe or unsupported.
	EnumSiblingsRetained int `json:"enum_siblings_retained"`

	// ObservablesRemoved is the number of observable entries removed from the event.
	ObservablesRemoved int `json:"observables_removed"`

	// ObservablesRetained is the number of observable entries retained because removal was unsafe.
	ObservablesRetained int `json:"observables_retained"`
}

// ProcessingIssue describes a non-fatal event-processing condition that does not cause ProcessEvent to return an
// error. Some mandatory issues explain why processing stopped before completing all requested work. Messages and
// details identify the affected location and processing condition without repeating event values so issues can be
// logged more safely.
type ProcessingIssue struct {
	// Prevent external positional literals so fields can be added compatibly; keyed literals remain supported.
	_ struct{}

	// Source identifies the part of event processing that produced the issue.
	Source issue.Source `json:"source"`

	// Code is a short machine-readable issue identifier suitable for searching, grouping,
	// metrics, and structured logs.
	Code issue.IssueCode `json:"code"`

	// Message is a human-readable issue description.
	Message string `json:"message"`

	// Details contains issue-specific structured context.
	Details jsonish.Map `json:"details,omitempty"`
}
