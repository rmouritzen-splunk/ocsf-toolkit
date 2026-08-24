package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestWriteFailureDetailsQuotesInputPaths(t *testing.T) {
	var output bytes.Buffer
	writeFailureDetails(&output, processSummary{Files: []fileSummary{{
		InputPath:  "events/file with spaces\nand newline.json",
		ParseError: "invalid JSON",
	}}})

	require.Equal(t, "\"events/file with spaces\\nand newline.json\": parse error: invalid JSON\n", output.String())
}

func TestHumanSummaryUsesHelpWrappingAndPreservesIndentation(t *testing.T) {
	report := summaryReport{
		Metadata: buildSummaryMetadata(),
		Validation: &validationSummaryReport{
			SuppressedErrorCount: 12,
		},
	}

	output := humanSummaryWithMetadata(report, 20)

	require.Contains(t, output, "  Errors suppressed:\n  12\n")
}

func TestSummaryAggregatesAllValidationCategoriesAndProcessorCounts(t *testing.T) {
	assert := require.New(t)
	summary := processSummary{SchemaPath: "schema.json"}
	files := []fileSummary{
		{InputPath: "clean.json", ProcessingCompleted: true, EventWritten: true, ReportWritten: true},
		{
			InputPath: "warning.json", ProcessingCompleted: true, ValidationWarningCount: 2,
			SuppressedValidationWarningCount: 1, EnumSiblingsAdded: 2, ObservablesAdded: 3,
			IssueCount: 1, SuppressedIssueCount: 2, EventWritten: true, ReportWritten: true,
		},
		{
			InputPath: "error.json", ProcessingCompleted: true, ValidationErrorCount: 3,
			SuppressedValidationErrorCount: 2, EnumSiblingsRemoved: 4, EnumSiblingsRetained: 1,
			ObservablesRemoved: 5, ObservablesRetained: 2, EventWritten: true, ReportWritten: true,
		},
		{InputPath: "both.json", ProcessingCompleted: true, ValidationErrorCount: 1, ValidationWarningCount: 1,
			EventWritten: true, ReportWritten: true},
	}
	for _, file := range files {
		updateSummary(&summary, file, true)
	}

	report := buildSummaryReport(processConfig{validate: true, enrich: true, enrichmentRemoval: true}, summary)

	assert.Equal(summaryVersion, report.SummaryVersion)
	assert.Equal(4, report.EventFilesProcessed)
	assert.Equal(1, report.Validation.EventsWithNoFindings)
	assert.Equal(1, report.Validation.EventsWithWarningsOnly)
	assert.Equal(1, report.Validation.EventsWithErrorsOnly)
	assert.Equal(1, report.Validation.EventsWithWarningsAndErrors)
	assert.Equal(4, report.Validation.TotalErrorCount)
	assert.Equal(3, report.Validation.TotalWarningCount)
	assert.Equal(2, report.Validation.SuppressedErrorCount)
	assert.Equal(1, report.Validation.SuppressedWarningCount)
	assert.Equal(2, report.Enrichment.EnumSiblingsAdded)
	assert.Equal(3, report.Enrichment.ObservablesAdded)
	assert.Equal(4, report.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Equal(1, report.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Equal(5, report.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(2, report.EnrichmentRemoval.ObservablesRetained)
	assert.Equal(1, report.Issues.ReportedCount)
	assert.Equal(2, report.Issues.SuppressedCount)
	assert.Equal(4, report.Outputs.EventsWritten)
	assert.Equal(4, report.Outputs.ReportsWritten)
	assert.Len(report.Files, 4)
	for _, file := range report.Files {
		assert.NotNil(file.Validation)
		assert.NotNil(file.Enrichment)
		assert.NotNil(file.EnrichmentRemoval)
	}
}

func TestProcessValidationSummaryCountsEventsWithWarningsOnly(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	eventPath := filepath.Join(eventsDir, "event.json")
	outputDir := filepath.Join(dir, "output")
	validationPath := filepath.Join(outputDir, "reports", "event.report.json")
	summaryPath := filepath.Join(dir, "summary.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--warn-on-missing-recommended",
		"--output-dir", outputDir,
		"--summary-json", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	assert.Equal(
		"ocsf-toolkit "+version+" "+runtime.GOOS+"/"+runtime.GOARCH+" "+runtime.Version()+"\n\n"+
			summaryText(validationHumanSummaryLines(0, 1, 0, 0, 0, 1)...),
		stdout,
	)

	report := readEventReport(assert, validationPath)
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelError))
	assert.NotEmpty(findingsAtLevel(report.Validation.Findings, validation.LevelWarning))

	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assertSummaryMetadata(assert, summary)
	assert.Equal(summaryVersion, summary.SummaryVersion)
	assert.Equal(1, summary.EventFilesProcessed)
	assert.NotNil(summary.Validation)
	assert.Equal(0, summary.Validation.EventsWithErrorsOnly)
	assert.Equal(1, summary.Validation.EventsWithWarningsOnly)
	assert.Equal(0, summary.Validation.TotalErrorCount)
	assert.Equal(1, summary.Validation.TotalWarningCount)
}

func TestProcessOverwriteReplacesExistingJSONSummary(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	outputDir := filepath.Join(dir, "output")
	summaryPath := filepath.Join(dir, "summary.json")
	assert.NoError(os.WriteFile(summaryPath, []byte(`{"unrelated": true}`), 0o600))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", outputDir,
		"--summary-json", summaryPath,
		"--overwrite",
	)

	assert.Equal(0, exitCode, stderr)
	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assert.Equal(1, summary.EventFilesProcessed)
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
		"--summary", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)

	summaryBytes, err := os.ReadFile(summaryPath)
	assert.NoError(err)
	expectedSummary :=
		"ocsf-toolkit " + version + " " + runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version() + "\n\n" +
			summaryText(validationHumanSummaryLines(1, 0, 0, 0, 0, 0)...)
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
	assert.NoError(os.WriteFile(filepath.Join(eventsDir, "ignored.txt"), []byte("{}"), 0o600))

	outputDir := filepath.Join(dir, "output")
	summaryPath := filepath.Join(dir, "summary.json")

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--enrich",
		"--output-dir", outputDir,
		"--summary-json", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)

	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "nested", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	assert.Equal("Do", enrichedEvent["activity_name"])

	report := readEventReport(assert, filepath.Join(outputDir, "reports", "nested", "event.report.json"))
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelError))

	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assertSummaryMetadata(assert, summary)
	assert.Equal(summaryVersion, summary.SummaryVersion)
	assert.Equal(1, summary.EventFilesProcessed)
	assert.NotNil(summary.Validation)
	assert.Equal(1, summary.Validation.EventsWithNoFindings)
	assert.Equal(0, summary.Validation.EventsWithWarningsOnly)
	assert.Equal(0, summary.Validation.TotalErrorCount)
	assert.Equal(0, summary.Validation.TotalWarningCount)
	assert.NotNil(summary.Enrichment)
	assert.Equal(1, summary.Outputs.EventsWritten)
	assert.Len(summary.Files, 1)
	assert.Equal(filepath.Join("nested", "event.json"), summary.Files[0].RelativePath)
	assert.Equal(filepath.Join(outputDir, "events", "nested", "event.json"), summary.Files[0].Outputs.EventDestination)
	assert.Equal(
		filepath.Join(outputDir, "reports", "nested", "event.report.json"),
		summary.Files[0].Outputs.ReportDestination,
	)
	assert.NotNil(summary.Files[0].Validation)
	assert.NotNil(summary.Files[0].Enrichment)
	summaryJSON, err := os.ReadFile(summaryPath)
	assert.NoError(err)
	assert.Contains(string(summaryJSON), `"event_files_processed":1`)
	assert.Contains(string(summaryJSON), `"summary_version":1`)
	assert.Contains(string(summaryJSON), `"events_with_no_errors_or_warnings":1`)
	assert.Contains(string(summaryJSON), `"events_with_errors_only":0`)
	assert.Contains(string(summaryJSON), `"events_with_warnings_and_errors":0`)
	assert.Contains(string(summaryJSON), `"events_with_warnings_only":0`)
	assert.Contains(string(summaryJSON), `"total_error_count":0`)
	assert.Contains(string(summaryJSON), `"total_warning_count":0`)
	assert.Contains(string(summaryJSON), `"suppressed_error_count":0`)
	assert.Contains(string(summaryJSON), `"suppressed_warning_count":0`)
	assert.NotContains(string(summaryJSON), `"suppressed_default_error_count"`)
	assert.NotContains(string(summaryJSON), `"suppressed_default_warning_count"`)
	assert.Contains(string(summaryJSON), `"events_written":1`)
	assert.Contains(string(summaryJSON), `"event_destination":`)
	assert.Contains(string(summaryJSON), `"report_destination":`)
	assert.NotContains(string(summaryJSON), `"enrichment_outputs_written"`)
	assert.NotContains(string(summaryJSON), `"validation_outputs_written"`)
	assert.NotContains(string(summaryJSON), `"enriched_events_written"`)
	assert.NotContains(string(summaryJSON), `"validation_results_written"`)
	assert.NotContains(string(summaryJSON), `"validation_errors"`)
	assert.NotContains(string(summaryJSON), `"validation_warnings"`)
	assert.NotContains(string(summaryJSON), `"validation_failures"`)
}

