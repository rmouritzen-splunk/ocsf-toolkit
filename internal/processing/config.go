package processing

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

// ValidationConfig is validationProcessor's resolved configuration.
type ValidationConfig struct {
	WarnOnMissingRecommended bool
	PathNotation             pathstyle.Style
	PathNotationConfigured   bool
	PolicyRules              []ValidationPolicyRule
}

// ValidationPolicyAction is the resolved action for validation-policy configuration.
type ValidationPolicyAction uint8

const (
	ValidationPolicySuppress ValidationPolicyAction = iota + 1
	ValidationPolicyWarning
	ValidationPolicyError
)

// ValidationPolicyRule selects explicit codes, or all codes with DefaultLevel when Codes is empty.
type ValidationPolicyRule struct {
	Action       ValidationPolicyAction
	DefaultLevel validation.Level
	Codes        []validation.Code
}

// IssueSuppressionConfig is the resolved processing-issue suppression configuration.
type IssueSuppressionConfig struct {
	Configured bool
	Codes      []issue.IssueCode
	Invalid    []string
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
// they configure: EnumSiblingsAction/ObservablesAction/Observables/IssueSuppression are shared by the mutation
// processors they apply to, IssueSuppression applies to initialization and processing issues, and Validation only
// matters when ValidationEnabled is set.
type PipelineConfig struct {
	EnumSiblingsAction enrichment.Action
	ObservablesAction  enrichment.Action
	Observables        ObservablesConfig
	IssueSuppression   IssueSuppressionConfig

	ValidationEnabled bool
	Validation        ValidationConfig
}

// Validate reports invalid or contradictory configuration.
func (config PipelineConfig) Validate() error {
	_, err := config.validate()
	return err
}

func (config PipelineConfig) validate() (validationPolicy, error) {
	var problems []error
	var policy validationPolicy

	if !config.EnumSiblingsAction.Valid() {
		problems = append(problems, fmt.Errorf("invalid enum siblings action %q", config.EnumSiblingsAction))
	}
	if !config.ObservablesAction.Valid() {
		problems = append(problems, fmt.Errorf("invalid observables action %q", config.ObservablesAction))
	}

	// enrichmentEnabled and ValidationEnabled are the only two ways a pipeline can do work today. A future processor
	// family that can run on its own (for example a lint or update processor) must be added to this condition, and
	// separately considered against requiresEventWalk in pipeline.go, which decides independently whether that
	// family needs the schema-guided event walk at all; the two conditions are not the same predicate.
	enrichmentEnabled := config.EnumSiblingsAction != enrichment.None || config.ObservablesAction != enrichment.None
	if !enrichmentEnabled && !config.ValidationEnabled {
		problems = append(problems, errors.New("at least one event processing action is required"))
	}

	addingObservables := config.ObservablesAction == enrichment.Add
	if config.Observables.PathNotationConfigured && !addingObservables {
		problems = append(problems, errors.New("observable path notation is configured without adding observables"))
	}
	if len(config.Observables.TypeIDs) > 0 && !addingObservables {
		problems = append(problems, errors.New("observable type IDs are configured without adding observables"))
	}
	if addingObservables && !config.Observables.PathNotation.Valid() {
		problems = append(problems, fmt.Errorf("invalid observable path notation %q", config.Observables.PathNotation))
	}

	problems = append(problems, validateIssueSuppression(config.IssueSuppression)...)

	if config.ValidationEnabled && config.Validation.PathNotationConfigured &&
		!config.Validation.PathNotation.Valid() {
		problems = append(
			problems,
			fmt.Errorf("validation has invalid observable path notation %q", config.Validation.PathNotation),
		)
	}
	if config.ValidationEnabled {
		var policyProblems []error
		policy, policyProblems = compileValidationPolicy(config.Validation.PolicyRules)
		problems = append(problems, policyProblems...)
	}

	return policy, errors.Join(problems...)
}

func compileValidationPolicy(rules []ValidationPolicyRule) (validationPolicy, []error) {
	policy := validationPolicy{}
	actions := make(map[validation.Code]ValidationPolicyAction)
	mandatorySuppressions := make([]string, 0)
	seenMandatorySuppression := make(map[validation.Code]struct{})
	var problems []error
	for _, rule := range rules {
		if rule.Action < ValidationPolicySuppress || rule.Action > ValidationPolicyError {
			problems = append(problems, fmt.Errorf("validation policy has invalid action %d", rule.Action))
			continue
		}
		codes, explicitCodes, err := validationPolicyRuleCodes(rule)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		for _, code := range codes {
			if !explicitCodes && rule.DefaultLevel.Valid() && code.DefaultLevel() != rule.DefaultLevel {
				continue
			}
			mandatorySuppression, err := applyValidationPolicyCode(rule, code, explicitCodes, actions)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			if mandatorySuppression {
				if _, seen := seenMandatorySuppression[code]; !seen {
					seenMandatorySuppression[code] = struct{}{}
					mandatorySuppressions = append(mandatorySuppressions, code.String())
				}
				continue
			}
			if explicitCodes || rule.Action == ValidationPolicySuppress && rule.DefaultLevel.Valid() {
				setValidationPolicyCode(&policy, rule.Action, code)
			}
		}
		if !explicitCodes {
			setValidationPolicyDefaultAction(&policy, rule)
		}
	}
	if len(mandatorySuppressions) != 0 {
		slices.Sort(mandatorySuppressions)
		problems = append(problems, fmt.Errorf(
			"validation policy cannot suppress mandatory validation codes: %s",
			strings.Join(mandatorySuppressions, ", "),
		))
	}
	return policy, problems
}

func setValidationPolicyCode(policy *validationPolicy, action ValidationPolicyAction, code validation.Code) {
	if policy.settings == nil {
		policy.settings = make(map[validation.Code]validationPolicySetting)
	}
	switch action {
	case ValidationPolicySuppress:
		policy.settings[code] = validationPolicySetting{suppressed: true}
	case ValidationPolicyWarning:
		policy.settings[code] = validationPolicySetting{level: validation.LevelWarning}
	case ValidationPolicyError:
		policy.settings[code] = validationPolicySetting{level: validation.LevelError}
	}
}

func setValidationPolicyDefaultAction(policy *validationPolicy, rule ValidationPolicyRule) {
	switch {
	case rule.Action == ValidationPolicySuppress && !rule.DefaultLevel.Valid():
		policy.suppressAll = true
	case rule.Action == ValidationPolicyWarning && rule.DefaultLevel == validation.LevelError:
		policy.errorsAsWarnings = true
	case rule.Action == ValidationPolicyError && rule.DefaultLevel == validation.LevelWarning:
		policy.warningsAsErrors = true
	}
}

func validationPolicyRuleCodes(rule ValidationPolicyRule) ([]validation.Code, bool, error) {
	if len(rule.Codes) != 0 {
		return rule.Codes, true, nil
	}
	allLevels := rule.Action == ValidationPolicySuppress && !rule.DefaultLevel.Valid()
	if !allLevels && !rule.DefaultLevel.Valid() {
		return nil, false, fmt.Errorf("validation policy has invalid default level %d", rule.DefaultLevel)
	}
	return validation.Codes(), false, nil
}

func applyValidationPolicyCode(
	rule ValidationPolicyRule,
	code validation.Code,
	explicitCode bool,
	actions map[validation.Code]ValidationPolicyAction,
) (bool, error) {
	if !code.Valid() {
		return false, fmt.Errorf("validation policy has unknown validation code %d", code)
	}
	mandatorySuppression := rule.Action == ValidationPolicySuppress && !code.Suppressible()
	if mandatorySuppression && !explicitCode {
		return false, nil
	}
	previous := actions[code]
	if previous != 0 && previous != rule.Action {
		return false, fmt.Errorf("validation policy has conflicting actions for %s", code)
	}
	actions[code] = rule.Action
	return mandatorySuppression, nil
}

func validateIssueSuppression(config IssueSuppressionConfig) []error {
	if !config.Configured {
		return nil
	}
	invalid := make([]string, 0, len(config.Invalid)+len(config.Codes))
	for _, code := range config.Invalid {
		invalid = append(invalid, strconv.Quote(code))
	}
	nonSuppressible := make([]string, 0)
	seen := make(map[issue.IssueCode]struct{}, len(config.Codes))
	for _, code := range config.Codes {
		if _, duplicate := seen[code]; duplicate {
			continue
		}
		seen[code] = struct{}{}
		if !code.Valid() {
			invalid = append(invalid, strconv.FormatUint(uint64(code), 10))
			continue
		}
		if !code.Suppressible() {
			nonSuppressible = append(nonSuppressible, code.String())
			continue
		}
	}
	slices.Sort(invalid)
	slices.Sort(nonSuppressible)
	problems := make([]error, 0, 2)
	if len(invalid) > 0 {
		problems = append(
			problems, fmt.Errorf("issue suppression has unknown issue codes: %s", strings.Join(invalid, ", ")),
		)
	}
	if len(nonSuppressible) > 0 {
		problems = append(problems, fmt.Errorf("issue suppression cannot suppress mandatory issue codes: %s",
			strings.Join(nonSuppressible, ", ")))
	}
	return problems
}
