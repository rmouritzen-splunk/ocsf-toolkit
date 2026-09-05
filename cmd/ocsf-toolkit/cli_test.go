package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	normalizedOutput := strings.Join(strings.Fields(stdout), " ")
	assert.Contains(normalizedOutput, "Issue codes:")

	var printedCodes []issue.Code
	var printedNames []string
	for line := range strings.SplitSeq(stdout, "\n") {
		name, _, present := strings.Cut(strings.TrimSpace(line), " (default: ")
		if !present {
			continue
		}
		code, ok := issue.ParseCode(name)
		if !ok {
			continue
		}

		entry := fmt.Sprintf("%s (default: %s)", code, code.DefaultLevel())
		if !code.Ignorable() {
			entry += " (mandatory, cannot be ignored)"
		}
		assert.Contains(normalizedOutput, entry)
		assert.Contains(normalizedOutput, strings.Join(strings.Fields(code.Description()), " "))
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
	assert.Contains(normalizedOutput, "Validation codes:")
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

func TestHelpTakesPrecedenceOverInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "unknown flag",
			args: []string{"--unknown", "--help"},
		},
		{
			name: "unexpected argument",
			args: []string{
				"--validation-level", "validation_attribute_value_regex_not_matched=ignored",
				"validation_attribute_value_regex_not_matched=error", "-h",
			},
		},
		{
			name: "missing flag value",
			args: []string{"--schema", "--help"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)

			exitCode, stdout, stderr := runCLI(test.args...)

			assert.Zero(exitCode)
			assert.Empty(stderr)
			assert.Contains(stdout, "Usage:")
		})
	}
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
			options:   []string{"--observable-id", "1000"},
			wantError: "--observable-id requires observable enrichment",
		},
		{
			name:      "observable IDs with observable enrichment disabled",
			options:   []string{"--enum-siblings", "--observable-id", "1000"},
			wantError: "--observable-id requires observable enrichment",
		},
		{
			name:      "validation failure exit without validation",
			options:   []string{"--fail-on-validation-errors"},
			wantError: "--fail-on-validation-errors requires --validate",
		},
		{
			name:      "validation level without validation",
			options:   []string{"--validation-level", "all=ignored"},
			wantError: "--validation-level requires --validate",
		},
		{
			name:      "unknown validation code",
			options:   []string{"--validate", "--validation-level", "validation_unknown=ignored"},
			wantError: `unknown validation code "validation_unknown" in --validation-level`,
		},
		{
			name:      "invalid validation level",
			options:   []string{"--validate", "--validation-level", "validation_attribute_unknown=loud"},
			wantError: `unknown validation level "loud" in --validation-level`,
		},
		{
			name: "all validations ignored",
			options: []string{
				"--validate", "--validation-level", "all=ignored", "--report-output", "-",
			},
			wantError: "validation is enabled, but all validation codes are ignored",
		},
		{
			name:      "malformed issue level",
			options:   []string{"--validate", "--issue-level", "issue_class_uid_missing"},
			wantError: `invalid value "issue_class_uid_missing" for --issue-level: expected ISSUE_CODE=LEVEL`,
		},
		{
			name:      "unknown issue code",
			options:   []string{"--validate", "--issue-level", "issue_unknown=warning"},
			wantError: `unknown issue code "issue_unknown" in --issue-level`,
		},
		{
			name:      "invalid issue level",
			options:   []string{"--validate", "--issue-level", "issue_class_uid_missing=loud"},
			wantError: `unknown issue level "loud" in --issue-level`,
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

func TestProcessRejectsInvalidLevelOptionOrderAndDuplicates(t *testing.T) {
	tests := []struct {
		name      string
		options   []string
		wantError string
	}{
		{
			name: "issue baseline after code",
			options: []string{
				"--enrich",
				"--issue-level", issue.EnrichmentEnumSiblingNotAdded.String() + "=warning",
				"--issue-level", "all=warning",
			},
			wantError: "invalid --issue-level order: all=LEVEL must precede specific codes",
		},
		{
			name: "duplicate validation code",
			options: []string{
				"--validate",
				"--validation-level", validation.AttributeRequiredMissing.String() + "=warning",
				"--validation-level", validation.AttributeRequiredMissing.String() + "=error",
			},
			wantError: "duplicate --validation-level: validation_attribute_required_missing",
		},
		{
			name: "duplicate issue code",
			options: []string{
				"--enrich",
				"--issue-level", issue.EnrichmentEnumSiblingNotAdded.String() + "=warning",
				"--issue-level", issue.EnrichmentEnumSiblingNotAdded.String() + "=error",
			},
			wantError: "duplicate --issue-level: issue_enrichment_enum_sibling_not_added",
		},
		{
			name: "duplicate issue baseline",
			options: []string{
				"--enrich",
				"--issue-level", "all=warning",
				"--issue-level", "all=error",
			},
			wantError: "duplicate --issue-level: all",
		},
		{
			name: "validation baseline after code",
			options: []string{
				"--validate",
				"--validation-level", validation.AttributeRequiredMissing.String() + "=warning",
				"--validation-level", "all=warning",
			},
			wantError: "invalid --validation-level order: all=LEVEL must precede specific codes",
		},
		{
			name: "duplicate validation baseline",
			options: []string{
				"--validate",
				"--validation-level", "all=warning",
				"--validation-level", "all=error",
			},
			wantError: "duplicate --validation-level: all",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			dir := t.TempDir()
			schemaPath := writeTestSchema(assert, dir)
			eventPath := filepath.Join(dir, "event.json")
			writeJSONFile(assert, eventPath, validCLIEvent())
			args := []string{"--schema", schemaPath, "--event", eventPath, "--output-dir", filepath.Join(dir, "out")}
			args = append(args, test.options...)

			exitCode, _, stderr := runCLI(args...)

			assert.Equal(2, exitCode)
			assert.Contains(stderr, test.wantError)
		})
	}
}

