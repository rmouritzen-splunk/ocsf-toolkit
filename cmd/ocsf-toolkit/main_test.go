package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-processor/jsonio"
	"github.com/ocsf/ocsf-processor/jsonish"
)

func TestProcessSingleEventEnrichesInPlaceAndWritesValidation(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	validationPath := filepath.Join(dir, "event.validation.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", validationPath,
		"--enrich",
		"--enrich-in-place",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Enriched event written: "+eventPath,
		"Validation errors: 0",
		"Validation warnings: 0",
		"Validation result written: "+validationPath,
	), stderr)

	event, err := jsonio.ReadObject(eventPath)
	assert.NoError(err)
	assert.Equal("Alpha", event["class_name"])
	assert.Equal("Do", event["activity_name"])

	validation := readValidationOutput(assert, validationPath)
	assert.Equal(eventPath, validation.InputPath)
	assert.Empty(validation.Validation.Errors)
	assert.Empty(validation.Validation.Warnings)
}

func TestProcessSingleEventSupportsCommonShortOptions(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "out")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"-s", schemaPath,
		"-e", eventPath,
		"-V",
		"-E",
		"-o", outputDir,
		"-q",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	validation := readValidationOutput(assert, filepath.Join(outputDir, "event-validation.json"))
	assert.Equal(eventPath, validation.InputPath)
}

func TestProcessSingleEventValidationRequiresOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "single event validation requires exactly one of --output-dir DIR, --validation-output FILE, or --validation-output-dir DIR")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE")
}

func TestProcessSingleEventValidationCanWriteToStdout(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", "-",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Validation errors: 0",
		"Validation warnings: 0",
		"Validation result written: stdout",
	), stderr)
	assert.NotContains(stderr, "enriched events")

	var validation validationOutput
	assert.NoError(json.Unmarshal([]byte(stdout), &validation))
	assert.Equal(eventPath, validation.InputPath)
	assert.Empty(validation.Validation.Errors)
	assert.Empty(validation.Validation.Warnings)
}

func TestProcessValidationSummaryCountsEventsWithWarningsOnly(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	validationPath := filepath.Join(dir, "event.validation.json")
	summaryPath := filepath.Join(dir, "summary.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--warn-on-missing-recommended",
		"--validation-output", validationPath,
		"--summary-json-output", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Validation errors: 0",
		"Validation warnings: 1",
		"Validation result written: "+validationPath,
	), stderr)

	validation := readValidationOutput(assert, validationPath)
	assert.Empty(validation.Validation.Errors)
	assert.NotEmpty(validation.Validation.Warnings)

	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assert.Equal(eventPath, summary.EventFileProcessed)
	assert.Nil(summary.EventFilesProcessed)
	assert.NotNil(summary.Validation)
	assert.Equal(0, *summary.Validation.ErrorCount)
	assert.Equal(1, *summary.Validation.WarningCount)
	assert.Equal(validationPath, summary.Validation.ResultWritten)
	assert.Nil(summary.Validation.EventsWithErrors)
	assert.Nil(summary.Validation.EventsWithWarningsOnly)
	assert.Nil(summary.Validation.TotalErrorCount)
	assert.Nil(summary.Validation.TotalWarningCount)
}