func TestDirectorySummaryFilesIncludeInitializationIssues(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeInitializationIssueTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	humanPath := filepath.Join(dir, "summary.txt")
	jsonPath := filepath.Join(dir, "summary.json")

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", filepath.Join(dir, "output"),
		"--validate",
		"--summary", humanPath,
		"--summary-json", jsonPath,
		"--quiet",
	)
	assert.Zero(exitCode, stderr)
	assert.Contains(stderr, "issue_at_init_schema_enum_sibling_target_not_string")
	humanSummary, err := os.ReadFile(humanPath)
	assert.NoError(err)
	assert.Contains(string(humanSummary), "issue_at_init_schema_enum_sibling_target_not_string")
	jsonSummary, err := os.ReadFile(jsonPath)
	assert.NoError(err)
	assert.Contains(string(jsonSummary), `"code":"issue_at_init_schema_enum_sibling_target_not_string"`)
}

func assertSummaryMetadata(assert *require.Assertions, summary summaryReport) {
	assert.Equal("ocsf-toolkit", summary.Metadata.Tool.Name)
	assert.Equal(version, summary.Metadata.Tool.Version)
	assert.Equal(runtime.Version(), summary.Metadata.Tool.GoVersion)
	assert.Equal(runtime.GOOS, summary.Metadata.Tool.Platform.OS)
	assert.Equal(runtime.GOARCH, summary.Metadata.Tool.Platform.Architecture)
}