// Invariant test: the CLI reports independent parsed-configuration problems in stable order with terse usage once.
func TestInvariantProcessReportsIndependentConfigurationProblems(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--observable-id", "1000",
		"--fail-on-validation-errors",
		"--validation-level", "all=ignored",
		"--observable-path-notation", "invalid",
		"--report-output", filepath.Join(dir, "report.json"),
		"--event-output", filepath.Join(dir, "event-output.json"),
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	wantProblems := []string{
		"--observable-id requires observable enrichment",
		"--fail-on-validation-errors requires --validate",
		"--validation-level requires --validate",
		"--observable-path-notation requires observable enrichment or --validate",
		`invalid --observable-path-notation value "invalid"`,
		"--report-output requires --enrich, --unenrich, --force-remove, --enum-siblings, --observables, or --validate",
		"at least one event processing action is required",
		"event output options require --enrich, --unenrich, --force-remove, --enum-siblings, or --observables",
	}
	previousProblem := -1
	for _, problem := range wantProblems {
		problemIndex := strings.Index(stderr, "error: "+problem+"\n")
		assert.Greater(problemIndex, previousProblem, "configuration problems should retain validation order")
		previousProblem = problemIndex
	}
	assert.Equal(len(wantProblems), strings.Count(stderr, "error: "))
	assert.Equal(1, strings.Count(stderr, "Usage:\n"))
}

// Invariant test: CLI issue and validation policy diagnostics remain aligned and accumulate independently.
func TestInvariantProcessReportsIndependentLevelPolicyProblems(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--output-dir", filepath.Join(dir, "out"),
		"--enrich",
		"--validate",
		"--issue-level", issue.ClassUIDMissing.String()+"=ignored",
		"--validation-level", "all=ignored",
	)

	assert.Equal(2, exitCode)
	assert.Contains(
		stderr,
		"error: issue policy cannot ignore mandatory issue code "+issue.ClassUIDMissing.String()+"\n",
	)
	assert.Contains(stderr, "error: validation is enabled, but all validation codes are ignored\n")
	assert.Equal(2, strings.Count(stderr, "error: "))
	assert.Equal(1, strings.Count(stderr, "Usage:\n"))
}

