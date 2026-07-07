package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionOptionPrintsVersionAndExits(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--version")

	assert.Equal(0, exitCode)
	assert.Equal("ocsf-toolkit "+version+"\n", stdout)
	assert.Empty(stderr)
}

func TestProcessRejectsConflictingEnrichmentActions(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--unenrich",
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "adding and removing enum siblings are mutually exclusive")
}

func TestProcessRejectsInvalidProcessorOptions(t *testing.T) {
	tests := []struct {
		name      string
		options   []string
		wantError string
	}{
		{
			name:      "enum enrichment modifier without enrich",
			options:   []string{"--no-enum-siblings"},
			wantError: "--no-enum-siblings and --no-observables require --enrich",
		},
		{
			name:      "observable enrichment modifier without enrich",
			options:   []string{"--no-observables"},
			wantError: "--no-enum-siblings and --no-observables require --enrich",
		},
		{
			name:      "retain enum siblings without unenrich",
			options:   []string{"--retain-enum-siblings"},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name:      "retain observables without unenrich",
			options:   []string{"--retain-observables"},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name:      "force enum sibling removal without unenrich",
			options:   []string{"--force-remove-enum-siblings"},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name:      "force observable removal without unenrich",
			options:   []string{"--force-remove-observables"},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name:      "issues output without unenrich",
			options:   []string{"--unenrich-issues-output", filepath.Join("unused", "issues.json")},
			wantError: "enrichment-removal options require --unenrich",
		},
		{
			name: "retain and force enum siblings",
			options: []string{
				"--unenrich", "--retain-enum-siblings", "--force-remove-enum-siblings",
			},
			wantError: "--retain-enum-siblings and --force-remove-enum-siblings are mutually exclusive",
		},
		{
			name: "retain and force observables",
			options: []string{
				"--unenrich", "--retain-observables", "--force-remove-observables",
			},
			wantError: "--retain-observables and --force-remove-observables are mutually exclusive",
		},
		{
			name: "enrichment without action",
			options: []string{
				"--enrich", "--no-enum-siblings", "--no-observables",
			},
			wantError: "at least one event processing action is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventPath := filepath.Join(dir, "event.json")
			writeJSONFile(assert, eventPath, validCLIEvent())
			args := []string{"--schema", schemaPath, "--event", eventPath}
			args = append(args, test.options...)

			exitCode, _, stderr := runCLI(args...)

			assert.Equal(2, exitCode)
			assert.Contains(stderr, test.wantError)
		})
	}
}