func validationHumanSummaryLines(noFindings, warningsOnly, errorsOnly, both, errors, warnings int) []string {
	return []string{
		"Event files processed: 1",
		"Validation:",
		fmt.Sprintf("  Events with no errors or warnings: %d", noFindings),
		fmt.Sprintf("  Events with warnings only: %d", warningsOnly),
		fmt.Sprintf("  Events with errors only: %d", errorsOnly),
		fmt.Sprintf("  Events with warnings and errors: %d", both),
		fmt.Sprintf("  Errors reported: %d", errors),
		fmt.Sprintf("  Warnings reported: %d", warnings),
		"  Errors suppressed: 0",
		"  Warnings suppressed: 0",
		"Processing issues:",
		"  Reported: 0",
		"  Suppressed: 0",
		"Outputs:",
		"  Processed events written: 0",
		"  Processing reports written: 1",
	}
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
			summaryText(validationHumanSummaryLines(1, 0, 0, 0, 0, 0)...),
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
		"--summary-json", summaryPath,
	)

	assert.Equal(0, exitCode, stderr)
	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assert.NotNil(summary.EnrichmentRemoval)
	assert.Equal(1, summary.EnrichmentRemoval.EventsWithRetainedEnumSiblings)
	assert.Equal(1, summary.EnrichmentRemoval.EventsWithRetainedObservables)
	assert.FileExists(filepath.Join(outputDir, "reports", "first.report.json"))
	assert.FileExists(filepath.Join(outputDir, "reports", "second.report.json"))
}