// Invariant test: CLI level-rule order and duplicate diagnostics accumulate for both policy families.
func TestInvariantProcessReportsIndependentLevelOrderAndDuplicateProblems(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())
	issueCode := issue.EnrichmentEnumSiblingNotAdded.String()
	validationCode := validation.AttributeRequiredMissing.String()

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--output-dir", filepath.Join(dir, "out"),
		"--enrich",
		"--validate",
		"--issue-level", issueCode+"=warning",
		"--issue-level", "all=warning",
		"--issue-level", issueCode+"=error",
		"--validation-level", validationCode+"=warning",
		"--validation-level", "all=warning",
		"--validation-level", validationCode+"=error",
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "error: invalid --issue-level order: all=LEVEL must precede specific codes\n")
	assert.Contains(stderr, "error: duplicate --issue-level: "+issueCode+"\n")
	assert.Contains(stderr, "error: invalid --validation-level order: all=LEVEL must precede specific codes\n")
	assert.Contains(stderr, "error: duplicate --validation-level: "+validationCode+"\n")
	assert.Equal(4, strings.Count(stderr, "error: "))
}

// Invariant test: independent output-configuration problems are reported together.
func TestInvariantProcessReportsIndependentOutputProblems(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", "-",
		"--summary", "-",
		"--quiet",
	)

	assert.Equal(2, exitCode)
	assert.Contains(stderr, "error: summary options require --events-dir\n")
	assert.Contains(stderr, "error: --quiet requires --events-dir\n")
	assert.Contains(stderr, "error: only one output option may use stdout\n")
	assert.Equal(3, strings.Count(stderr, "error: "))
	assert.Equal(1, strings.Count(stderr, "Usage:\n"))
}

// Invariant test: unresolved prerequisite selections suppress diagnostics that depend on those selections.
func TestInvariantProcessSuppressesDerivativeConfigurationProblems(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeTestSchema(require.New(t), dir)
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(require.New(t), eventPath, validCLIEvent())

	t.Run("unresolved mutation selection", func(t *testing.T) {
		assert := require.New(t)
		exitCode, _, stderr := runCLI("--enrich", "--unenrich")

		assert.Equal(2, exitCode)
		assert.Equal(1, strings.Count(stderr, "error: "))
		assert.Contains(stderr, "--enrich, --unenrich, and --force-remove cannot be combined with each other")
		assert.NotContains(stderr, "--schema is required")
	})

	t.Run("unresolved input mode", func(t *testing.T) {
		assert := require.New(t)
		exitCode, _, stderr := runCLI(
			"--schema", schemaPath,
			"--event", eventPath,
			"--events-dir", dir,
			"--enrich",
			"--output-dir", filepath.Join(dir, "out"),
		)

		assert.Equal(2, exitCode)
		assert.Equal(1, strings.Count(stderr, "error: "))
		assert.Contains(stderr, "exactly one of --event or --events-dir is required")
		assert.NotContains(stderr, "summary options require --events-dir")
		assert.NotContains(stderr, "directory processing requires --output-dir DIR")
	})

	t.Run("unresolved validation enablement", func(t *testing.T) {
		assert := require.New(t)
		exitCode, _, stderr := runCLI(
			"--schema", schemaPath,
			"--event", eventPath,
			"--validation-level", "all=ignored",
		)

		assert.Equal(2, exitCode)
		assert.Contains(stderr, "error: --validation-level requires --validate\n")
		assert.Contains(stderr, "error: at least one event processing action is required\n")
		assert.NotContains(stderr, "validation is enabled, but all validation codes are ignored")
	})
}

