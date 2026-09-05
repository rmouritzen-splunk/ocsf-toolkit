package eventpipeline

import (
	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

// PipelineOption configures a Pipeline created by NewPipeline.
type PipelineOption interface {
	applyPipeline(*pipelineConfig)
}

// SchemaPipelineOption configures how a pipeline uses a schema supplied through WithSchema. Schema-pipeline options
// are reserved for future use.
type SchemaPipelineOption interface {
	applySchemaPipeline(*schemaPipelineConfig)
}

type schemaPipelineConfig struct{}

type pipelineOptionFunc func(*pipelineConfig)

func (f pipelineOptionFunc) applyPipeline(config *pipelineConfig) {
	f(config)
}

type issuePolicyConfig struct {
	levelRules []issueLevelRule
}

type issueLevelRule struct {
	code  issue.Code
	level issue.Level
	all   bool
}

// WithIssueLevel sets how the pipeline handles code. Ignored omits an ignorable issue, Warning reports it and
// continues processing, and Error returns a ProcessingIssueError and stops processing. Issue codes default to
// Warning. A mandatory issue may be Warning or Error but cannot be Ignored. Each code may be configured once.
func WithIssueLevel(code issue.Code, level issue.Level) PipelineOption {
	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.issuePolicy.levelRules = append(config.issuePolicy.levelRules, issueLevelRule{
			code:  code,
			level: level,
		})
	})
}

// WithAllIssueLevels sets the baseline handling level for every issue code. The baseline may be configured once and
// must occur before individual WithIssueLevel options. Ignored applies only to ignorable codes; mandatory codes retain
// their default Warning level unless explicitly set to Error.
func WithAllIssueLevels(level issue.Level) PipelineOption {
	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.issuePolicy.levelRules = append(config.issuePolicy.levelRules, issueLevelRule{
			level: level,
			all:   true,
		})
	})
}

type validationConfig struct {
	preferredPathNotation           pathstyle.Style
	preferredPathNotationConfigured bool
	preferredPathNotationCount      int
	policyRules                     []validationPolicyRule
}

type validationPolicyRule struct {
	code  validation.Code
	level validation.Level
	all   bool
}

// ValidationOption configures the validation behavior enabled by WithValidation.
type ValidationOption interface {
	applyValidation(*validationConfig)
}

type validationOptionFunc func(*validationConfig)

func (f validationOptionFunc) applyValidation(config *validationConfig) {
	f(config)
}

// WithValidation enables OCSF event validation, optionally configured with ValidationOption values such as
// WithValidationObservablePathNotation and WithValidationLevel. It may be used once per pipeline. Pipeline
// construction fails if the resolved options ignore every validation code.
func WithValidation(options ...ValidationOption) PipelineOption {
	config := validationConfig{}
	for _, option := range options {
		if option != nil {
			option.applyValidation(&config)
		}
	}
	return pipelineOptionFunc(func(pipeline *pipelineConfig) {
		pipeline.validationCount++
		pipeline.validationEnabled = true
		pipeline.validation = config
	})
}

// WithValidationObservablePathNotation reports observable names that do not use the preferred notation as findings.
func WithValidationObservablePathNotation(style pathstyle.Style) ValidationOption {
	return validationOptionFunc(func(config *validationConfig) {
		config.preferredPathNotationCount++
		config.preferredPathNotation = style
		config.preferredPathNotationConfigured = true
	})
}

// WithValidationLevel sets the effective reporting level for code. Ignored omits the finding, while Warning and Error
// report it at that level. Each code may be configured once.
func WithValidationLevel(code validation.Code, level validation.Level) ValidationOption {
	return validationOptionFunc(func(config *validationConfig) {
		config.policyRules = append(config.policyRules, validationPolicyRule{
			code:  code,
			level: level,
		})
	})
}

// WithAllValidationLevels sets the baseline effective level for every validation code. Explicit WithValidationLevel
// settings must follow this baseline. Pipeline construction fails if this baseline and the explicit settings resolve
// every validation code to Ignored.
func WithAllValidationLevels(level validation.Level) ValidationOption {
	return validationOptionFunc(func(config *validationConfig) {
		config.policyRules = append(config.policyRules, validationPolicyRule{
			level: level,
			all:   true,
		})
	})
}

// pipelineConfig is the fully resolved public-facing pipeline configuration, folded into a processing.PipelineConfig
// by NewPipeline.
type pipelineConfig struct {
	schema           *Schema
	schemaConfigured bool
	schemaCount      int

	enumSiblingsAction     enrichment.Action
	enumSiblingsCount      int
	observablesAction      enrichment.Action
	observablesCount       int
	observableTypeIDs      []int64
	pathNotation           pathstyle.Style
	pathNotationConfigured bool
	pathNotationCount      int
	issuePolicy            issuePolicyConfig

	validationEnabled bool
	validationCount   int
	validation        validationConfig
}

// WithEnumSiblings controls the action taken on supported scalar and array enum siblings: enrichment.Add adds them,
// enrichment.Remove or enrichment.ForceRemove removes them, and enrichment.None (the default) leaves them alone.
// WithEnumSiblings may be used once per pipeline. Regardless of the order options are passed to NewPipeline,
// enum-sibling work always runs before observable work.
func WithEnumSiblings(action enrichment.Action) PipelineOption {
	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.enumSiblingsCount++
		config.enumSiblingsAction = action
	})
}

// WithObservables controls the action taken on observables: enrichment.Add adds them, enrichment.Remove or
// enrichment.ForceRemove removes them, and enrichment.None (the default) leaves them alone. When action is
// enrichment.Add, an optional list of observable type IDs restricts which observables are added; an empty list adds
// all observable types. The option copies the supplied IDs; pipeline construction deduplicates them and reports every
// ID absent from the schema. Supplying IDs when action is not enrichment.Add is invalid. WithObservables may be used
// once per pipeline. Regardless of the order options are passed to NewPipeline, observable work always runs after
// enum-sibling work; see WithEnumSiblings.
func WithObservables(action enrichment.Action, observableTypeIDs ...int64) PipelineOption {
	ids := append([]int64(nil), observableTypeIDs...)
	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.observablesCount++
		config.observablesAction = action
		config.observableTypeIDs = ids
	})
}

// WithEnrichmentObservablePathNotation selects the notation used for generated observable names. It has no effect
// unless observables are added.
func WithEnrichmentObservablePathNotation(style pathstyle.Style) PipelineOption {
	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.pathNotationCount++
		config.pathNotation = style
		config.pathNotationConfigured = true
	})
}