func TestHelp(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--help")

	assert.Equal(0, exitCode)
	assert.Empty(stderr)
	assert.Contains(stdout, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --unenrich | --validate) [options]")
	assert.Contains(stdout, "General Options:")
	assert.Contains(stdout, "Enrichment Options:")
	assert.Contains(stdout, "Enrichment Removal Options:")
	assert.Contains(stdout, "Validation Options:")
	assert.Contains(stdout, "-s, --schema=COMPILED_SCHEMA_FILE")
	assert.Contains(stdout, "-e, --event=FILE")
	assert.Contains(stdout, "-d, --events-dir=DIR")
	assert.Contains(stdout, "-o, --output-dir=DIR")
	assert.Contains(stdout, "--fail-on-validation-errors")
	assert.Contains(stdout, "--validation-output=FILE")
	assert.Contains(stdout, "--no-enum-siblings")
	assert.Contains(stdout, "--no-observables")
	assert.Contains(stdout, "-V, --validate")
	assert.Contains(stdout, "-E, --enrich")
	assert.Contains(stdout, "-i, --update-in-place")
	assert.Contains(stdout, "--event-output=FILE")
	assert.Contains(stdout, "-u, --unenrich")
	assert.Contains(stdout, "--retain-enum-siblings")
	assert.Contains(stdout, "--retain-observables")
	assert.Contains(stdout, "--force-remove-enum-siblings")
	assert.Contains(stdout, "--force-remove-observables")
	assert.Contains(stdout, "--unenrich-issues-output=FILE")
	assert.Contains(stdout, "--skip-invalid-output")
	assert.Greater(strings.Index(stdout, "--skip-invalid-output"), strings.Index(stdout, "Validation Options:"))
	assert.Greater(strings.Index(stdout, "--validation-output=FILE"), strings.Index(stdout, "Validation Options:"))
	assert.Greater(strings.Index(stdout, "--event-output=FILE"), strings.Index(stdout, "General Options:"))
	assert.Less(strings.Index(stdout, "--event-output=FILE"), strings.Index(stdout, "Enrichment Options:"))
	assert.Contains(stdout, "Do not write non-validation")
	assert.Contains(stdout, "outputs for")
	assert.Contains(stdout, "events with validation errors")
	assert.Contains(stdout, "Enrich events; adds enum siblings and")
	assert.Contains(stdout, "observables by default")
	assert.Contains(stdout, "Do not add enum siblings")
	assert.Contains(stdout, "Do not add observables")
	assert.Contains(stdout, "--summary-json-output")
	assert.Contains(stdout, "--overwrite")
	assert.Contains(stdout, "-p, --pretty-json")
	assert.Contains(stdout, "-q, --quiet")
	assert.Contains(stdout, "--output-dir writes processed events and selected reports to one output tree.")
	assert.Contains(stdout, "Directory outputs preserve input-relative paths.")
	assert.Contains(stdout, "    With --events-dir, paths are relative to that directory.")
	assert.Contains(stdout, "    With --event, safe relative paths are preserved;")
	assert.Contains(stdout, "absolute paths and paths with .. use the basename.")
	assert.Contains(stdout, "Validation files use <base>-validation.json.")
	assert.Contains(stdout, "Enrichment-removal issue files use <base>-unenrich-issues.json.")
	assert.Contains(stdout, "Output directories are created if necessary.")
	assert.Contains(stdout, "Output files are not replaced without --overwrite.")
	assert.Contains(stdout, "--update-in-place replaces input event files without --overwrite.")
	assert.Greater(strings.Index(stdout, "Notes:"), strings.Index(stdout, "Help Options:"))
}

func TestShortHelpMatchesLongHelp(t *testing.T) {
	assert := require.New(t)

	longHelpExitCode, longHelpStdout, longHelpStderr := runCLI("--help")
	shortHelpExitCode, shortHelpStdout, shortHelpStderr := runCLI("-h")

	assert.Equal(0, longHelpExitCode)
	assert.Empty(longHelpStderr)
	assert.Equal(0, shortHelpExitCode)
	assert.Empty(shortHelpStderr)
	assert.Equal(longHelpStdout, shortHelpStdout)
}

func TestParameterErrorPrintsTerseUsage(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--validate")

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "--schema is required")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --unenrich | --validate) [options]")
	assert.Contains(stderr, `Run "ocsf-toolkit --help" for full usage.`)
	assert.NotContains(stderr, "General Options:")
	assert.NotContains(stderr, "--schema=COMPILED_SCHEMA_FILE")
}

func TestMissingInputErrorPrintsTerseUsage(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--schema", "schema.json")

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "exactly one of --event or --events-dir is required")
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --unenrich | --validate) [options]")
	assert.NotContains(stderr, "General Options:")
	assert.NotContains(stderr, "--schema=COMPILED_SCHEMA_FILE")
}

func TestSkipInvalidOutputRequiresValidate(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputPath := filepath.Join(dir, "enriched.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", outputPath,
		"--skip-invalid-output",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "validation options require --validate")
}

func TestSkipInvalidOutputRequiresEnrich(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	validationPath := filepath.Join(dir, "event-validation.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--validation-output", validationPath,
		"--skip-invalid-output",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "--skip-invalid-output requires --enrich")
}
