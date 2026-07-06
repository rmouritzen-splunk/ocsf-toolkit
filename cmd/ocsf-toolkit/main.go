package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	flags "github.com/jessevdk/go-flags"

	"github.com/ocsf/ocsf-toolkit/eventschema"
	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

const (
	cliUsage               = "--schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --unenrich | --validate) [options]"
	stdioPath              = "-"
	stdinEventRelativePath = "event.json"
)

var version = "dev"

type cliOptions struct {
	General    generalOptions
	Enrichment enrichmentOptions
	Removal    enrichmentRemovalOptions
	Validation validationOptions
}

type generalOptions struct {
	Schema            string `short:"s" long:"schema" value-name:"COMPILED_SCHEMA_FILE" description:"Compiled OCSF schema file"`
	Event             string `short:"e" long:"event" value-name:"FILE" description:"Single event JSON file, or - for stdin"`
	EventsDir         string `short:"d" long:"events-dir" value-name:"DIR" description:"Directory tree of event JSON files"`
	OutputDir         string `short:"o" long:"output-dir" value-name:"DIR" description:"Directory for generated event and report output files"`
	UpdateInPlace     bool   `short:"i" long:"update-in-place" description:"Replace input event files with processed output"`
	EventOutput       string `long:"event-output" value-name:"FILE" description:"Single processed event output file, or - for stdout"`
	SummaryOutput     string `long:"summary-output" value-name:"FILE" description:"Human-readable summary output file, or - for stdout"`
	SummaryJSONOutput string `long:"summary-json-output" value-name:"FILE" description:"JSON summary output file, or - for stdout"`
	Overwrite         bool   `long:"overwrite" description:"Allow replacing existing output files"`
	PrettyJSON        bool   `short:"p" long:"pretty-json" description:"Write pretty-printed JSON output"`
	Quiet             bool   `short:"q" long:"quiet" description:"Suppress the default human-readable summary on stderr"`
	Version           bool   `long:"version" description:"Print version information and exit"`
}

type validationOptions struct {
	Validate                 bool   `short:"V" long:"validate" description:"Validate events"`
	WarnOnMissingRecommended bool   `long:"warn-on-missing-recommended" description:"Warn when recommended attributes are missing"`
	FailOnValidationErrors   bool   `long:"fail-on-validation-errors" description:"Exit non-zero when validation errors are found"`
	SkipInvalidOutput        bool   `long:"skip-invalid-output" description:"Do not write non-validation outputs for events with validation errors"`
	ValidationOutput         string `long:"validation-output" value-name:"FILE" description:"Validation result output file, or - for stdout"`
}

type enrichmentOptions struct {
	Enrich         bool `short:"E" long:"enrich" description:"Enrich events; adds enum siblings and observables by default"`
	NoEnumSiblings bool `long:"no-enum-siblings" description:"Do not add enum siblings"`
	NoObservables  bool `long:"no-observables" description:"Do not add observables"`
}

type enrichmentRemovalOptions struct {
	Unenrich                bool   `short:"u" long:"unenrich" description:"Remove enum siblings and observables when they are safely redundant"`
	RetainEnumSiblings      bool   `long:"retain-enum-siblings" description:"Do not remove enum siblings"`
	RetainObservables       bool   `long:"retain-observables" description:"Do not remove observables"`
	ForceRemoveEnumSiblings bool   `long:"force-remove-enum-siblings" description:"Remove supported enum siblings even when they differ from schema captions"`
	ForceRemoveObservables  bool   `long:"force-remove-observables" description:"Remove the observables attribute regardless of its contents"`
	IssuesOutput            string `long:"unenrich-issues-output" value-name:"FILE" description:"Single enrichment-removal issues output file, or - for stdout"`
}

type processConfig struct {
	schemaPath string

	eventPath string
	eventsDir string

	validate                 bool
	warnOnMissingRecommended bool
	failOnValidationErrors   bool

	enrich                  bool
	unenrich                bool
	addEnumSiblings         bool
	addObservables          bool
	removeEnumSiblings      bool
	removeObservables       bool
	forceRemoveEnumSiblings bool
	forceRemoveObservables  bool
	skipInvalidOutput       bool

	updateInPlace        bool
	outputDir            string
	eventOutput          string
	validationOutput     string
	unenrichIssuesOutput string
	summaryOutput        string
	summaryJSONOutput    string
	overwrite            bool
	prettyJSON           bool
	quiet                bool
}

type processSummary struct {
	SchemaPath                       string        `json:"schema_path"`
	Processed                        int           `json:"processed"`
	ParseFailures                    int           `json:"parse_failures"`
	ProcessingFailures               int           `json:"processing_failures"`
	TotalValidationErrorCount        int           `json:"total_validation_error_count"`
	TotalValidationWarningCount      int           `json:"total_validation_warning_count"`
	EventWriteFailures               int           `json:"event_write_failures"`
	ValidationResultWriteFailures    int           `json:"validation_result_write_failures"`
	UnenrichIssuesWriteFailures      int           `json:"unenrich_issues_write_failures"`
	EventsWithValidationErrors       int           `json:"events_with_validation_errors"`
	EventsWithValidationWarningsOnly int           `json:"events_with_validation_warnings_only"`
	EventsWritten                    int           `json:"events_written"`
	ValidationResultsWritten         int           `json:"validation_results_written"`
	EventsSkipped                    int           `json:"events_skipped"`
	EventsWithRetainedEnumSiblings   int           `json:"events_with_retained_enum_siblings"`
	EventsWithRetainedObservables    int           `json:"events_with_retained_observables"`
	Files                            []fileSummary `json:"files"`
}

type summaryReport struct {
	Metadata            summaryMetadataReport           `json:"metadata"`
	SchemaPath          string                          `json:"schema_path"`
	EventFileProcessed  string                          `json:"event_file_processed,omitempty"`
	EventFilesProcessed *int                            `json:"event_files_processed,omitempty"`
	Validation          *validationSummaryReport        `json:"validation,omitempty"`
	EventProcessing     *eventProcessingSummaryReport   `json:"event_processing,omitempty"`
	EnrichmentRemoval   *enrichmentRemovalSummaryReport `json:"enrichment_removal,omitempty"`
	Files               []fileSummary                   `json:"files,omitempty"`
}

type summaryMetadataReport struct {
	Tool toolMetadataReport `json:"tool"`
}

