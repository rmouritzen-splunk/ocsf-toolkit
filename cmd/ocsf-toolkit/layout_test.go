package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/jsonio"
)

func TestProcessDirectoryAcceptsRelativeInputAndOutputDirectories(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	writeJSONFile(assert, filepath.Join(dir, "events", "event.json"), validCLIEvent())

	t.Chdir(dir)

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

func TestProcessDirectoryRejectsMissingInputDirectoryDuringPreflight(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "missing-events")
	outputDir := filepath.Join(dir, "output")

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputDir,
		"--validate",
	)

	assert.Equal(1, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "events input directory "+strconv.Quote(eventsDir)+" does not exist")
	assert.NoDirExists(outputDir)
}

func TestProcessDirectoryRejectsRegularFileInputRootDuringPreflight(t *testing.T) {
	for _, extension := range []string{".json", ".txt"} {
		t.Run(extension, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventsPath := filepath.Join(dir, "events"+extension)
			outputDir := filepath.Join(dir, "output")
			writeJSONFile(assert, eventsPath, validCLIEvent())

			exitCode, stdout, stderr := runCLI(
				"--schema", schemaPath,
				"--events-dir", eventsPath,
				"--output-dir", outputDir,
				"--validate",
			)

			assert.Equal(1, exitCode)
			assert.Empty(stdout)
			assert.Contains(stderr, "events input path "+strconv.Quote(eventsPath)+" is not a directory")
			assert.NoDirExists(outputDir)
		})
	}
}

func TestProcessDirectoryRejectsSymlinkInputRootDuringPreflight(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	actualEventsDir := filepath.Join(dir, "actual-events")
	eventsLink := filepath.Join(dir, "events-link")
	outputDir := filepath.Join(dir, "output")
	assert.NoError(os.Mkdir(actualEventsDir, 0o750))
	makeTestSymlink(t, actualEventsDir, eventsLink)

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsLink,
		"--output-dir", outputDir,
		"--validate",
	)

	assert.Equal(1, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "events input path "+strconv.Quote(eventsLink)+" is a symbolic link")
	assert.NoDirExists(outputDir)
}

func TestProcessDirectoryAcceptsEmptyInputDirectory(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(dir, "output")
	assert.NoError(os.Mkdir(eventsDir, 0o750))

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputDir,
		"--validate",
		"--summary", "-",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Contains(stdout, "Event files processed: 0")
	assert.Empty(stderr)
	assert.NoDirExists(outputDir)
}

func TestProcessDirectoryIgnoresSymlinksWithinInputTree(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outsideDir := filepath.Join(dir, "outside")
	outsideEvent := filepath.Join(outsideDir, "event.json")
	outputDir := filepath.Join(dir, "output")
	assert.NoError(os.Mkdir(eventsDir, 0o750))
	writeJSONFile(assert, outsideEvent, validCLIEvent())
	makeTestSymlink(t, outsideEvent, filepath.Join(eventsDir, "linked-event.json"))
	makeTestSymlink(t, outsideDir, filepath.Join(eventsDir, "linked-directory"))

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputDir,
		"--validate",
		"--summary", "-",
	)

	assert.Equal(0, exitCode, stderr)
	assert.Contains(stdout, "Event files processed: 0")
	assert.Empty(stderr)
	assert.NoDirExists(outputDir)
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

func TestProcessDirectoryRejectsCaseAliasedOverlap(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "Events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	eventsDirAlias := caseAliasPath(t, eventsDir)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", filepath.Join(eventsDirAlias, "out"),
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
	assert.NoError(os.Mkdir(actualOutput, 0o750))
	makeTestSymlink(t, actualOutput, outputAlias)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--output-dir", outputAlias,
		"--validate",
	)

	assert.Equal(0, exitCode, stderr)
	assert.FileExists(filepath.Join(actualOutput, "reports", "event.report.json"))
}

func TestNewFilesystemPathResolvesExistingPrefixAndRetainsMissingSuffix(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual")
	alias := filepath.Join(dir, "alias")
	assert.NoError(os.Mkdir(actual, 0o750))
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
		{
			name:  "relative traversal",
			input: inputEvent{path: filepath.Join("..", "outside", "event.json")},
			want:  "event.json",
		},
		{
			name:  "directory traversal",
			input: inputEvent{path: "event.json", rel: filepath.Join("..", "event.json")},
			want:  "event.json",
		},
		{
			name:  "internal parent navigation",
			input: inputEvent{path: filepath.Join("nested", "..", "other", "event.json")},
			want:  filepath.Join("other", "event.json"),
		},
		{
			name:  "safe relative path",
			input: inputEvent{path: filepath.Join("nested", "event.json")},
			want:  filepath.Join("nested", "event.json"),
		},
		{name: "absolute path", input: inputEvent{path: filepath.Join(t.TempDir(), "event.json")}, want: "event.json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, eventOutputRelativePath(test.input))
		})
	}
}