func TestProcessDirectoryPreservesRelativeOutputPathsAndWritesSummary(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	eventPath := filepath.Join(eventsDir, "nested", "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(filepath.Join(eventsDir, "ignored.txt"), []byte("{}"), 0o644))

	outputDir := filepath.Join(dir, "output")
	summaryPath := filepath.Join(dir, "summary.json")

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--enrich",
		"--output-dir", outputDir,
		"--summary-json-output", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)

	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "nested", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	assert.Equal("Do", enrichedEvent["activity_name"])

	validation := readValidationOutput(assert, filepath.Join(outputDir, "nested", "event-validation.json"))
	assert.Empty(validation.Validation.Errors)

	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assert.Equal("", summary.EventFileProcessed)
	assert.Equal(1, *summary.EventFilesProcessed)
	assert.NotNil(summary.Validation)
	assert.Equal(0, *summary.Validation.EventsWithErrors)
	assert.Equal(0, *summary.Validation.EventsWithWarningsOnly)
	assert.Equal(0, *summary.Validation.TotalErrorCount)
	assert.Equal(0, *summary.Validation.TotalWarningCount)
	assert.Nil(summary.Validation.ErrorCount)
	assert.Nil(summary.Validation.WarningCount)
	assert.NotNil(summary.Enrichment)
	assert.Equal(1, *summary.Enrichment.EventsWritten)
	assert.Equal(0, *summary.Enrichment.EventsSkipped)
	assert.Empty(summary.Enrichment.EventWritten)
	assert.Empty(summary.Enrichment.EventSkipped)
	assert.Len(summary.Files, 1)
	assert.Equal(filepath.Join("nested", "event.json"), summary.Files[0].RelativePath)
	assert.Equal(filepath.Join(outputDir, "nested", "event.json"), summary.Files[0].EnrichedEventPath)
	assert.Equal(filepath.Join(outputDir, "nested", "event-validation.json"), summary.Files[0].ValidationResultPath)
	summaryJSON, err := os.ReadFile(summaryPath)
	assert.NoError(err)
	assert.Contains(string(summaryJSON), `"event_files_processed":1`)
	assert.Contains(string(summaryJSON), `"events_with_errors":0`)
	assert.Contains(string(summaryJSON), `"events_with_warnings_only":0`)
	assert.Contains(string(summaryJSON), `"total_error_count":0`)
	assert.Contains(string(summaryJSON), `"total_warning_count":0`)
	assert.Contains(string(summaryJSON), `"events_written":1`)
	assert.Contains(string(summaryJSON), `"events_skipped":0`)
	assert.Contains(string(summaryJSON), `"enriched_event_path":`)
	assert.Contains(string(summaryJSON), `"validation_result_path":`)
	assert.NotContains(string(summaryJSON), `"enrichment_outputs_written"`)
	assert.NotContains(string(summaryJSON), `"validation_outputs_written"`)
	assert.NotContains(string(summaryJSON), `"enriched_events_written"`)
	assert.NotContains(string(summaryJSON), `"validation_results_written"`)
	assert.NotContains(string(summaryJSON), `"validation_errors"`)
	assert.NotContains(string(summaryJSON), `"validation_warnings"`)
	assert.NotContains(string(summaryJSON), `"validation_failures"`)
}

func TestProcessDirectoryValidationRequiresOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "directory validation requires exactly one of --output-dir DIR or --validation-output-dir DIR")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE")
}

func TestProcessValidationFailureCanSetExitCode(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	validationPath := filepath.Join(dir, "event.validation.json")
	event := validCLIEvent()
	delete(event, "activity_id")
	writeJSONFile(assert, eventPath, event)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", validationPath,
		"--fail-on-validation-errors",
	)

	assert.Equal(1, exitCode, stderr)
	validation := readValidationOutput(assert, validationPath)
	assert.NotEmpty(validation.Validation.Errors)
}

func TestProcessSkipInvalidOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	enrichPath := filepath.Join(dir, "enriched.json")
	validationPath := filepath.Join(dir, "event.validation.json")
	event := validCLIEvent()
	delete(event, "activity_id")
	writeJSONFile(assert, eventPath, event)

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--enrich-output", enrichPath,
		"--validate",
		"--validation-output", validationPath,
		"--skip-invalid-output",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Enriched event skipped: validation errors found",
		"Validation errors: 1",
		"Validation warnings: 0",
		"Validation result written: "+validationPath,
	), stderr)
	assert.NoFileExists(enrichPath)
	validation := readValidationOutput(assert, validationPath)
	assert.NotEmpty(validation.Validation.Errors)
}

func TestProcessRejectsValidationOutputOverwritingEventFile(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", eventPath,
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "validation output must not overwrite the event file")
}

