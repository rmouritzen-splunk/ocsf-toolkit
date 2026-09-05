package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventpipeline"
	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/internal/fserror"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/schemaresult"
	"github.com/ocsf/ocsf-toolkit/validation"
)

const eventReportVersion = 1

// walkEventDirectory is replaceable so tests can inject directory traversal behavior and failures.
var walkEventDirectory = filepath.WalkDir

type commandConfigurationError struct {
	cause error
}

func (e *commandConfigurationError) Error() string {
	return e.cause.Error()
}

func (e *commandConfigurationError) Unwrap() error {
	return e.cause
}

type eventReport struct {
	ReportVersion     int                                  `json:"report_version"`
	EventSource       string                               `json:"event_source"`
	EventDestination  string                               `json:"event_destination,omitempty"`
	Validation        *eventresult.ValidationResult        `json:"validation,omitempty"`
	Enrichment        *eventresult.EnrichmentResult        `json:"enrichment,omitempty"`
	EnrichmentRemoval *eventresult.EnrichmentRemovalResult `json:"enrichment_removal,omitempty"`
	Issues            []eventresult.ProcessingIssue        `json:"issues,omitempty"`
}

func processEvents(
	config processConfig,
	pipeline *eventpipeline.Pipeline,
	destinations processingDestinations,
	stdin io.Reader,
	outputs *destinationWriter,
	summary *summaryReport,
) (bool, error) {
	if config.eventPath != "" {
		input := singleEventInput(config)
		return processOneEvent(
			config,
			pipeline,
			input,
			destinations.eventOutput,
			destinations.reportOutput,
			stdin,
			outputs,
			summary,
		)
	}

	hasValidationErrors := false
	firstEntry := true
	err := walkEventDirectory(config.eventsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fserror.QuotePaths(walkErr)
		}
		isDir := entry.IsDir()
		if firstEntry {
			firstEntry = false
			if !isDir {
				// config.eventsDir is no longer a directory.
				return nil
			}
		}
		if isDir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fserror.QuotePaths(err)
		}
		if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		rel, err := filepath.Rel(config.eventsDir, path)
		if err != nil {
			return err
		}
		input := inputEvent{path: path, rel: rel}
		// Unlike single-event mode's destinations (see resolveDistinctDestination), these per-file
		// paths are never checked against paths derived for other files in this same walk. Two distinct
		// input files whose derived paths alias the same output file (for example, on a case-insensitive
		// output filesystem, or because --events-dir is reachable through more than one name) are not
		// detected; with --overwrite the later file's output silently replaces the earlier one's. This is
		// intentional: guarding against it requires the kind of cross-file identity tracking that made
		// earlier revisions of this package increasingly complicated to get right, for a scenario that
		// requires an unusual setup for what is fundamentally a local testing tool. See "Can two different
		// input files produce the same output path?" in docs/FAQ.md.
		var eventOutput *outputDestination
		if config.mutatesEvent() {
			eventOutput = displayDestination(eventOutputPath(config, input))
		}
		var reportOutput *outputDestination
		if config.generatesReport() {
			reportOutput = displayDestination(reportOutputPath(config, input))
		}
		eventHasValidationErrors, err := processOneEvent(
			config, pipeline, input, eventOutput, reportOutput, stdin, outputs, summary,
		)
		hasValidationErrors = hasValidationErrors || eventHasValidationErrors
		return err
	})
	if err != nil {
		return hasValidationErrors, fmt.Errorf(
			"failed to process events directory %q: %w",
			config.eventsDir,
			fserror.QuotePaths(err),
		)
	}
	return hasValidationErrors, nil
}

