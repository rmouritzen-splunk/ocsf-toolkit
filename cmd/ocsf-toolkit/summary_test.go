package main

import (
	"bytes"
	"encoding/json"
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

func TestHumanSummaryUsesHelpWrappingAndPreservesIndentation(t *testing.T) {
	report := summaryReport{
		Metadata: buildSummaryMetadata(),
		Validation: &validationSummaryReport{
			WarningOnlyEvents: 12,
		},
	}

	output := humanSummaryWithMetadata(report, 20)

	require.Contains(t, output, "  Events with\n  warnings only: 12\n")
}

func TestNewSummaryReportIncludesOnlySelectedSections(t *testing.T) {
	assert := require.New(t)
	report := newSummaryReport(processConfig{
		schemaPath:        "schema.json",
		validate:          true,
		enrichmentRemoval: true,
	}, nil)

	assert.Equal(summaryVersion, report.SummaryVersion)
	assert.Equal("schema.json", report.SchemaPath)
	assert.NotNil(report.Validation)
	assert.Nil(report.Enrichment)
	assert.NotNil(report.EnrichmentRemoval)
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
		"--validation-level", validation.AttributeRecommendedMissing.String()+"=warning",
		"--output-dir", outputDir,
		"--summary", summaryPath,
		"--summary-format", "json",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	assert.Empty(stdout)

	report := readEventReport(assert, validationPath)
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelError))
	assert.NotEmpty(findingsAtLevel(report.Validation.Findings, validation.LevelWarning))

	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assertSummaryMetadata(assert, summary)
	assert.Equal(summaryVersion, summary.SummaryVersion)
	assert.Equal(1, summary.EventFilesProcessed)
	assert.NotNil(summary.Validation)
	assert.Equal(1, summary.Validation.WarningOnlyEvents)
	assert.Equal(0, summary.Validation.ErrorEvents)
}

func TestProcessDirectorySummaryAggregatesValidationEnrichmentIssuesAndOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(dir, "output")
	summaryPath := filepath.Join(dir, "summary.json")

	writeJSONFile(assert, filepath.Join(eventsDir, "warning.json"), validCLIEvent())
	errorEvent := validCLIEvent()
	delete(errorEvent, "activity_id")
	writeJSONFile(assert, filepath.Join(eventsDir, "error.json"), errorEvent)
	issueEvent := validCLIEvent()
	issueEvent["activity_id"] = json.Number("1234")
	writeJSONFile(assert, filepath.Join(eventsDir, "issue.json"), issueEvent)

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--validate",
		"--validation-level", validation.AttributeRecommendedMissing.String()+"=warning",
		"--output-dir", outputDir,
		"--summary", summaryPath,
		"--summary-format", "json",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)

	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assert.Equal(3, summary.EventFilesProcessed)
	assert.Equal(1, summary.Validation.WarningOnlyEvents)
	assert.Equal(2, summary.Validation.ErrorEvents)
	assert.Equal(4, summary.Enrichment.EnumSiblingsAdded)
	assert.Equal(0, summary.Enrichment.ObservablesAdded)
	assert.Equal(1, summary.Issues.ReportedCount)
	assert.Equal(3, summary.Output.EventsWritten)
	assert.Equal(3, summary.Output.ReportsWritten)
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
		"--summary", summaryPath,
		"--summary-format", "json",
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
		"--summary-format", "text",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)

	summaryBytes, err := os.ReadFile(summaryPath)
	assert.NoError(err)
	expectedSummary :=
		"ocsf-toolkit " + version + " " + runtime.GOOS + "/" + runtime.GOARCH + " " + runtime.Version() + "\n\n" +
			summaryText(validationHumanSummaryLines(0, 0)...)
	assert.Empty(stdout)
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
		"--summary", summaryPath,
		"--summary-format", "json",
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
	assert.Equal(0, summary.Validation.WarningOnlyEvents)
	assert.Equal(0, summary.Validation.ErrorEvents)
	assert.NotNil(summary.Enrichment)
	assert.Equal(1, summary.Output.EventsWritten)
	summaryJSON, err := os.ReadFile(summaryPath)
	assert.NoError(err)
	assert.Equal(1, bytes.Count(summaryJSON, []byte(`"output":`)))
	assert.NotContains(string(summaryJSON), `"outputs":`)
	assert.NotContains(string(summaryJSON), `"files":`)
	assert.Contains(string(summaryJSON), `"event_files_processed":1`)
	assert.Contains(string(summaryJSON), `"summary_version":1`)
	assert.Contains(string(summaryJSON), `"warning_only_events":0`)
	assert.Contains(string(summaryJSON), `"error_events":0`)
	assert.NotContains(string(summaryJSON), `"events_with_no_errors_or_warnings"`)
	assert.NotContains(string(summaryJSON), `"events_with_errors_only"`)
	assert.NotContains(string(summaryJSON), `"events_with_warnings_and_errors"`)
	assert.NotContains(string(summaryJSON), `"events_with_warnings_only"`)
	assert.NotContains(string(summaryJSON), `"total_error_count"`)
	assert.NotContains(string(summaryJSON), `"total_warning_count"`)
	assert.NotContains(string(summaryJSON), `"ignored_error_count"`)
	assert.NotContains(string(summaryJSON), `"ignored_warning_count"`)
	assert.NotContains(string(summaryJSON), `"ignored_issue_count"`)
	assert.Contains(string(summaryJSON), `"events_written":1`)
	assert.NotContains(string(summaryJSON), `"event_write_failures"`)
	assert.NotContains(string(summaryJSON), `"report_write_failures"`)
	assert.NotContains(string(summaryJSON), `"event_destination":`)
	assert.NotContains(string(summaryJSON), `"report_destination":`)
	assert.NotContains(string(summaryJSON), `"enrichment_outputs_written"`)
	assert.NotContains(string(summaryJSON), `"validation_outputs_written"`)
	assert.NotContains(string(summaryJSON), `"enriched_events_written"`)
	assert.NotContains(string(summaryJSON), `"validation_results_written"`)
	assert.NotContains(string(summaryJSON), `"validation_errors"`)
	assert.NotContains(string(summaryJSON), `"validation_warnings"`)
	assert.NotContains(string(summaryJSON), `"validation_failures"`)
}

