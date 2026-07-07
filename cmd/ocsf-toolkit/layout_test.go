package main

import (
	"errors"
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
	eventPath := filepath.Join(dir, "events", "error_event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	t.Chdir(dir)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", "events",
		"--output-dir", "out",
		"--enrich",
		"--validate",
	)

	assert.Equal(0, exitCode, stderr)
	assert.FileExists(filepath.Join(dir, "out", "error_event.json"))
	assert.FileExists(filepath.Join(dir, "out", "error_event-validation.json"))
}

func TestValidatePathBeneathOutputRootNormalizesRelativeAndAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	absoluteRoot := filepath.Join(dir, "out")
	absoluteOutput := filepath.Join(absoluteRoot, "nested", "event.json")

	testCases := []struct {
		name    string
		root    string
		path    string
		wantErr bool
	}{
		{
			name: "relative root and path",
			root: "out",
			path: filepath.Join("out", "nested", "event.json"),
		},
		{
			name: "absolute root and relative path",
			root: absoluteRoot,
			path: filepath.Join("out", "nested", "event.json"),
		},
		{
			name: "relative root and absolute path",
			root: "out",
			path: absoluteOutput,
		},
		{
			name:    "relative path escapes root",
			root:    "out",
			path:    filepath.Join("out", "..", "event.json"),
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root, err := newFilesystemPath(testCase.root)
			require.NoError(t, err)
			path, err := newFilesystemPath(testCase.path)
			require.NoError(t, err)
			err = validatePathBeneathOutputRoot(root, path)
			if testCase.wantErr {
				require.ErrorContains(t, err, "escapes output root")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestProcessDirectoryValidationRequiresOutput(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "directory validation requires --output-dir DIR")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE")
}

func TestProcessDirectoryRejectsOutputDirectoryInsideInputTree(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(eventsDir, "processed")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "input and output directory trees must not overlap")
}

func TestProcessDirectoryRejectsSymlinkResolvedDirectoryOverlap(t *testing.T) {
	testCases := []struct {
		name  string
		paths func(*testing.T, *require.Assertions, string) (string, string)
	}{
		{
			name: "nonexistent output descendant reached through symlink",
			paths: func(t *testing.T, assert *require.Assertions, dir string) (string, string) {
				actualRoot := filepath.Join(dir, "actual")
				eventsDir := filepath.Join(actualRoot, "events")
				writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
				alias := filepath.Join(dir, "alias")
				makeTestSymlink(t, actualRoot, alias)
				return eventsDir, filepath.Join(alias, "events", "new", "output")
			},
		},
		{
			name: "input descendant reached through output symlink",
			paths: func(t *testing.T, assert *require.Assertions, dir string) (string, string) {
				actualOutput := filepath.Join(dir, "actual-output")
				eventsDir := filepath.Join(actualOutput, "events")
				writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
				outputAlias := filepath.Join(dir, "output-alias")
				makeTestSymlink(t, actualOutput, outputAlias)
				return eventsDir, outputAlias
			},
		},
		{
			name: "output directory is direct ancestor of input",
			paths: func(_ *testing.T, assert *require.Assertions, dir string) (string, string) {
				outputDir := filepath.Join(dir, "output")
				eventsDir := filepath.Join(outputDir, "events")
				writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
				return eventsDir, outputDir
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventsDir, outputDir := testCase.paths(t, assert, dir)

			exitCode, _, stderr := runCLI(
				"--schema", schemaPath,
				"--events-dir", eventsDir,
				"--enrich",
				"--output-dir", outputDir,
			)

			assert.Equal(1, exitCode)
			assert.Contains(stderr, "input and output directory trees must not overlap")
			assert.NoDirExists(filepath.Join(outputDir, "new"))
		})
	}
}

func TestProcessDirectoryAllowsSymlinkAsSelectedOutputRoot(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
	actualOutput := filepath.Join(dir, "actual-output")
	assert.NoError(os.Mkdir(actualOutput, 0o755))
	outputAlias := filepath.Join(dir, "output-alias")
	makeTestSymlink(t, actualOutput, outputAlias)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputAlias,
	)

	assert.Equal(0, exitCode, stderr)
	assert.FileExists(filepath.Join(actualOutput, "event.json"))
}