func newPipeline(config processConfig) (*eventpipeline.Pipeline, []schemaresult.InitializationIssue, error) {
	schema, initializationIssues, err := eventpipeline.NewSchema(config.schemaPath)
	if err != nil {
		return nil, nil, err
	}

	options := make([]eventpipeline.PipelineOption, 0, 4)
	if config.enrich || config.enrichmentRemoval {
		options = append(options,
			eventpipeline.WithEnumSiblings(config.enumSiblingsAction),
			eventpipeline.WithObservables(config.observablesAction, config.observableTypeIDs...),
		)
		if config.observablePathNotation != "" && config.observablesAction == enrichment.Add {
			options = append(options, eventpipeline.WithEnrichmentObservablePathNotation(
				pathstyle.Style(config.observablePathNotation),
			))
		}
		options = append(options, eventpipeline.WithObservableDeduplication(config.observableDeduplication))
	}
	if config.validate {
		validationOptions := make([]eventpipeline.ValidationOption, 0, 1+len(config.validationLevels.rules))
		if config.observablePathNotation != "" {
			validationOptions = append(validationOptions, eventpipeline.WithValidationObservablePathNotation(
				pathstyle.Style(config.observablePathNotation),
			))
		}
		for _, rule := range config.validationLevels.rules {
			if rule.all {
				validationOptions = append(validationOptions, eventpipeline.WithAllValidationLevels(rule.level))
			} else {
				validationOptions = append(validationOptions, eventpipeline.WithValidationLevel(rule.code, rule.level))
			}
		}
		options = append(options, eventpipeline.WithValidation(validationOptions...))
	}
	for _, rule := range config.issueLevels.rules {
		if rule.all {
			options = append(options, eventpipeline.WithAllIssueLevels(rule.level))
		} else {
			options = append(options, eventpipeline.WithIssueLevel(rule.code, rule.level))
		}
	}
	options = append(options, eventpipeline.WithSchema(schema))
	pipeline, err := eventpipeline.NewPipeline(options...)
	if err != nil {
		if cliError := cliPipelineOptionError(err); cliError != nil {
			return nil, nil, &commandConfigurationError{cause: cliError}
		}
		return nil, nil, &commandConfigurationError{
			cause: fmt.Errorf("configure event processor pipeline: %w", err),
		}
	}
	reported, err := applyInitializationIssueLevels(config, initializationIssues)
	return pipeline, reported, err
}

func applyInitializationIssueLevels(
	config processConfig,
	initializationIssues []schemaresult.InitializationIssue,
) ([]schemaresult.InitializationIssue, error) {
	reported := make([]schemaresult.InitializationIssue, 0, len(initializationIssues))
	for _, found := range initializationIssues {
		level := effectiveInitializationIssueLevel(config.issueLevels.rules, found.Code)
		switch level {
		case issue.LevelIgnored:
			continue
		case issue.LevelError:
			return nil, fmt.Errorf("initialization issue %s: %s", found.Code, found.Message)
		default:
			reported = append(reported, found)
		}
	}
	return reported, nil
}

func effectiveInitializationIssueLevel(rules []issueLevelRule, code issue.Code) issue.Level {
	level := issue.LevelWarning
	for _, rule := range rules {
		if rule.all {
			if rule.level != issue.LevelIgnored || code.Ignorable() {
				level = rule.level
			}
			continue
		}
		if rule.code == code {
			level = rule.level
		}
	}
	return level
}

func cliPipelineOptionError(err error) error {
	var duplicate *eventpipeline.PipelineOptionDuplicateError
	if errors.As(err, &duplicate) {
		switch duplicate.Option() {
		case eventpipeline.PipelineOptionAllIssueLevels:
			return errors.New("duplicate --issue-level: all")
		case eventpipeline.PipelineOptionAllValidationLevels:
			return errors.New("duplicate --validation-level: all")
		default:
			return fmt.Errorf("%s may only be specified once", cliPipelineOptionName(duplicate.Option()))
		}
	}

	var issueAllAfterCode *eventpipeline.PipelineOptionIssueLevelAllAfterCodeError
	if errors.As(err, &issueAllAfterCode) {
		return errors.New("invalid --issue-level order: all=LEVEL must precede specific codes")
	}
	var issueDuplicate *eventpipeline.PipelineOptionIssueLevelDuplicateCodeError
	if errors.As(err, &issueDuplicate) {
		return fmt.Errorf("duplicate --issue-level: %s", issueDuplicate.Code())
	}
	var validationAllAfterCode *eventpipeline.PipelineOptionValidationLevelAllAfterCodeError
	if errors.As(err, &validationAllAfterCode) {
		return errors.New("invalid --validation-level order: all=LEVEL must precede specific codes")
	}
	var validationDuplicate *eventpipeline.PipelineOptionValidationLevelDuplicateCodeError
	if errors.As(err, &validationDuplicate) {
		return fmt.Errorf("duplicate --validation-level: %s", validationDuplicate.Code())
	}
	return nil
}

