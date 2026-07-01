package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func TestVersionOptionPrintsVersionAndExits(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--version")

	assert.Equal(0, exitCode)
	assert.Equal("ocsf-toolkit "+version+"\n", stdout)
	assert.Empty(stderr)
}

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
		"--update-in-place",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Processed event written: "+eventPath,
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
	assert.Contains(stderr, "single event validation requires exactly one of --output-dir DIR or --validation-output FILE")
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
	assert.NotContains(stderr, "processed events")

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
	assertSummaryMetadata(assert, summary)
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

func TestProcessHumanSummaryOutputIncludesMetadata(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	validationPath := filepath.Join(dir, "event.validation.json")
	summaryPath := filepath.Join(dir, "summary.txt")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", validationPath,
		"--summary-output", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Validation errors: 0",
		"Validation warnings: 0",
		"Validation result written: "+validationPath,
	), stderr)

	summaryBytes, err := os.ReadFile(summaryPath)
	assert.NoError(err)
	assert.Equal(
		"ocsf-toolkit "+version+" "+runtime.GOOS+"/"+runtime.GOARCH+" "+runtime.Version()+"\n\n"+
			summaryText(
				"Event file processed: "+eventPath,
				"Validation errors: 0",
				"Validation warnings: 0",
				"Validation result written: "+validationPath,
			),
		string(summaryBytes),
	)
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
	assertSummaryMetadata(assert, summary)
	assert.Equal("", summary.EventFileProcessed)
	assert.Equal(1, *summary.EventFilesProcessed)
	assert.NotNil(summary.Validation)
	assert.Equal(0, *summary.Validation.EventsWithErrors)
	assert.Equal(0, *summary.Validation.EventsWithWarningsOnly)
	assert.Equal(0, *summary.Validation.TotalErrorCount)
	assert.Equal(0, *summary.Validation.TotalWarningCount)
	assert.Nil(summary.Validation.ErrorCount)
	assert.Nil(summary.Validation.WarningCount)
	assert.NotNil(summary.EventProcessing)
	assert.Equal(1, *summary.EventProcessing.EventsWritten)
	assert.Equal(0, *summary.EventProcessing.EventsSkipped)
	assert.Empty(summary.EventProcessing.EventWritten)
	assert.Empty(summary.EventProcessing.EventSkipped)
	assert.Len(summary.Files, 1)
	assert.Equal(filepath.Join("nested", "event.json"), summary.Files[0].RelativePath)
	assert.Equal(filepath.Join(outputDir, "nested", "event.json"), summary.Files[0].EventPath)
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
	assert.Contains(string(summaryJSON), `"event_path":`)
	assert.Contains(string(summaryJSON), `"validation_result_path":`)
	assert.NotContains(string(summaryJSON), `"enrichment_outputs_written"`)
	assert.NotContains(string(summaryJSON), `"validation_outputs_written"`)
	assert.NotContains(string(summaryJSON), `"enriched_events_written"`)
	assert.NotContains(string(summaryJSON), `"validation_results_written"`)
	assert.NotContains(string(summaryJSON), `"validation_errors"`)
	assert.NotContains(string(summaryJSON), `"validation_warnings"`)
	assert.NotContains(string(summaryJSON), `"validation_failures"`)
}

func assertSummaryMetadata(assert *require.Assertions, summary summaryReport) {
	assert.Equal("ocsf-toolkit", summary.Metadata.Tool.Name)
	assert.Equal(version, summary.Metadata.Tool.Version)
	assert.Equal(runtime.Version(), summary.Metadata.Tool.GoVersion)
	assert.Equal(runtime.GOOS, summary.Metadata.Tool.Platform.OS)
	assert.Equal(runtime.GOARCH, summary.Metadata.Tool.Platform.Architecture)
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
	assert.Contains(stderr, "directory validation requires --output-dir DIR")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE")
}

