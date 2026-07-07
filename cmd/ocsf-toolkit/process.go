package main

import (
	"fmt"
	"io"

	"github.com/ocsf/ocsf-toolkit/eventschema"
	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

type validationOutput struct {
	InputPath  string                       `json:"input_path"`
	Validation eventschema.ValidationResult `json:"validation"`
}

type unenrichIssuesOutput struct {
	InputPath         string                              `json:"input_path"`
	EnrichmentRemoval eventschema.EnrichmentRemovalResult `json:"enrichment_removal"`
	Issues            []eventschema.ProcessingIssue       `json:"issues"`
}

func processEvents(
	config processConfig,
	layout processingLayout,
	stdin io.Reader,
	stdout io.Writer,
) (processSummary, bool, error) {
	schema, err := eventschema.New(config.schemaPath)
	if err != nil {
		return processSummary{}, false, err
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
	// Keep validation last so it checks the event after all local processing,
	// including enrichment. Future event processors should be inserted before it.
	if config.validate {
		validationOptions := make([]eventschema.ValidationOption, 0, 1)
		if config.warnOnMissingRecommended {
			validationOptions = append(validationOptions, eventschema.WithWarnOnMissingRecommended())
		}
		processors = append(processors, eventschema.NewValidation(validationOptions...))
	}
	pipeline, err := schema.NewEventProcessorPipeline(processors...)
	if err != nil {
		return processSummary{}, false, fmt.Errorf("configure event processor pipeline: %w", err)
	}

	summary := processSummary{
		SchemaPath: config.schemaPath,
		Files:      make([]fileSummary, 0, len(layout.events)),
	}
	runtimeFailure := false
	for _, eventLayout := range layout.events {
		fileResult := processOneEvent(config, pipeline, eventLayout, stdin, stdout)
		updateSummary(&summary, fileResult)
		if fileResult.ParseError != "" || fileResult.ProcessingError != "" || fileResult.EventWriteError != "" ||
			fileResult.ValidationResultWriteError != "" || fileResult.UnenrichIssuesWriteError != "" {
			runtimeFailure = true
			break
		}
	}
	return summary, runtimeFailure, nil
}

func processOneEvent(
	config processConfig,
	pipeline eventschema.EventProcessorPipeline,
	eventLayout eventFileLayout,
	stdin io.Reader,
	stdout io.Writer,
) fileSummary {
	input := eventLayout.input
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

	if config.mutatesEvent() {
		if err := writeEventOutput(config, eventLayout.eventOutput, event, result, &fileResult, stdout); err != nil {
			fileResult.EventWriteError = err.Error()
			return fileResult
		}
	}
	if config.unenrich {
		if err := writeUnenrichIssuesOutput(config, input, eventLayout.unenrichIssues, result, &fileResult, stdout); err != nil {
			fileResult.UnenrichIssuesWriteError = err.Error()
			return fileResult
		}
	}
	if config.validate {
		if err := writeValidationOutput(config, input, eventLayout.validationOutput, result, &fileResult, stdout); err != nil {
			fileResult.ValidationResultWriteError = err.Error()
			return fileResult
		}
	}
	return fileResult
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

func writeEventOutput(
	config processConfig,
	destination *outputDestination,
	event jsonish.Map,
	result eventschema.ProcessingResult,
	fileResult *fileSummary,
	stdout io.Writer,
) error {
	if config.skipInvalidOutput && len(result.Validation.Errors) > 0 {
		fileResult.EventSkipped = true
		return nil
	}

	outputPath := destination.path.display
	fileResult.EventPath = outputPath
	writeOptions := config.writeOptions()
	if config.updateInPlace {
		writeOptions.overwrite = true
	}
	if err := writeJSONDestination(outputPath, event, writeOptions, stdout); err != nil {
		return err
	}
	fileResult.EventWritten = true
	return nil
}
func writeValidationOutput(
	config processConfig,
	input inputEvent,
	destination *outputDestination,
	result eventschema.ProcessingResult,
	fileResult *fileSummary,
	stdout io.Writer,
) error {
	output := validationOutput{
		InputPath:  input.path,
		Validation: result.Validation,
	}
	outputPath := destination.path.display

	fileResult.ValidationResultPath = outputPath
	if err := writeJSONDestination(outputPath, output, config.writeOptions(), stdout); err != nil {
		return err
	}
	fileResult.ValidationResultWritten = true
	return nil
}
func writeUnenrichIssuesOutput(
	config processConfig,
	input inputEvent,
	destination *outputDestination,
	result eventschema.ProcessingResult,
	fileResult *fileSummary,
	stdout io.Writer,
) error {
	issues := make([]eventschema.ProcessingIssue, 0)
	for _, issue := range result.Issues {
		if issue.Phase == "enrichment_removal" {
			issues = append(issues, issue)
		}
	}
	output := unenrichIssuesOutput{
		InputPath:         input.path,
		EnrichmentRemoval: result.EnrichmentRemoval,
		Issues:            issues,
	}
	outputPath := destination.path.display
	fileResult.UnenrichIssuesPath = outputPath
	if err := writeJSONDestination(outputPath, output, config.writeOptions(), stdout); err != nil {
		return err
	}
	fileResult.UnenrichIssuesWritten = true
	return nil
}
