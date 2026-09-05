package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/internal/fserror"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

type processConfig struct {
	schemaPath string

	eventPath string
	eventsDir string

	validate               bool
	failOnValidationErrors bool
	validationLevels       validationLevelConfig

	enrich             bool
	enrichmentRemoval  bool
	enumSiblingsAction enrichment.Action
	observablesAction  enrichment.Action
	observableTypeIDs  []int64
	issueLevels        issueLevelConfig

	outputDir       string
	eventOutput     string
	reportOutput    string
	summaryFile     string
	summaryJSONFile string
	overwrite       bool
	prettyJSON      bool
	quiet           bool

	observablePathNotation string
}

type validationLevelConfig struct {
	rules []validationLevelRule
}

type issueLevelConfig struct {
	rules []issueLevelRule
}

func (options cliOptions) toConfig() (processConfig, []error) {
	enumSiblingsAction, observablesAction, err := options.Mutation.actions()
	if err != nil {
		return processConfig{}, []error{err}
	}
	config := options.processConfig(enumSiblingsAction, observablesAction)
	if problems := options.validateProcessConfig(config); len(problems) != 0 {
		return processConfig{}, problems
	}
	return config, nil
}

func (mutation mutationOptions) actions() (enrichment.Action, enrichment.Action, error) {
	shorthandCount := countSet(mutation.Enrich, mutation.Unenrich, mutation.ForceRemove)
	if shorthandCount > 1 {
		return enrichment.None, enrichment.None, errors.New(
			"--enrich, --unenrich, and --force-remove cannot be combined with each other",
		)
	}
	componentsConfigured := mutation.EnumSiblings.configured || mutation.Observables.configured
	if shorthandCount == 1 && componentsConfigured {
		return enrichment.None, enrichment.None, errors.New(
			"--enrich, --unenrich, and --force-remove cannot be combined with --enum-siblings or --observables",
		)
	}

	enumSiblingsAction := enrichment.None
	observablesAction := enrichment.None
	switch {
	case mutation.Enrich:
		enumSiblingsAction, observablesAction = enrichment.Add, enrichment.Add
	case mutation.Unenrich:
		enumSiblingsAction, observablesAction = enrichment.Remove, enrichment.Remove
	case mutation.ForceRemove:
		enumSiblingsAction, observablesAction = enrichment.ForceRemove, enrichment.ForceRemove
	default:
		if mutation.EnumSiblings.configured {
			enumSiblingsAction = mutation.EnumSiblings.action
		}
		if mutation.Observables.configured {
			observablesAction = mutation.Observables.action
		}
	}
	return enumSiblingsAction, observablesAction, nil
}

func (options cliOptions) processConfig(
	enumSiblingsAction enrichment.Action,
	observablesAction enrichment.Action,
) processConfig {
	return processConfig{
		schemaPath:             options.General.Schema,
		eventPath:              options.General.Event,
		eventsDir:              options.General.EventsDir,
		validate:               options.Validation.Validate,
		failOnValidationErrors: options.Validation.FailOnValidationErrors,
		validationLevels:       copyValidationLevels(options.Validation.Levels),
		enrich:                 enumSiblingsAction == enrichment.Add || observablesAction == enrichment.Add,
		enrichmentRemoval:      enumSiblingsAction.IsRemoval() || observablesAction.IsRemoval(),
		enumSiblingsAction:     enumSiblingsAction,
		observablesAction:      observablesAction,
		observableTypeIDs:      append([]int64(nil), options.Mutation.ObservableIDs.values...),
		issueLevels:            copyIssueLevels(options.Issues.Levels),
		outputDir:              options.General.OutputDir,
		eventOutput:            options.General.EventOutput,
		reportOutput:           options.General.ReportOutput,
		summaryFile:            options.General.SummaryFile,
		summaryJSONFile:        options.General.SummaryJSONFile,
		overwrite:              options.General.Overwrite,
		prettyJSON:             options.General.PrettyJSON,
		quiet:                  options.General.Quiet,
		observablePathNotation: options.General.ObservablePathNotation,
	}
}

