package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	assert.NoError(os.WriteFile(outputPath, []byte("existing\n"), 0o600))

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

func TestProcessWritesWithoutHardLinkSupport(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	originalCreateOutputHardLink := createOutputHardLink
	createOutputHardLink = func(_, _ string) error { return errors.New("hard links unsupported") }
	t.Cleanup(func() { createOutputHardLink = originalCreateOutputHardLink })

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", outputPath,
		"--report-output", reportPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.FileExists(outputPath)
	assert.FileExists(reportPath)
}

func TestProcessDoesNotOverwriteWithoutHardLinkSupport(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(outputPath, []byte("existing\n"), 0o600))
	originalCreateOutputHardLink := createOutputHardLink
	createOutputHardLink = func(_, _ string) error { return errors.New("hard links unsupported") }
	t.Cleanup(func() { createOutputHardLink = originalCreateOutputHardLink })

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", outputPath,
		"--report-output", reportPath,
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "already exists")
	actual, err := os.ReadFile(outputPath)
	assert.NoError(err)
	assert.Equal("existing\n", string(actual))
}

func TestProcessStopsAfterFirstOutputWriteError(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "out")
	enrichedPath := filepath.Join(outputDir, "events", "event.json")
	reportPath := filepath.Join(outputDir, "reports", "event.report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.MkdirAll(filepath.Dir(enrichedPath), 0o750))
	assert.NoError(os.MkdirAll(filepath.Dir(reportPath), 0o750))
	assert.NoError(os.WriteFile(enrichedPath, []byte("existing enriched\n"), 0o600))
	assert.NoError(os.WriteFile(reportPath, []byte("existing report\n"), 0o600))

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
	assert.NoError(os.WriteFile(outputPath, []byte("existing\n"), 0o600))

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

func TestWriteTextOutputFilePreservesExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}

	assert := require.New(t)
	path := filepath.Join(t.TempDir(), "output.json")
	assert.NoError(os.WriteFile(path, []byte("existing\n"), 0o600))
	assert.NoError(os.Chmod(path, 0o400))

	assert.NoError(writeTextOutputFile(path, "replacement\n", true))
	info, err := os.Stat(path)
	assert.NoError(err)
	assert.Equal(os.FileMode(0o400), info.Mode().Perm())
}

func TestWriteTextOutputFileSupportsLongDestinationBasename(t *testing.T) {
	assert := require.New(t)
	path := filepath.Join(t.TempDir(), strings.Repeat("a", 250))
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Skipf("filesystem does not support the long destination basename: %v", err)
	}
	assert.NoError(os.Remove(path))

	assert.NoError(writeTextOutputFile(path, "output\n", false))
	data, err := os.ReadFile(path)
	assert.NoError(err)
	assert.Equal("output\n", string(data))
}

func TestProcessOverwriteReplacesExistingReport(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(reportPath, []byte(`{"unrelated": true}`), 0o600))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", reportPath,
		"--overwrite",
	)

	assert.Equal(0, exitCode, stderr)
	report := readEventReport(assert, reportPath)
	assert.Equal(eventPath, report.EventSource)
}

func TestDestinationWriterOverwriteReplacesEarlierOutput(t *testing.T) {
	assert := require.New(t)
	path := filepath.Join(t.TempDir(), "event.json")
	writer := newDestinationWriter(io.Discard, writeOptions{overwrite: true})

	assert.NoError(writer.writeJSON(path, map[string]string{"source": "first"}))
	err := writer.writeJSON(path, map[string]string{"source": "second"})

	assert.NoError(err)
	var output map[string]string
	data, readErr := os.ReadFile(path)
	assert.NoError(readErr)
	assert.NoError(json.Unmarshal(data, &output))
	assert.Equal(map[string]string{"source": "second"}, output)
}

func TestDestinationWriterOverwriteReplacesFilesystemAlias(t *testing.T) {
	assert := require.New(t)
	path := filepath.Join(t.TempDir(), "event.json")
	writer := newDestinationWriter(io.Discard, writeOptions{overwrite: true})

	assert.NoError(writer.writeJSON(path, map[string]string{"source": "first"}))
	alias := caseAliasPath(t, path)
	err := writer.writeJSON(alias, map[string]string{"source": "second"})

	assert.NoError(err)
	var output map[string]string
	data, readErr := os.ReadFile(path)
	assert.NoError(readErr)
	assert.NoError(json.Unmarshal(data, &output))
	assert.Equal(map[string]string{"source": "second"}, output)
}

