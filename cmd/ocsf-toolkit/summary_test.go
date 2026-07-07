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
	eventsDir := filepath.Join(dir, "events")
	eventPath := filepath.Join(eventsDir, "event.json")
	outputDir := filepath.Join(dir, "output")
	validationPath := filepath.Join(outputDir, "reports", "event.json")
	summaryPath := filepath.Join(dir, "summary.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--warn-on-missing-recommended",
		"--output-dir", outputDir,
		"--summary-json-file", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	assert.Equal(
		"ocsf-toolkit "+version+" "+runtime.GOOS+"/"+runtime.GOARCH+" "+runtime.Version()+"\n\n"+
			summaryText(
				"Event files processed: 1",
				"Events with validation errors: 0",
				"Events with validation warnings (no errors): 1",
			),
		stdout,
	)

	validation := readEventReport(assert, validationPath)
	assert.Empty(validation.Validation.Errors)
	assert.NotEmpty(validation.Validation.Warnings)

	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assertSummaryMetadata(assert, summary)
	assert.Equal(1, *summary.EventFilesProcessed)
	assert.NotNil(summary.Validation)
	assert.Equal(0, *summary.Validation.EventsWithErrors)
	assert.Equal(1, *summary.Validation.EventsWithWarningsOnly)
	assert.Equal(0, *summary.Validation.TotalErrorCount)
	assert.Equal(1, *summary.Validation.TotalWarningCount)
}

func TestProcessHumanSummaryFileIncludesMetadata(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	eventPath := filepath.Join(eventsDir, "event.json")
	outputDir := filepath.Join(dir, "output")
	summaryPath := filepath.Join(dir, "summary.txt")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", outputDir,
		"--summary-file", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)

	summaryBytes, err := os.ReadFile(summaryPath)
	assert.NoError(err)
	expectedSummary :=
		"ocsf-toolkit " + version + " " + runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version() + "\n\n" +
			summaryText(
				"Event files processed: 1",
				"Events with validation errors: 0",
				"Events with validation warnings (no errors): 0",
			)
	assert.Equal(expectedSummary, stdout)
	assert.Equal(expectedSummary, string(summaryBytes))
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
		"--summary-json-file", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)

	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "nested", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	assert.Equal("Do", enrichedEvent["activity_name"])

	report := readEventReport(assert, filepath.Join(outputDir, "reports", "nested", "event.json"))
	assert.Empty(report.Validation.Errors)

	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assertSummaryMetadata(assert, summary)
	assert.Equal(1, *summary.EventFilesProcessed)
	assert.NotNil(summary.Validation)
	assert.Equal(0, *summary.Validation.EventsWithErrors)
	assert.Equal(0, *summary.Validation.EventsWithWarningsOnly)
	assert.Equal(0, *summary.Validation.TotalErrorCount)
	assert.Equal(0, *summary.Validation.TotalWarningCount)
	assert.NotNil(summary.EventProcessing)
	assert.Equal(1, *summary.EventProcessing.EventsWritten)
	assert.Equal(0, *summary.EventProcessing.EventsSkipped)
	assert.Len(summary.Files, 1)
	assert.Equal(filepath.Join("nested", "event.json"), summary.Files[0].RelativePath)
	assert.Equal(filepath.Join(outputDir, "events", "nested", "event.json"), summary.Files[0].EventPath)
	assert.Equal(filepath.Join(outputDir, "reports", "nested", "event.json"), summary.Files[0].ReportPath)
	summaryJSON, err := os.ReadFile(summaryPath)
	assert.NoError(err)
	assert.Contains(string(summaryJSON), `"event_files_processed":1`)
	assert.Contains(string(summaryJSON), `"events_with_errors":0`)
	assert.Contains(string(summaryJSON), `"events_with_warnings_only":0`)
	assert.Contains(string(summaryJSON), `"total_error_count":0`)
	assert.Contains(string(summaryJSON), `"total_warning_count":0`)
	assert.Contains(string(summaryJSON), `"events_written":1`)
	assert.Contains(string(summaryJSON), `"events_skipped":0`)
	assert.Contains(string(summaryJSON), `"event_destination":`)
	assert.Contains(string(summaryJSON), `"report_path":`)
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

func TestProcessDirectoryWritesDefaultSummaryToStdout(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	assert.Equal(
		"ocsf-toolkit "+version+" "+runtime.GOOS+"/"+runtime.GOARCH+" "+runtime.Version()+"\n\n"+
			summaryText(
				"Event files processed: 1",
				"Events with validation errors: 0",
				"Events with validation warnings (no errors): 0",
			),
		stdout,
	)
}

func TestProcessQuietSuppressesDefaultSummary(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	eventPath := filepath.Join(eventsDir, "event.json")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", outputDir,
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
		"--summary-json-file", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assert.NotNil(summary.EnrichmentRemoval)
	assert.Equal(1, *summary.EnrichmentRemoval.EventsWithRetainedEnumSiblings)
	assert.Equal(1, *summary.EnrichmentRemoval.EventsWithRetainedObservables)
	assert.FileExists(filepath.Join(outputDir, "reports", "first.json"))
	assert.FileExists(filepath.Join(outputDir, "reports", "second.json"))
}