func TestSafeOutputRelativePathRejectsNonLocalPath(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "nested", "event.json")

	require.Equal(t, "event.json", safeOutputRelativePath(absolute))
}

func TestProcessDirectoryRequiresEmptyOutputWithoutOverwrite(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(outputDir, 0o750))
	existing := filepath.Join(outputDir, "existing.txt")
	assert.NoError(os.WriteFile(existing, []byte("keep"), 0o600))

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
	assert.NoError(os.MkdirAll(outputDir, 0o750))
	existing := filepath.Join(outputDir, "existing.txt")
	assert.NoError(os.WriteFile(existing, []byte("keep"), 0o600))
	existingEvent := filepath.Join(outputDir, "events", "event.json")
	assert.NoError(os.MkdirAll(filepath.Dir(existingEvent), 0o750))
	assert.NoError(os.WriteFile(existingEvent, []byte("old"), 0o600))

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
	assert.NoError(os.MkdirAll(filepath.Join(outputDir, "events"), 0o750))
	assert.NoError(os.MkdirAll(outside, 0o750))
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

func TestProcessRejectsCaseAliasedOutputOverwritingInput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "Event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	originalEvent, err := os.ReadFile(eventPath)
	assert.NoError(err)
	eventAlias := caseAliasPath(t, eventPath)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", eventAlias,
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
	actualEvent, err := os.ReadFile(eventPath)
	assert.NoError(err)
	assert.Equal(originalEvent, actualEvent)
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

func TestProcessRejectsOverlappingExplicitOutputPathsBeforeWriting(t *testing.T) {
	for _, test := range []struct {
		name        string
		eventOutput func(string) string
		report      func(string) string
	}{
		{
			name:        "processed event is ancestor",
			eventOutput: func(root string) string { return root },
			report:      func(root string) string { return filepath.Join(root, "report.json") },
		},
		{
			name:        "processing report is ancestor",
			eventOutput: func(root string) string { return filepath.Join(root, "event.json") },
			report:      func(root string) string { return root },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventPath := filepath.Join(dir, "input.json")
			outputRoot := filepath.Join(dir, "selected-output")
			writeJSONFile(assert, eventPath, validCLIEvent())

			exitCode, _, stderr := runCLI(
				"--schema", schemaPath,
				"--event", eventPath,
				"--enrich",
				"--event-output", test.eventOutput(outputRoot),
				"--report-output", test.report(outputRoot),
			)

			assert.Equal(1, exitCode)
			assert.Contains(stderr, "overlaps")
			assert.NoFileExists(test.eventOutput(outputRoot))
			assert.NoFileExists(test.report(outputRoot))
		})
	}
}

func TestProcessRejectsCaseAliasedOutputOverwritingSchema(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	originalSchema, err := os.ReadFile(schemaPath)
	assert.NoError(err)
	schemaAlias := caseAliasPath(t, schemaPath)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", schemaAlias,
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
		"--summary", filepath.Join(outputDir, "events", "summary.json"),
		"--summary-format", "json",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "conflicts with a reserved output namespace")
}

func TestProcessRejectsSummaryContainingReservedNamespace(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	destination := filepath.Join(dir, "destination")
	outputDir := filepath.Join(destination, "output")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
		"--summary", destination,
		"--summary-format", "json",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "conflicts with a reserved output namespace")
	assert.NoDirExists(outputDir)
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
		"--summary", filepath.Join(eventsDir, "summary.txt"),
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "conflicts with the input event directory")
}

func TestPathsOverlapUsesResolvedSymlinks(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual")
	alias := filepath.Join(dir, "alias")
	require.NoError(t, os.Mkdir(actual, 0o750))
	makeTestSymlink(t, actual, alias)

	left, err := newFilesystemPath(actual)
	require.NoError(t, err)
	right, err := newFilesystemPath(filepath.Join(alias, "child"))
	require.NoError(t, err)
	require.True(t, pathsOverlap(left, right))
}
