package eventpipeline

import (
	"encoding/json"
	"errors"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/internal/processing"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

var (
	errSchemaNotConfigured = errors.New("pipeline schema is not configured; use eventpipeline.WithSchema")
	errUninitializedSchema = errors.New("schema is not initialized; create it with eventpipeline.NewSchema")
)

// Pipeline processes OCSF events in place. Pipeline values must be created with NewPipeline and are safe for
// concurrent use when each call receives a distinct event map.
type Pipeline struct {
	impl *processing.PipelineImpl
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

// Issues returns warning-level event-processing issues that are separate from OCSF validation findings. The returned
// slice is owned by the result and may be modified by the caller.
func (r ProcessingResult) Issues() []eventresult.ProcessingIssue {
	return r.state.issues
}

// MarshalJSON preserves the stable processor-result object used by earlier releases while the Go representation
// remains opaque and free to grow through new accessor methods.
func (r ProcessingResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(processingResultJSON{
		Validation:        r.state.validation,
		Enrichment:        r.state.enrichment,
		EnrichmentRemoval: r.state.enrichmentRemoval,
		Issues:            r.state.issues,
	})
}

type processingResultJSON struct {
	Validation        eventresult.ValidationResult        `json:"validation"`
	Enrichment        eventresult.EnrichmentResult        `json:"enrichment"`
	EnrichmentRemoval eventresult.EnrichmentRemovalResult `json:"enrichment_removal"`
	Issues            []eventresult.ProcessingIssue       `json:"issues,omitempty"`
}

// ProcessEvent adds or removes enrichment and/or validates event in place.
//
// The event map and any nested maps or slices it contains must not be accessed or mutated concurrently
// while ProcessEvent is running. Processing is not transactional: the event may be partially modified
// if ProcessEvent returns an error. Invalid OCSF events are reported in ProcessingResult; the error
// return is reserved for an uninitialized pipeline, processing failures, error-level processing issues, or unusable
// caller input. A non-nil error is always accompanied by the zero ProcessingResult. Defined Go slice and array
// containers are accepted; their elements follow the same validation rules as values in []any. Processing results and
// errors do not repeat event values in their diagnostic text or details; a future error may use the event's
// metadata.uid as a correlation identifier.
func (p *Pipeline) ProcessEvent(event jsonish.Map) (ProcessingResult, error) {
	if p == nil {
		return ProcessingResult{}, processing.ErrUninitializedPipeline
	}
	result, err := p.impl.ProcessEvent(event)
	if err != nil {
		var issueErr interface {
			ProcessingIssue() eventresult.ProcessingIssue
		}
		if errors.As(err, &issueErr) {
			return ProcessingResult{}, newProcessingIssueError(issueErr.ProcessingIssue())
		}
		return ProcessingResult{}, err
	}
	return ProcessingResult{state: processingResultState{
		validation:        result.Validation,
		enrichment:        result.Enrichment,
		enrichmentRemoval: result.EnrichmentRemoval,
		issues:            result.Issues,
	}}, nil
}

// WithSchema configures the schema used by a pipeline. Schema-pipeline options are reserved for future use.
// WithSchema may be used once per pipeline. The Schema must have been created by one of the NewSchema functions.
func WithSchema(schema *Schema, options ...SchemaPipelineOption) PipelineOption {
	config := schemaPipelineConfig{}
	for _, option := range options {
		if option != nil {
			option.applySchemaPipeline(&config)
		}
	}

	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.schemaCount++
		config.schema = schema
		config.schemaConfigured = true
	})
}