func TestProcessDirectoryRejectsOutputDirectoryInsideInputTree(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(eventsDir, "processed")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "output directory must not be the events input directory or one of its descendants")
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
		"--event-output", enrichPath,
		"--validate",
		"--validation-output", validationPath,
		"--skip-invalid-output",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Processed event skipped: validation errors found",
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
		"--event-output", outputPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Processed event written: "+outputPath,
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
		"--event-output", outputPath,
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
		"--update-in-place",
		"--event-output", filepath.Join(dir, "enriched.json"),
		"--overwrite",
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "--update-in-place and --event-output are mutually exclusive")
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
	outputDir := filepath.Join(dir, "output")
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
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, eventPath))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	validation := readValidationOutput(assert, filepath.Join(outputDir, "events", "windows", "event-validation.json"))
	assert.Equal(eventPath, validation.InputPath)
	assert.Contains(stderr, "Processed event written: "+filepath.Join(outputDir, eventPath))
	assert.Contains(stderr, "Validation result written: "+filepath.Join(outputDir, "events", "windows", "event-validation.json"))
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
		"--event-output", outputPath,
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
	assert.Contains(stderr, "event write error")
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
	assert.Contains(stderr, "a.json: event write error")
	assert.Contains(stderr, "Event files processed before error: 1")
	assert.NoFileExists(filepath.Join(outputDir, "b.json"))
}

func TestProcessDirectoryRejectsGeneratedOutputCollisionsBeforeWriting(t *testing.T) {
	testCases := []struct {
		name       string
		colliding  string
		operations []string
	}{
		{
			name:       "validation report collides with processed event",
			colliding:  "event-validation.json",
			operations: []string{"--enrich", "--validate"},
		},
		{
			name:       "enrichment-removal report collides with processed event",
			colliding:  "event-unenrich-issues.json",
			operations: []string{"--unenrich"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventsDir := filepath.Join(dir, "events")
			outputDir := filepath.Join(dir, "output")
			writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
			writeJSONFile(assert, filepath.Join(eventsDir, testCase.colliding), validCLIEvent())

			args := []string{"--schema", schemaPath, "--events-dir", eventsDir, "--output-dir", outputDir, "--overwrite"}
			args = append(args, testCase.operations...)
			exitCode, _, stderr := runCLI(args...)

			assert.Equal(1, exitCode)
			assert.Contains(stderr, "is selected for both")
			assert.NoDirExists(outputDir, "collision planning should fail before any output is written")
		})
	}
}