func TestDirectorySummariesIncludeInitializationIssues(t *testing.T) {
	tests := []struct {
		name          string
		format        string
		expectedValue string
	}{
		{name: "text", expectedValue: "issue_at_init_schema_enum_sibling_target_not_string"},
		{name: "json", format: "json", expectedValue: `"code":"issue_at_init_schema_enum_sibling_target_not_string"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeInitializationIssueTestSchema(assert, dir)
			eventsDir := filepath.Join(dir, "events")
			writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
			summaryPath := filepath.Join(dir, "summary."+test.name)
			args := []string{
				"--schema", schemaPath,
				"--events-dir", eventsDir,
				"--output-dir", filepath.Join(dir, "output"),
				"--validate",
				"--summary", summaryPath,
			}
			if test.format != "" {
				args = append(args, "--summary-format", test.format)
			}

			exitCode, stdout, stderr := runCLI(args...)

			assert.Zero(exitCode, stderr)
			assert.Empty(stdout)
			assert.Contains(stderr, "issue_at_init_schema_enum_sibling_target_not_string")
			summary, err := os.ReadFile(summaryPath)
			assert.NoError(err)
			assert.Contains(string(summary), test.expectedValue)
		})
	}
}

func assertSummaryMetadata(assert *require.Assertions, summary summaryReport) {
	assert.Equal("ocsf-toolkit", summary.Metadata.Tool.Name)
	assert.Equal(version, summary.Metadata.Tool.Version)
	assert.Equal(runtime.Version(), summary.Metadata.Tool.GoVersion)
	assert.Equal(runtime.GOOS, summary.Metadata.Tool.Platform.OS)
	assert.Equal(runtime.GOARCH, summary.Metadata.Tool.Platform.Architecture)
}

func validationHumanSummaryLines(warningOnlyEvents, errorEvents int) []string {
	return []string{
		"Event files processed: 1",
		"Validation:",
		fmt.Sprintf("  Events with warnings only: %d", warningOnlyEvents),
		fmt.Sprintf("  Events with errors: %d", errorEvents),
		"Processing issues:",
		"  Reported: 0",
		"Output:",
		"  Processed events written: 0",
		"  Processing reports written: 1",
	}
}

func TestProcessDirectoryIsQuietByDefault(t *testing.T) {
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
	assert.Empty(stdout)
}

func TestProcessDirectoryWritesExplicitSummaryToStdout(t *testing.T) {
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
		"--summary", "-",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	assert.Equal(
		"ocsf-toolkit "+version+" "+runtime.GOOS+"/"+runtime.GOARCH+" "+runtime.Version()+"\n\n"+
			summaryText(validationHumanSummaryLines(0, 0)...),
		stdout,
	)
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
		"--summary", summaryPath,
		"--summary-format", "json",
	)

	assert.Equal(0, exitCode, stderr)
	var summary summaryReport
	readJSONFile(assert, summaryPath, &summary)
	assert.Equal(2, summary.EventFilesProcessed)
	assert.NotNil(summary.EnrichmentRemoval)
	assert.Equal(1, summary.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Equal(1, summary.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Equal(0, summary.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, summary.EnrichmentRemoval.ObservablesRetained)
	assert.Equal(1, summary.EnrichmentRemoval.EventsWithRetainedEnumSiblings)
	assert.Equal(1, summary.EnrichmentRemoval.EventsWithRetainedObservables)
	assert.Equal(2, summary.Output.EventsWritten)
	assert.Equal(2, summary.Output.ReportsWritten)
	assert.FileExists(filepath.Join(outputDir, "reports", "first.report.json"))
	assert.FileExists(filepath.Join(outputDir, "reports", "second.report.json"))
}