// NewPipeline builds a reusable pipeline from the requested options. Exactly one schema must be configured with
// WithSchema. Validation runs after mutating processors regardless of the supplied option order.
func NewPipeline(options ...PipelineOption) (*Pipeline, error) {
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
	if err := validatePipelineOptionConfiguration(config); err != nil {
		return nil, err
	}
	if !config.schemaConfigured {
		return nil, errSchemaNotConfigured
	}
	if config.schema == nil || config.schema.compiled == nil {
		return nil, errUninitializedSchema
	}

	internalConfig := processing.PipelineConfig{
		EnumSiblingsAction: config.enumSiblingsAction,
		ObservablesAction:  config.observablesAction,
		Observables: processing.ObservablesConfig{
			TypeIDs:                config.observableTypeIDs,
			PathNotation:           config.pathNotation,
			PathNotationConfigured: config.pathNotationConfigured,
		},
		IssuePolicy: processing.IssuePolicyConfig{
			LevelRules: make([]processing.IssueLevelRule, len(config.issuePolicy.levelRules)),
		},

		ValidationEnabled: config.validationEnabled,
		Validation: processing.ValidationConfig{
			PathNotation:           config.validation.preferredPathNotation,
			PathNotationConfigured: config.validation.preferredPathNotationConfigured,
			PolicyRules:            make([]processing.ValidationPolicyRule, len(config.validation.policyRules)),
		},
	}
	for index, rule := range config.issuePolicy.levelRules {
		internalConfig.IssuePolicy.LevelRules[index] = processing.IssueLevelRule{
			Code:  rule.code,
			Level: rule.level,
			All:   rule.all,
		}
	}
	for index, rule := range config.validation.policyRules {
		internalConfig.Validation.PolicyRules[index] = processing.ValidationPolicyRule{
			Code:  rule.code,
			Level: rule.level,
			All:   rule.all,
		}
	}

	impl, err := processing.NewPipelineImpl(config.schema.compiled, internalConfig)
	if err != nil {
		return nil, err
	}
	return &Pipeline{impl: impl}, nil
}

func validatePipelineOptionConfiguration(config pipelineConfig) error {
	for _, singleton := range []struct {
		count  int
		option PipelineOptionName
	}{
		{config.schemaCount, PipelineOptionSchema}, {config.enumSiblingsCount, PipelineOptionEnumSiblings},
		{config.observablesCount, PipelineOptionObservables},
		{config.pathNotationCount, PipelineOptionEnrichmentObservablePathNotation},
		{config.validationCount, PipelineOptionValidation},
	} {
		if singleton.count > 1 {
			return newPipelineOptionDuplicateError(singleton.option)
		}
	}
	if config.validation.preferredPathNotationCount > 1 {
		return newPipelineOptionDuplicateError(PipelineOptionValidationObservablePathNotation)
	}
	if err := validateIssueLevelRules(config.issuePolicy.levelRules); err != nil {
		return err
	}
	return validateValidationLevelRules(config.validation.policyRules)
}

func validateIssueLevelRules(rules []issueLevelRule) error {
	seenCodes := make(map[issue.Code]struct{}, len(rules))
	seenCode, seenAll := false, false
	for _, rule := range rules {
		if rule.all {
			if seenAll {
				return newPipelineOptionDuplicateError(PipelineOptionAllIssueLevels)
			}
			if seenCode {
				return newPipelineOptionIssueLevelAllAfterCodeError()
			}
			seenAll = true
			continue
		}
		seenCode = true
		if _, present := seenCodes[rule.code]; present {
			return newPipelineOptionIssueLevelDuplicateCodeError(rule.code)
		}
		seenCodes[rule.code] = struct{}{}
	}
	return nil
}

func validateValidationLevelRules(rules []validationPolicyRule) error {
	seenCodes := make(map[validation.Code]struct{}, len(rules))
	seenCode, seenAll := false, false
	for _, rule := range rules {
		if rule.all {
			if seenAll {
				return newPipelineOptionDuplicateError(PipelineOptionAllValidationLevels)
			}
			if seenCode {
				return newPipelineOptionValidationLevelAllAfterCodeError()
			}
			seenAll = true
			continue
		}
		seenCode = true
		if _, present := seenCodes[rule.code]; present {
			return newPipelineOptionValidationLevelDuplicateCodeError(rule.code)
		}
		seenCodes[rule.code] = struct{}{}
	}
	return nil
}
