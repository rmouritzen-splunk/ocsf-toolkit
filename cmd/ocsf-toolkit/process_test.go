package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

func TestProcessSingleEventEnrichesAndWritesReport(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "processed.json")
	reportPath := filepath.Join(dir, "event-report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", reportPath,
		"--enrich",
		"--event-output", outputPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)

	event, err := jsonio.ReadObject(outputPath)
	assert.NoError(err)
	assert.Equal("Alpha", event["class_name"])
	assert.Equal("Do", event["activity_name"])
	original, err := jsonio.ReadObject(eventPath)
	assert.NoError(err)
	assert.NotContains(original, "class_name")

	report := readEventReport(assert, reportPath)
	assert.Equal(eventPath, report.EventSource)
	assert.Equal(outputPath, report.EventDestination)
	assert.Empty(report.Validation.Errors)
	assert.Empty(report.Validation.Warnings)
	assert.NotNil(report.Enrichment)
	assert.Equal(2, report.Enrichment.EnumSiblingsAdded)
	assert.Zero(report.Enrichment.ObservablesAdded)
}

func TestProcessSingleEventEnrichmentWritesIssuesToReport(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	eventOutput := filepath.Join(dir, "processed.json")
	reportOutput := filepath.Join(dir, "report.json")
	event := validCLIEvent()
	event["activity_id"] = json.Number("1234")
	writeJSONFile(assert, eventPath, event)

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", eventOutput,
		"--report-output", reportOutput,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)
	report := readEventReport(assert, reportOutput)
	assert.NotNil(report.Enrichment)
	assert.Equal(1, report.Enrichment.EnumSiblingsAdded)
	assert.Len(report.Issues, 1)
	assert.Equal("enrichment", report.Issues[0].Phase)
	assert.Equal("enrichment_enum_sibling_not_added", report.Issues[0].Code)
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
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	report := readEventReport(assert, filepath.Join(outputDir, "reports", "event.json"))
	assert.Equal(eventPath, report.EventSource)
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
	assert.Contains(stderr, "single event reporting requires exactly one of --output-dir DIR or --report-output FILE")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE")
}

func TestProcessStdinHasNoDefaultOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventBytes, err := json.Marshal(validCLIEvent())
	assert.NoError(err)

	exitCode, stdout, stderr := runCLIWithInput(
		string(eventBytes),
		"--schema", schemaPath,
		"--event", "-",
		"--validate",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "single event reporting requires exactly one of --output-dir DIR or --report-output FILE")
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
		"--report-output", "-",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)

	var report eventReport
	assert.NoError(json.Unmarshal([]byte(stdout), &report))
	assert.Equal(eventPath, report.EventSource)
	assert.Empty(report.EventDestination)
	assert.Empty(report.Validation.Errors)
	assert.Empty(report.Validation.Warnings)
}

func TestDirectoryEventAndReportNamespacesCannotCollide(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	writeJSONFile(assert, filepath.Join(eventsDir, "event-validation.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--validate",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.FileExists(filepath.Join(outputDir, "events", "event.json"))
	assert.FileExists(filepath.Join(outputDir, "events", "event-validation.json"))
	assert.FileExists(filepath.Join(outputDir, "reports", "event.json"))
	assert.FileExists(filepath.Join(outputDir, "reports", "event-validation.json"))
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
		"--report-output", validationPath,
		"--fail-on-validation-errors",
	)

	assert.Equal(1, exitCode, stderr)
	report := readEventReport(assert, validationPath)
	assert.NotEmpty(report.Validation.Errors)
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
	event["class_name"] = "Alpha"
	writeJSONFile(assert, eventPath, event)

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--unenrich",
		"--event-output", enrichPath,
		"--validate",
		"--report-output", validationPath,
		"--skip-invalid-output",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)
	assert.NoFileExists(enrichPath)
	report := readEventReport(assert, validationPath)
	assert.NotEmpty(report.Validation.Errors)
	assert.Empty(report.EventDestination)
	assert.Nil(report.EnrichmentRemoval)
}

func TestProcessRejectsReportOutputOverwritingEventFile(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", eventPath,
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
}

func TestProcessEventOutputWithoutExtensionIsOrdinaryFilePath(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "in-place")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", outputPath,
		"--report-output", reportPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stderr)
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
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--no-enum-siblings",
		"--event-output", outputPath,
		"--report-output", reportPath,
	)

	assert.Equal(0, exitCode, stderr)
	enrichedEvent, err := jsonio.ReadObject(outputPath)
	assert.NoError(err)
	assert.NotContains(enrichedEvent, "class_name")
	assert.NotContains(enrichedEvent, "activity_name")
}

func TestProcessRejectsOutputDirWithEventOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--output-dir", filepath.Join(dir, "output"),
		"--event-output", filepath.Join(dir, "enriched.json"),
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "--output-dir cannot be used with operation-specific output options")
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
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])

	report := readEventReport(assert, filepath.Join(outputDir, "reports", "event.json"))
	assert.Empty(report.Validation.Errors)
	assert.FileExists(filepath.Join(outputDir, "existing.txt"))
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
	assert.NoError(os.MkdirAll(filepath.Join(outputDir, "events", "a.json"), 0o755))

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "a.json: event write error")
	assert.Contains(stderr, "Event files processed before error: 1")
	assert.NoFileExists(filepath.Join(outputDir, "events", "b.json"))
}

func TestProcessDirectoryReportsFilesProcessedBeforeWalkError(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	eventPath := filepath.Join(eventsDir, "event.json")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, eventPath, validCLIEvent())
	entries, err := os.ReadDir(eventsDir)
	assert.NoError(err)
	assert.Len(entries, 1)

	originalWalk := walkEventDirectory
	walkEventDirectory = func(_ string, walk fs.WalkDirFunc) error {
		assert.NoError(walk(eventPath, entries[0], nil))
		return fs.ErrPermission
	}
	t.Cleanup(func() { walkEventDirectory = originalWalk })

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", outputDir,
		"--quiet",
	)

	assert.Equal(1, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "failed to walk events directory")
	assert.Contains(stderr, "permission denied")
	assert.Contains(stderr, "Event files processed before error: 1")
	assert.FileExists(filepath.Join(outputDir, "reports", "event.json"))
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
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	report := readEventReport(assert, filepath.Join(outputDir, "reports", "event.json"))
	assert.Equal("-", report.EventSource)
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
		"--report-output", issuesPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)

	processed, err := jsonio.ReadObject(outputPath)
	assert.NoError(err)
	assert.NotContains(processed, "class_name")
	assert.Equal("source-specific", processed["activity_name"])
	assert.Len(processed["observables"], 1)

	report := readEventReport(assert, issuesPath)
	assert.Equal(eventPath, report.EventSource)
	assert.Equal(outputPath, report.EventDestination)
	assert.Equal(1, report.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Equal(1, report.EnrichmentRemoval.EnumSiblingsRetained)
	assert.Equal(1, report.EnrichmentRemoval.ObservablesRemoved)
	assert.Equal(1, report.EnrichmentRemoval.ObservablesRetained)
	assert.Len(report.Issues, 1)
	assert.Equal("observable_value_not_found", report.Issues[0].Code)
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
	processed, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", processed["class_name"])
	assert.Equal("Do", processed["activity_name"])
	assert.NotContains(processed, "observables")
	assert.FileExists(filepath.Join(outputDir, "reports", "event.json"))
}