func TestNewFilesystemPathResolvesExistingPrefixAndRetainsMissingSuffix(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual")
	assert.NoError(os.Mkdir(actual, 0o755))
	alias := filepath.Join(dir, "alias")
	makeTestSymlink(t, actual, alias)
	display := filepath.Join(alias, "missing", "nested")

	path, err := newFilesystemPath(display)

	assert.NoError(err)
	resolvedActual, err := filepath.EvalSymlinks(actual)
	assert.NoError(err)
	assert.Equal(display, path.display)
	assert.Equal(filepath.Clean(display), path.absolute)
	assert.Equal(filepath.Join(resolvedActual, "missing", "nested"), path.resolved)
}

func TestNewFilesystemPathRejectsDanglingSymlink(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	dangling := filepath.Join(dir, "dangling")
	makeTestSymlink(t, filepath.Join(dir, "missing-target"), dangling)

	_, err := newFilesystemPath(filepath.Join(dangling, "output"))

	assert.ErrorContains(err, "resolve symlinks")
}

func TestProcessSingleEventDirectoryOutputPreservesRelativeEventPath(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join("events", "windows", "event.json")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, filepath.Join(dir, eventPath), validCLIEvent())

	previousWd, err := os.Getwd()
	assert.NoError(err)
	assert.NoError(os.Chdir(dir))
	defer func() {
		assert.NoError(os.Chdir(previousWd))
	}()

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--validate",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	enrichedEvent, err := jsonio.ReadObject(filepath.Join(outputDir, eventPath))
	assert.NoError(err)
	assert.Equal("Alpha", enrichedEvent["class_name"])
	validation := readValidationOutput(assert, filepath.Join(outputDir, "events", "windows", "event-validation.json"))
	assert.Equal(eventPath, validation.InputPath)
	assert.Contains(stderr, "Processed event written: "+filepath.Join(outputDir, eventPath))
	assert.Contains(stderr, "Validation result written: "+filepath.Join(outputDir, "events", "windows", "event-validation.json"))
}

