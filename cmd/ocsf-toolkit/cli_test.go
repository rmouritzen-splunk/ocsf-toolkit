package main

import (
	"path/filepath"
	"runtime"
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
			name:      "report output without report processor",
			options:   []string{"--report-output", filepath.Join("unused", "issues.json")},
			wantError: "--report-output requires --enrich, --unenrich, or --validate",
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
	assert.Contains(stdout, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--validate] [options]")
	assert.Contains(stdout, "Select at least one processing action; compatible")
	assert.Contains(stdout, "actions may be combined.")
	assert.Contains(stdout, "General Options:")
	assert.Contains(stdout, "Enrichment Options:")
	assert.Contains(stdout, "Enrichment Removal Options:")
	assert.Contains(stdout, "Validation Options:")
	assert.Contains(stdout, helpShortAndLongOption("s", "schema", "COMPILED_SCHEMA_FILE"))
	assert.Contains(stdout, helpShortAndLongOption("e", "event", "FILE"))
	assert.Contains(stdout, helpShortAndLongOption("d", "events-dir", "DIR"))
	assert.Contains(stdout, helpShortAndLongOption("o", "output-dir", "DIR"))
	assert.Contains(stdout, "Output root containing subdirectories")
	assert.Contains(stdout, `named "events" and "reports"`)
	assert.Contains(stdout, helpLongOption("fail-on-validation-errors", ""))
	assert.Contains(stdout, helpLongOption("report-output", "FILE"))
	assert.Contains(stdout, helpLongOption("no-enum-siblings", ""))
	assert.Contains(stdout, helpLongOption("no-observables", ""))
	assert.Contains(stdout, helpShortAndLongOption("V", "validate", ""))
	assert.Contains(stdout, helpShortAndLongOption("E", "enrich", ""))
	assert.NotContains(stdout, "update-in-place")
	assert.Contains(stdout, helpLongOption("event-output", "FILE"))
	assert.Contains(stdout, helpShortAndLongOption("u", "unenrich", ""))
	assert.Contains(stdout, helpLongOption("retain-enum-siblings", ""))
	assert.Contains(stdout, helpLongOption("retain-observables", ""))
	assert.Contains(stdout, helpLongOption("force-remove-enum-siblings", ""))
	assert.Contains(stdout, "Remove enum siblings except those")
	assert.Contains(stdout, "required for enum ID 99")
	assert.Contains(stdout, helpLongOption("force-remove-observables", ""))
	reportOutputOption := helpLongOption("report-output", "FILE")
	eventOutputOption := helpLongOption("event-output", "FILE")
	skipInvalidOutputOption := helpLongOption("skip-invalid-output", "")
	assert.Contains(stdout, reportOutputOption)
	assert.Contains(stdout, skipInvalidOutputOption)
	assert.Greater(strings.Index(stdout, skipInvalidOutputOption), strings.Index(stdout, "Validation Options:"))
	assert.Greater(strings.Index(stdout, reportOutputOption), strings.Index(stdout, "General Options:"))
	assert.Less(strings.Index(stdout, reportOutputOption), strings.Index(stdout, "Enrichment Options:"))
	assert.Greater(strings.Index(stdout, eventOutputOption), strings.Index(stdout, "General Options:"))
	assert.Less(strings.Index(stdout, eventOutputOption), strings.Index(stdout, "Enrichment Options:"))
	assert.Contains(stdout, "Write only the validation report for")
	assert.Contains(stdout, "events with validation errors")
	assert.Contains(stdout, "Enrich events; adds enum siblings and")
	assert.Contains(stdout, "observables by default")
	assert.Contains(stdout, "Do not add enum siblings")
	assert.Contains(stdout, "Do not add observables")
	assert.Contains(stdout, helpLongOption("summary-json-file", "FILE"))
	assert.Contains(stdout, helpLongOption("summary-file", "FILE"))
	assert.Contains(stdout, helpLongOption("overwrite", ""))
	assert.Contains(stdout, helpShortAndLongOption("p", "pretty-json", ""))
	assert.Contains(stdout, "Pretty-print JSON output, including")
	assert.Contains(stdout, helpShortAndLongOption("q", "quiet", ""))
	assert.Contains(stdout, "--output-dir writes processed events beneath events/ and processing reports beneath reports/.")
	assert.Contains(stdout, "Both output subdirectories preserve input-relative paths.")
	assert.Contains(stdout, "    With --events-dir, paths are relative to that directory.")
	assert.Contains(stdout, "    With --event, safe relative paths are preserved;")
	assert.Contains(stdout, "absolute paths and paths with .. use the basename.")
	assert.Contains(stdout, "When an event and report share stdout, the event is written first.")
	assert.Contains(stdout, "When human-readable and JSON summaries share stdout, the human-readable summary is written first.")
	assert.Greater(strings.Index(stdout, "Notes:"), strings.Index(stdout, "Help Options:"))
}

func helpShortAndLongOption(shortName, longName, valueName string) string {
	if runtime.GOOS == "windows" {
		return "/" + shortName + ", " + helpLongOption(longName, valueName)
	}
	return "-" + shortName + ", " + helpLongOption(longName, valueName)
}

func helpLongOption(name, valueName string) string {
	if runtime.GOOS == "windows" {
		if valueName != "" {
			return "/" + name + ":" + valueName
		}
		return "/" + name
	}
	if valueName != "" {
		return "--" + name + "=" + valueName
	}
	return "--" + name
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
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--validate] [options]")
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
	assert.Contains(stderr, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--validate] [options]")
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
		"--report-output", validationPath,
		"--skip-invalid-output",
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "--skip-invalid-output requires --enrich")
}
