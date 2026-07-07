package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

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