func TestProcessSingleEventDirectoryOutputDoesNotEscapeOutputDir(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join("..", "event.json")
	outputDir := filepath.Join(dir, "out")
	workDir := filepath.Join(dir, "out-work")
	writeJSONFile(assert, filepath.Join(dir, "event.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(workDir, 0o755))

	previousWd, err := os.Getwd()
	assert.NoError(err)
	assert.NoError(os.Chdir(workDir))
	defer func() {
		assert.NoError(os.Chdir(previousWd))
	}()

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
	assert.FileExists(filepath.Join(outputDir, "event-validation.json"))
	assert.NoFileExists(filepath.Join(dir, "event-validation.json"))
}

func TestEventOutputRelativePathDoesNotPreserveTraversal(t *testing.T) {
	assert := require.New(t)

	testCases := []struct {
		name  string
		input inputEvent
		want  string
	}{
		{
			name:  "relative input starts with traversal",
			input: inputEvent{path: filepath.Join("..", "event.json")},
			want:  "event.json",
		},
		{
			name:  "relative input contains traversal",
			input: inputEvent{path: filepath.Join("events", "..", "event.json")},
			want:  "event.json",
		},
		{
			name:  "directory relative path contains traversal",
			input: inputEvent{path: filepath.Join("events", "event.json"), rel: filepath.Join("nested", "..", "event.json")},
			want:  "event.json",
		},
		{
			name:  "safe relative path",
			input: inputEvent{path: filepath.Join("events", "windows", "event.json")},
			want:  filepath.Join("events", "windows", "event.json"),
		},
		{
			name:  "absolute path uses basename",
			input: inputEvent{path: filepath.Join(string(filepath.Separator), "tmp", "event.json")},
			want:  "event.json",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(testCase.want, eventOutputRelativePath(testCase.input))
		})
	}
}

func TestProcessDirectoryRejectsGeneratedOutputCollisionsBeforeWriting(t *testing.T) {
	testCases := []struct {
		name       string
		colliding  string
		operations []string
	}{
		{
			name:       "validation report collides with processed event",
			colliding:  "event-validation.json",
			operations: []string{"--enrich", "--validate"},
		},
		{
			name:       "enrichment-removal report collides with processed event",
			colliding:  "event-unenrich-issues.json",
			operations: []string{"--unenrich"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventsDir := filepath.Join(dir, "events")
			outputDir := filepath.Join(dir, "output")
			writeJSONFile(assert, filepath.Join(eventsDir, "event.json"), validCLIEvent())
			writeJSONFile(assert, filepath.Join(eventsDir, testCase.colliding), validCLIEvent())

			args := []string{"--schema", schemaPath, "--events-dir", eventsDir, "--output-dir", outputDir, "--overwrite"}
			args = append(args, testCase.operations...)
			exitCode, _, stderr := runCLI(args...)

			assert.Equal(1, exitCode)
			assert.Contains(stderr, "is selected for both")
			assert.NoDirExists(outputDir, "layout resolution should fail before any output is written")
		})
	}
}

func TestProcessDirectoryRejectsOutputPathThroughSymlink(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events")
	outputDir := filepath.Join(dir, "output")
	outsideDir := filepath.Join(dir, "outside")
	writeJSONFile(assert, filepath.Join(eventsDir, "nested", "event.json"), validCLIEvent())
	assert.NoError(os.MkdirAll(outputDir, 0o755))
	assert.NoError(os.MkdirAll(outsideDir, 0o755))
	makeTestSymlink(t, outsideDir, filepath.Join(outputDir, "nested"))

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "traverses symbolic link")
	assert.NoFileExists(filepath.Join(outsideDir, "event.json"))
}

func TestProcessRejectsSummaryCollisionBeforeOverwritingAnotherArtifact(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "processed.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", outputPath,
		"--summary-json-output", outputPath,
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
	assert.NoFileExists(outputPath)
}

func TestProcessRejectsSymlinkAliasedOutputCollision(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	validationAlias := filepath.Join(dir, "validation-alias.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	makeTestSymlink(t, eventPath, validationAlias)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", validationAlias,
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
	event, err := jsonio.ReadObject(eventPath)
	assert.NoError(err)
	assert.Equal(validCLIEvent(), event)
}

func TestProcessRejectsCaseAliasedOutputCollision(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	caseProbe := filepath.Join(dir, "CaseProbe")
	assert.NoError(os.Mkdir(caseProbe, 0o755))
	if _, err := os.Stat(filepath.Join(dir, "caseprobe")); errors.Is(err, os.ErrNotExist) {
		t.Skip("filesystem is case-sensitive")
	} else {
		assert.NoError(err)
	}

	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	eventOutput := filepath.Join(dir, "processed.json")
	validationOutput := filepath.Join(dir, "PROCESSED.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", eventOutput,
		"--validate",
		"--validation-output", validationOutput,
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "is selected for both")
	assert.NoFileExists(eventOutput)
}

func TestReserveLayoutPathUsesFilesystemCaseSensitivity(t *testing.T) {
	reserved := make(map[string]string)
	first := filesystemPath{display: "result.json", resolved: "/output/result.json", caseInsensitive: true}
	second := filesystemPath{display: "RESULT.json", resolved: "/output/RESULT.json", caseInsensitive: true}

	require.NoError(t, reserveLayoutPath(reserved, first, "first output"))
	require.ErrorContains(t, reserveLayoutPath(reserved, second, "second output"), "is selected for both")

	reserved = make(map[string]string)
	first.caseInsensitive = false
	second.caseInsensitive = false
	require.NoError(t, reserveLayoutPath(reserved, first, "first output"))
	require.NoError(t, reserveLayoutPath(reserved, second, "second output"))
}