func TestProcessEnrichOutputInPlaceIsOrdinaryFilePath(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "in-place")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--enrich-output", outputPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Enriched event written: "+outputPath,
	), stderr)
	assert.NotContains(stderr, "Validation errors")
	enrichedEvent, err := jsonio.ReadObject(outputPath)
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])

	originalEvent, err := jsonio.ReadObject(eventPath)
	assert.NoError(err)
	assert.NotContains(originalEvent, "class_name")
}

func TestProcessCanDisableDefaultEnrichmentOptions(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--no-enum-siblings",
		"--no-observables",
		"--enrich-output", outputPath,
	)

	assert.Equal(0, exitCode, stderr)
	enrichedEvent, err := jsonio.ReadObject(outputPath)
	assert.NoError(err)
	assert.NotContains(enrichedEvent, "class_name")
	assert.NotContains(enrichedEvent, "activity_name")
}

func TestProcessRejectsMultipleEnrichmentOutputModes(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--enrich-in-place",
		"--enrich-output", filepath.Join(dir, "enriched.json"),
		"--overwrite",
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "single event enrichment requires exactly one of --enrich-in-place, --output-dir DIR, --enrich-output FILE, or --enrich-output-dir DIR")
}

func TestProcessSingleEventOutputDirWritesBothOutputs(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.MkdirAll(outputDir, 0o755))
	assert.NoError(os.WriteFile(filepath.Join(outputDir, "existing.txt"), []byte("kept"), 0o644))

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--validate",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])

	validation := readValidationOutput(assert, filepath.Join(outputDir, "event-validation.json"))
	assert.Empty(validation.Validation.Errors)
	assert.FileExists(filepath.Join(outputDir, "existing.txt"))
}

func TestProcessSingleEventDirectoryOutputPreservesRelativeEventPath(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join("events", "windows", "event.json")
	enrichOutputDir := filepath.Join(dir, "enriched")
	validationOutputDir := filepath.Join(dir, "validation")
	writeJSONFile(assert, filepath.Join(dir, eventPath), validCLIEvent())

	previousWd, err := os.Getwd()
	assert.NoError(err)
	assert.NoError(os.Chdir(dir))
	defer func() {
		assert.NoError(os.Chdir(previousWd))
	}()

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--validate",
		"--enrich-output-dir", enrichOutputDir,
		"--validation-output-dir", validationOutputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(enrichOutputDir, eventPath))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	validation := readValidationOutput(assert, filepath.Join(validationOutputDir, "events", "windows", "event-validation.json"))
	assert.Equal(eventPath, validation.InputPath)
	assert.Contains(stderr, "Enriched event written: "+filepath.Join(enrichOutputDir, eventPath))
	assert.Contains(stderr, "Validation result written: "+filepath.Join(validationOutputDir, "events", "windows", "event-validation.json"))
}

func TestProcessSingleEventDirectoryOutputDoesNotEscapeOutputDir(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join("..", "event.json")
	outputDir := filepath.Join(dir, "out")
	workDir := filepath.Join(dir, "out-work")
	writeJSONFile(assert, filepath.Join(dir, "event.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(workDir, 0o755))

	previousWd, err := os.Getwd()
	assert.NoError(err)
	assert.NoError(os.Chdir(workDir))
	defer func() {
		assert.NoError(os.Chdir(previousWd))
	}()

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--validate",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	assert.FileExists(filepath.Join(outputDir, "event-validation.json"))
	assert.NoFileExists(filepath.Join(dir, "event-validation.json"))
}

func TestEventOutputRelativePathDoesNotPreserveTraversal(t *testing.T) {
	assert := require.New(t)

	testCases := []struct {
		name  string
		input inputEvent
		want  string
	}{
		{
			name:  "relative input starts with traversal",
			input: inputEvent{path: filepath.Join("..", "event.json")},
			want:  "event.json",
		},
		{
			name:  "relative input contains traversal",
			input: inputEvent{path: filepath.Join("events", "..", "event.json")},
			want:  "event.json",
		},
		{
			name:  "directory relative path contains traversal",
			input: inputEvent{path: filepath.Join("events", "event.json"), rel: filepath.Join("nested", "..", "event.json")},
			want:  "event.json",
		},
		{
			name:  "safe relative path",
			input: inputEvent{path: filepath.Join("events", "windows", "event.json")},
			want:  filepath.Join("events", "windows", "event.json"),
		},
		{
			name:  "absolute path uses basename",
			input: inputEvent{path: filepath.Join(string(filepath.Separator), "tmp", "event.json")},
			want:  "event.json",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(testCase.want, eventOutputRelativePath(testCase.input))
		})
	}
}

func TestProcessRejectsExistingOutputWithoutOverwrite(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(outputPath, []byte("existing\n"), 0o644))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--enrich-output", outputPath,
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "already exists")
	assert.NotContains(stderr, "Event files processed")
	data, err := os.ReadFile(outputPath)
	assert.NoError(err)
	assert.Equal("existing\n", string(data))
}