// Invariant test: parser failures remain fail-fast and do not trigger parsed-configuration validation.
func TestInvariantProcessKeepsParserFailuresFailFast(t *testing.T) {
	exitCode, _, stderr := runCLI("--validation-level", "unknown=ignored")

	require.Equal(t, 2, exitCode)
	require.Equal(t, 1, strings.Count(stderr, "error: "))
	require.Contains(t, stderr, `unknown validation code "unknown" in --validation-level`)
	require.NotContains(t, stderr, "--schema is required")
}

// Invariant test: schema preflight requires a readable regular target, follows symlinks, and defers content checks.
func TestInvariantProcessPreflightsSchemaFile(t *testing.T) {
	dir := t.TempDir()
	eventPath := filepath.Join(dir, "event.json")
	writeJSONFile(require.New(t), eventPath, validCLIEvent())

	t.Run("missing", func(t *testing.T) {
		assert := require.New(t)
		missingPath := filepath.Join(dir, "missing-schema.json")
		exitCode, _, stderr := runCLI(
			"--schema", missingPath,
			"--event", eventPath,
			"--validate",
			"--report-output", "-",
		)

		assert.Equal(2, exitCode)
		assert.Contains(stderr, "error: open --schema ")
		assert.Contains(stderr, " for reading:")
		assert.Contains(stderr, strconv.Quote(missingPath))
		assert.Equal(1, strings.Count(stderr, "error: "))
	})

	t.Run("missing path is escaped", func(t *testing.T) {
		assert := require.New(t)
		missingPath := filepath.Join(dir, "missing\nschema.json")
		exitCode, _, stderr := runCLI(
			"--schema", missingPath,
			"--event", eventPath,
			"--validate",
			"--report-output", "-",
		)

		assert.Equal(2, exitCode)
		assert.Contains(stderr, `missing\nschema.json`)
		assert.NotContains(stderr, "missing\nschema.json")
		assert.Equal(1, strings.Count(stderr, "error: "))
	})

	t.Run("directory", func(t *testing.T) {
		assert := require.New(t)
		exitCode, _, stderr := runCLI(
			"--schema", dir,
			"--event", eventPath,
			"--validate",
			"--report-output", "-",
		)

		assert.Equal(2, exitCode)
		assert.Contains(stderr, fmt.Sprintf("error: --schema %q must name a regular file\n", dir))
		assert.Equal(1, strings.Count(stderr, "error: "))
	})

	t.Run("unreadable regular file", func(t *testing.T) {
		assert := require.New(t)
		schemaPath := writeTestSchema(assert, filepath.Join(dir, "unreadable"))
		assert.NoError(os.Chmod(schemaPath, 0))
		t.Cleanup(func() {
			_ = os.Chmod(schemaPath, 0o600)
		})
		if file, err := os.Open(schemaPath); err == nil {
			_ = file.Close()
			t.Skip("file permissions do not prevent reading in this environment")
		}

		exitCode, _, stderr := runCLI(
			"--schema", schemaPath,
			"--event", eventPath,
			"--validate",
			"--report-output", "-",
		)

		assert.Equal(2, exitCode)
		assert.Contains(stderr, "error: open --schema ")
		assert.Contains(stderr, " for reading:")
		assert.Equal(1, strings.Count(stderr, "error: "))
	})

	t.Run("symbolic link to regular file", func(t *testing.T) {
		assert := require.New(t)
		schemaPath := writeTestSchema(assert, filepath.Join(dir, "schema-target"))
		aliasPath := filepath.Join(dir, "schema-link.json")
		if err := os.Symlink(schemaPath, aliasPath); err != nil {
			t.Skipf("cannot create symbolic link: %v", err)
		}

		exitCode, _, stderr := runCLI(
			"--schema", aliasPath,
			"--event", eventPath,
			"--validate",
			"--report-output", "-",
		)

		assert.Zero(exitCode)
		assert.Empty(stderr)
	})

	t.Run("schema content is deferred to loading", func(t *testing.T) {
		assert := require.New(t)
		schemaPath := filepath.Join(dir, "malformed-schema.json")
		assert.NoError(os.WriteFile(schemaPath, []byte("{"), 0o600))

		exitCode, _, stderr := runCLI(
			"--schema", schemaPath,
			"--event", eventPath,
			"--validate",
			"--report-output", "-",
		)

		assert.Equal(1, exitCode)
		assert.Equal(1, strings.Count(stderr, "error: "))
		assert.NotContains(stderr, "must name a regular file")
		assert.NotContains(stderr, "Usage:\n")
	})
}