type toolMetadataReport struct {
	Name      string                 `json:"name"`
	Version   string                 `json:"version"`
	GoVersion string                 `json:"go_version"`
	Platform  platformMetadataReport `json:"platform"`
}

type platformMetadataReport struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type validationSummaryReport struct {
	ErrorCount             *int   `json:"error_count,omitempty"`
	WarningCount           *int   `json:"warning_count,omitempty"`
	ResultWritten          string `json:"result_written,omitempty"`
	EventsWithErrors       *int   `json:"events_with_errors,omitempty"`
	EventsWithWarningsOnly *int   `json:"events_with_warnings_only,omitempty"`
	TotalErrorCount        *int   `json:"total_error_count,omitempty"`
	TotalWarningCount      *int   `json:"total_warning_count,omitempty"`
}

type eventProcessingSummaryReport struct {
	EventWritten  string `json:"event_written,omitempty"`
	EventSkipped  string `json:"event_skipped,omitempty"`
	EventsWritten *int   `json:"events_written,omitempty"`
	EventsSkipped *int   `json:"events_skipped,omitempty"`
}

type enrichmentRemovalSummaryReport struct {
	EnumSiblingsRemoved            *int   `json:"enum_siblings_removed,omitempty"`
	ObservablesRemoved             *int   `json:"observables_removed,omitempty"`
	IssuesWritten                  string `json:"issues_written,omitempty"`
	EventsWithRetainedEnumSiblings *int   `json:"events_with_retained_enum_siblings,omitempty"`
	EventsWithRetainedObservables  *int   `json:"events_with_retained_observables,omitempty"`
}

type fileSummary struct {
	InputPath                  string `json:"input_path"`
	RelativePath               string `json:"relative_path,omitempty"`
	Processed                  bool   `json:"processed"`
	ParseError                 string `json:"parse_error,omitempty"`
	ProcessingError            string `json:"processing_error,omitempty"`
	EventPath                  string `json:"event_path,omitempty"`
	EventWriteError            string `json:"event_write_error,omitempty"`
	ValidationResultPath       string `json:"validation_result_path,omitempty"`
	ValidationResultWriteError string `json:"validation_result_write_error,omitempty"`
	ValidationErrorCount       int    `json:"validation_error_count"`
	ValidationWarningCount     int    `json:"validation_warning_count"`
	EventWritten               bool   `json:"event_written"`
	ValidationResultWritten    bool   `json:"validation_result_written"`
	EventSkipped               bool   `json:"event_skipped"`
	EnumSiblingsRemoved        int    `json:"enum_siblings_removed"`
	EnumSiblingsRetained       int    `json:"enum_siblings_retained"`
	ObservablesRemoved         int    `json:"observables_removed"`
	ObservablesRetained        int    `json:"observables_retained"`
	UnenrichIssuesPath         string `json:"unenrich_issues_path,omitempty"`
	UnenrichIssuesWriteError   string `json:"unenrich_issues_write_error,omitempty"`
	UnenrichIssuesWritten      bool   `json:"unenrich_issues_written"`
}

type validationOutput struct {
	InputPath  string                       `json:"input_path"`
	Validation eventschema.ValidationResult `json:"validation"`
}

type unenrichIssuesOutput struct {
	InputPath         string                              `json:"input_path"`
	EnrichmentRemoval eventschema.EnrichmentRemovalResult `json:"enrichment_removal"`
	Issues            []eventschema.ProcessingIssue       `json:"issues"`
}

// inputEvent identifies an event source and, for directory input, its path relative to the
// selected input root. It retains layout information; plannedEvent adds filesystem identity
// and calculated output destinations.
type inputEvent struct {
	path string
	rel  string
}

// filesystemPath keeps the representations needed to reason about one filesystem location.
// display is the user-supplied or calculated path shown in output, absolute is its cleaned
// absolute form without resolving symlinks, and resolved follows symlinks in the existing
// prefix while retaining any suffix that does not exist yet. caseInsensitive records how
// the filesystem containing that existing prefix compares path names.
type filesystemPath struct {
	display         string
	absolute        string
	resolved        string
	caseInsensitive bool
}

// outputDestination associates an enabled output with its filesystem identity. A nil
// destination means that output is disabled; a destination whose display path is "-"
// writes to stdout and has empty absolute and resolved forms.
type outputDestination struct {
	path filesystemPath
}

// plannedEvent binds an input to every output selected for that event. It prevents runtime
// path validation and writing from independently regenerating or normalizing destinations.
type plannedEvent struct {
	input            inputEvent
	inputPath        *filesystemPath
	eventOutput      *outputDestination
	validationOutput *outputDestination
	unenrichIssues   *outputDestination
}

// processingPlan is the immutable filesystem plan for one command. Path safety and
// collisions are checked while building it; processing and summary writing consume the
// same destinations afterward.
type processingPlan struct {
	events            []plannedEvent
	summaryOutput     *outputDestination
	summaryJSONOutput *outputDestination
}

type writeOptions struct {
	overwrite  bool
	prettyJSON bool
}

func main() {
	os.Exit(runWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runWithIO(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	parser, options := newParser()
	remaining, err := parser.ParseArgs(args)
	return handleParseResult(err, remaining, parser, options, stdin, stdout, stderr)
}

func newParser() (*flags.Parser, *cliOptions) {
	options := &cliOptions{}
	parser := flags.NewNamedParser("ocsf-toolkit", flags.HelpFlag|flags.PassDoubleDash)
	parser.Usage = cliUsage
	parser.ShortDescription = "Process OCSF event files."
	parser.LongDescription = "Process OCSF event files by adding or removing enrichment and/or validating them with a compiled OCSF schema."
	addParserGroup(parser, "General Options", &options.General)
	addParserGroup(parser, "Enrichment Options", &options.Enrichment)
	addParserGroup(parser, "Enrichment Removal Options", &options.Removal)
	addParserGroup(parser, "Validation Options", &options.Validation)
	return parser, options
}

func addParserGroup(parser *flags.Parser, name string, options any) {
	if _, err := parser.AddGroup(name, "", options); err != nil {
		panic(err)
	}
}

func handleParseResult(
	err error,
	remaining []string,
	parser *flags.Parser,
	options *cliOptions,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) int {
	if err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			writeHelp(stdout, flagsErr.Message)
			return 0
		}
		writef(stderr, "error: %s\n", err)
		writeErrorUsage(stderr, parser)
		return 2
	}
	if len(remaining) != 0 {
		writef(stderr, "error: unexpected argument %q\n", remaining[0])
		writeErrorUsage(stderr, parser)
		return 2
	}
	if options.General.Version {
		writef(stdout, "ocsf-toolkit %s\n", version)
		return 0
	}

	config, err := options.toConfig()
	if err != nil {
		writef(stderr, "error: %s\n", err)
		writeErrorUsage(stderr, parser)
		return 2
	}
	return runProcessCommand(config, stdin, stdout, stderr)
}