func TestProcessRejectsFilesystemAliasesAcrossOutputsWithOverwrite(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	eventOutput := filepath.Join(dir, "processed.json")
	reportOutput := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(eventOutput, []byte("existing\n"), 0o600))
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
	eventBytes, err := os.ReadFile(eventOutput)
	assert.NoError(err)
	assert.Equal("existing\n", string(eventBytes))
	reportBytes, err := os.ReadFile(reportOutput)
	assert.NoError(err)
	assert.Equal("existing\n", string(reportBytes))
}

func TestEngineeringInvariantProcessRejectsCaseAliasedOutputs(t *testing.T) {
	// Engineering invariant test: case-aliased event and report outputs must be rejected during best-effort
	// preflight or, when the alias becomes observable only after creation, before the report can replace the event.
	for _, overwrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("overwrite=%t", overwrite), func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventPath := filepath.Join(dir, "event.json")
			eventOutput := filepath.Join(dir, "Processed.json")
			writeJSONFile(assert, eventPath, validCLIEvent())
			assert.NoError(os.WriteFile(eventOutput, []byte("case-sensitivity probe\n"), 0o600))
			reportOutput := caseAliasPath(t, eventOutput)
			assert.NoError(os.Remove(eventOutput))

			args := []string{
				"--schema", schemaPath,
				"--event", eventPath,
				"--enrich",
				"--event-output", eventOutput,
				"--validate",
				"--report-output", reportOutput,
			}
			if overwrite {
				args = append(args, "--overwrite")
			}

			exitCode, _, stderr := runCLI(args...)

			assert.Equal(1, exitCode)
			if strings.Contains(stderr, "selected for processing report overlaps processed event path") {
				assert.NoFileExists(eventOutput)
				return
			}
			assert.Contains(stderr, "processing report was not written")
			assert.Contains(stderr, fmt.Sprintf("report output %q", reportOutput))
			assert.Contains(stderr, "names the same file as event output")
			assert.Contains(stderr, fmt.Sprintf("event output %q", eventOutput))
			assert.Contains(stderr, "which was already written")
			event, err := jsonio.ReadObject(eventOutput)
			assert.NoError(err)
			assert.Equal("Alpha", event["class_name"])
		})
	}
}

func TestEngineeringInvariantProcessAllowsCaseVariantOutputsWhenDistinct(t *testing.T) {
	// Engineering invariant test: case-variant event and report outputs remain valid when the filesystem
	// identifies them as distinct files.
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	eventOutput := filepath.Join(dir, "Processed.json")
	reportOutput := filepath.Join(dir, "processed.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	assert.NoError(os.WriteFile(eventOutput, []byte("case-sensitivity probe\n"), 0o600))
	if _, err := os.Stat(reportOutput); !errors.Is(err, os.ErrNotExist) {
		t.Skip("filesystem is case-insensitive")
	}
	assert.NoError(os.Remove(eventOutput))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", eventOutput,
		"--validate",
		"--report-output", reportOutput,
	)

	assert.Equal(0, exitCode, stderr)
	event, err := jsonio.ReadObject(eventOutput)
	assert.NoError(err)
	assert.Equal("Alpha", event["class_name"])
	report := readEventReport(assert, reportOutput)
	assert.Equal(eventPath, report.EventSource)
}

func TestInvariantProcessAllowsOnlyOneStdoutOutput(t *testing.T) {
	// Invariant test: one invocation may select stdout for at most one output option.
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

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "only one output option may use stdout")
}

func TestInvariantProcessAllowsEventOutputOnStdout(t *testing.T) {
	// Invariant test: an event output may use stdout when every other selected output uses a file.
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", "-",
		"--report-output", reportPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	var event map[string]any
	assert.NoError(json.Unmarshal([]byte(stdout), &event))
	assert.Equal("Alpha", event["class_name"])
	report := readEventReport(assert, reportPath)
	assert.Equal("-", report.EventDestination)
}

func TestInvariantProcessAllowsJSONSummaryOnStdout(t *testing.T) {
	// Invariant test: a JSON directory summary may use stdout when no other output option does.
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
		"--summary", "-",
		"--summary-format", "json",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
	var summary summaryReport
	assert.NoError(json.Unmarshal([]byte(stdout), &summary))
	assert.Equal(1, summary.EventFilesProcessed)
}

func TestInvariantProcessRejectsMultipleOutputsOnStdout(t *testing.T) {
	// Invariant test: stdout contains at most one selected output representation.
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
		"--summary", "-",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "only one output option may use stdout")
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
		"--summary", "-",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "summary options require --events-dir")
}

func TestProcessDirectorySummaryBehavior(t *testing.T) {
	tests := []struct {
		name         string
		summaryArgs  []string
		summaryCount int
	}{
		{name: "default", summaryCount: 0},
		{name: "explicit stdout", summaryArgs: []string{"--summary", "-"}, summaryCount: 1},
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