func TestLevelOptionParserPreservesRulesForPipelineValidation(t *testing.T) {
	tests := []struct {
		name            string
		arguments       []string
		issueRules      int
		validationRules int
	}{
		{
			name: "issue rules",
			arguments: []string{
				"--issue-level", issue.EnrichmentEnumSiblingNotAdded.String() + "=warning",
				"--issue-level", "all=warning",
				"--issue-level", issue.EnrichmentEnumSiblingNotAdded.String() + "=error",
			},
			issueRules: 3,
		},
		{
			name: "validation rules",
			arguments: []string{
				"--validation-level", validation.AttributeRequiredMissing.String() + "=warning",
				"--validation-level", "all=warning",
				"--validation-level", validation.AttributeRequiredMissing.String() + "=error",
			},
			validationRules: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			parser, options := newParser()

			err := parser.parse(test.arguments)

			assert.NoError(err)
			assert.Len(options.Issues.Levels.rules, test.issueRules)
			assert.Len(options.Validation.Levels.rules, test.validationRules)
		})
	}
}

func TestProcessExplainsUnexpectedPositionalArguments(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
	}{
		{
			name: "unprefixed validation level",
			arguments: []string{
				"--validation-level", validation.AttributeValueRegexNotMatched.String() + "=ignored",
				validation.AttributeValueRegexNotMatched.String() + "=error",
			},
		},
		{
			name: "unprefixed issue level",
			arguments: []string{
				"--issue-level", issue.EnrichmentEnumSiblingNotAdded.String() + "=warning",
				issue.EnrichmentEnumSiblingNotAdded.String() + "=error",
			},
		},
		{
			name:      "ordinary argument",
			arguments: []string{"event.json"},
		},
		{
			name: "argument between options",
			arguments: []string{
				"--validate", "event.json", "--schema", "schema.json",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)

			exitCode, _, stderr := runCLI(test.arguments...)

			assert.Equal(2, exitCode)
			assert.Contains(stderr, "unexpected positional argument")
			assert.Contains(stderr, "did you forget to repeat an option?")
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
			name:                   "attached component actions",
			args:                   []string{"--enum-siblings=remove", "--observables=force-remove"},
			wantEnumSiblingsAction: enrichment.Remove,
			wantObservablesAction:  enrichment.ForceRemove,
		},
		{
			name:                   "separated component actions",
			args:                   []string{"--enum-siblings", "remove", "--observables", "force-remove"},
			wantEnumSiblingsAction: enrichment.Remove,
			wantObservablesAction:  enrichment.ForceRemove,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := require.New(t)
			parser, options := newParser()

			err := parser.parse(append([]string{
				"--schema", "schema.json",
				"--event", "event.json",
				"--output-dir", "output",
			}, test.args...))

			assert.NoError(err)
			config, problems := options.toConfig()
			assert.Empty(problems)
			assert.Equal(test.wantEnumSiblingsAction, config.enumSiblingsAction)
			assert.Equal(test.wantObservablesAction, config.observablesAction)
		})
	}
}

func TestMutationActionValueAcceptsSeparatedForm(t *testing.T) {
	assert := require.New(t)
	parser, options := newParser()

	err := parser.parse([]string{"--enum-siblings", "remove"})

	assert.NoError(err)
	assert.Empty(parser.flags.Args())
	assert.Equal(enrichment.Remove, options.Mutation.EnumSiblings.action)
}