func (options cliOptions) validateProcessConfig(config processConfig) []error {
	var problems []error
	if config.schemaPath == "" {
		problems = append(problems, errors.New("--schema is required"))
	}
	inputModeResolved := (config.eventPath == "") != (config.eventsDir == "")
	if !inputModeResolved {
		problems = append(problems, errors.New("exactly one of --event or --events-dir is required"))
	}
	if options.Mutation.ObservableIDs.configured && config.observablesAction != enrichment.Add {
		problems = append(problems, errors.New("--observable-id requires observable enrichment"))
	}
	if options.Validation.FailOnValidationErrors && !config.validate {
		problems = append(problems, errors.New("--fail-on-validation-errors requires --validate"))
	}
	if len(config.validationLevels.rules) != 0 && !config.validate {
		problems = append(problems, errors.New("--validation-level requires --validate"))
	}
	if config.observablePathNotation != "" && config.observablesAction != enrichment.Add && !config.validate {
		problems = append(problems, errors.New(
			"--observable-path-notation requires observable enrichment or --validate",
		))
	}
	if !validObservablePathNotation(config.observablePathNotation) {
		problems = append(problems, fmt.Errorf(
			"invalid --observable-path-notation value %q", config.observablePathNotation,
		))
	}

	issueLevelProblems := validateIssueLevelRules(config.issueLevels.rules)
	problems = append(problems, issueLevelProblems...)
	if len(issueLevelProblems) == 0 {
		problems = append(problems, validateIssuePolicyRules(config.issueLevels.rules)...)
	}
	if config.validate {
		validationLevelProblems := validateValidationLevelRules(config.validationLevels.rules)
		problems = append(problems, validationLevelProblems...)
		if len(validationLevelProblems) == 0 && !validationPolicyHasEnabledCode(config.validationLevels.rules) {
			problems = append(problems, errors.New("validation is enabled, but all validation codes are ignored"))
		}
	}
	if config.reportOutput != "" && !config.generatesReport() {
		problems = append(problems, errors.New(
			"--report-output requires"+
				" --enrich, --unenrich, --force-remove, --enum-siblings, --observables, or --validate",
		))
	}
	if !config.validate && !config.mutatesEvent() {
		problems = append(problems, errors.New("at least one event processing action is required"))
	}
	problems = append(problems, validateOutputConfig(config, inputModeResolved)...)
	return problems
}

func validateIssueLevelRules(rules []issueLevelRule) []error {
	var problems []error
	seenCodes := make(map[issue.Code]struct{}, len(rules))
	seenAll := false
	seenSpecific := false
	reportedOrder := false
	for _, rule := range rules {
		if rule.all {
			if seenSpecific && !reportedOrder {
				problems = append(problems, errors.New(
					"invalid --issue-level order: all=LEVEL must precede specific codes",
				))
				reportedOrder = true
			}
			if seenAll {
				problems = append(problems, errors.New("duplicate --issue-level: all"))
			}
			seenAll = true
			continue
		}
		seenSpecific = true
		if _, exists := seenCodes[rule.code]; exists {
			problems = append(problems, fmt.Errorf("duplicate --issue-level: %s", rule.code))
		}
		seenCodes[rule.code] = struct{}{}
	}
	return problems
}

func validateIssuePolicyRules(rules []issueLevelRule) []error {
	var problems []error
	for _, rule := range rules {
		if !rule.all && rule.level == issue.LevelIgnored && !rule.code.Ignorable() {
			problems = append(problems, fmt.Errorf(
				"issue policy cannot ignore mandatory issue code %s", rule.code,
			))
		}
	}
	return problems
}

func validateValidationLevelRules(rules []validationLevelRule) []error {
	var problems []error
	seenCodes := make(map[validation.Code]struct{}, len(rules))
	seenAll := false
	seenSpecific := false
	reportedOrder := false
	for _, rule := range rules {
		if rule.all {
			if seenSpecific && !reportedOrder {
				problems = append(problems, errors.New(
					"invalid --validation-level order: all=LEVEL must precede specific codes",
				))
				reportedOrder = true
			}
			if seenAll {
				problems = append(problems, errors.New("duplicate --validation-level: all"))
			}
			seenAll = true
			continue
		}
		seenSpecific = true
		if _, exists := seenCodes[rule.code]; exists {
			problems = append(problems, fmt.Errorf("duplicate --validation-level: %s", rule.code))
		}
		seenCodes[rule.code] = struct{}{}
	}
	return problems
}