func cliPipelineOptionName(option eventpipeline.PipelineOptionName) string {
	switch option {
	case eventpipeline.PipelineOptionSchema:
		return "--schema"
	case eventpipeline.PipelineOptionEnumSiblings:
		return "--enum-siblings"
	case eventpipeline.PipelineOptionObservables:
		return "--observables"
	case eventpipeline.PipelineOptionEnrichmentObservablePathNotation,
		eventpipeline.PipelineOptionValidationObservablePathNotation:
		return "--observable-path-notation"
	case eventpipeline.PipelineOptionObservableDeduplication:
		return "--deduplicate-observables"
	case eventpipeline.PipelineOptionValidation:
		return "--validate"
	case eventpipeline.PipelineOptionIssueLevel, eventpipeline.PipelineOptionAllIssueLevels:
		return "--issue-level"
	case eventpipeline.PipelineOptionValidationLevel, eventpipeline.PipelineOptionAllValidationLevels:
		return "--validation-level"
	default:
		return "pipeline option " + string(option)
	}
}

func processOneEvent(
	config processConfig,
	pipeline *eventpipeline.Pipeline,
	input inputEvent,
	eventOutput *outputDestination,
	reportOutput *outputDestination,
	stdin io.Reader,
	outputs *destinationWriter,
	summary *summaryReport,
) (bool, error) {
	event, err := readInputEvent(input, stdin)
	if err != nil {
		return false, fmt.Errorf("%q: parse error: %w", input.path, err)
	}

	result, err := pipeline.ProcessEvent(event)
	if err != nil {
		return false, fmt.Errorf("%q: processing error: %w", input.path, err)
	}
	hasValidationErrors := false
	if summary != nil {
		resultHasValidationErrors := summary.addProcessingResult(result)
		if config.failOnValidationErrors {
			hasValidationErrors = resultHasValidationErrors
		}
	} else if config.failOnValidationErrors {
		hasValidationErrors = result.Validation().Count(validation.LevelError) > 0
	}

	eventDestination := ""
	if config.mutatesEvent() {
		outputPath := eventOutput.path.display
		if err := outputs.writeJSON(outputPath, event); err != nil {
			return hasValidationErrors, fmt.Errorf("%q: event write error: %w", input.path, err)
		}
		eventDestination = outputPath
		if summary != nil {
			summary.Output.EventsWritten++
		}
	}

	if config.generatesReport() {
		outputPath := reportOutput.path.display
		if config.eventOutput != "" && config.eventOutput != stdioPath &&
			config.reportOutput != "" && config.reportOutput != stdioPath &&
			sameFilesystemPath(eventOutput.path, reportOutput.path) {
			return hasValidationErrors, fmt.Errorf(
				"%q: report write error: processing report was not written because report output %q"+
					" names the same file as event output %q, which was already written",
				input.path,
				reportOutput.path.display,
				eventOutput.path.display,
			)
		}
		report := buildEventReport(config, input, result, eventDestination)
		if err := outputs.writeJSON(outputPath, report); err != nil {
			return hasValidationErrors, fmt.Errorf("%q: report write error: %w", input.path, err)
		}
		if summary != nil {
			summary.Output.ReportsWritten++
		}
	}
	return hasValidationErrors, nil
}

func buildEventReport(
	config processConfig,
	input inputEvent,
	result eventpipeline.ProcessingResult,
	eventDestination string,
) eventReport {
	report := eventReport{
		ReportVersion: eventReportVersion,
		EventSource:   input.path,
	}
	report.EventDestination = eventDestination
	if config.validate {
		validation := result.Validation()
		report.Validation = &validation
	}
	if config.enrich {
		enrichment := result.Enrichment()
		report.Enrichment = &enrichment
	}
	if config.enrichmentRemoval {
		removal := result.EnrichmentRemoval()
		report.EnrichmentRemoval = &removal
	}
	report.Issues = result.Issues()
	return report
}

func displayDestination(path string) *outputDestination {
	return &outputDestination{path: filesystemPath{display: path}}
}

func readInputEvent(input inputEvent, stdin io.Reader) (jsonish.Map, error) {
	if input.path == stdioPath {
		event, err := jsonio.DecodeObject(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to decode JSON object from stdin: %w", fserror.QuotePaths(err))
		}
		return event, nil
	}
	return jsonio.ReadObject(input.path)
}