func TestProcessRejectsInvalidObservableIDs(t *testing.T) {
	tests := []struct {
		name      string
		options   []string
		wantError string
	}{
		{
			name:      "missing argument",
			options:   []string{"--observable-id"},
			wantError: "flag needs an argument: --observable-id",
		},
		{
			name:      "empty",
			options:   []string{"--observable-id="},
			wantError: "--observable-id requires an observable type ID",
		},
		{
			name:      "comma-separated list",
			options:   []string{"--observable-id", "1000,2000"},
			wantError: `invalid observable type ID "1000,2000" in --observable-id`,
		},
		{
			name:      "not an integer",
			options:   []string{"--observable-id", "value"},
			wantError: `invalid observable type ID "value" in --observable-id`,
		},
		{
			name:      "outside signed 64-bit range",
			options:   []string{"--observable-id", "9223372036854775808"},
			wantError: `invalid observable type ID "9223372036854775808" in --observable-id`,
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
	assert.Contains(stdout, "Issue Options:")
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
	assert.Contains(stdout, "--issue-level ISSUE_CODE=LEVEL")
	assert.Contains(stdout, "--validation-level VALIDATION_CODE=LEVEL")
	descriptionIndent := strings.Repeat(" ", helpDescriptionColumn(defaultHelpWidth))
	assert.NotContains(stdout, "--issue-level=ISSUE_CODE=LEVEL")
	assert.Contains(stdout, "Set an issue level to ignored")
	assert.Contains(stdout, "or set all=LEVEL")
	assert.Contains(
		stdout,
		"--validation-level VALIDATION_CODE=LEVEL\n"+descriptionIndent+"Set a validation level",
	)
	assert.Contains(stdout, "Flag values may be separated by a space or attached with =; for example:")
	assert.Contains(stdout, "--event my_event.json")
	assert.Contains(stdout, "--event=my_event.json")
	noteOrder := []string{
		"Flag values may be separated by a space or attached with =; for example:",
		"Only one output option may use stdout.",
		"--output-dir writes processed events beneath events/",
		"--events-dir must name an existing directory",
		"Both output subdirectories preserve input-relative paths.",
		"Enum sibling work always runs before observable work",
		"Forced observable removal deletes the entire observables attribute",
		"Forced enum sibling removal retains siblings required for enum ID 99.",
	}
	previousPosition := -1
	for _, note := range noteOrder {
		position := strings.Index(stdout, note)
		assert.Greater(position, previousPosition, "help note %q should follow the preceding note", note)
		previousPosition = position
	}
	assert.Contains(stdout, "--enum-siblings add")
	assert.Contains(stdout, "--observables add")
	assert.NotContains(stdout, "--enum-siblings=")
	assert.NotContains(stdout, "--observables=")
	assert.Contains(stdout, helpLongOption("report-output", "FILE"))
	assert.Contains(stdout, helpShortAndLongOption("V", "validate", ""))
	assert.Contains(stdout, helpShortAndLongOption("E", "enrich", ""))
	assert.NotContains(stdout, "update-in-place")
	assert.Contains(stdout, helpLongOption("event-output", "FILE"))
	assert.Contains(stdout, helpShortAndLongOption("U", "unenrich", ""))
	assert.Contains(stdout, helpLongOption("force-remove", ""))
	assert.NotContains(stdout, "\n      --enum-siblings                     ")
	assert.Contains(stdout, helpLongOption("enum-siblings", "[ACTION]"))
	assert.NotContains(stdout, "\n      --observables                       ")
	assert.Contains(stdout, helpLongOption("observables", "[ACTION]"))
	assert.NotContains(stdout, "warn-on-missing-recommended")
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
	assert.Contains(stdout, "Set the enum sibling action:")
	assert.Contains(strings.Join(strings.Fields(stdout), " "), "defaults to add")
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
		return "--" + name + " " + valueName
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
