package main

import (
	"os"
	"path/filepath"
	"strconv"
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