func TestProcessStopsAfterFirstOutputWriteError(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "out")
	enrichedPath := filepath.Join(outputDir, "event.json")
	validationPath := filepath.Join(outputDir, "event-validation.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.MkdirAll(outputDir, 0o755))
	assert.NoError(os.WriteFile(enrichedPath, []byte("existing enriched\n"), 0o644))
	assert.NoError(os.WriteFile(validationPath, []byte("existing validation\n"), 0o644))

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--validate",
		"--output-dir", outputDir,
	)

	assert.Equal(1, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "enrichment write error")
	assert.Contains(stderr, "output file "+strconv.Quote(enrichedPath)+" already exists")
	assert.NotContains(stderr, "validation write error")
	assert.NotContains(stderr, "output file "+strconv.Quote(validationPath)+" already exists")
	assert.NotContains(stderr, "Event files processed")
	enrichedData, err := os.ReadFile(enrichedPath)
	assert.NoError(err)
	assert.Equal("existing enriched\n", string(enrichedData))
	validationData, err := os.ReadFile(validationPath)
	assert.NoError(err)
	assert.Equal("existing validation\n", string(validationData))
}

func TestProcessDirectoryReportsFilesProcessedBeforeError(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(dir, "out")
	writeJSONFile(assert, filepath.Join(eventsDir, "a.json"), validCLIEvent())
	writeJSONFile(assert, filepath.Join(eventsDir, "b.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(outputDir, 0o755))
	assert.NoError(os.WriteFile(filepath.Join(outputDir, "a.json"), []byte("existing\n"), 0o644))

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
	)

	assert.Equal(1, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "a.json: enrichment write error")
	assert.Contains(stderr, "Event files processed before error: 1")
	assert.NoFileExists(filepath.Join(outputDir, "b.json"))
}

func TestProcessOverwriteAllowsExistingOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(outputPath, []byte("existing\n"), 0o644))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--enrich-output", outputPath,
		"--overwrite",
	)

	assert.Equal(0, exitCode, stderr)
	enrichedEvent, err := jsonio.ReadObject(outputPath)
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
}

func TestProcessRejectsMultipleStdoutOutputs(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", "-",
		"--summary-json-output", "-",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "only one output option can write to stdout")
}

