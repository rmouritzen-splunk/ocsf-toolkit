package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

func TestVersionOptionPrintsVersionAndExits(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--version")

	assert.Equal(0, exitCode)
	assert.Equal("ocsf-toolkit "+version+"\n", stdout)
	assert.Empty(stderr)
}

func TestVersionShortOptionPrintsVersionAndExits(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("-v")

	assert.Equal(0, exitCode)
	assert.Equal("ocsf-toolkit "+version+"\n", stdout)
	assert.Empty(stderr)
}

func TestListIssueCodesPrintsIssueCodesSorted(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--list-issue-codes")

	assert.Equal(0, exitCode)
	assert.Empty(stderr)
	assert.Contains(stdout, "Issue codes (suppressible with --suppress-issues unless noted otherwise):")

	var printedCodes []issue.IssueCode
	var printedNames []string
	for line := range strings.SplitSeq(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		name, mandatory := strings.CutSuffix(trimmed, " (mandatory, cannot be suppressed)")
		code, ok := issue.ParseCode(name)
		if !ok {
			continue
		}

		assert.Equal(!code.Suppressible(), mandatory, "code %q mandatory annotation should match Suppressible()", name)
		assert.Contains(stdout, code.Description(), "description for %s should be printed", name)
		printedCodes = append(printedCodes, code)
		printedNames = append(printedNames, name)
	}

	assert.ElementsMatch(issue.Codes(), printedCodes, "every issue code should be listed exactly once")
	assert.True(sort.StringsAreSorted(printedNames), "codes should be printed in sorted order")
}

func TestListValidationCodesPrintsMetadataSorted(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--list-validation-codes")

	assert.Equal(0, exitCode)
	assert.Empty(stderr)
	normalizedOutput := strings.Join(strings.Fields(stdout), " ")
	assert.Contains(normalizedOutput,
		"Validation codes (suppressible with --suppress-validations unless noted otherwise):")
	var printedCodes []validation.Code
	var printedNames []string
	for line := range strings.SplitSeq(stdout, "\n") {
		name, _, present := strings.Cut(strings.TrimSpace(line), " (default: ")
		if !present {
			continue
		}
		code, ok := validation.ParseCode(name)
		if !ok {
			continue
		}
		entry := fmt.Sprintf("%s (default: %s)", code, code.DefaultLevel())
		if !code.Suppressible() {
			entry += " (mandatory, cannot be suppressed)"
		}
		assert.Contains(normalizedOutput, entry)
		assert.Contains(normalizedOutput, strings.Join(strings.Fields(code.Description()), " "))
		printedCodes = append(printedCodes, code)
		printedNames = append(printedNames, name)
	}
	assert.ElementsMatch(validation.Codes(), printedCodes)
	assert.True(sort.StringsAreSorted(printedNames))
}

func TestFormatHelpEntryIndentsPrimaryAndDetails(t *testing.T) {
	assert := require.New(t)

	assert.Equal("  primary", formatHelpEntry("primary"))
	assert.Equal("  primary\n    detail one\n    detail two", formatHelpEntry("primary", "detail one", "detail two"))
}

func TestJoinHelpEntriesSeparatesEntriesWithABlankLine(t *testing.T) {
	require.Equal(t, "a\n\nb\n\nc", joinHelpEntries([]string{"a", "b", "c"}))
}

func TestJoinHelpSectionSeparatesHeaderFromEntriesWithABlankLine(t *testing.T) {
	require.Equal(t, "Header:\n\na\n\nb", joinHelpSection("Header:", []string{"a", "b"}))
}

func TestHelpTakesPrecedenceOverVersionAndListIssueCodes(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--help", "--version", "--list-issue-codes")

	assert.Equal(0, exitCode)
	assert.Empty(stderr)
	assert.Contains(stdout, "Usage:")
	assert.NotContains(stdout, "ocsf-toolkit "+version)
}

func TestVersionTakesPrecedenceOverListIssueCodes(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--version", "--list-issue-codes")

	assert.Equal(0, exitCode)
	assert.Empty(stderr)
	assert.Equal("ocsf-toolkit "+version+"\n", stdout)
}

