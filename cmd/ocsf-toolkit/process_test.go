package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
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

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
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