func writeErrorUsage(w io.Writer, parser *flags.Parser) {
	writef(w, "Usage:\n")
	writef(w, "  %s %s\n", parser.Name, parser.Usage)
	writef(w, "Run \"ocsf-toolkit --help\" for full usage.\n")
}

func writeHelp(w io.Writer, help string) {
	writef(w, "%s\n\n%s\n", strings.TrimRight(help, "\n"), processHelpNotes())
}

func runProcessCommand(config processConfig, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	plan, err := buildProcessingPlan(config)
	if err != nil {
		writef(stderr, "error: %s\n", err)
		return 1
	}

	summary, runtimeFailure, err := processEvents(config, plan, stdin, stdout)
	if err != nil {
		writef(stderr, "error: %s\n", err)
		return 1
	}

	if runtimeFailure {
		writeFailureDetails(stderr, summary)
		if !config.quiet && config.eventsDir != "" {
			writef(stderr, "Event files processed before error: %d\n", summary.Processed)
		}
		return 1
	}

	report := buildSummaryReport(config, summary)
	if plan.summaryJSONOutput != nil {
		path := plan.summaryJSONOutput.path.display
		if err := writeJSONDestination(path, report, config.writeOptions(), stdout); err != nil {
			writef(stderr, "error: failed to write JSON summary %q: %s\n", path, err)
			return 1
		}
	}
	if plan.summaryOutput != nil {
		path := plan.summaryOutput.path.display
		if err := writeTextDestination(path, humanSummaryWithMetadata(report), config.overwrite, stdout); err != nil {
			writef(stderr, "error: failed to write summary %q: %s\n", path, err)
			return 1
		}
	}

	if !config.quiet {
		writef(stderr, "%s", humanSummary(report))
	}

	if config.failOnValidationErrors && summary.EventsWithValidationErrors > 0 {
		return 1
	}
	return 0
}

func (config processConfig) writeOptions() writeOptions {
	return writeOptions{
		overwrite:  config.overwrite,
		prettyJSON: config.prettyJSON,
	}
}

func (options cliOptions) toConfig() (processConfig, error) {
	config := processConfig{
		schemaPath:               options.General.Schema,
		eventPath:                options.General.Event,
		eventsDir:                options.General.EventsDir,
		validate:                 options.Validation.Validate,
		warnOnMissingRecommended: options.Validation.WarnOnMissingRecommended,
		failOnValidationErrors:   options.Validation.FailOnValidationErrors,
		enrich:                   options.Enrichment.Enrich,
		unenrich:                 options.Removal.Unenrich,
		addEnumSiblings:          options.Enrichment.Enrich && !options.Enrichment.NoEnumSiblings,
		addObservables:           options.Enrichment.Enrich && !options.Enrichment.NoObservables,
		removeEnumSiblings:       options.Removal.Unenrich && !options.Removal.RetainEnumSiblings,
		removeObservables:        options.Removal.Unenrich && !options.Removal.RetainObservables,
		forceRemoveEnumSiblings:  options.Removal.ForceRemoveEnumSiblings,
		forceRemoveObservables:   options.Removal.ForceRemoveObservables,
		skipInvalidOutput:        options.Validation.SkipInvalidOutput,
		updateInPlace:            options.General.UpdateInPlace,
		outputDir:                options.General.OutputDir,
		eventOutput:              options.General.EventOutput,
		validationOutput:         options.Validation.ValidationOutput,
		unenrichIssuesOutput:     options.Removal.IssuesOutput,
		summaryOutput:            options.General.SummaryOutput,
		summaryJSONOutput:        options.General.SummaryJSONOutput,
		overwrite:                options.General.Overwrite,
		prettyJSON:               options.General.PrettyJSON,
		quiet:                    options.General.Quiet,
	}
	if config.schemaPath == "" {
		return processConfig{}, errors.New("--schema is required")
	}
	if (config.eventPath == "") == (config.eventsDir == "") {
		return processConfig{}, errors.New("exactly one of --event or --events-dir is required")
	}
	if options.Enrichment.NoEnumSiblings || options.Enrichment.NoObservables {
		if !config.enrich {
			return processConfig{}, errors.New("--no-enum-siblings and --no-observables require --enrich")
		}
	}
	if options.Removal.RetainEnumSiblings || options.Removal.RetainObservables ||
		options.Removal.ForceRemoveEnumSiblings || options.Removal.ForceRemoveObservables || options.Removal.IssuesOutput != "" {
		if !config.unenrich {
			return processConfig{}, errors.New("enrichment-removal options require --unenrich")
		}
	}
	if options.Removal.RetainEnumSiblings && options.Removal.ForceRemoveEnumSiblings {
		return processConfig{}, errors.New("--retain-enum-siblings and --force-remove-enum-siblings are mutually exclusive")
	}
	if options.Removal.RetainObservables && options.Removal.ForceRemoveObservables {
		return processConfig{}, errors.New("--retain-observables and --force-remove-observables are mutually exclusive")
	}
	if config.addEnumSiblings && config.removeEnumSiblings {
		return processConfig{}, errors.New("adding and removing enum siblings are mutually exclusive")
	}
	if config.addObservables && config.removeObservables {
		return processConfig{}, errors.New("adding and removing observables are mutually exclusive")
	}
	if !config.validate && !config.addEnumSiblings && !config.addObservables &&
		!config.removeEnumSiblings && !config.removeObservables {
		return processConfig{}, errors.New("at least one event processing action is required")
	}
	if err := validateOutputConfig(config); err != nil {
		return processConfig{}, err
	}
	return config, nil
}

func processHelpNotes() string {
	return strings.Join([]string{
		"Notes:",
		"  --output-dir writes processed events and selected reports to one output tree.",
		"  Directory outputs preserve input-relative paths.",
		"    With --events-dir, paths are relative to that directory.",
		"    With --event, safe relative paths are preserved; absolute paths and paths with .. use the basename.",
		"  Validation files use <base>-validation.json.",
		"  Enrichment-removal issue files use <base>-unenrich-issues.json.",
		"  Output directories are created if necessary.",
		"  Input and output directory trees must not overlap, including through symbolic links.",
		"  Output files are not replaced without --overwrite.",
		"  --update-in-place replaces input event files without --overwrite.",
	}, "\n")
}