func TestProcessReadsSingleEventFromStdin(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	outputDir := filepath.Join(dir, "output")
	input, err := json.Marshal(validCLIEvent())
	assert.NoError(err)

	exitCode, stdout, stderr := runCLIWithInput(
		string(input),
		"--schema", schemaPath,
		"--event", "-",
		"--enrich",
		"--validate",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	validation := readValidationOutput(assert, filepath.Join(outputDir, "event-validation.json"))
	assert.Equal("-", validation.InputPath)
}

func TestProcessQuietSuppressesDefaultSummary(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	validationPath := filepath.Join(dir, "event-validation.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", validationPath,
		"--quiet",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)
}

func TestHelp(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--help")

	assert.Equal(0, exitCode)
	assert.Empty(stderr)
	assert.Contains(stdout, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --validate) [options]")
	assert.Contains(stdout, "General Options:")
	assert.Contains(stdout, "Enrichment Options:")
	assert.Contains(stdout, "Validation Options:")
	assert.Contains(stdout, "-s, --schema=COMPILED_SCHEMA_FILE")
	assert.Contains(stdout, "-e, --event=FILE")
	assert.Contains(stdout, "-d, --events-dir=DIR")
	assert.Contains(stdout, "-o, --output-dir=DIR")
	assert.Contains(stdout, "--fail-on-validation-errors")
	assert.Contains(stdout, "--validation-output=FILE")
	assert.Contains(stdout, "--validation-output-dir=DIR")
	assert.Contains(stdout, "--no-enum-siblings")
	assert.Contains(stdout, "--no-observables")
	assert.Contains(stdout, "-V, --validate")
	assert.Contains(stdout, "-E, --enrich")
	assert.Contains(stdout, "-i, --enrich-in-place")
	assert.Contains(stdout, "--enrich-output=FILE")
	assert.Contains(stdout, "--enrich-output-dir=DIR")
	assert.Contains(stdout, "--skip-invalid-output")
	assert.Greater(strings.Index(stdout, "--skip-invalid-output"), strings.Index(stdout, "Validation Options:"))
	assert.Greater(strings.Index(stdout, "--validation-output=FILE"), strings.Index(stdout, "Validation Options:"))
	assert.Greater(strings.Index(stdout, "--enrich-output=FILE"), strings.Index(stdout, "Enrichment Options:"))
	assert.Less(strings.Index(stdout, "--enrich-output=FILE"), strings.Index(stdout, "Validation Options:"))
	assert.Contains(stdout, "Do not write non-validation")
	assert.Contains(stdout, "outputs for")
	assert.Contains(stdout, "events with validation errors")
	assert.Contains(stdout, "Enrich events: add enum siblings and")
	assert.Contains(stdout, "observables")
	assert.Contains(stdout, "Do not add enum siblings")
	assert.Contains(stdout, "Do not add observables")
	assert.Contains(stdout, "--summary-json-output")
	assert.Contains(stdout, "--overwrite")
	assert.Contains(stdout, "-p, --pretty-json")
	assert.Contains(stdout, "-q, --quiet")
	assert.Contains(stdout, "--output-dir uses one output tree.")
	assert.Contains(stdout, "Use --enrich-output-dir and --validation-output-dir for separate trees.")
	assert.Contains(stdout, "Directory outputs preserve input-relative paths.")
	assert.Contains(stdout, "    With --events-dir, paths are relative to that directory.")
	assert.Contains(stdout, "    With --event, safe relative paths are preserved;")
	assert.Contains(stdout, "absolute paths and paths with .. use the basename.")
	assert.Contains(stdout, "Validation files use <base>-validation.json.")
	assert.Contains(stdout, "Output directories are created if necessary.")
	assert.Contains(stdout, "Output files are not replaced without --overwrite.")
	assert.Contains(stdout, "--enrich-in-place replaces input event files without --overwrite.")
	assert.Greater(strings.Index(stdout, "Notes:"), strings.Index(stdout, "Help Options:"))
}

func TestShortHelpMatchesLongHelp(t *testing.T) {
	assert := require.New(t)

	longHelpExitCode, longHelpStdout, longHelpStderr := runCLI("--help")
	shortHelpExitCode, shortHelpStdout, shortHelpStderr := runCLI("-h")

	assert.Equal(0, longHelpExitCode)
	assert.Empty(longHelpStderr)
	assert.Equal(0, shortHelpExitCode)
	assert.Empty(shortHelpStderr)
	assert.Equal(longHelpStdout, shortHelpStdout)
}

func TestParameterErrorPrintsTerseUsage(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--validate")

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "--schema is required")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --validate) [options]")
	assert.Contains(stderr, `Run "ocsf-toolkit --help" for full usage.`)
	assert.NotContains(stderr, "General Options:")
	assert.NotContains(stderr, "--schema=COMPILED_SCHEMA_FILE")
}

