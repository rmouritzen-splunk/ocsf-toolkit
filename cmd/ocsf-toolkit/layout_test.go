package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonio"
)

func TestProcessDirectoryAcceptsRelativeInputAndOutputDirectories(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	writeJSONFile(assert, filepath.Join(dir, "events", "event.json"), validCLIEvent())

	previousWD, err := os.Getwd()
	assert.NoError(err)
	assert.NoError(os.Chdir(dir))
	t.Cleanup(func() { assert.NoError(os.Chdir(previousWD)) })

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", "events",
		"--output-dir", "out",
		"--enrich",
	)

	assert.Equal(0, exitCode, stderr)
	event, err := jsonio.ReadObject(filepath.Join(dir, "out", "events", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", event["class_name"])
}

func TestProcessDirectoryRequiresOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "directory processing requires --output-dir DIR")
}

func TestProcessDirectoryRejectsOverlappingInputAndOutputTrees(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", filepath.Join(eventsDir, "out"),
		"--validate",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "input and output directory trees must not overlap")
}

func TestProcessDirectoryRejectsSymlinkResolvedOverlap(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	alias := filepath.Join(dir, "events-alias")
	makeTestSymlink(t, eventsDir, alias)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", filepath.Join(alias, "out"),
		"--validate",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "input and output directory trees must not overlap")
}

func TestProcessDirectoryAllowsSymlinkAsSelectedOutputRoot(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	actualOutput := filepath.Join(dir, "actual-output")
	outputAlias := filepath.Join(dir, "output-alias")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	assert.NoError(os.Mkdir(actualOutput, 0o755))
	makeTestSymlink(t, actualOutput, outputAlias)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputAlias,
		"--validate",
	)

	assert.Equal(0, exitCode, stderr)
	assert.FileExists(filepath.Join(actualOutput, "reports", "event.json"))
}

func TestNewFilesystemPathResolvesExistingPrefixAndRetainsMissingSuffix(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual")
	alias := filepath.Join(dir, "alias")
	assert.NoError(os.Mkdir(actual, 0o755))
	makeTestSymlink(t, actual, alias)

	path, err := newFilesystemPath(filepath.Join(alias, "missing", "event.json"))

	assert.NoError(err)
	assert.Equal(filepath.Join(alias, "missing", "event.json"), path.absolute)
	actualResolved, err := filepath.EvalSymlinks(actual)
	assert.NoError(err)
	assert.Equal(filepath.Join(actualResolved, "missing", "event.json"), path.resolved)
}

func TestNewFilesystemPathRejectsDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	makeTestSymlink(t, filepath.Join(dir, "missing-target"), link)

	_, err := newFilesystemPath(filepath.Join(link, "event.json"))

	require.ErrorContains(t, err, "resolve symlinks")
}

func TestEventOutputRelativePathDoesNotPreserveTraversal(t *testing.T) {
	tests := []struct {
		name  string
		input inputEvent
		want  string
	}{
		{name: "relative traversal", input: inputEvent{path: filepath.Join("..", "outside", "event.json")}, want: "event.json"},
		{name: "directory traversal", input: inputEvent{path: "event.json", rel: filepath.Join("..", "event.json")}, want: "event.json"},
		{name: "safe relative path", input: inputEvent{path: filepath.Join("nested", "event.json")}, want: filepath.Join("nested", "event.json")},
		{name: "absolute path", input: inputEvent{path: filepath.Join(string(filepath.Separator), "tmp", "event.json")}, want: "event.json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, eventOutputRelativePath(test.input))
		})
	}
}

func TestProcessDirectoryRequiresEmptyOutputWithoutOverwrite(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(outputDir, 0o755))
	existing := filepath.Join(outputDir, "existing.txt")
	assert.NoError(os.WriteFile(existing, []byte("keep"), 0o644))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputDir,
		"--enrich",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is not empty")
	assert.FileExists(existing)
	assert.NoDirExists(filepath.Join(outputDir, "events"))
}

func TestProcessDirectoryOverwriteAllowsNonemptyOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(outputDir, 0o755))
	existing := filepath.Join(outputDir, "existing.txt")
	assert.NoError(os.WriteFile(existing, []byte("keep"), 0o644))
	existingEvent := filepath.Join(outputDir, "events", "event.json")
	assert.NoError(os.MkdirAll(filepath.Dir(existingEvent), 0o755))
	assert.NoError(os.WriteFile(existingEvent, []byte("old"), 0o644))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputDir,
		"--enrich",
		"--overwrite",
	)

	assert.Equal(0, exitCode, stderr)
	assert.FileExists(existing)
	event, err := jsonio.ReadObject(existingEvent)
	assert.NoError(err)
	assert.Equal("Alpha", event["class_name"])
}

func TestProcessDirectoryRejectsSymlinkInsideOutputNamespace(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	outside := filepath.Join(dir, "outside")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(filepath.Join(outputDir, "events"), 0o755))
	assert.NoError(os.MkdirAll(outside, 0o755))
	makeTestSymlink(t, outside, filepath.Join(outputDir, "events", "nested"))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputDir,
		"--enrich",
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "contains symbolic link")
}

func TestProcessRejectsExplicitOutputAliasingInput(t *testing.T) {
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
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
}

func TestProcessRejectsExplicitOutputAliasingSchema(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	originalSchema, err := os.ReadFile(schemaPath)
	assert.NoError(err)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", schemaPath,
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
	actualSchema, err := os.ReadFile(schemaPath)
	assert.NoError(err)
	assert.Equal(originalSchema, actualSchema)
}

func TestProcessDirectoryRejectsSchemaInsideOutputNamespace(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	schemaPath := writeTestSchema(assert, filepath.Join(outputDir, reportsOutputDirectory))
	originalSchema, err := os.ReadFile(schemaPath)
	assert.NoError(err)
	writeJSONFile(assert, filepath.Join(eventsDir, "schema.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputDir,
		"--validate",
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "schema path")
	assert.Contains(stderr, "reserved output namespace")
	actualSchema, err := os.ReadFile(schemaPath)
	assert.NoError(err)
	assert.Equal(originalSchema, actualSchema)
}

func TestProcessRejectsSummaryInsideReservedNamespace(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	eventPath := filepath.Join(eventsDir, "event.json")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
		"--summary-json-file", filepath.Join(outputDir, "events", "summary.json"),
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "conflicts with a reserved output namespace")
}

func TestProcessRejectsSummaryInsideInputDirectory(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", filepath.Join(dir, "output"),
		"--summary-file", filepath.Join(eventsDir, "summary.txt"),
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "conflicts with the input event directory")
}

func TestProcessRejectsSameFileForBothSummaries(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	summaryPath := filepath.Join(dir, "summary")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", filepath.Join(dir, "output"),
		"--summary-file", summaryPath,
		"--summary-json-file", summaryPath,
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both human-readable summary and JSON summary")
}

func TestPathsOverlapUsesResolvedSymlinks(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual")
	alias := filepath.Join(dir, "alias")
	require.NoError(t, os.Mkdir(actual, 0o755))
	makeTestSymlink(t, actual, alias)

	left, err := newFilesystemPath(actual)
	require.NoError(t, err)
	right, err := newFilesystemPath(filepath.Join(alias, "child"))
	require.NoError(t, err)
	require.True(t, pathsOverlap(left, right))
}