func TestProcessDirectoryRejectsOutputPathThroughSymlink(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(dir, "output")
	outsideDir := filepath.Join(dir, "outside")
	writeJSONFile(assert, filepath.Join(eventsDir, "nested", "event.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(outputDir, 0o755))
	assert.NoError(os.MkdirAll(outsideDir, 0o755))
	assert.NoError(os.Symlink(outsideDir, filepath.Join(outputDir, "nested")))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "traverses symbolic link")
	assert.NoFileExists(filepath.Join(outsideDir, "event.json"))
}

func TestProcessRejectsSummaryCollisionBeforeOverwritingAnotherArtifact(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "processed.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", outputPath,
		"--summary-json-output", outputPath,
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
	assert.NoFileExists(outputPath)
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
		"--event-output", outputPath,
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

func TestProcessSingleEventSafelyRemovesEnrichmentAndWritesIssues(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "processed.json")
	issuesPath := filepath.Join(dir, "event-unenrich-issues.json")
	event := validCLIEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "source-specific"
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": 1, "value": "go"},
		jsonish.Map{"name": "ball.green", "type_id": 1, "value": "missing"},
	}
	writeJSONFile(assert, eventPath, event)

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--unenrich",
		"--event-output", outputPath,
		"--unenrich-issues-output", issuesPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Equal(summaryText(
		"Event file processed: "+eventPath,
		"Processed event written: "+outputPath,
		"Enum siblings removed: 1",
		"Observables removed: 1",
		"Enrichment-removal issues written: "+issuesPath,
	), stderr)

	processed, err := jsonio.ReadObject(outputPath)
	assert.NoError(err)
	assert.NotContains(processed, "class_name")
	assert.Equal("source-specific", processed["activity_name"])
	assert.Len(processed["observables"], 1)

	var issues unenrichIssuesOutput
	readJSONFile(assert, issuesPath, &issues)
	assert.Equal(1, issues.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Equal(1, issues.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Equal(1, issues.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, issues.EnrichmentRemoval.ObservablesRetained)
	assert.Len(issues.Issues, 1)
	assert.Equal("observable_value_not_found", issues.Issues[0].Code)
}

func TestProcessRejectsConflictingEnrichmentActions(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--unenrich",
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "adding and removing enum siblings are mutually exclusive")
}

func TestProcessRejectsInvalidProcessorOptions(t *testing.T) {
	tests := []struct {
		name      string
		options   []string
		wantError string
	}{
		{
			name:      "enum enrichment modifier without enrich",
			options:   []string{"--no-enum-siblings"},
			wantError: "--no-enum-siblings and --no-observables require --enrich",
		},
		{
			name:      "observable enrichment modifier without enrich",
			options:   []string{"--no-observables"},
			wantError: "--no-enum-siblings and --no-observables require --enrich",
		},
		{
			name:      "retain enum siblings without unenrich",
			options:   []string{"--retain-enum-siblings"},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name:      "retain observables without unenrich",
			options:   []string{"--retain-observables"},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name:      "force enum sibling removal without unenrich",
			options:   []string{"--force-remove-enum-siblings"},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name:      "force observable removal without unenrich",
			options:   []string{"--force-remove-observables"},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name:      "issues output without unenrich",
			options:   []string{"--unenrich-issues-output", filepath.Join("unused", "issues.json")},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name: "retain and force enum siblings",
			options: []string{
				"--unenrich", "--retain-enum-siblings", "--force-remove-enum-siblings",
			},
			wantError: "--retain-enum-siblings and --force-remove-enum-siblings are mutually exclusive",
		},
		{
			name: "retain and force observables",
			options: []string{
				"--unenrich", "--retain-observables", "--force-remove-observables",
			},
			wantError: "--retain-observables and --force-remove-observables are mutually exclusive",
		},
		{
			name: "enrichment without action",
			options: []string{
				"--enrich", "--no-enum-siblings", "--no-observables",
			},
			wantError: "at least one event processing action is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventPath := filepath.Join(dir, "event.json")
			writeJSONFile(assert, eventPath, validCLIEvent())
			args := []string{"--schema", schemaPath, "--event", eventPath}
			args = append(args, test.options...)

			exitCode, _, stderr := runCLI(args...)

			assert.Equal(2, exitCode)
			assert.Contains(stderr, test.wantError)
		})
	}
}

func TestProcessAllowsIndependentAddAndRemoveActions(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "output")
	event := validCLIEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": 1, "value": "go"},
	}
	writeJSONFile(assert, eventPath, event)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich", "--no-observables",
		"--unenrich", "--retain-enum-siblings",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	processed, err := jsonio.ReadObject(filepath.Join(outputDir, "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", processed["class_name"])
	assert.Equal("Do", processed["activity_name"])
	assert.NotContains(processed, "observables")
	assert.FileExists(filepath.Join(outputDir, "event-unenrich-issues.json"))
}

func TestProcessDirectorySummaryCountsEventsWithRetainedRemovalValues(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(dir, "output")
	summaryPath := filepath.Join(dir, "summary.json")
	first := validCLIEvent()
	first["activity_name"] = "source-specific"
	first["observables"] = []any{jsonish.Map{"name": "ball.green", "value": "missing"}}
	writeJSONFile(assert, filepath.Join(eventsDir, "first.json"), first)
	second := validCLIEvent()
	second["class_name"] = "Alpha"
	writeJSONFile(assert, filepath.Join(eventsDir, "second.json"), second)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--unenrich",
		"--output-dir", outputDir,
		"--summary-json-output", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assert.NotNil(summary.EnrichmentRemoval)
	assert.Equal(1, *summary.EnrichmentRemoval.EventsWithRetainedEnumSiblings)
	assert.Equal(1, *summary.EnrichmentRemoval.EventsWithRetainedObservables)
	assert.FileExists(filepath.Join(outputDir, "first-unenrich-issues.json"))
	assert.FileExists(filepath.Join(outputDir, "second-unenrich-issues.json"))
}

