package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/validation"
)

func findingsAtLevel(findings []eventresult.ValidationFinding, level validation.Level) []eventresult.ValidationFinding {
	selected := make([]eventresult.ValidationFinding, 0)
	for _, finding := range findings {
		if finding.Level == level {
			selected = append(selected, finding)
		}
	}
	return selected
}

func TestCLIReportsAndIgnoresSchemaInitializationIssues(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeInitializationIssueTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", reportPath,
	)
	assert.Zero(exitCode, stderr)
	assert.Contains(stderr, "initialization issue issue_at_init_schema_enum_sibling_target_not_string:")
	assert.Contains(stderr, "status_code")

	exitCode, _, stderr = runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", reportPath,
		"--overwrite",
		"--issue-level", "issue_at_init_schema_enum_sibling_target_not_string=ignored",
	)
	assert.Zero(exitCode, stderr)
	assert.NotContains(stderr, "issue_at_init_schema_enum_sibling_target_not_string")
}

func TestCLIErrorIssueLevelFailsOnSchemaInitializationIssue(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeInitializationIssueTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	reportPath := filepath.Join(dir, "report.json")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", reportPath,
		"--issue-level", "issue_at_init_schema_enum_sibling_target_not_string=error",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "initialization issue issue_at_init_schema_enum_sibling_target_not_string:")
	assert.NoFileExists(reportPath)
}

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
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelError))
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelWarning))
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
	assert.Equal(issue.SourceEnrichment, report.Issues[0].Source)
	assert.Equal(issue.EnrichmentEnumSiblingNotAdded, report.Issues[0].Code)
	reportJSON, err := os.ReadFile(reportOutput)
	assert.NoError(err)
	var serialized struct {
		Issues []map[string]any `json:"issues"`
	}
	assert.NoError(json.Unmarshal(reportJSON, &serialized))
	assert.Len(serialized.Issues, 1)
	assert.NotContains(serialized.Issues[0], "severity")
	assert.NotContains(serialized.Issues[0], "attribute_path")
	assert.NotContains(serialized.Issues[0], "attribute")
	assert.NotContains(serialized.Issues[0], "value")
	assert.Contains(serialized.Issues[0]["details"], "attribute_path")
}

func TestProcessIssueLevelsIgnoreNoneOneMultipleAndAllCodes(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeIgnoredIssueTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	event := validCLIEvent()
	event["activity_id"] = json.Number("1234")
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
	}
	writeJSONFile(assert, eventPath, event)

	reportFor := func(extraArgs ...string) eventReport {
		reportPath := filepath.Join(t.TempDir(), "report.json")
		args := append([]string{
			"--schema", schemaPath,
			"--event", eventPath,
			"--enrich",
			"--report-output", reportPath,
			"--event-output", filepath.Join(t.TempDir(), "event.json"),
		}, extraArgs...)

		exitCode, stdout, stderr := runCLI(args...)

		assert.Equal(0, exitCode, stderr)
		assert.Empty(stdout)
		assert.Empty(stderr)
		report := readEventReport(assert, reportPath)
		assert.Equal(eventReportVersion, report.ReportVersion)
		return report
	}
	issueCodes := func(report eventReport) []issue.Code {
		codes := make([]issue.Code, len(report.Issues))
		for i, found := range report.Issues {
			codes[i] = found.Code
		}
		return codes
	}

	assert.ElementsMatch(
		[]issue.Code{issue.EnrichmentEnumSiblingNotAdded},
		issueCodes(reportFor()),
		"default-ignored duplicate diagnostics are omitted",
	)
	oneIgnored := reportFor("--issue-level", issue.EnrichmentEnumSiblingNotAdded.String()+"=ignored")
	assert.Empty(issueCodes(oneIgnored), "ignoring the default warning omits both conditions")
	multipleIgnored := reportFor(
		"--issue-level", issue.EnrichmentEnumSiblingNotAdded.String()+"=ignored",
		"--issue-level", issue.ObservableDuplicate.String()+"=ignored",
	)
	assert.Empty(issueCodes(multipleIgnored), "ignoring multiple selected codes ignores each of them")
	allIgnored := reportFor("--issue-level", "all=ignored")
	assert.Empty(
		issueCodes(allIgnored),
		"all=ignored omits every ignorable issue",
	)
}

// Invariant test: the CLI defaults observable deduplication to disabled and enables it only for explicit generated
// mode.
func TestInvariantCLIGeneratedObservableDeduplicationIsOptIn(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeIgnoredIssueTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	event := validCLIEvent()
	event["balls"] = []any{jsonish.Map{"green": "same"}, jsonish.Map{"green": "same"}}
	writeJSONFile(assert, eventPath, event)

	process := func(name string, extraArgs ...string) jsonish.Map {
		outputPath := filepath.Join(dir, name+".json")
		reportPath := filepath.Join(dir, name+"-report.json")
		args := append([]string{
			"--schema", schemaPath,
			"--event", eventPath,
			"--observables", "add",
			"--event-output", outputPath,
			"--report-output", reportPath,
		}, extraArgs...)
		exitCode, stdout, stderr := runCLI(args...)
		assert.Equal(0, exitCode, stderr)
		assert.Empty(stdout)
		assert.Empty(stderr)
		processed, err := jsonio.ReadObject(outputPath)
		assert.NoError(err)
		return processed
	}

	assert.Len(process("disabled")["observables"], 2)
	assert.Len(process("generated", "--deduplicate-observables", "generated")["observables"], 1)
}