func validationPolicyHasEnabledCode(rules []validationLevelRule) bool {
	allLevel := validation.Level(0)
	allConfigured := false
	explicitLevels := make(map[validation.Code]validation.Level, len(rules))
	for _, rule := range rules {
		if rule.all {
			allLevel = rule.level
			allConfigured = true
		} else {
			explicitLevels[rule.code] = rule.level
		}
	}
	for _, code := range validation.Codes() {
		level := code.DefaultLevel()
		if allConfigured {
			level = allLevel
		}
		if explicit, exists := explicitLevels[code]; exists {
			level = explicit
		}
		if level != validation.LevelIgnored {
			return true
		}
	}
	return false
}

func copyValidationLevels(option validationLevelsOption) validationLevelConfig {
	return validationLevelConfig{
		rules: append([]validationLevelRule(nil), option.rules...),
	}
}

func copyIssueLevels(option issueLevelsOption) issueLevelConfig {
	return issueLevelConfig{
		rules: append([]issueLevelRule(nil), option.rules...),
	}
}

func validObservablePathNotation(style string) bool {
	return style == "" || pathstyle.Style(style).Valid()
}

func validateOutputConfig(config processConfig, inputModeResolved bool) []error {
	var problems []error
	if !config.mutatesEvent() && config.eventOutput != "" {
		problems = append(problems, errors.New(
			"event output options require --enrich, --unenrich, --force-remove, --enum-siblings, or --observables",
		))
	}
	if config.eventsDir == stdioPath {
		problems = append(problems, errors.New("--events-dir cannot be -"))
	}
	if config.outputDir == stdioPath {
		problems = append(problems, errors.New("directory output options cannot be -"))
	}
	if config.outputDir != "" && (config.eventOutput != "" || config.reportOutput != "") {
		problems = append(problems, errors.New("--output-dir cannot be used with operation-specific output options"))
	}

	if inputModeResolved && config.eventPath != "" {
		if config.summaryFile != "" || config.summaryJSONFile != "" {
			problems = append(problems, errors.New("summary options require --events-dir"))
		}
		if config.quiet {
			problems = append(problems, errors.New("--quiet requires --events-dir"))
		}
		if config.mutatesEvent() {
			if countSet(config.outputDir != "", config.eventOutput != "") != 1 {
				problems = append(problems, errors.New(
					"single event mutation requires exactly one of --output-dir DIR or --event-output FILE",
				))
			}
		}
		if config.generatesReport() && countSet(config.outputDir != "", config.reportOutput != "") != 1 {
			problems = append(problems, errors.New(
				"single event reporting requires exactly one of --output-dir DIR or --report-output FILE",
			))
		}
	} else if inputModeResolved {
		if config.mutatesEvent() && config.eventOutput != "" {
			problems = append(problems, errors.New("--event-output cannot be used with --events-dir"))
		}
		if config.reportOutput != "" {
			problems = append(problems, errors.New("--report-output cannot be used with --events-dir"))
		}
		if config.outputDir == "" {
			problems = append(problems, errors.New("directory processing requires --output-dir DIR"))
		}
	}
	if countSet(
		config.eventOutput == stdioPath,
		config.reportOutput == stdioPath,
		config.summaryFile == stdioPath,
		config.summaryJSONFile == stdioPath,
	) > 1 {
		problems = append(problems, errors.New("only one output option may use stdout"))
	}
	return problems
}

func preflightSchemaFile(path string) error {
	// Reject an observed special file before opening it because a read-only open can block on a FIFO. Stat follows
	// symlinks; the descriptor check below verifies the target again after opening.
	if info, err := os.Stat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("--schema %q must name a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open --schema %q for reading: %w", path, fserror.QuotePaths(err))
	}
	defer func() {
		_ = file.Close()
	}()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect --schema %q: %w", path, fserror.QuotePaths(err))
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("--schema %q must name a regular file", path)
	}
	return nil
}

func countSet(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func (config processConfig) mutatesEvent() bool {
	return config.enumSiblingsAction != enrichment.None || config.observablesAction != enrichment.None
}

func (config processConfig) generatesReport() bool {
	return config.enrich || config.enrichmentRemoval || config.validate
}
