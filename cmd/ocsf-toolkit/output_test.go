package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonio"
)

func TestProcessRejectsExistingOutputWithoutOverwrite(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(outputPath, []byte("existing\n"), 0o644))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", outputPath,
		"--report-output", reportPath,
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
	enrichedPath := filepath.Join(outputDir, "events", "event.json")
	reportPath := filepath.Join(outputDir, "reports", "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.MkdirAll(filepath.Dir(enrichedPath), 0o755))
	assert.NoError(os.MkdirAll(filepath.Dir(reportPath), 0o755))
	assert.NoError(os.WriteFile(enrichedPath, []byte("existing enriched\n"), 0o644))
	assert.NoError(os.WriteFile(reportPath, []byte("existing report\n"), 0o644))

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
	assert.NotContains(stderr, "report write error")
	assert.NotContains(stderr, "output file "+strconv.Quote(reportPath)+" already exists")
	assert.NotContains(stderr, "Event files processed")
	enrichedData, err := os.ReadFile(enrichedPath)
	assert.NoError(err)
	assert.Equal("existing enriched\n", string(enrichedData))
	reportData, err := os.ReadFile(reportPath)
	assert.NoError(err)
	assert.Equal("existing report\n", string(reportData))
}

func TestProcessOverwriteAllowsExistingOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(outputPath, []byte("existing\n"), 0o644))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", outputPath,
		"--report-output", reportPath,
		"--overwrite",
	)

	assert.Equal(0, exitCode, stderr)
	enrichedEvent, err := jsonio.ReadObject(outputPath)
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
}

func TestProcessRejectsFilesystemAliasesAcrossOutputsWithOverwrite(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	eventOutput := filepath.Join(dir, "processed.json")
	reportOutput := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(eventOutput, []byte("existing\n"), 0o644))
	if err := os.Link(eventOutput, reportOutput); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", eventOutput,
		"--validate",
		"--report-output", reportOutput,
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both processed event and processing report")
	processed, err := jsonio.ReadObject(eventOutput)
	assert.NoError(err)
	assert.Equal("Alpha", processed["class_name"])
	reportBytes, err := os.ReadFile(reportOutput)
	assert.NoError(err)
	assert.Equal("existing\n", string(reportBytes))
}

func TestProcessWritesMultiplePrettyJSONOutputsSequentially(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", "-",
		"--validate",
		"--report-output", "-",
		"--pretty-json",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var event map[string]any
	assert.NoError(decoder.Decode(&event))
	assert.Equal("Alpha", event["class_name"])
	var report eventReport
	assert.NoError(decoder.Decode(&report))
	assert.Equal(eventPath, report.EventSource)
	assert.Equal("-", report.EventDestination)
	var extra any
	assert.ErrorIs(decoder.Decode(&extra), io.EOF)
	assert.Contains(stdout, "\n  \"class_uid\":")
}

func TestProcessWritesMultipleCompactJSONOutputsAsJSONLines(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", "-",
		"--validate",
		"--report-output", "-",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Len(lines, 2)
	var event map[string]any
	assert.NoError(json.Unmarshal([]byte(lines[0]), &event))
	assert.Equal("Alpha", event["class_name"])
	var report eventReport
	assert.NoError(json.Unmarshal([]byte(lines[1]), &report))
	assert.Equal(eventPath, report.EventSource)
}

func TestProcessSequentialJSONOmitsSkippedEvent(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	event := validCLIEvent()
	delete(event, "activity_id")
	writeJSONFile(assert, eventPath, event)

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", "-",
		"--validate",
		"--report-output", "-",
		"--skip-invalid-output",
	)

	assert.Equal(0, exitCode, stderr)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Len(lines, 1)
	var report eventReport
	assert.NoError(json.Unmarshal([]byte(lines[0]), &report))
	assert.Empty(report.EventDestination)
	assert.NotEmpty(report.Validation.Errors)
}

func TestProcessDirectoryWritesBothSummariesToStdout(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	eventPath := filepath.Join(eventsDir, "event.json")
	outputDir := filepath.Join(dir, "out")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", outputDir,
		"--summary-file", "-",
		"--summary-json-file", "-",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	assert.True(strings.HasPrefix(stdout, "ocsf-toolkit "))
	jsonStart := strings.LastIndex(stdout, "\n{") + 1
	assert.Positive(jsonStart)
	var summary summaryReport
	assert.NoError(json.Unmarshal([]byte(stdout[jsonStart:]), &summary))
	assert.Equal(1, *summary.EventFilesProcessed)
}

func TestProcessRejectsSingleEventSummaryFile(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", "-",
		"--summary-file", "-",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "summary options require --events-dir")
}

func TestProcessDirectoryDefaultSummaryBehavior(t *testing.T) {
	tests := []struct {
		name         string
		summaryArgs  []string
		summaryCount int
	}{
		{name: "default", summaryCount: 1},
		{name: "explicit stdout", summaryArgs: []string{"--summary-file", "-"}, summaryCount: 1},
		{name: "explicit stdout with quiet", summaryArgs: []string{"--summary-file", "-", "--quiet"}, summaryCount: 1},
		{name: "quiet", summaryArgs: []string{"--quiet"}, summaryCount: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventsDir := filepath.Join(dir, "events")
			writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
			args := []string{
				"--schema", schemaPath,
				"--events-dir", eventsDir,
				"--validate",
				"--output-dir", filepath.Join(dir, "output"),
			}
			args = append(args, test.summaryArgs...)

			exitCode, stdout, stderr := runCLI(args...)

			assert.Equal(0, exitCode, stderr)
			assert.Empty(stderr)
			assert.Equal(test.summaryCount, strings.Count(stdout, "Event files processed: 1"))
		})
	}
}
