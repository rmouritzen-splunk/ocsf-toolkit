package eventschema

import (
	"encoding/json"
	"errors"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/internal/processing"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
)

var errUninitializedSchema = errors.New("schema is not initialized; create it with eventschema.Load")

// Pipeline processes OCSF events in place. Pipeline values must be created with Schema.NewPipeline and are safe for
// concurrent use when each call receives a distinct event map.
type Pipeline struct {
	state *processing.Pipeline
}

// ProcessingResult is the result returned by Pipeline.ProcessEvent. Its private value storage allows new
// processor-family results and public accessor methods to be added without changing its public structure. The
// supported Go compiler inlines the simple, statically dispatched accessors into direct private-state access, making
// this a zero-cost abstraction without interface boxing or an inherently necessary allocation. The zero value is a
// valid empty result.
//
// Validation errors and warnings are reported here instead of through the Go error return.
type ProcessingResult struct {
	state processingResultState
}

type processingResultState struct {
	validation        eventresult.ValidationResult
	enrichment        eventresult.EnrichmentResult
	enrichmentRemoval eventresult.EnrichmentRemovalResult
	issues            []eventresult.ProcessingIssue
	suppressedIssues  int
}

// Validation returns validation findings and their effective reporting levels.
func (r ProcessingResult) Validation() eventresult.ValidationResult {
	return r.state.validation
}

// Enrichment returns counts for values added to the event during enrichment.
func (r ProcessingResult) Enrichment() eventresult.EnrichmentResult {
	return r.state.enrichment
}

// EnrichmentRemoval returns counts for values removed or retained during enrichment removal.
func (r ProcessingResult) EnrichmentRemoval() eventresult.EnrichmentRemovalResult {
	return r.state.enrichmentRemoval
}

// Issues returns non-fatal event-processing issues that are separate from OCSF validation findings. The returned
// slice is owned by the result and may be modified by the caller.
func (r ProcessingResult) Issues() []eventresult.ProcessingIssue {
	return r.state.issues
}

// SuppressedIssueCount returns the number of non-fatal processing issues omitted by pipeline issue suppression.
func (r ProcessingResult) SuppressedIssueCount() int {
	return r.state.suppressedIssues
}

// MarshalJSON preserves the stable processor-result object used by earlier releases while the Go representation
// remains opaque and free to grow through new accessor methods.
func (r ProcessingResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(processingResultJSON{
		Validation:           r.state.validation,
		Enrichment:           r.state.enrichment,
		EnrichmentRemoval:    r.state.enrichmentRemoval,
		Issues:               r.state.issues,
		SuppressedIssueCount: r.state.suppressedIssues,
	})
}

type processingResultJSON struct {
	Validation           eventresult.ValidationResult        `json:"validation"`
	Enrichment           eventresult.EnrichmentResult        `json:"enrichment"`
	EnrichmentRemoval    eventresult.EnrichmentRemovalResult `json:"enrichment_removal"`
	Issues               []eventresult.ProcessingIssue       `json:"issues,omitempty"`
	SuppressedIssueCount int                                 `json:"suppressed_issue_count,omitempty"`
}

// ProcessEvent adds or removes enrichment and/or validates event in place.
//
// The event map and any nested maps or slices it contains must not be accessed or mutated concurrently
// while ProcessEvent is running. Processing is not transactional: the event may be partially modified
// if ProcessEvent returns an error. Invalid OCSF events are reported in ProcessingResult; the error
// return is reserved for an uninitialized pipeline, processing failures, or unusable caller input. Defined Go slice
// and array containers are accepted; their elements follow the same validation rules as values in []any.
// Processing
// results and errors do not repeat event values in their diagnostic text or details; a future error may use the
// event's metadata.uid as a correlation identifier.
func (p *Pipeline) ProcessEvent(event jsonish.Map) (ProcessingResult, error) {
	var state *processing.Pipeline
	if p != nil {
		state = p.state
	}
	result, err := state.ProcessEvent(event)
	return ProcessingResult{state: processingResultState{
		validation:        result.Validation,
		enrichment:        result.Enrichment,
		enrichmentRemoval: result.EnrichmentRemoval,
		issues:            result.Issues,
		suppressedIssues:  result.SuppressedIssues,
	}}, err
}

// NewPipeline builds a reusable pipeline from the requested options. Validation runs after mutating processors
// regardless of the supplied option order.
func (s *Schema) NewPipeline(options ...PipelineOption) (*Pipeline, error) {
	config := pipelineConfig{
		enumSiblingsAction: enrichment.None,
		observablesAction:  enrichment.None,
		pathNotation:       pathstyle.Simple,
	}
	for _, option := range options {
		if option != nil {
			option.applyPipeline(&config)
		}
	}

	internalConfig := processing.PipelineConfig{
		EnumSiblingsAction: config.enumSiblingsAction,
		ObservablesAction:  config.observablesAction,
		Observables: processing.ObservablesConfig{
			TypeIDs:                config.observableTypeIDs,
			PathNotation:           config.pathNotation,
			PathNotationConfigured: config.pathNotationConfigured,
		},
		IssueSuppression: processing.IssueSuppressionConfig{
			Configured: config.issueSuppression.configured,
			Codes:      config.issueSuppression.codes,
			Invalid:    config.issueSuppression.invalid,
		},

		ValidationEnabled: config.validationEnabled,
		Validation: processing.ValidationConfig{
			WarnOnMissingRecommended: config.validation.warnOnMissingRecommended,
			PathNotation:             config.validation.preferredPathNotation,
			PathNotationConfigured:   config.validation.preferredPathNotationConfigured,
			PolicyRules:              make([]processing.ValidationPolicyRule, len(config.validation.policyRules)),
		},
	}
	for index, rule := range config.validation.policyRules {
		internalConfig.Validation.PolicyRules[index] = processing.ValidationPolicyRule{
			Action:       processing.ValidationPolicyAction(rule.action),
			DefaultLevel: rule.defaultLevel,
			Codes:        rule.codes,
		}
	}

	if s == nil || s.pipelineFactory == nil {
		if err := internalConfig.Validate(); err != nil {
			return nil, err
		}
		return nil, errUninitializedSchema
	}
	pipeline, err := s.pipelineFactory.NewPipeline(internalConfig)
	if err != nil {
		return nil, err
	}
	return &Pipeline{state: pipeline}, nil
}