func TestProcessRejectsConflictingMutationOptions(t *testing.T) {
	tests := []struct {
		name      string
		options   []string
		wantError string
	}{
		{
			name:      "enrich and unenrich",
			options:   []string{"--enrich", "--unenrich"},
			wantError: "--enrich, --unenrich, and --force-remove cannot be combined with each other",
		},
		{
			name:      "enrich and force-remove",
			options:   []string{"--enrich", "--force-remove"},
			wantError: "--enrich, --unenrich, and --force-remove cannot be combined with each other",
		},
		{
			name:      "unenrich and force-remove",
			options:   []string{"--unenrich", "--force-remove"},
			wantError: "--enrich, --unenrich, and --force-remove cannot be combined with each other",
		},
		{
			name:      "enrich with enum-siblings",
			options:   []string{"--enrich", "--enum-siblings=remove"},
			wantError: "--enrich, --unenrich, and --force-remove cannot be combined with --enum-siblings or --observables", //nolint:lll // Test assertion string; not worth splitting.
		},
		{
			name:      "unenrich with observables",
			options:   []string{"--unenrich", "--observables"},
			wantError: "--enrich, --unenrich, and --force-remove cannot be combined with --enum-siblings or --observables", //nolint:lll // Test assertion string; not worth splitting.
		},
		{
			name:      "force-remove with enum-siblings",
			options:   []string{"--force-remove", "--enum-siblings"},
			wantError: "--enrich, --unenrich, and --force-remove cannot be combined with --enum-siblings or --observables", //nolint:lll // Test assertion string; not worth splitting.
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

func TestProcessRejectsInvalidProcessorOptions(t *testing.T) {
	tests := []struct {
		name      string
		options   []string
		wantError string
	}{
		{
			name:      "observable IDs without enrich",
			options:   []string{"--observable-ids", "1000"},
			wantError: "--observable-ids requires observable enrichment",
		},
		{
			name:      "observable IDs with observable enrichment disabled",
			options:   []string{"--enum-siblings", "--observable-ids", "1000"},
			wantError: "--observable-ids requires observable enrichment",
		},
		{
			name:      "recommended warning without validation",
			options:   []string{"--warn-on-missing-recommended"},
			wantError: "--warn-on-missing-recommended requires --validate",
		},
		{
			name:      "validation failure exit without validation",
			options:   []string{"--fail-on-validation-errors"},
			wantError: "--fail-on-validation-errors requires --validate",
		},
		{
			name:      "validation suppression without validation",
			options:   []string{"--suppress-validations"},
			wantError: "--suppress-validations requires --validate",
		},
		{
			name:      "validation level change without validation",
			options:   []string{"--validation-warnings-as-errors"},
			wantError: "--validation-warnings-as-errors requires --validate",
		},
		{
			name:      "unknown validation code",
			options:   []string{"--validate", "--suppress-validations=validation_unknown"},
			wantError: `unknown validation code "validation_unknown" in --suppress-validations`,
		},
		{
			name: "empty validation code",
			options: []string{
				"--validate",
				"--validation-errors-as-warnings=validation_attribute_required_missing,",
			},
			wantError: "--validation-errors-as-warnings contains an empty validation code",
		},
		{
			name: "repeated validation policy option",
			options: []string{
				"--validate",
				"--suppress-validations",
				"--suppress-validations=validation_attribute_deprecated",
			},
			wantError: "--suppress-validations may only be specified once",
		},
		{
			name:      "report output without report processor",
			options:   []string{"--report-output", filepath.Join("unused", "issues.json")},
			wantError: "--report-output requires --enrich, --unenrich, --force-remove, --enum-siblings, --observables, or --validate", //nolint:lll // Test assertion string; not worth splitting.
		},
		{
			name:      "observable path notation without consumer",
			options:   []string{"--unenrich", "--observable-path-notation", "indexed"},
			wantError: "--observable-path-notation requires observable enrichment or --validate",
		},
		{
			name:      "invalid observable path notation",
			options:   []string{"--validate", "--observable-path-notation", "invalid"},
			wantError: `invalid --observable-path-notation value "invalid"`,
		},
		{
			name:      "invalid enum siblings action",
			options:   []string{"--enum-siblings=bogus"},
			wantError: `invalid value "bogus" for --enum-siblings: expected add, remove, or force-remove`,
		},
		{
			name:      "invalid observables action",
			options:   []string{"--observables=bogus"},
			wantError: `invalid value "bogus" for --observables: expected add, remove, or force-remove`,
		},
		{
			name:      "none is not an enum siblings action",
			options:   []string{"--enum-siblings=none"},
			wantError: `invalid value "none" for --enum-siblings: expected add, remove, or force-remove`,
		},
		{
			name:      "none is not an observables action",
			options:   []string{"--observables=none"},
			wantError: `invalid value "none" for --observables: expected add, remove, or force-remove`,
		},
		{
			name:      "repeated enum siblings",
			options:   []string{"--enum-siblings", "--enum-siblings=remove"},
			wantError: "--enum-siblings may only be specified once",
		},
		{
			name:      "repeated observables",
			options:   []string{"--observables", "--observables=remove"},
			wantError: "--observables may only be specified once",
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

func TestMutationOptionsAcceptBareAndAssignedForms(t *testing.T) {
	tests := []struct {
		name                   string
		args                   []string
		wantEnumSiblingsAction enrichment.Action
		wantObservablesAction  enrichment.Action
	}{
		{
			name:                   "enrich shorthand",
			args:                   []string{"--enrich"},
			wantEnumSiblingsAction: enrichment.Add,
			wantObservablesAction:  enrichment.Add,
		},
		{
			name:                   "unenrich shorthand",
			args:                   []string{"--unenrich"},
			wantEnumSiblingsAction: enrichment.Remove,
			wantObservablesAction:  enrichment.Remove,
		},
		{
			name:                   "force-remove shorthand",
			args:                   []string{"--force-remove"},
			wantEnumSiblingsAction: enrichment.ForceRemove,
			wantObservablesAction:  enrichment.ForceRemove,
		},
		{
			name:                   "bare enum-siblings",
			args:                   []string{"--enum-siblings"},
			wantEnumSiblingsAction: enrichment.Add,
			wantObservablesAction:  enrichment.None,
		},
		{
			name:                   "bare observables",
			args:                   []string{"--observables"},
			wantEnumSiblingsAction: enrichment.None,
			wantObservablesAction:  enrichment.Add,
		},
		{
			name:                   "explicit component actions",
			args:                   []string{"--enum-siblings=remove", "--observables=force-remove"},
			wantEnumSiblingsAction: enrichment.Remove,
			wantObservablesAction:  enrichment.ForceRemove,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			parser, options := newParser()

			err := parser.flags.Parse(append([]string{
				"--schema", "schema.json",
				"--event", "event.json",
				"--output-dir", "output",
			}, test.args...))

			assert.NoError(err)
			config, err := options.toConfig()
			assert.NoError(err)
			assert.Equal(test.wantEnumSiblingsAction, config.enumSiblingsAction)
			assert.Equal(test.wantObservablesAction, config.observablesAction)
		})
	}
}

func TestMutationActionValueRequiresEqualsSign(t *testing.T) {
	assert := require.New(t)
	parser, _ := newParser()

	err := parser.flags.Parse([]string{"--enum-siblings", "remove"})

	assert.NoError(err)
	assert.Equal([]string{"remove"}, parser.flags.Args())
}

func TestProcessRejectsInvalidObservableIDLists(t *testing.T) {
	tests := []struct {
		name      string
		options   []string
		wantError string
	}{
		{
			name:      "missing argument",
			options:   []string{"--observable-ids"},
			wantError: "flag needs an argument: --observable-ids",
		},
		{
			name:      "empty",
			options:   []string{"--observable-ids="},
			wantError: "--observable-ids requires at least one observable type ID",
		},
		{
			name:      "empty component",
			options:   []string{"--observable-ids", "1000,,2000"},
			wantError: "--observable-ids contains an empty observable type ID",
		},
		{
			name:      "not an integer",
			options:   []string{"--observable-ids", "value"},
			wantError: `invalid observable type ID "value" in --observable-ids`,
		},
		{
			name:      "outside signed 64-bit range",
			options:   []string{"--observable-ids", "9223372036854775808"},
			wantError: `invalid observable type ID "9223372036854775808" in --observable-ids`,
		},
		{
			name:      "repeated option",
			options:   []string{"--observable-ids", "1000", "--observable-ids", "2000"},
			wantError: "--observable-ids may only be specified once",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventPath := filepath.Join(dir, "event.json")
			writeJSONFile(assert, eventPath, validCLIEvent())
			args := []string{"--schema", schemaPath, "--event", eventPath, "--enrich"}
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
	assert.Contains(
		stdout,
		"ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--force-remove] [--validate] [options]", //nolint:lll // Test assertion string; not worth splitting.
	)
	assert.Contains(stdout, "Select at least one processing action;")
	assert.Contains(stdout, "compatible actions may be combined.")
	assert.Contains(stdout, "General Options:")
	assert.Contains(stdout, "Enrichment Options:")
	assert.Contains(stdout, "Validation Options:")
	assert.Contains(stdout, "Help Options:")
	assert.Contains(stdout, helpLongOption("list-issue-codes", ""))
	assert.Contains(stdout, helpLongOption("list-validation-codes", ""))
	assert.Contains(stdout, "List all issue codes and exit")
	assert.Contains(stdout, helpShortAndLongOption("s", "schema", "COMPILED_SCHEMA_FILE"))
	assert.Contains(stdout, helpShortAndLongOption("e", "event", "FILE"))
	assert.Contains(stdout, helpShortAndLongOption("d", "events-dir", "DIR"))
	assert.Contains(stdout, helpShortAndLongOption("o", "output-dir", "DIR"))
	assert.Contains(stdout, "Output root containing")
	assert.Contains(stdout, "subdirectories named")
	assert.Contains(stdout, `subdirectories named "events" and`)
	assert.Contains(stdout, `"reports"`)
	assert.Contains(stdout, helpLongOption("fail-on-validation-errors", ""))
	assert.Contains(stdout, helpLongOption("suppress-validations", "VALIDATION_CODES"))
	assert.Contains(stdout, helpLongOption("validation-warnings-as-errors", "VALIDATION_CODES"))
	assert.Contains(stdout, helpLongOption("validation-errors-as-warnings", "VALIDATION_CODES"))
	descriptionIndent := strings.Repeat(" ", helpDescriptionColumn(defaultHelpWidth))
	assert.Contains(
		stdout,
		"--suppress-validations=VALIDATION_CODES\n"+descriptionIndent+"Suppress comma-separated validation",
	)
	assert.Contains(
		stdout,
		"--validation-warnings-as-errors=VALIDATION_CODES\n"+descriptionIndent+"Report comma-separated validation",
	)
	assert.Contains(
		stdout,
		"--validation-errors-as-warnings=VALIDATION_CODES\n"+descriptionIndent+"Report comma-separated validation",
	)
	assert.Contains(stdout, helpLongOption("report-output", "FILE"))
	assert.Contains(stdout, helpShortAndLongOption("V", "validate", ""))
	assert.Contains(stdout, helpShortAndLongOption("E", "enrich", ""))
	assert.NotContains(stdout, "update-in-place")
	assert.Contains(stdout, helpLongOption("event-output", "FILE"))
	assert.Contains(stdout, helpShortAndLongOption("U", "unenrich", ""))
	assert.Contains(stdout, helpLongOption("force-remove", ""))
	assert.Contains(stdout, helpLongOption("enum-siblings", ""))
	assert.Contains(stdout, helpLongOption("enum-siblings", "ACTION"))
	assert.Contains(stdout, helpLongOption("observables", ""))
	assert.Contains(stdout, helpLongOption("observables", "ACTION"))
	reportOutputOption := helpLongOption("report-output", "FILE")
	eventOutputOption := helpLongOption("event-output", "FILE")
	assert.Contains(stdout, reportOutputOption)
	assert.Greater(strings.Index(stdout, reportOutputOption), strings.Index(stdout, "General Options:"))
	assert.Less(strings.Index(stdout, reportOutputOption), strings.Index(stdout, "Enrichment Options:"))
	assert.Greater(strings.Index(stdout, eventOutputOption), strings.Index(stdout, "General Options:"))
	assert.Less(strings.Index(stdout, eventOutputOption), strings.Index(stdout, "Enrichment Options:"))
	assert.Contains(stdout, "Add enum siblings and observables")
	assert.Contains(stdout, "Safely remove enum siblings and")
	assert.Contains(stdout, "Force-remove enum siblings and")
	assert.Contains(stdout, "Add enum siblings")
	assert.Contains(stdout, "Set the enum sibling action:")
	assert.Contains(stdout, "Add observables")
	assert.Contains(stdout, "Set the observable action:")
	assert.Contains(stdout, "Forced observable removal deletes the entire observables attribute without")
	assert.Contains(stdout, "inspecting it.")
	assert.Contains(stdout, "Forced enum sibling removal retains siblings required for enum ID 99.")
	assert.NotContains(stdout, "skip-invalid-output")
	assert.Contains(stdout, helpLongOption("summary-json", "FILE"))
	assert.Contains(stdout, helpLongOption("summary", "FILE"))
	assert.Contains(stdout, helpLongOption("overwrite", ""))
	assert.Contains(stdout, helpShortAndLongOption("p", "pretty-json", ""))
	assert.Contains(stdout, "Pretty-print JSON output, including")
	assert.Contains(stdout, helpShortAndLongOption("q", "quiet", ""))
	assert.Contains(stdout, helpShortAndLongOption("v", "version", ""))
	assert.Contains(stdout, "--output-dir writes processed events beneath events/ and processing reports")
	assert.Contains(stdout, "beneath reports/.")
	assert.Contains(stdout, "Both output subdirectories preserve input-relative paths.")
	assert.Contains(stdout, "    With --events-dir, paths are relative to that directory.")
	assert.Contains(stdout, "--events-dir must name an existing directory, not a symbolic link.")
	assert.Contains(stdout, "Symbolic links within the directory tree are ignored.")
	assert.Contains(stdout, "    With --event, relative paths are cleaned and preserved; absolute paths and")
	assert.Contains(stdout, "paths that would escape the current directory using .. use a safe")
	assert.Contains(stdout, "basename.")
	assert.Contains(stdout, "Only one output option may use stdout.")
	notesOffset := strings.Index(stdout, "Notes:")
	assert.Greater(notesOffset, strings.Index(stdout, "Help Options:"))
	for line := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(line, "ocsf-toolkit --schema COMPILED_SCHEMA_FILE") {
			continue
		}
		assert.LessOrEqual(len(line), defaultHelpWidth, "help line exceeds the configured line width: %q", line)
	}
}

func helpShortAndLongOption(shortName, longName, valueName string) string {
	return "-" + shortName + ", " + helpLongOption(longName, valueName)
}

func helpLongOption(name, valueName string) string {
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

func TestDetectHelpOutputWidth(t *testing.T) {
	tests := []struct {
		name       string
		writer     *descriptorTestWriter
		isTerminal bool
		width      int
		sizeError  error
		want       int
	}{
		{name: "wide terminal", writer: &descriptorTestWriter{fd: 12}, isTerminal: true, width: 120, want: 118},
		{
			name: "standard terminal", writer: &descriptorTestWriter{fd: 12}, isTerminal: true,
			width: 80, want: defaultHelpWidth,
		},
		{name: "narrow terminal", writer: &descriptorTestWriter{fd: 12}, isTerminal: true, width: 60, want: 58},
		{
			name: "very narrow terminal", writer: &descriptorTestWriter{fd: 12}, isTerminal: true,
			width: 30, want: minimumHelpWidth,
		},
		{name: "not terminal", writer: &descriptorTestWriter{fd: 12}, width: 120, want: defaultHelpWidth},
		{
			name: "size error", writer: &descriptorTestWriter{fd: 12}, isTerminal: true,
			sizeError: errors.New("size"), want: defaultHelpWidth,
		},
		{name: "invalid width", writer: &descriptorTestWriter{fd: 12}, isTerminal: true, want: defaultHelpWidth},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			actual := detectHelpOutputWidth(
				test.writer,
				func(fd int) bool {
					assert.Equal(12, fd)
					return test.isTerminal
				},
				func(fd int) (int, int, error) {
					assert.Equal(12, fd)
					return test.width, 40, test.sizeError
				},
			)

			assert.Equal(test.want, actual)
		})
	}
}

func TestDetectHelpOutputWidthUsesFallbackForWriterWithoutDescriptor(t *testing.T) {
	actual := detectHelpOutputWidth(
		&strings.Builder{},
		func(int) bool {
			t.Fatal("terminal check should not run")
			return false
		},
		func(int) (int, int, error) {
			t.Fatal("terminal size check should not run")
			return 0, 0, nil
		},
	)

	require.Equal(t, defaultHelpWidth, actual)
}

func TestHelpFlagDescriptionsUseConfiguredWidth(t *testing.T) {
	assert := require.New(t)
	parser, _ := newParser()
	var output strings.Builder

	writeHelpFlag(&output, parser.flags.Lookup("event"), 118)

	assert.Contains(output.String(), "Single event JSON file, or - for stdin\n")
}

type descriptorTestWriter struct {
	strings.Builder
	fd uintptr
}

func (w *descriptorTestWriter) Fd() uintptr {
	return w.fd
}

func TestParameterErrorPrintsTerseUsage(t *testing.T) {
	assert := require.New(t)

	exitCode, stdout, stderr := runCLI("--validate")

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "--schema is required")
	assert.Contains(
		stderr,
		"ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--force-remove] [--validate] [options]", //nolint:lll // Test assertion string; not worth splitting.
	)
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
	assert.Contains(
		stderr,
		"ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--force-remove] [--validate] [options]", //nolint:lll // Test assertion string; not worth splitting.
	)
	assert.NotContains(stderr, "General Options:")
	assert.NotContains(stderr, "--schema=COMPILED_SCHEMA_FILE")
}