func writef(w io.Writer, format string, args ...any) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return
	}
}

func buildSummaryReport(config processConfig, summary processSummary) summaryReport {
	report := summaryReport{
		Metadata:   buildSummaryMetadata(),
		SchemaPath: summary.SchemaPath,
	}
	if config.eventPath != "" {
		report.EventFileProcessed = displayInputPath(config.eventPath)
		if len(summary.Files) > 0 {
			file := summary.Files[0]
			if config.validate {
				report.Validation = &validationSummaryReport{
					ErrorCount:   ptrTo(file.ValidationErrorCount),
					WarningCount: ptrTo(file.ValidationWarningCount),
				}
				if file.ValidationResultWritten {
					report.Validation.ResultWritten = displaySummaryPath(file.ValidationResultPath)
				}
			}
			if config.mutatesEvent() {
				report.EventProcessing = &eventProcessingSummaryReport{}
				switch {
				case file.EventWritten:
					report.EventProcessing.EventWritten = displaySummaryPath(file.EventPath)
				case file.EventSkipped:
					report.EventProcessing.EventSkipped = "validation_errors_found"
				}
			}
			if config.unenrich {
				report.EnrichmentRemoval = &enrichmentRemovalSummaryReport{
					EnumSiblingsRemoved: ptrTo(file.EnumSiblingsRemoved),
					ObservablesRemoved:  ptrTo(file.ObservablesRemoved),
				}
				if file.UnenrichIssuesWritten {
					report.EnrichmentRemoval.IssuesWritten = displaySummaryPath(file.UnenrichIssuesPath)
				}
			}
		}
		return report
	}

	report.EventFilesProcessed = ptrTo(summary.Processed)
	report.Files = summary.Files
	if config.validate {
		report.Validation = &validationSummaryReport{
			EventsWithErrors:       ptrTo(summary.EventsWithValidationErrors),
			EventsWithWarningsOnly: ptrTo(summary.EventsWithValidationWarningsOnly),
			TotalErrorCount:        ptrTo(summary.TotalValidationErrorCount),
			TotalWarningCount:      ptrTo(summary.TotalValidationWarningCount),
		}
	}
	if config.mutatesEvent() {
		report.EventProcessing = &eventProcessingSummaryReport{
			EventsWritten: ptrTo(summary.EventsWritten),
			EventsSkipped: ptrTo(summary.EventsSkipped),
		}
	}
	if config.unenrich {
		report.EnrichmentRemoval = &enrichmentRemovalSummaryReport{
			EventsWithRetainedEnumSiblings: ptrTo(summary.EventsWithRetainedEnumSiblings),
			EventsWithRetainedObservables:  ptrTo(summary.EventsWithRetainedObservables),
		}
	}
	return report
}

func ptrTo[T any](value T) *T {
	return &value
}

func buildSummaryMetadata() summaryMetadataReport {
	return summaryMetadataReport{
		Tool: toolMetadataReport{
			Name:      "ocsf-toolkit",
			Version:   version,
			GoVersion: runtime.Version(),
			Platform: platformMetadataReport{
				OS:           runtime.GOOS,
				Architecture: runtime.GOARCH,
			},
		},
	}
}

func humanSummaryWithMetadata(report summaryReport) string {
	tool := report.Metadata.Tool
	return fmt.Sprintf("%s %s %s/%s %s\n\n%s",
		tool.Name,
		tool.Version,
		tool.Platform.OS,
		tool.Platform.Architecture,
		tool.GoVersion,
		humanSummary(report),
	)
}

func humanSummary(report summaryReport) string {
	if report.EventFileProcessed != "" {
		return singleEventHumanSummary(report)
	}

	lines := []string{fmt.Sprintf("Event files processed: %d", *report.EventFilesProcessed)}
	if report.Validation != nil {
		lines = append(lines,
			fmt.Sprintf("Events with validation errors: %d", *report.Validation.EventsWithErrors),
			fmt.Sprintf("Events with validation warnings (no errors): %d", *report.Validation.EventsWithWarningsOnly),
		)
	}
	if report.EventProcessing != nil {
		lines = append(lines, fmt.Sprintf("Processed events written: %d", *report.EventProcessing.EventsWritten))
		if *report.EventProcessing.EventsSkipped > 0 {
			lines = append(lines, fmt.Sprintf("Processed events skipped: %d", *report.EventProcessing.EventsSkipped))
		}
	}
	if report.EnrichmentRemoval != nil {
		lines = append(lines,
			fmt.Sprintf("Events with retained enum siblings: %d", *report.EnrichmentRemoval.EventsWithRetainedEnumSiblings),
			fmt.Sprintf("Events with retained observables: %d", *report.EnrichmentRemoval.EventsWithRetainedObservables),
		)
	}
	return strings.Join(lines, "\n") + "\n"
}