func TestHelp(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--help")

	assert.Equal(0, exitCode)
	assert.Empty(stderr)
	assert.Contains(stdout, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --unenrich | --validate) [options]")
	assert.Contains(stdout, "General Options:")
	assert.Contains(stdout, "Enrichment Options:")
	assert.Contains(stdout, "Enrichment Removal Options:")
	assert.Contains(stdout, "Validation Options:")
	assert.Contains(stdout, "-s, --schema=COMPILED_SCHEMA_FILE")
	assert.Contains(stdout, "-e, --event=FILE")
	assert.Contains(stdout, "-d, --events-dir=DIR")
	assert.Contains(stdout, "-o, --output-dir=DIR")
	assert.Contains(stdout, "--fail-on-validation-errors")
	assert.Contains(stdout, "--validation-output=FILE")
	assert.Contains(stdout, "--no-enum-siblings")
	assert.Contains(stdout, "--no-observables")
	assert.Contains(stdout, "-V, --validate")
	assert.Contains(stdout, "-E, --enrich")
	assert.Contains(stdout, "-i, --update-in-place")
	assert.Contains(stdout, "--event-output=FILE")
	assert.Contains(stdout, "-u, --unenrich")
	assert.Contains(stdout, "--retain-enum-siblings")
	assert.Contains(stdout, "--retain-observables")
	assert.Contains(stdout, "--force-remove-enum-siblings")
	assert.Contains(stdout, "--force-remove-observables")
	assert.Contains(stdout, "--unenrich-issues-output=FILE")
	assert.Contains(stdout, "--skip-invalid-output")
	assert.Greater(strings.Index(stdout, "--skip-invalid-output"), strings.Index(stdout, "Validation Options:"))
	assert.Greater(strings.Index(stdout, "--validation-output=FILE"), strings.Index(stdout, "Validation Options:"))
	assert.Greater(strings.Index(stdout, "--event-output=FILE"), strings.Index(stdout, "General Options:"))
	assert.Less(strings.Index(stdout, "--event-output=FILE"), strings.Index(stdout, "Enrichment Options:"))
	assert.Contains(stdout, "Do not write non-validation")
	assert.Contains(stdout, "outputs for")
	assert.Contains(stdout, "events with validation errors")
	assert.Contains(stdout, "Enrich events; adds enum siblings and")
	assert.Contains(stdout, "observables by default")
	assert.Contains(stdout, "Do not add enum siblings")
	assert.Contains(stdout, "Do not add observables")
	assert.Contains(stdout, "--summary-json-output")
	assert.Contains(stdout, "--overwrite")
	assert.Contains(stdout, "-p, --pretty-json")
	assert.Contains(stdout, "-q, --quiet")
	assert.Contains(stdout, "--output-dir writes processed events and selected reports to one output tree.")
	assert.Contains(stdout, "Directory outputs preserve input-relative paths.")
	assert.Contains(stdout, "    With --events-dir, paths are relative to that directory.")
	assert.Contains(stdout, "    With --event, safe relative paths are preserved;")
	assert.Contains(stdout, "absolute paths and paths with .. use the basename.")
	assert.Contains(stdout, "Validation files use <base>-validation.json.")
	assert.Contains(stdout, "Enrichment-removal issue files use <base>-unenrich-issues.json.")
	assert.Contains(stdout, "Output directories are created if necessary.")
	assert.Contains(stdout, "Output files are not replaced without --overwrite.")
	assert.Contains(stdout, "--update-in-place replaces input event files without --overwrite.")
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
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --unenrich | --validate) [options]")
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
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --unenrich | --validate) [options]")
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
		"--event-output", outputPath,
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
					"ball": jsonish.Map{
						"type":        "object_t",
						"object_type": "ball",
					},
					"observables": jsonish.Map{
						"type":        "object_t",
						"object_type": "observable",
						"is_array":    true,
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
			"ball": jsonish.Map{
				"name": "ball",
				"attributes": jsonish.Map{
					"green": jsonish.Map{"type": "string_t"},
				},
			},
			"observable": jsonish.Map{
				"name": "observable",
				"attributes": jsonish.Map{
					"name":    jsonish.Map{"type": "string_t"},
					"value":   jsonish.Map{"type": "string_t"},
					"type_id": jsonish.Map{"type": "integer_t"},
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
					"object_t":  jsonish.Map{"caption": "Object"},
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