func TestProcessIssueLevelErrorStopsPipeline(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	eventOutput := filepath.Join(dir, "processed.json")
	reportOutput := filepath.Join(dir, "report.json")
	event := validCLIEvent()
	event["activity_id"] = json.Number("1234")
	writeJSONFile(assert, eventPath, event)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--event-output", eventOutput,
		"--report-output", reportOutput,
		"--issue-level", issue.EnrichmentEnumSiblingNotAdded.String()+"=error",
	)

	assert.Equal(1, exitCode)
	assert.Contains(stderr, "processing issue issue_enrichment_enum_sibling_not_added from enrichment:")
	assert.NoFileExists(eventOutput)
	assert.NoFileExists(reportOutput)
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
	report := readEventReport(assert, filepath.Join(outputDir, "reports", "event.report.json"))
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
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelError))
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelWarning))
}

func TestProcessValidationWarnsAboutObservablePathNotation(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	reportPath := filepath.Join(dir, "report.json")
	event := validCLIEvent()
	event["ball"] = jsonish.Map{"green": "go"}
	event["observables"] = []any{
		jsonish.Map{"name": "ball.green", "type_id": json.Number("1000"), "value": "go"},
	}
	writeJSONFile(assert, eventPath, event)

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--observable-path-notation", "jsonpath",
		"--report-output", reportPath,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)
	report := readEventReport(assert, reportPath)
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelError))
	warnings := findingsAtLevel(report.Validation.Findings, validation.LevelWarning)
	assert.Len(warnings, 1)
	assert.Equal(validation.ObservableNamePathNotation, warnings[0].Code)
	assert.Equal("observables[0].name", warnings[0].Details["attribute_path"])
}

func TestProcessAppliesPathNotationOnlyToEnabledConsumers(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "out")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enum-siblings",
		"--validate",
		"--observable-path-notation", "indexed",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)
}

func TestProcessAcceptsSelectedObservableTypeIDs(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--observable-id", "0",
		"--observable-id", "1000",
		"--observable-id", "1000",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Empty(stdout)
	assert.Empty(stderr)
	assert.FileExists(filepath.Join(outputDir, "events", "event.json"))
}

func TestProcessRejectsUnknownObservableTypeIDs(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "output")
	writeJSONFile(assert, eventPath, validCLIEvent())

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enrich",
		"--observable-id", "3000",
		"--observable-id", "-1",
		"--observable-id", "3000",
		"--output-dir", outputDir,
	)

	assert.Equal(2, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "enrichment processor has unknown observable type IDs: -1, 3000")
	assert.Contains(stderr, `Run "ocsf-toolkit --help" for full usage.`)
	assert.NoDirExists(outputDir)
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
	assert.FileExists(filepath.Join(outputDir, "reports", "event.report.json"))
	assert.FileExists(filepath.Join(outputDir, "reports", "event-validation.report.json"))
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
	assert.NotEmpty(findingsAtLevel(report.Validation.Findings, validation.LevelError))
	assert.Empty(report.Issues)
	reportJSON, err := os.ReadFile(validationPath)
	assert.NoError(err)
	assert.NotContains(string(reportJSON), `"source"`)
	assert.NotContains(string(reportJSON), `"severity"`)
}

func TestProcessValidationPolicyControlsEffectiveLevelsAndIgnoredCounts(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	event := validCLIEvent()
	delete(event, "activity_id")
	writeJSONFile(assert, eventPath, event)

	warningReportPath := filepath.Join(dir, "warning-report.json")
	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", warningReportPath,
		"--validation-level", "all=warning",
		"--fail-on-validation-errors",
	)
	assert.Equal(0, exitCode, stderr)
	warningReport := readEventReport(assert, warningReportPath)
	assert.Empty(findingsAtLevel(warningReport.Validation.Findings, validation.LevelError))
	assert.NotEmpty(findingsAtLevel(warningReport.Validation.Findings, validation.LevelWarning))

	ignoredReportPath := filepath.Join(dir, "ignored-report.json")
	exitCode, _, stderr = runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", ignoredReportPath,
		"--validation-level", validation.AttributeRequiredMissing.String()+"=ignored",
		"--fail-on-validation-errors",
	)
	assert.Equal(0, exitCode, stderr)
	ignoredReport := readEventReport(assert, ignoredReportPath)
	assert.Empty(ignoredReport.Validation.Findings)
}

func TestProcessExplicitValidationLevelOverridesAllLevel(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	event := validCLIEvent()
	delete(event, "activity_id")
	writeJSONFile(assert, eventPath, event)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--validate",
		"--report-output", filepath.Join(dir, "report.json"),
		"--validation-level", "all=ignored",
		"--validation-level", validation.AttributeRequiredMissing.String()+"=warning",
	)

	assert.Equal(0, exitCode, stderr)
	report := readEventReport(assert, filepath.Join(dir, "report.json"))
	assert.Equal(validation.LevelWarning, report.Validation.Findings[0].Level)
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

