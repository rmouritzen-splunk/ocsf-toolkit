package processing

import (
	"errors"
	"fmt"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

// ValidationConfig is validationProcessor's resolved configuration.
type ValidationConfig struct {
	PathNotation           pathstyle.Style
	PathNotationConfigured bool
	PolicyRules            []ValidationPolicyRule
}

// ValidationPolicyRule sets one explicit code or the all-code baseline to Level.
type ValidationPolicyRule struct {
	Code  validation.Code
	Level validation.Level
	All   bool
}

// IssueLevelRule sets the handling level for one processing issue code.
type IssueLevelRule struct {
	Code  issue.Code
	Level issue.Level
	All   bool
}

// IssuePolicyConfig is the resolved processing-issue level configuration.
type IssuePolicyConfig struct {
	LevelRules []IssueLevelRule
}

// ObservablesConfig configures the observable-generation behavior of the enrichment.Add mutation processor. It has
// no effect for any other action.
type ObservablesConfig struct {
	TypeIDs                []int64
	PathNotation           pathstyle.Style
	PathNotationConfigured bool
}

// PipelineConfig is the fully resolved configuration for one eventpipeline.NewPipeline call. The public facade collects
// options into exactly one PipelineConfig, so each component and each processor kind appears at most once by
// construction; there is no cross-processor duplication to detect here. Fields are grouped by which processor
// they configure: EnumSiblingsAction/ObservablesAction/Observables/IssuePolicy are shared by the mutation
// processors they apply to, IssuePolicy applies to processing issues, and Validation only
// matters when ValidationEnabled is set.
type PipelineConfig struct {
	EnumSiblingsAction enrichment.Action
	ObservablesAction  enrichment.Action
	Observables        ObservablesConfig
	IssuePolicy        IssuePolicyConfig

	ValidationEnabled bool
	Validation        ValidationConfig
}

type compiledLevelPolicies struct {
	validation levelPolicy
	issues     levelPolicy
}

func (config PipelineConfig) validateAndCompileLevelPolicies() (compiledLevelPolicies, error) {
	var policies compiledLevelPolicies

	if !config.EnumSiblingsAction.Valid() {
		return policies, fmt.Errorf("invalid enum siblings action %q", config.EnumSiblingsAction)
	}
	if !config.ObservablesAction.Valid() {
		return policies, fmt.Errorf("invalid observables action %q", config.ObservablesAction)
	}

	// enrichmentEnabled and ValidationEnabled are the only two ways a pipeline can do work today. A future processor
	// family that can run on its own (for example a lint or update processor) must be added to this condition, and
	// separately considered against requiresEventWalk in pipeline.go, which decides independently whether that
	// family needs the schema-guided event walk at all; the two conditions are not the same predicate.
	enrichmentEnabled := config.EnumSiblingsAction != enrichment.None || config.ObservablesAction != enrichment.None
	if !enrichmentEnabled && !config.ValidationEnabled {
		return policies, errors.New("at least one event processing action is required")
	}

	addingObservables := config.ObservablesAction == enrichment.Add
	if config.Observables.PathNotationConfigured && !addingObservables {
		return policies, errors.New("observable path notation is configured without adding observables")
	}
	if len(config.Observables.TypeIDs) > 0 && !addingObservables {
		return policies, errors.New("observable type IDs are configured without adding observables")
	}
	if addingObservables && !config.Observables.PathNotation.Valid() {
		return policies, fmt.Errorf("invalid observable path notation %q", config.Observables.PathNotation)
	}

	issuePolicy, err := compileIssuePolicy(config.IssuePolicy.LevelRules)
	if err != nil {
		return policies, err
	}
	policies.issues = issuePolicy

	if config.ValidationEnabled && config.Validation.PathNotationConfigured &&
		!config.Validation.PathNotation.Valid() {
		return policies, fmt.Errorf(
			"validation has invalid observable path notation %q", config.Validation.PathNotation,
		)
	}
	if config.ValidationEnabled {
		validationPolicy, err := compileValidationPolicy(config.Validation.PolicyRules)
		if err != nil {
			return policies, err
		}
		if validationPolicy.warning == 0 && validationPolicy.error == 0 {
			return policies, errors.New("validation is enabled, but all validation codes are ignored")
		}
		policies.validation = validationPolicy
	}

	return policies, nil
}
