package main

import (
	"errors"
	"fmt"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

type processConfig struct {
	schemaPath string

	eventPath string
	eventsDir string

	validate                   bool
	warnOnMissingRecommended   bool
	failOnValidationErrors     bool
	suppressValidation         validationCodeSelection
	validationWarningsAsErrors validationCodeSelection
	validationErrorsAsWarnings validationCodeSelection

	enrich                   bool
	enrichmentRemoval        bool
	enumSiblingsAction       enrichment.Action
	observablesAction        enrichment.Action
	observableTypeIDs        []int64
	suppressIssuesConfigured bool
	suppressIssueCodes       []string

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

type validationCodeSelection struct {
	configured bool
	codes      []validation.Code
}

func (options cliOptions) toConfig() (processConfig, error) {
	enumSiblingsAction, observablesAction, err := options.Mutation.actions()
	if err != nil {
		return processConfig{}, err
	}
	config := options.processConfig(enumSiblingsAction, observablesAction)
	if err := options.validateProcessConfig(config); err != nil {
		return processConfig{}, err
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
		schemaPath:                 options.General.Schema,
		eventPath:                  options.General.Event,
		eventsDir:                  options.General.EventsDir,
		validate:                   options.Validation.Validate,
		warnOnMissingRecommended:   options.Validation.WarnOnMissingRecommended,
		failOnValidationErrors:     options.Validation.FailOnValidationErrors,
		suppressValidation:         validationSelection(options.Validation.Suppress),
		validationWarningsAsErrors: validationSelection(options.Validation.WarningsAsErrors),
		validationErrorsAsWarnings: validationSelection(options.Validation.ErrorsAsWarnings),
		enrich:                     enumSiblingsAction == enrichment.Add || observablesAction == enrichment.Add,
		enrichmentRemoval:          enumSiblingsAction.IsRemoval() || observablesAction.IsRemoval(),
		enumSiblingsAction:         enumSiblingsAction,
		observablesAction:          observablesAction,
		observableTypeIDs:          append([]int64(nil), options.Mutation.ObservableIDs.values...),
		suppressIssuesConfigured:   options.Mutation.SuppressIssues.configured,
		suppressIssueCodes:         append([]string(nil), options.Mutation.SuppressIssues.values...),
		outputDir:                  options.General.OutputDir,
		eventOutput:                options.General.EventOutput,
		reportOutput:               options.General.ReportOutput,
		summaryFile:                options.General.SummaryFile,
		summaryJSONFile:            options.General.SummaryJSONFile,
		overwrite:                  options.General.Overwrite,
		prettyJSON:                 options.General.PrettyJSON,
		quiet:                      options.General.Quiet,
		observablePathNotation:     options.General.ObservablePathNotation,
	}
}

func (options cliOptions) validateProcessConfig(config processConfig) error {
	if config.schemaPath == "" {
		return errors.New("--schema is required")
	}
	if (config.eventPath == "") == (config.eventsDir == "") {
		return errors.New("exactly one of --event or --events-dir is required")
	}
	if options.Mutation.ObservableIDs.configured && config.observablesAction != enrichment.Add {
		return errors.New("--observable-ids requires observable enrichment")
	}
	if options.Validation.WarnOnMissingRecommended && !config.validate {
		return errors.New("--warn-on-missing-recommended requires --validate")
	}
	if options.Validation.FailOnValidationErrors && !config.validate {
		return errors.New("--fail-on-validation-errors requires --validate")
	}
	for _, policy := range []struct {
		name       string
		configured bool
	}{
		{"--suppress-validations", config.suppressValidation.configured},
		{"--validation-warnings-as-errors", config.validationWarningsAsErrors.configured},
		{"--validation-errors-as-warnings", config.validationErrorsAsWarnings.configured},
	} {
		if policy.configured && !config.validate {
			return fmt.Errorf("%s requires --validate", policy.name)
		}
	}
	if config.observablePathNotation != "" && config.observablesAction != enrichment.Add && !config.validate {
		return errors.New("--observable-path-notation requires observable enrichment or --validate")
	}
	if !validObservablePathNotation(config.observablePathNotation) {
		return fmt.Errorf("invalid --observable-path-notation value %q", config.observablePathNotation)
	}
	if config.reportOutput != "" && !config.generatesReport() {
		return errors.New(
			"--report-output requires" +
				" --enrich, --unenrich, --force-remove, --enum-siblings, --observables, or --validate",
		)
	}
	if !config.validate && !config.mutatesEvent() {
		return errors.New("at least one event processing action is required")
	}
	if err := validateOutputConfig(config); err != nil {
		return err
	}
	return nil
}

func validationSelection(option validationCodesOption) validationCodeSelection {
	return validationCodeSelection{
		configured: option.configured,
		codes:      append([]validation.Code(nil), option.codes...),
	}
}

func validObservablePathNotation(style string) bool {
	return style == "" || pathstyle.Style(style).Valid()
}

func validateOutputConfig(config processConfig) error {
	if !config.mutatesEvent() && config.eventOutput != "" {
		return errors.New(
			"event output options require --enrich, --unenrich, --force-remove, --enum-siblings, or --observables",
		)
	}
	if config.eventsDir == stdioPath {
		return errors.New("--events-dir cannot be -")
	}
	if config.outputDir == stdioPath {
		return errors.New("directory output options cannot be -")
	}
	if config.outputDir != "" && (config.eventOutput != "" || config.reportOutput != "") {
		return errors.New("--output-dir cannot be used with operation-specific output options")
	}

	singleEventMode := config.eventPath != ""
	if singleEventMode {
		if config.summaryFile != "" || config.summaryJSONFile != "" {
			return errors.New("summary options require --events-dir")
		}
		if config.quiet {
			return errors.New("--quiet requires --events-dir")
		}
		if config.mutatesEvent() {
			if countSet(config.outputDir != "", config.eventOutput != "") != 1 {
				return errors.New(
					"single event mutation requires exactly one of --output-dir DIR or --event-output FILE",
				)
			}
		}
		if config.generatesReport() && countSet(config.outputDir != "", config.reportOutput != "") != 1 {
			return errors.New("single event reporting requires exactly one of --output-dir DIR or --report-output FILE")
		}
	} else {
		if config.mutatesEvent() && config.eventOutput != "" {
			return errors.New("--event-output cannot be used with --events-dir")
		}
		if config.reportOutput != "" {
			return errors.New("--report-output cannot be used with --events-dir")
		}
		if config.outputDir == "" {
			return errors.New("directory processing requires --output-dir DIR")
		}
	}
	if countSet(
		config.eventOutput == stdioPath,
		config.reportOutput == stdioPath,
		config.summaryFile == stdioPath,
		config.summaryJSONFile == stdioPath,
	) > 1 {
		return errors.New("only one output option may use stdout")
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
