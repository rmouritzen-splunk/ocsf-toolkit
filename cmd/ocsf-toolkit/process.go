package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ocsf/ocsf-toolkit/eventschema"
	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

var errStopProcessing = errors.New("stop event processing")

var walkEventDirectory = filepath.WalkDir

type eventReport struct {
	EventSource       string                               `json:"event_source"`
	EventDestination  string                               `json:"event_destination,omitempty"`
	Validation        *eventschema.ValidationResult        `json:"validation,omitempty"`
	Enrichment        *eventschema.EnrichmentResult        `json:"enrichment,omitempty"`
	EnrichmentRemoval *eventschema.EnrichmentRemovalResult `json:"enrichment_removal,omitempty"`
	Issues            []eventschema.ProcessingIssue        `json:"issues,omitempty"`
}

func processEvents(
	config processConfig,
	destinations processingDestinations,
	stdin io.Reader,
	outputs *destinationWriter,
) (processSummary, bool, error) {
	pipeline, err := newEventProcessorPipeline(config)
	if err != nil {
		return processSummary{}, false, err
	}

	summary := processSummary{SchemaPath: config.schemaPath}
	retainFileSummaries := config.eventPath != "" || config.summaryJSONFile != ""
	if config.eventPath != "" {
		input := inputEvent{path: config.eventPath, rel: stdinEventRelativePath}
		if config.eventPath != stdioPath {
			input.rel = ""
		}
		fileResult := processOneEvent(
			config,
			pipeline,
			input,
			destinations.eventOutput,
			destinations.reportOutput,
			stdin,
			outputs,
		)
		updateSummary(&summary, fileResult, retainFileSummaries)
		return summary, fileResult.failed(), nil
	}

	err = walkEventDirectory(config.eventsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		rel, err := filepath.Rel(config.eventsDir, path)
		if err != nil {
			return err
		}
		input := inputEvent{path: path, rel: rel}
		var eventOutput *outputDestination
		if config.mutatesEvent() {
			eventOutput = displayDestination(eventOutputPath(config, input))
		}
		var reportOutput *outputDestination
		if config.generatesReport() {
			reportOutput = displayDestination(reportOutputPath(config, input))
		}
		fileResult := processOneEvent(config, pipeline, input, eventOutput, reportOutput, stdin, outputs)
		updateSummary(&summary, fileResult, retainFileSummaries)
		if fileResult.failed() {
			return errStopProcessing
		}
		return nil
	})
	if errors.Is(err, errStopProcessing) {
		return summary, true, nil
	}
	if err != nil {
		return summary, false, fmt.Errorf("failed to walk events directory %q: %w", config.eventsDir, err)
	}
	return summary, false, nil
}

func newEventProcessorPipeline(config processConfig) (eventschema.EventProcessorPipeline, error) {
	schema, err := eventschema.New(config.schemaPath)
	if err != nil {
		return nil, err
	}

	processors := make([]eventschema.EventProcessor, 0, 3)
	if config.enrich {
		processors = append(processors, eventschema.NewEnrichment(
			eventschema.WithAddEnumSiblings(config.addEnumSiblings),
			eventschema.WithAddObservables(config.addObservables),
		))
	}
	if config.unenrich {
		removalOptions := []eventschema.EnrichmentRemovalOption{
			eventschema.WithRemoveEnumSiblings(config.removeEnumSiblings),
			eventschema.WithRemoveObservables(config.removeObservables),
		}
		if config.forceRemoveEnumSiblings {
			removalOptions = append(removalOptions, eventschema.WithForceRemoveEnumSiblings())
		}
		if config.forceRemoveObservables {
			removalOptions = append(removalOptions, eventschema.WithForceRemoveObservables())
		}
		processors = append(processors, eventschema.NewEnrichmentRemoval(removalOptions...))
	}
	// Keep validation last so it checks the event after all local processing.
	if config.validate {
		validationOptions := make([]eventschema.ValidationOption, 0, 1)
		if config.warnOnMissingRecommended {
			validationOptions = append(validationOptions, eventschema.WithWarnOnMissingRecommended())
		}
		processors = append(processors, eventschema.NewValidation(validationOptions...))
	}
	pipeline, err := schema.NewEventProcessorPipeline(processors...)
	if err != nil {
		return nil, fmt.Errorf("configure event processor pipeline: %w", err)
	}
	return pipeline, nil
}