func TestMissingInputErrorPrintsTerseUsage(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--schema", "schema.json")

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "exactly one of --event or --events-dir is required")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --validate) [options]")
	assert.NotContains(stderr, "General Options:")
	assert.NotContains(stderr, "--schema=COMPILED_SCHEMA_FILE")
}

func TestSkipInvalidOutputRequiresValidate(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--enrich-output", outputPath,
		"--skip-invalid-output",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "validation options require --validate")
}

func TestSkipInvalidOutputRequiresEnrich(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	validationPath := filepath.Join(dir, "event-validation.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", validationPath,
		"--skip-invalid-output",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "--skip-invalid-output requires --enrich")
}

func runCLI(args ...string) (int, string, string) {
	return runCLIWithInput("", args...)
}

func runCLIWithInput(input string, args ...string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithIO(args, strings.NewReader(input), &stdout, &stderr)
	return exitCode, stdout.String(), stderr.String()
}

func summaryText(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func writeTestSchema(assert *require.Assertions, dir string) string {
	schemaPath := filepath.Join(dir, "schema.json")
	writeJSONFile(assert, schemaPath, jsonish.Map{
		"compile_version": 1,
		"version":         "1.0.0",
		"classes": jsonish.Map{
			"alpha": jsonish.Map{
				"name":     "alpha",
				"uid":      1,
				"category": "test",
				"attributes": jsonish.Map{
					"class_uid": jsonish.Map{
						"type":        "integer_t",
						"requirement": "required",
						"sibling":     "class_name",
						"enum": jsonish.Map{
							"1": jsonish.Map{"caption": "Alpha"},
						},
					},
					"class_name": jsonish.Map{"type": "string_t"},
					"activity_id": jsonish.Map{
						"type":        "integer_t",
						"requirement": "required",
						"sibling":     "activity_name",
						"enum": jsonish.Map{
							"1": jsonish.Map{"caption": "Do"},
						},
					},
					"activity_name": jsonish.Map{"type": "string_t"},
					"message": jsonish.Map{
						"type":        "string_t",
						"requirement": "recommended",
					},
					"type_uid": jsonish.Map{
						"type":        "long_t",
						"requirement": "required",
					},
					"metadata": jsonish.Map{
						"type":        "object_t",
						"object_type": "metadata",
						"requirement": "required",
					},
				},
			},
		},
		"objects": jsonish.Map{
			"metadata": jsonish.Map{
				"name": "metadata",
				"attributes": jsonish.Map{
					"version": jsonish.Map{
						"type":        "string_t",
						"requirement": "required",
					},
				},
			},
		},
		"dictionary": jsonish.Map{
			"attributes": jsonish.Map{},
			"types": jsonish.Map{
				"attributes": jsonish.Map{
					"integer_t": jsonish.Map{"caption": "Integer"},
					"long_t":    jsonish.Map{"caption": "Long"},
					"string_t":  jsonish.Map{"caption": "String"},
				},
			},
		},
		"profiles": jsonish.Map{},
	})
	return schemaPath
}

func validCLIEvent() jsonish.Map {
	return jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("101"),
		"metadata": jsonish.Map{
			"version": "1.0.0",
		},
	}
}

func writeJSONFile(assert *require.Assertions, path string, value any) {
	assert.NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	data, err := json.MarshalIndent(value, "", "  ")
	assert.NoError(err)
	data = append(data, '\n')
	assert.NoError(os.WriteFile(path, data, 0o644))
}

func readValidationOutput(assert *require.Assertions, path string) validationOutput {
	var output validationOutput
	readJSONFile(assert, path, &output)
	return output
}

func readJSONFile(assert *require.Assertions, path string, target any) {
	data, err := os.ReadFile(path)
	assert.NoError(err)
	assert.NoError(json.Unmarshal(data, target))
}