func TestProcessCanSelectObservableEnrichment(t *testing.T) {
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
		"--observables",
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
	assert.NoError(os.MkdirAll(outputDir, 0o750))
	assert.NoError(os.WriteFile(filepath.Join(outputDir, "existing.txt"), []byte("kept"), 0o600))

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

	report := readEventReport(assert, filepath.Join(outputDir, "reports", "event.report.json"))
	assert.Empty(findingsAtLevel(report.Validation.Findings, validation.LevelError))
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
	assert.NoError(os.MkdirAll(outputDir, 0o750))
	assert.NoError(os.MkdirAll(filepath.Join(outputDir, "events", "a.json"), 0o750))

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--enrich",
		"--output-dir", outputDir,
		"--overwrite",
	)

	assert.Equal(1, exitCode)
	assert.Empty(stdout)
	assert.Contains(stderr, "a.json\": event write error")
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
	rootInfo, err := os.Lstat(eventsDir)
	assert.NoError(err)

	originalWalk := walkEventDirectory
	walkEventDirectory = func(_ string, walk fs.WalkDirFunc) error {
		assert.NoError(walk(eventsDir, fs.FileInfoToDirEntry(rootInfo), nil))
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
	assert.FileExists(filepath.Join(outputDir, "reports", "event.report.json"))
}

func TestProcessDirectoryToleratesInputRootBecomingAFileBeforeWalk(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventsDir := filepath.Join(dir, "events.json")
	replacementPath := filepath.Join(dir, "replacement.json")
	outputDir := filepath.Join(dir, "output")
	assert.NoError(os.Mkdir(eventsDir, 0o750))
	writeJSONFile(assert, replacementPath, validCLIEvent())
	replacementInfo, err := os.Lstat(replacementPath)
	assert.NoError(err)

	originalWalk := walkEventDirectory
	walkEventDirectory = func(_ string, walk fs.WalkDirFunc) error {
		return walk(eventsDir, fs.FileInfoToDirEntry(replacementInfo), nil)
	}
	t.Cleanup(func() { walkEventDirectory = originalWalk })

	exitCode, stdout, stderr := runCLI(
		"--schema", schemaPath,
		"--events-dir", eventsDir,
		"--validate",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	assert.Contains(stdout, "Event files processed: 0")
	assert.Empty(stderr)
	assert.NoDirExists(outputDir)
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
	report := readEventReport(assert, filepath.Join(outputDir, "reports", "event.report.json"))
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
	assert.Len(report.Issues, 2)
	assert.ElementsMatch(
		[]issue.Code{issue.ObservableValueNotFound, issue.EnrichmentRemovalEnumSiblingNotRemoved},
		[]issue.Code{report.Issues[0].Code, report.Issues[1].Code},
	)
}

func TestProcessBareForceRemoveSelectsAllComponents(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "output")
	event := validCLIEvent()
	event["class_name"] = "source-specific"
	event["activity_name"] = "source-specific"
	event["observables"] = "malformed"
	writeJSONFile(assert, eventPath, event)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--force-remove",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	processed, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "event.json"))
	assert.NoError(err)
	assert.NotContains(processed, "class_name")
	assert.NotContains(processed, "activity_name")
	assert.NotContains(processed, "observables")
	report := readEventReport(assert, filepath.Join(outputDir, "reports", "event.report.json"))
	assert.Equal(2, report.EnrichmentRemoval.EnumSiblingsRemoved)
	assert.Empty(report.Issues)
}

func TestProcessAllowsIndependentSafeAndForcedRemovalActions(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	schemaPath := writeTestSchema(assert, dir)
	eventPath := filepath.Join(dir, "event.json")
	outputDir := filepath.Join(dir, "output")
	event := validCLIEvent()
	event["class_name"] = "Alpha"
	event["activity_name"] = "source-specific"
	event["observables"] = "malformed"
	writeJSONFile(assert, eventPath, event)

	exitCode, _, stderr := runCLI(
		"--schema", schemaPath,
		"--event", eventPath,
		"--enum-siblings=remove",
		"--observables=force-remove",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	processed, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "event.json"))
	assert.NoError(err)
	assert.NotContains(processed, "class_name")
	assert.Equal("source-specific", processed["activity_name"])
	assert.NotContains(processed, "observables")
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
		"--enum-siblings=add",
		"--observables=remove",
		"--output-dir", outputDir,
	)

	assert.Equal(0, exitCode, stderr)
	processed, err := jsonio.ReadObject(filepath.Join(outputDir, "events", "event.json"))
	assert.NoError(err)
	assert.Equal("Alpha", processed["class_name"])
	assert.Equal("Do", processed["activity_name"])
	assert.NotContains(processed, "observables")
	assert.FileExists(filepath.Join(outputDir, "reports", "event.report.json"))
}