func processOneEvent(
	config processConfig,
	pipeline eventschema.EventProcessorPipeline,
	input inputEvent,
	eventOutput *outputDestination,
	reportOutput *outputDestination,
	stdin io.Reader,
	outputs *destinationWriter,
) fileSummary {
	fileResult := fileSummary{
		InputPath:    input.path,
		RelativePath: input.rel,
	}

	event, err := readInputEvent(input, stdin)
	if err != nil {
		fileResult.ParseError = err.Error()
		return fileResult
	}

	result, err := pipeline.ProcessEvent(event)
	if err != nil {
		fileResult.ProcessingError = err.Error()
		return fileResult
	}
	fileResult.Processed = true
	fileResult.ValidationErrorCount = len(result.Validation.Errors)
	fileResult.ValidationWarningCount = len(result.Validation.Warnings)
	fileResult.EnumSiblingsRemoved = result.EnrichmentRemoval.EnumSiblingsRemoved
	fileResult.EnumSiblingsRetained = result.EnrichmentRemoval.EnumSiblingsRetained
	fileResult.ObservablesRemoved = result.EnrichmentRemoval.ObservablesRemoved
	fileResult.ObservablesRetained = result.EnrichmentRemoval.ObservablesRetained

	skipOutputs := config.skipInvalidOutput && len(result.Validation.Errors) > 0
	if skipOutputs {
		fileResult.EventSkipped = config.mutatesEvent()
	} else if config.mutatesEvent() {
		outputPath := eventOutput.path.display
		fileResult.EventPath = outputPath
		if err := writeProcessedJSON(config, outputs, outputPath, "processed event", event); err != nil {
			fileResult.EventWriteError = err.Error()
			return fileResult
		}
		fileResult.EventWritten = true
	}

	if config.generatesReport() {
		report := buildEventReport(config, input, result, fileResult, skipOutputs)
		outputPath := reportOutput.path.display
		fileResult.ReportPath = outputPath
		if err := writeProcessedJSON(config, outputs, outputPath, "processing report", report); err != nil {
			fileResult.ReportWriteError = err.Error()
			return fileResult
		}
		fileResult.ReportWritten = true
	}
	return fileResult
}

func writeProcessedJSON(
	config processConfig,
	outputs *destinationWriter,
	path string,
	description string,
	value any,
) error {
	if config.eventsDir != "" {
		return outputs.writeDerivedJSON(path, value)
	}
	return outputs.writeJSON(path, description, value)
}

func buildEventReport(
	config processConfig,
	input inputEvent,
	result eventschema.ProcessingResult,
	fileResult fileSummary,
	validationOnly bool,
) eventReport {
	report := eventReport{EventSource: input.path}
	if fileResult.EventWritten {
		report.EventDestination = fileResult.EventPath
	}
	if config.validate {
		validation := result.Validation
		report.Validation = &validation
	}
	if !validationOnly && config.enrich {
		enrichment := result.Enrichment
		report.Enrichment = &enrichment
	}
	if !validationOnly && config.unenrich {
		removal := result.EnrichmentRemoval
		report.EnrichmentRemoval = &removal
	}
	if !validationOnly {
		report.Issues = result.Issues
	}
	return report
}

func displayDestination(path string) *outputDestination {
	return &outputDestination{path: filesystemPath{display: path}}
}

func readInputEvent(input inputEvent, stdin io.Reader) (jsonish.Map, error) {
	if input.path == stdioPath {
		event, err := jsonio.DecodeObject(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to decode JSON object from stdin: %w", err)
		}
		return event, nil
	}
	return jsonio.ReadObject(input.path)
}

func (file fileSummary) failed() bool {
	return file.ParseError != "" || file.ProcessingError != "" || file.EventWriteError != "" || file.ReportWriteError != ""
}
