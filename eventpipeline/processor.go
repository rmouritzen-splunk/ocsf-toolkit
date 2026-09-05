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

type issueSuppressionConfig struct {
	configured bool
	codes      []issue.IssueCode
	invalid    []string
}

type issueSuppressionOption struct {
	codes   []issue.IssueCode
	invalid []string
}

func (o issueSuppressionOption) applyPipeline(config *pipelineConfig) {
	config.issueSuppression.configured = true
	config.issueSuppression.codes = o.codes
	config.issueSuppression.invalid = o.invalid
}

// WithSuppressIssues suppresses reporting of the selected tolerable, non-fatal processor issue codes. An empty list
// suppresses all suppressible issues. When WithSuppressIssues or WithSuppressIssuesByStrings is used more than once
// for a pipeline, the last option wins. Suppression does not affect event mutations, result counters, validation
// findings, returned Go errors, or mandatory issues reporting class-resolution failure or limited event traversal.
func WithSuppressIssues(codes ...issue.IssueCode) PipelineOption {
	return issueSuppressionOption{codes: append([]issue.IssueCode(nil), codes...)}
}

// WithSuppressIssuesByStrings is the string-based form of WithSuppressIssues for codes loaded dynamically from
// configuration. Unknown codes are reported during pipeline construction, and an empty list suppresses all
// suppressible issues. When either suppression option is used more than once for a pipeline, the last option wins.
func WithSuppressIssuesByStrings(codes ...string) PipelineOption {
	converted := make([]issue.IssueCode, 0, len(codes))
	invalid := make([]string, 0)
	for _, code := range codes {
		if parsed, ok := issue.ParseCode(code); ok {
			converted = append(converted, parsed)
		} else {
			invalid = append(invalid, code)
		}
	}
	return issueSuppressionOption{codes: converted, invalid: invalid}
}

type validationConfig struct {
	warnOnMissingRecommended        bool
	preferredPathNotation           pathstyle.Style
	preferredPathNotationConfigured bool
	policyRules                     []validationPolicyRule
}

type validationPolicyAction uint8

const (
	validationPolicySuppress validationPolicyAction = iota + 1
	validationPolicyWarning
	validationPolicyError
)

type validationPolicyRule struct {
	action       validationPolicyAction
	defaultLevel validation.Level
	codes        []validation.Code
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
// WithWarnOnMissingRecommended and WithValidationObservablePathNotation. When WithValidation is used more than once
// for a pipeline, the last option wins, including any nested options it carries.
func WithValidation(options ...ValidationOption) PipelineOption {
	config := validationConfig{}
	for _, option := range options {
		if option != nil {
			option.applyValidation(&config)
		}
	}
	return pipelineOptionFunc(func(pipeline *pipelineConfig) {
		pipeline.validationEnabled = true
		pipeline.validation = config
	})
}

// WithWarnOnMissingRecommended reports missing recommended attributes as validation warnings.
func WithWarnOnMissingRecommended() ValidationOption {
	return validationOptionFunc(func(config *validationConfig) {
		config.warnOnMissingRecommended = true
	})
}

// WithValidationObservablePathNotation reports observable names that do not use the preferred notation as warnings.
func WithValidationObservablePathNotation(style pathstyle.Style) ValidationOption {
	return validationOptionFunc(func(config *validationConfig) {
		config.preferredPathNotation = style
		config.preferredPathNotationConfigured = true
	})
}

// WithSuppressValidation suppresses the selected validation codes independently of their default or effective
// levels. With no codes, it suppresses every suppressible validation finding. Class-resolution findings are mandatory
// because processing cannot continue without a resolved class; explicitly selecting one makes pipeline construction
// fail.
func WithSuppressValidation(codes ...validation.Code) ValidationOption {
	return validationPolicyOption(validationPolicySuppress, 0, codes)
}

// WithValidationWarningsAsErrors reports the selected validation codes as errors. With no codes, it selects all
// codes whose toolkit default is warning. Explicitly selecting a code already defaulted to error is valid and
// preserves the configured intent if the toolkit changes that default in a future release.
func WithValidationWarningsAsErrors(codes ...validation.Code) ValidationOption {
	return validationPolicyOption(validationPolicyError, validation.LevelWarning, codes)
}

// WithValidationErrorsAsWarnings reports the selected validation codes as warnings. With no codes, it selects all
// codes whose toolkit default is error. Explicitly selecting a code already defaulted to warning is valid and
// preserves the configured intent if the toolkit changes that default in a future release.
func WithValidationErrorsAsWarnings(codes ...validation.Code) ValidationOption {
	return validationPolicyOption(validationPolicyWarning, validation.LevelError, codes)
}

func validationPolicyOption(
	action validationPolicyAction,
	defaultLevel validation.Level,
	codes []validation.Code,
) ValidationOption {
	selected := append([]validation.Code(nil), codes...)
	return validationOptionFunc(func(config *validationConfig) {
		config.policyRules = append(config.policyRules, validationPolicyRule{
			action:       action,
			defaultLevel: defaultLevel,
			codes:        selected,
		})
	})
}

// pipelineConfig is the fully resolved public-facing pipeline configuration, folded into a processing.PipelineConfig
// by NewPipeline.
type pipelineConfig struct {
	schema           *Schema
	schemaConfigured bool

	enumSiblingsAction     enrichment.Action
	observablesAction      enrichment.Action
	observableTypeIDs      []int64
	pathNotation           pathstyle.Style
	pathNotationConfigured bool
	issueSuppression       issueSuppressionConfig

	validationEnabled bool
	validation        validationConfig
}

// WithEnumSiblings controls the action taken on supported scalar and array enum siblings: enrichment.Add adds them,
// enrichment.Remove or enrichment.ForceRemove removes them, and enrichment.None (the default) leaves them alone. When
// WithEnumSiblings is used more than once for a pipeline, the last option wins. Regardless of the order options are
// passed to NewPipeline, enum-sibling work always runs before observable work.
func WithEnumSiblings(action enrichment.Action) PipelineOption {
	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.enumSiblingsAction = action
	})
}

// WithObservables controls the action taken on observables: enrichment.Add adds them, enrichment.Remove or
// enrichment.ForceRemove removes them, and enrichment.None (the default) leaves them alone. When action is
// enrichment.Add, an optional list of observable type IDs restricts which observables are added; an empty list adds
// all observable types. The option copies the supplied IDs; pipeline construction deduplicates them and reports every
// ID absent from the schema. Supplying IDs when action is not enrichment.Add is invalid. When WithObservables is used
// more than once for a pipeline, the last option wins. Regardless of the order options are passed to
// NewPipeline, observable work always runs after enum-sibling work; see WithEnumSiblings.
func WithObservables(action enrichment.Action, observableTypeIDs ...int64) PipelineOption {
	ids := append([]int64(nil), observableTypeIDs...)
	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.observablesAction = action
		config.observableTypeIDs = ids
	})
}

// WithEnrichmentObservablePathNotation selects the notation used for generated observable names. It has no effect
// unless observables are added.
func WithEnrichmentObservablePathNotation(style pathstyle.Style) PipelineOption {
	return pipelineOptionFunc(func(config *pipelineConfig) {
		config.pathNotation = style
		config.pathNotationConfigured = true
	})
}