func singleEventHumanSummary(report summaryReport) string {
	lines := []string{"Event file processed: " + report.EventFileProcessed}
	if report.EventProcessing != nil {
		switch {
		case report.EventProcessing.EventWritten != "":
			lines = append(lines, "Processed event written: "+report.EventProcessing.EventWritten)
		case report.EventProcessing.EventSkipped != "":
			lines = append(lines, "Processed event skipped: validation errors found")
		}
	}
	if report.EnrichmentRemoval != nil {
		lines = append(lines,
			fmt.Sprintf("Enum siblings removed: %d", *report.EnrichmentRemoval.EnumSiblingsRemoved),
			fmt.Sprintf("Observables removed: %d", *report.EnrichmentRemoval.ObservablesRemoved),
		)
		if report.EnrichmentRemoval.IssuesWritten != "" {
			lines = append(lines, "Enrichment-removal issues written: "+report.EnrichmentRemoval.IssuesWritten)
		}
	}
	if report.Validation != nil {
		lines = append(lines,
			fmt.Sprintf("Validation errors: %d", *report.Validation.ErrorCount),
			fmt.Sprintf("Validation warnings: %d", *report.Validation.WarningCount),
		)
		if report.Validation.ResultWritten != "" {
			lines = append(lines, "Validation result written: "+report.Validation.ResultWritten)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func displayInputPath(path string) string {
	if path == stdioPath {
		return "stdin"
	}
	return path
}

func displaySummaryPath(path string) string {
	if path == stdioPath {
		return "stdout"
	}
	return path
}

func writeFailureDetails(w io.Writer, summary processSummary) {
	for _, file := range summary.Files {
		if file.ParseError != "" {
			writef(w, "%s: parse error: %s\n", file.InputPath, file.ParseError)
		}
		if file.ProcessingError != "" {
			writef(w, "%s: processing error: %s\n", file.InputPath, file.ProcessingError)
		}
		if file.EventWriteError != "" {
			writef(w, "%s: event write error: %s\n", file.InputPath, file.EventWriteError)
		}
		if file.ValidationResultWriteError != "" {
			writef(w, "%s: validation write error: %s\n", file.InputPath, file.ValidationResultWriteError)
		}
		if file.UnenrichIssuesWriteError != "" {
			writef(w, "%s: enrichment-removal issues write error: %s\n", file.InputPath, file.UnenrichIssuesWriteError)
		}
	}
}

func validateOutputConfig(config processConfig) error {
	if !config.validate && (config.warnOnMissingRecommended || config.failOnValidationErrors || config.skipInvalidOutput) {
		return errors.New("validation options require --validate")
	}
	if !config.mutatesEvent() && (config.updateInPlace || config.eventOutput != "") {
		return errors.New("event output options require --enrich or --unenrich")
	}
	if config.skipInvalidOutput && !config.mutatesEvent() {
		return errors.New("--skip-invalid-output requires --enrich or --unenrich")
	}
	if !config.validate && config.validationOutput != "" {
		return errors.New("validation output options require --validate")
	}
	if config.eventsDir == stdioPath {
		return errors.New("--events-dir cannot be -")
	}
	if config.outputDir == stdioPath {
		return errors.New("directory output options cannot be -")
	}
	if stdoutDestinationCount(config) > 1 {
		return errors.New("only one output option can write to stdout")
	}
	if config.outputDir != "" && (config.eventOutput != "" || config.validationOutput != "" ||
		config.unenrichIssuesOutput != "") {
		return errors.New("--output-dir cannot be used with operation-specific output options")
	}
	if config.updateInPlace && config.eventPath == stdioPath {
		return errors.New("--update-in-place cannot be used with --event -")
	}

	singleEventMode := config.eventPath != ""
	if singleEventMode {
		if config.mutatesEvent() {
			if config.updateInPlace && config.eventOutput != "" {
				return errors.New("--update-in-place and --event-output are mutually exclusive")
			}
			if !config.updateInPlace && countSet(config.outputDir != "", config.eventOutput != "") != 1 {
				return errors.New("single event mutation requires exactly one of --update-in-place, --output-dir DIR, or --event-output FILE")
			}
		}
		if config.validate && countSet(
			config.outputDir != "",
			config.validationOutput != "",
		) != 1 {
			return errors.New("single event validation requires exactly one of --output-dir DIR or --validation-output FILE")
		}
		if config.unenrich && countSet(config.outputDir != "", config.unenrichIssuesOutput != "") != 1 {
			return errors.New("single event enrichment removal requires exactly one of --output-dir DIR or --unenrich-issues-output FILE")
		}
		return nil
	}

	if config.mutatesEvent() {
		if config.eventOutput != "" {
			return errors.New("--event-output cannot be used with --events-dir")
		}
		if !config.updateInPlace && config.outputDir == "" {
			return errors.New("directory event mutation requires exactly one of --update-in-place or --output-dir DIR")
		}
	}

	if config.validate {
		if config.validationOutput != "" {
			return errors.New("--validation-output cannot be used with --events-dir")
		}
		if config.outputDir == "" {
			return errors.New("directory validation requires --output-dir DIR")
		}
	}
	if config.unenrich {
		if config.unenrichIssuesOutput != "" {
			return errors.New("--unenrich-issues-output cannot be used with --events-dir")
		}
		if config.outputDir == "" {
			return errors.New("directory enrichment removal requires --output-dir DIR")
		}
	}
	return nil
}

func countSet(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func newFilesystemPath(display string) (filesystemPath, error) {
	absolute, err := filepath.Abs(display)
	if err != nil {
		return filesystemPath{}, fmt.Errorf("resolve absolute path for %q: %w", display, err)
	}
	absolute = filepath.Clean(absolute)
	resolved, existingPrefix, err := resolveExistingPathPrefix(absolute)
	if err != nil {
		return filesystemPath{}, fmt.Errorf("resolve symlinks for %q: %w", display, err)
	}
	return filesystemPath{
		display:         display,
		absolute:        absolute,
		resolved:        resolved,
		caseInsensitive: pathLookupIsCaseInsensitive(existingPrefix),
	}, nil
}

func resolveExistingPathPrefix(absolute string) (string, string, error) {
	current := absolute
	missing := make([]string, 0)
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolvedPrefix, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", "", err
			}
			resolved := resolvedPrefix
			for _, component := range slices.Backward(missing) {
				resolved = filepath.Join(resolved, component)
			}
			return filepath.Clean(resolved), filepath.Clean(resolvedPrefix), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", "", fmt.Errorf("no existing ancestor for %q", absolute)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func pathLookupIsCaseInsensitive(existingPath string) bool {
	for current := existingPath; ; current = filepath.Dir(current) {
		parent := filepath.Dir(current)
		if parent == current {
			return defaultPathLookupIsCaseInsensitive()
		}
		alternate, changed := togglePathNameCase(filepath.Base(current))
		if !changed {
			continue
		}
		info, err := os.Stat(current)
		if err != nil {
			return defaultPathLookupIsCaseInsensitive()
		}
		alternateInfo, err := os.Stat(filepath.Join(parent, alternate))
		if err == nil {
			return os.SameFile(info, alternateInfo)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
		return defaultPathLookupIsCaseInsensitive()
	}
}

func defaultPathLookupIsCaseInsensitive() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

func togglePathNameCase(name string) (string, bool) {
	for index, char := range name {
		switch {
		case char >= 'a' && char <= 'z':
			return name[:index] + string(char-'a'+'A') + name[index+1:], true
		case char >= 'A' && char <= 'Z':
			return name[:index] + string(char-'A'+'a') + name[index+1:], true
		}
	}
	return name, false
}

func newOutputDestination(display string) (*outputDestination, error) {
	if display == stdioPath {
		return &outputDestination{path: filesystemPath{display: display}}, nil
	}
	path, err := newFilesystemPath(display)
	if err != nil {
		return nil, err
	}
	return &outputDestination{path: path}, nil
}

func pathIdentity(path filesystemPath) string {
	if path.caseInsensitive {
		return strings.ToUpper(path.resolved)
	}
	return path.resolved
}

func pathContains(root, path filesystemPath) bool {
	rootResolved := root.resolved
	pathResolved := path.resolved
	if root.caseInsensitive || path.caseInsensitive {
		rootResolved = strings.ToUpper(rootResolved)
		pathResolved = strings.ToUpper(pathResolved)
	}
	relative, err := filepath.Rel(rootResolved, pathResolved)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func pathsOverlap(left, right filesystemPath) bool {
	return pathContains(left, right) || pathContains(right, left)
}

func stdoutDestinationCount(config processConfig) int {
	return countSet(
		config.eventOutput == stdioPath,
		config.validationOutput == stdioPath,
		config.unenrichIssuesOutput == stdioPath,
		config.summaryOutput == stdioPath,
		config.summaryJSONOutput == stdioPath,
	)
}

func processEvents(
	config processConfig,
	plan processingPlan,
	stdin io.Reader,
	stdout io.Writer,
) (processSummary, bool, error) {
	schema, err := eventschema.New(config.schemaPath)
	if err != nil {
		return processSummary{}, false, err
	}

	processors := make([]eventschema.EventProcessor, 0, 3)
	if config.enrich {
		processors = append(processors, eventschema.NewEnrichment(
			eventschema.WithAddEnumSiblings(config.addEnumSiblings),
			eventschema.WithAddObservables(config.addObservables),
		))
	}
	if config.unenrich {
		removalOptions := []eventschema.EnrichmentRemovalOption{
			eventschema.WithRemoveEnumSiblings(config.removeEnumSiblings),
			eventschema.WithRemoveObservables(config.removeObservables),
		}
		if config.forceRemoveEnumSiblings {
			removalOptions = append(removalOptions, eventschema.WithForceRemoveEnumSiblings())
		}
		if config.forceRemoveObservables {
			removalOptions = append(removalOptions, eventschema.WithForceRemoveObservables())
		}
		processors = append(processors, eventschema.NewEnrichmentRemoval(removalOptions...))
	}
	// Keep validation last so it checks the event after all local processing,
	// including enrichment. Future event processors should be inserted before it.
	if config.validate {
		validationOptions := make([]eventschema.ValidationOption, 0, 1)
		if config.warnOnMissingRecommended {
			validationOptions = append(validationOptions, eventschema.WithWarnOnMissingRecommended())
		}
		processors = append(processors, eventschema.NewValidation(validationOptions...))
	}
	pipeline, err := schema.NewEventProcessorPipeline(processors...)
	if err != nil {
		return processSummary{}, false, fmt.Errorf("configure event processor pipeline: %w", err)
	}

	summary := processSummary{
		SchemaPath: config.schemaPath,
		Files:      make([]fileSummary, 0, len(plan.events)),
	}
	runtimeFailure := false
	for _, planned := range plan.events {
		fileResult := processOneEvent(config, pipeline, planned, stdin, stdout)
		updateSummary(&summary, fileResult)
		if fileResult.ParseError != "" || fileResult.ProcessingError != "" || fileResult.EventWriteError != "" ||
			fileResult.ValidationResultWriteError != "" || fileResult.UnenrichIssuesWriteError != "" {
			runtimeFailure = true
			break
		}
	}
	return summary, runtimeFailure, nil
}

func buildProcessingPlan(config processConfig) (processingPlan, error) {
	var outputRoot *filesystemPath
	if config.outputDir != "" {
		path, err := newFilesystemPath(config.outputDir)
		if err != nil {
			return processingPlan{}, fmt.Errorf("resolve output directory: %w", err)
		}
		outputRoot = &path
	}
	if config.eventsDir != "" && outputRoot != nil {
		inputRoot, err := newFilesystemPath(config.eventsDir)
		if err != nil {
			return processingPlan{}, fmt.Errorf("resolve events input directory: %w", err)
		}
		if pathsOverlap(inputRoot, *outputRoot) {
			return processingPlan{}, errors.New("input and output directory trees must not overlap")
		}
	}

	inputs, err := collectInputs(config)
	if err != nil {
		return processingPlan{}, err
	}
	plan := processingPlan{events: make([]plannedEvent, 0, len(inputs))}
	reserved := make(map[string]string)
	for _, input := range inputs {
		planned := plannedEvent{input: input}
		if input.path != stdioPath {
			path, err := newFilesystemPath(input.path)
			if err != nil {
				return processingPlan{}, fmt.Errorf("resolve input event %q: %w", input.path, err)
			}
			planned.inputPath = &path
			if err := reservePlannedPath(reserved, path, fmt.Sprintf("input event %q", input.path)); err != nil {
				return processingPlan{}, err
			}
		}

		if config.mutatesEvent() {
			planned.eventOutput, err = planOutputDestination(
				outputRoot, eventOutputPath(config, input), "processed event", input.path, reserved, !config.updateInPlace,
			)
			if err != nil {
				return processingPlan{}, err
			}
		}
		if config.validate {
			planned.validationOutput, err = planOutputDestination(
				outputRoot, validationOutputPath(config, input), "validation report", input.path, reserved, true,
			)
			if err != nil {
				return processingPlan{}, err
			}
		}
		if config.unenrich {
			planned.unenrichIssues, err = planOutputDestination(
				outputRoot, unenrichIssuesOutputPath(config, input), "enrichment-removal report", input.path, reserved, true,
			)
			if err != nil {
				return processingPlan{}, err
			}
		}
		plan.events = append(plan.events, planned)
	}

	if config.summaryOutput != "" {
		plan.summaryOutput, err = planOutputDestination(
			nil, config.summaryOutput, "human-readable summary", "", reserved, true,
		)
		if err != nil {
			return processingPlan{}, err
		}
	}
	if config.summaryJSONOutput != "" {
		plan.summaryJSONOutput, err = planOutputDestination(
			nil, config.summaryJSONOutput, "JSON summary", "", reserved, true,
		)
		if err != nil {
			return processingPlan{}, err
		}
	}
	return plan, nil
}

func planOutputDestination(
	outputRoot *filesystemPath,
	display string,
	kind string,
	input string,
	reserved map[string]string,
	reserve bool,
) (*outputDestination, error) {
	destination, err := newOutputDestination(display)
	if err != nil {
		return nil, fmt.Errorf("resolve %s path %q: %w", kind, display, err)
	}
	if destination.path.display == stdioPath {
		return destination, nil
	}
	if outputRoot != nil {
		if err := validatePathBeneathOutputRoot(*outputRoot, destination.path); err != nil {
			return nil, err
		}
	}
	if !reserve {
		return destination, nil
	}
	description := kind
	if input != "" {
		description = fmt.Sprintf("%s for input %q", kind, input)
	}
	if err := reservePlannedPath(reserved, destination.path, description); err != nil {
		return nil, err
	}
	return destination, nil
}

func reservePlannedPath(paths map[string]string, path filesystemPath, description string) error {
	identity := pathIdentity(path)
	if previous, collision := paths[identity]; collision {
		return fmt.Errorf("path %q is selected for both %s and %s", path.display, previous, description)
	}
	paths[identity] = description
	return nil
}

func validatePathBeneathOutputRoot(root, path filesystemPath) error {
	relative, err := filepath.Rel(root.absolute, path.absolute)
	if err != nil {
		return fmt.Errorf("failed to resolve output path %q relative to root %q: %w", path.display, root.display, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("output path %q escapes output root %q", path.display, root.display)
	}
	current := root.absolute
	directory := filepath.Dir(relative)
	if directory != "." {
		for component := range strings.SplitSeq(directory, string(filepath.Separator)) {
			current = filepath.Join(current, component)
			info, err := os.Lstat(current)
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to inspect output directory %q: %w", current, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf(
					"output path %q traverses symbolic link %q beneath output root %q",
					path.display,
					current,
					root.display,
				)
			}
			if !info.IsDir() {
				return fmt.Errorf("output path %q has non-directory parent %q", path.display, current)
			}
		}
	}
	if !pathContains(root, path) {
		return fmt.Errorf("output path %q resolves outside output root %q", path.display, root.display)
	}
	return nil
}

func collectInputs(config processConfig) ([]inputEvent, error) {
	if config.eventPath != "" {
		if config.eventPath == stdioPath {
			return []inputEvent{{path: stdioPath, rel: stdinEventRelativePath}}, nil
		}
		return []inputEvent{{path: config.eventPath}}, nil
	}

	inputs := make([]inputEvent, 0)
	err := filepath.WalkDir(config.eventsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		rel, err := filepath.Rel(config.eventsDir, path)
		if err != nil {
			return err
		}
		inputs = append(inputs, inputEvent{path: path, rel: rel})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk events directory %q: %w", config.eventsDir, err)
	}
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].rel < inputs[j].rel
	})
	return inputs, nil
}

func processOneEvent(
	config processConfig,
	pipeline eventschema.EventProcessorPipeline,
	planned plannedEvent,
	stdin io.Reader,
	stdout io.Writer,
) fileSummary {
	input := planned.input
	fileResult := fileSummary{
		InputPath:    input.path,
		RelativePath: input.rel,
	}

	event, err := readInputEvent(input, stdin)
	if err != nil {
		fileResult.ParseError = err.Error()
		return fileResult
	}

	result, err := pipeline.ProcessEvent(event)
	if err != nil {
		fileResult.ProcessingError = err.Error()
		return fileResult
	}
	fileResult.Processed = true
	fileResult.ValidationErrorCount = len(result.Validation.Errors)
	fileResult.ValidationWarningCount = len(result.Validation.Warnings)
	fileResult.EnumSiblingsRemoved = result.EnrichmentRemoval.EnumSiblingsRemoved
	fileResult.EnumSiblingsRetained = result.EnrichmentRemoval.EnumSiblingsRetained
	fileResult.ObservablesRemoved = result.EnrichmentRemoval.ObservablesRemoved
	fileResult.ObservablesRetained = result.EnrichmentRemoval.ObservablesRetained

	if config.mutatesEvent() {
		if err := writeEventOutput(config, planned.eventOutput, event, result, &fileResult, stdout); err != nil {
			fileResult.EventWriteError = err.Error()
			return fileResult
		}
	}
	if config.unenrich {
		if err := writeUnenrichIssuesOutput(config, input, planned.unenrichIssues, result, &fileResult, stdout); err != nil {
			fileResult.UnenrichIssuesWriteError = err.Error()
			return fileResult
		}
	}
	if config.validate {
		if err := writeValidationOutput(config, input, planned.validationOutput, result, &fileResult, stdout); err != nil {
			fileResult.ValidationResultWriteError = err.Error()
			return fileResult
		}
	}
	return fileResult
}

func readInputEvent(input inputEvent, stdin io.Reader) (jsonish.Map, error) {
	if input.path == stdioPath {
		event, err := jsonio.DecodeObject(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to decode JSON object from stdin: %w", err)
		}
		return event, nil
	}
	return jsonio.ReadObject(input.path)
}

func writeEventOutput(
	config processConfig,
	destination *outputDestination,
	event jsonish.Map,
	result eventschema.ProcessingResult,
	fileResult *fileSummary,
	stdout io.Writer,
) error {
	if config.skipInvalidOutput && len(result.Validation.Errors) > 0 {
		fileResult.EventSkipped = true
		return nil
	}

	outputPath := destination.path.display
	fileResult.EventPath = outputPath
	writeOptions := config.writeOptions()
	if config.updateInPlace {
		writeOptions.overwrite = true
	}
	if err := writeJSONDestination(outputPath, event, writeOptions, stdout); err != nil {
		return err
	}
	fileResult.EventWritten = true
	return nil
}

func eventOutputPath(config processConfig, input inputEvent) string {
	if config.updateInPlace {
		return input.path
	}
	if config.eventPath != "" && config.eventOutput != "" {
		return config.eventOutput
	}
	return filepath.Join(config.outputDir, eventOutputRelativePath(input))
}

func writeValidationOutput(
	config processConfig,
	input inputEvent,
	destination *outputDestination,
	result eventschema.ProcessingResult,
	fileResult *fileSummary,
	stdout io.Writer,
) error {
	output := validationOutput{
		InputPath:  input.path,
		Validation: result.Validation,
	}
	outputPath := destination.path.display

	fileResult.ValidationResultPath = outputPath
	if err := writeJSONDestination(outputPath, output, config.writeOptions(), stdout); err != nil {
		return err
	}
	fileResult.ValidationResultWritten = true
	return nil
}

func validationOutputPath(config processConfig, input inputEvent) string {
	if config.eventPath != "" && config.validationOutput != "" {
		return config.validationOutput
	}
	if config.outputDir == "" {
		return ""
	}
	return filepath.Join(config.outputDir, validationRelativePath(eventOutputRelativePath(input)))
}

func writeUnenrichIssuesOutput(
	config processConfig,
	input inputEvent,
	destination *outputDestination,
	result eventschema.ProcessingResult,
	fileResult *fileSummary,
	stdout io.Writer,
) error {
	issues := make([]eventschema.ProcessingIssue, 0)
	for _, issue := range result.Issues {
		if issue.Phase == "enrichment_removal" {
			issues = append(issues, issue)
		}
	}
	output := unenrichIssuesOutput{
		InputPath:         input.path,
		EnrichmentRemoval: result.EnrichmentRemoval,
		Issues:            issues,
	}
	outputPath := destination.path.display
	fileResult.UnenrichIssuesPath = outputPath
	if err := writeJSONDestination(outputPath, output, config.writeOptions(), stdout); err != nil {
		return err
	}
	fileResult.UnenrichIssuesWritten = true
	return nil
}

func unenrichIssuesOutputPath(config processConfig, input inputEvent) string {
	if config.eventPath != "" && config.unenrichIssuesOutput != "" {
		return config.unenrichIssuesOutput
	}
	if config.outputDir == "" {
		return ""
	}
	return filepath.Join(config.outputDir, unenrichIssuesRelativePath(eventOutputRelativePath(input)))
}

func eventOutputRelativePath(input inputEvent) string {
	if input.rel != "" {
		return safeOutputRelativePath(input.rel)
	}
	if input.path != stdioPath && !filepath.IsAbs(input.path) {
		return safeOutputRelativePath(input.path)
	}
	return filepath.Base(input.path)
}

func safeOutputRelativePath(path string) string {
	cleanPath := filepath.Clean(path)
	if slices.Contains(strings.Split(cleanPath, string(filepath.Separator)), "..") {
		return filepath.Base(cleanPath)
	}
	return cleanPath
}

func validationRelativePath(inputRel string) string {
	return reportRelativePath(inputRel, "-validation.json")
}

func unenrichIssuesRelativePath(inputRel string) string {
	return reportRelativePath(inputRel, "-unenrich-issues.json")
}

func reportRelativePath(inputRel, suffix string) string {
	dir := filepath.Dir(inputRel)
	base := filepath.Base(inputRel)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	reportName := name + suffix
	if dir == "." {
		return reportName
	}
	return filepath.Join(dir, reportName)
}

func writeJSON(w io.Writer, value any, pretty bool) error {
	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}

func updateSummary(summary *processSummary, fileResult fileSummary) {
	summary.Files = append(summary.Files, fileResult)
	if fileResult.ParseError != "" {
		summary.ParseFailures++
		return
	}
	if fileResult.ProcessingError != "" {
		summary.ProcessingFailures++
		return
	}
	if fileResult.Processed {
		summary.Processed++
	}
	summary.TotalValidationErrorCount += fileResult.ValidationErrorCount
	summary.TotalValidationWarningCount += fileResult.ValidationWarningCount
	if fileResult.ValidationErrorCount > 0 {
		summary.EventsWithValidationErrors++
	} else if fileResult.ValidationWarningCount > 0 {
		summary.EventsWithValidationWarningsOnly++
	}
	if fileResult.EventWriteError != "" {
		summary.EventWriteFailures++
	}
	if fileResult.ValidationResultWriteError != "" {
		summary.ValidationResultWriteFailures++
	}
	if fileResult.UnenrichIssuesWriteError != "" {
		summary.UnenrichIssuesWriteFailures++
	}
	if fileResult.EventWritten {
		summary.EventsWritten++
	}
	if fileResult.ValidationResultWritten {
		summary.ValidationResultsWritten++
	}
	if fileResult.EventSkipped {
		summary.EventsSkipped++
	}
	if fileResult.EnumSiblingsRetained > 0 {
		summary.EventsWithRetainedEnumSiblings++
	}
	if fileResult.ObservablesRetained > 0 {
		summary.EventsWithRetainedObservables++
	}
}

func (config processConfig) mutatesEvent() bool {
	return config.addEnumSiblings || config.addObservables || config.removeEnumSiblings || config.removeObservables
}

func writeJSONAtomic(path string, value any, options writeOptions) error {
	return writeFileAtomic(path, options.overwrite, func(w io.Writer) error {
		return writeJSON(w, value, options.prettyJSON)
	})
}

func writeJSONDestination(path string, value any, options writeOptions, stdout io.Writer) error {
	if path == stdioPath {
		return writeJSON(stdout, value, options.prettyJSON)
	}
	return writeJSONAtomic(path, value, options)
}

func writeTextAtomic(path string, text string, overwrite bool) error {
	return writeFileAtomic(path, overwrite, func(w io.Writer) error {
		if _, err := io.WriteString(w, text); err != nil {
			return fmt.Errorf("failed to write text: %w", err)
		}
		return nil
	})
}

func writeTextDestination(path string, text string, overwrite bool, stdout io.Writer) error {
	if path == stdioPath {
		if _, err := io.WriteString(stdout, text); err != nil {
			return fmt.Errorf("failed to write text to stdout: %w", err)
		}
		return nil
	}
	return writeTextAtomic(path, text, overwrite)
}

func writeFileAtomic(path string, overwrite bool, write func(io.Writer) error) error {
	if path == "" {
		return errors.New("output path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %q: %w", dir, err)
	}
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary output file for %q: %w", path, err)
	}
	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := write(tempFile); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to write temporary output file %q: %w", tempPath, err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to sync temporary output file %q: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary output file %q: %w", tempPath, err)
	}
	if overwrite {
		if err := os.Rename(tempPath, path); err != nil {
			return fmt.Errorf("failed to replace %q with temporary output file %q: %w", path, tempPath, err)
		}
	} else if err := os.Link(tempPath, path); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("output file %q already exists (use --overwrite to replace it)", path)
		}
		return fmt.Errorf("failed to create %q from temporary output file %q: %w", path, tempPath, err)
	} else {
		return nil
	}
	removeTemp = false
	return nil
}
