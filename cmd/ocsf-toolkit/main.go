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

	"github.com/ocsf/ocsf-processor/eventschema"
	"github.com/ocsf/ocsf-processor/jsonio"
	"github.com/ocsf/ocsf-processor/jsonish"
)

const (
	cliUsage               = "--schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --validate) [options]"
	stdioPath              = "-"
	stdinEventRelativePath = "event.json"
)

var version = "dev"

type cliOptions struct {
	General    generalOptions
	Validation validationOptions
	Enrichment enrichmentOptions
}

type generalOptions struct {
	Schema            string `short:"s" long:"schema" value-name:"COMPILED_SCHEMA_FILE" description:"Compiled OCSF schema file"`
	Event             string `short:"e" long:"event" value-name:"FILE" description:"Single event JSON file, or - for stdin"`
	EventsDir         string `short:"d" long:"events-dir" value-name:"DIR" description:"Directory tree of event JSON files"`
	OutputDir         string `short:"o" long:"output-dir" value-name:"DIR" description:"Directory for generated event and validation output files"`
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
	ValidationOutputDir      string `long:"validation-output-dir" value-name:"DIR" description:"Directory for validation result output tree"`
}

type enrichmentOptions struct {
	Enrich          bool   `short:"E" long:"enrich" description:"Enrich events: add enum siblings and observables"`
	NoEnumSiblings  bool   `long:"no-enum-siblings" description:"Do not add enum siblings"`
	NoObservables   bool   `long:"no-observables" description:"Do not add observables"`
	EnrichInPlace   bool   `short:"i" long:"enrich-in-place" description:"Overwrite input event files with enriched output"`
	EnrichOutput    string `long:"enrich-output" value-name:"FILE" description:"Single enriched event output file, or - for stdout"`
	EnrichOutputDir string `long:"enrich-output-dir" value-name:"DIR" description:"Directory for enriched event output tree"`
}

type processConfig struct {
	schemaPath string

	eventPath string
	eventsDir string

	validate                 bool
	warnOnMissingRecommended bool
	failOnValidationErrors   bool

	enrich            bool
	addEnumSiblings   bool
	addObservables    bool
	noEnumSiblings    bool
	noObservables     bool
	skipInvalidOutput bool

	enrichInPlace       bool
	outputDir           string
	enrichOutput        string
	enrichOutputDir     string
	validationOutput    string
	validationOutputDir string
	summaryOutput       string
	summaryJSONOutput   string
	overwrite           bool
	prettyJSON          bool
	quiet               bool
}

type processSummary struct {
	SchemaPath                       string        `json:"schema_path"`
	Processed                        int           `json:"processed"`
	ParseFailures                    int           `json:"parse_failures"`
	ProcessingFailures               int           `json:"processing_failures"`
	TotalValidationErrorCount        int           `json:"total_validation_error_count"`
	TotalValidationWarningCount      int           `json:"total_validation_warning_count"`
	EnrichedEventWriteFailures       int           `json:"enriched_event_write_failures"`
	ValidationResultWriteFailures    int           `json:"validation_result_write_failures"`
	EventsWithValidationErrors       int           `json:"events_with_validation_errors"`
	EventsWithValidationWarningsOnly int           `json:"events_with_validation_warnings_only"`
	EnrichedEventsWritten            int           `json:"enriched_events_written"`
	ValidationResultsWritten         int           `json:"validation_results_written"`
	EnrichedEventsSkipped            int           `json:"enriched_events_skipped"`
	Files                            []fileSummary `json:"files"`
}

type summaryReport struct {
	Metadata            summaryMetadataReport    `json:"metadata"`
	SchemaPath          string                   `json:"schema_path"`
	EventFileProcessed  string                   `json:"event_file_processed,omitempty"`
	EventFilesProcessed *int                     `json:"event_files_processed,omitempty"`
	Validation          *validationSummaryReport `json:"validation,omitempty"`
	Enrichment          *enrichmentSummaryReport `json:"enrichment,omitempty"`
	Files               []fileSummary            `json:"files,omitempty"`
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

type enrichmentSummaryReport struct {
	EventWritten  string `json:"event_written,omitempty"`
	EventSkipped  string `json:"event_skipped,omitempty"`
	EventsWritten *int   `json:"events_written,omitempty"`
	EventsSkipped *int   `json:"events_skipped,omitempty"`
}

type fileSummary struct {
	InputPath                  string `json:"input_path"`
	RelativePath               string `json:"relative_path,omitempty"`
	Processed                  bool   `json:"processed"`
	ParseError                 string `json:"parse_error,omitempty"`
	ProcessingError            string `json:"processing_error,omitempty"`
	EnrichedEventPath          string `json:"enriched_event_path,omitempty"`
	EnrichedEventWriteError    string `json:"enriched_event_write_error,omitempty"`
	ValidationResultPath       string `json:"validation_result_path,omitempty"`
	ValidationResultWriteError string `json:"validation_result_write_error,omitempty"`
	ValidationErrorCount       int    `json:"validation_error_count"`
	ValidationWarningCount     int    `json:"validation_warning_count"`
	EnrichedEventWritten       bool   `json:"enriched_event_written"`
	ValidationResultWritten    bool   `json:"validation_result_written"`
	EnrichedEventSkipped       bool   `json:"enriched_event_skipped"`
}

type validationOutput struct {
	InputPath  string                       `json:"input_path"`
	Validation eventschema.ValidationResult `json:"validation"`
}

type inputEvent struct {
	path string
	rel  string
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
	parser.LongDescription = "Process OCSF event files by enriching and/or validating them with a compiled OCSF schema."
	addParserGroup(parser, "General Options", &options.General)
	addParserGroup(parser, "Enrichment Options", &options.Enrichment)
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
	summary, runtimeFailure, err := processEvents(config, stdin, stdout)
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
	if config.summaryJSONOutput != "" {
		if err := writeJSONDestination(config.summaryJSONOutput, report, config.writeOptions(), stdout); err != nil {
			writef(stderr, "error: failed to write JSON summary %q: %s\n", config.summaryJSONOutput, err)
			return 1
		}
	}
	if config.summaryOutput != "" {
		if err := writeTextDestination(config.summaryOutput, humanSummaryWithMetadata(report), config.overwrite, stdout); err != nil {
			writef(stderr, "error: failed to write summary %q: %s\n", config.summaryOutput, err)
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
		addEnumSiblings:          !options.Enrichment.NoEnumSiblings,
		addObservables:           !options.Enrichment.NoObservables,
		noEnumSiblings:           options.Enrichment.NoEnumSiblings,
		noObservables:            options.Enrichment.NoObservables,
		skipInvalidOutput:        options.Validation.SkipInvalidOutput,
		enrichInPlace:            options.Enrichment.EnrichInPlace,
		outputDir:                options.General.OutputDir,
		enrichOutput:             options.Enrichment.EnrichOutput,
		enrichOutputDir:          options.Enrichment.EnrichOutputDir,
		validationOutput:         options.Validation.ValidationOutput,
		validationOutputDir:      options.Validation.ValidationOutputDir,
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
	if !config.validate && !config.enrich {
		return processConfig{}, errors.New("at least one of --validate or --enrich is required")
	}
	if err := validateOutputConfig(config); err != nil {
		return processConfig{}, err
	}
	return config, nil
}

func processHelpNotes() string {
	return strings.Join([]string{
		"Notes:",
		"  --output-dir uses one output tree.",
		"  Use --enrich-output-dir and --validation-output-dir for separate trees.",
		"  Directory outputs preserve input-relative paths.",
		"    With --events-dir, paths are relative to that directory.",
		"    With --event, safe relative paths are preserved; absolute paths and paths with .. use the basename.",
		"  Validation files use <base>-validation.json.",
		"  Output directories are created if necessary.",
		"  Output files are not replaced without --overwrite.",
		"  --enrich-in-place replaces input event files without --overwrite.",
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
					ErrorCount:   new(file.ValidationErrorCount),
					WarningCount: new(file.ValidationWarningCount),
				}
				if file.ValidationResultWritten {
					report.Validation.ResultWritten = displaySummaryPath(file.ValidationResultPath)
				}
			}
			if config.enrich {
				report.Enrichment = &enrichmentSummaryReport{}
				switch {
				case file.EnrichedEventWritten:
					report.Enrichment.EventWritten = displaySummaryPath(file.EnrichedEventPath)
				case file.EnrichedEventSkipped:
					report.Enrichment.EventSkipped = "validation_errors_found"
				}
			}
		}
		return report
	}

	report.EventFilesProcessed = new(summary.Processed)
	report.Files = summary.Files
	if config.validate {
		report.Validation = &validationSummaryReport{
			EventsWithErrors:       new(summary.EventsWithValidationErrors),
			EventsWithWarningsOnly: new(summary.EventsWithValidationWarningsOnly),
			TotalErrorCount:        new(summary.TotalValidationErrorCount),
			TotalWarningCount:      new(summary.TotalValidationWarningCount),
		}
	}
	if config.enrich {
		report.Enrichment = &enrichmentSummaryReport{
			EventsWritten: new(summary.EnrichedEventsWritten),
			EventsSkipped: new(summary.EnrichedEventsSkipped),
		}
	}
	return report
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
	if report.Enrichment != nil {
		lines = append(lines, fmt.Sprintf("Enriched events written: %d", *report.Enrichment.EventsWritten))
		if *report.Enrichment.EventsSkipped > 0 {
			lines = append(lines, fmt.Sprintf("Enriched events skipped: %d", *report.Enrichment.EventsSkipped))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func singleEventHumanSummary(report summaryReport) string {
	lines := []string{"Event file processed: " + report.EventFileProcessed}
	if report.Enrichment != nil {
		switch {
		case report.Enrichment.EventWritten != "":
			lines = append(lines, "Enriched event written: "+report.Enrichment.EventWritten)
		case report.Enrichment.EventSkipped != "":
			lines = append(lines, "Enriched event skipped: validation errors found")
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
		if file.EnrichedEventWriteError != "" {
			writef(w, "%s: enrichment write error: %s\n", file.InputPath, file.EnrichedEventWriteError)
		}
		if file.ValidationResultWriteError != "" {
			writef(w, "%s: validation write error: %s\n", file.InputPath, file.ValidationResultWriteError)
		}
	}
}

func validateOutputConfig(config processConfig) error {
	if !config.validate && (config.warnOnMissingRecommended || config.failOnValidationErrors || config.skipInvalidOutput) {
		return errors.New("validation options require --validate")
	}
	if !config.enrich && (config.noEnumSiblings || config.noObservables ||
		config.enrichInPlace || config.enrichOutput != "" || config.enrichOutputDir != "") {
		return errors.New("enrichment options require --enrich")
	}
	if config.skipInvalidOutput && !config.enrich {
		return errors.New("--skip-invalid-output requires --enrich")
	}
	if !config.validate && (config.validationOutput != "" || config.validationOutputDir != "") {
		return errors.New("validation output options require --validate")
	}
	if config.eventsDir == stdioPath {
		return errors.New("--events-dir cannot be -")
	}
	if config.outputDir == stdioPath || config.enrichOutputDir == stdioPath || config.validationOutputDir == stdioPath {
		return errors.New("directory output options cannot be -")
	}
	if stdoutDestinationCount(config) > 1 {
		return errors.New("only one output option can write to stdout")
	}
	if config.outputDir != "" && (config.enrichOutput != "" || config.enrichOutputDir != "" ||
		config.validationOutput != "" || config.validationOutputDir != "") {
		return errors.New("--output-dir cannot be used with operation-specific output options")
	}
	if config.summaryOutput != "" && samePath(config.summaryOutput, config.summaryJSONOutput) {
		return errors.New("--summary-output and --summary-json-output must be different files")
	}
	if config.enrichInPlace && config.eventPath == stdioPath {
		return errors.New("--enrich-in-place cannot be used with --event -")
	}

	singleEventMode := config.eventPath != ""
	if singleEventMode {
		if config.enrich {
			enrichOutputModes := countSet(
				config.enrichInPlace,
				config.outputDir != "",
				config.enrichOutput != "",
				config.enrichOutputDir != "",
			)
			if enrichOutputModes != 1 {
				return errors.New("single event enrichment requires exactly one of --enrich-in-place, --output-dir DIR, --enrich-output FILE, or --enrich-output-dir DIR")
			}
		}
		if config.validate && countSet(
			config.outputDir != "",
			config.validationOutput != "",
			config.validationOutputDir != "",
		) != 1 {
			return errors.New("single event validation requires exactly one of --output-dir DIR, --validation-output FILE, or --validation-output-dir DIR")
		}
		if err := validateSingleFileOutputs(config); err != nil {
			return err
		}
		return nil
	}

	if config.enrich {
		if config.enrichOutput != "" {
			return errors.New("--enrich-output cannot be used with --events-dir")
		}
		enrichOutputModes := countSet(config.enrichInPlace, config.outputDir != "", config.enrichOutputDir != "")
		if enrichOutputModes != 1 {
			return errors.New("directory enrichment requires exactly one of --enrich-in-place, --output-dir DIR, or --enrich-output-dir DIR")
		}
	}

	if config.validate {
		if config.validationOutput != "" {
			return errors.New("--validation-output cannot be used with --events-dir")
		}
		if countSet(config.outputDir != "", config.validationOutputDir != "") != 1 {
			return errors.New("directory validation requires exactly one of --output-dir DIR or --validation-output-dir DIR")
		}
	}
	if config.enrich && selectedEnrichmentOutputDir(config) != "" && samePath(config.eventsDir, selectedEnrichmentOutputDir(config)) {
		return errors.New("directory enrichment output must not be the events input directory; use --enrich-in-place")
	}
	if config.validate && samePath(config.eventsDir, selectedValidationOutputDir(config)) {
		return errors.New("directory validation output must not be the events input directory")
	}
	return nil
}

func validateSingleFileOutputs(config processConfig) error {
	input := inputEvent{path: config.eventPath}
	enrichmentOutput := ""
	if config.enrich {
		enrichmentOutput = enrichmentOutputPath(config, input)
		if !config.enrichInPlace && samePath(config.eventPath, enrichmentOutput) {
			return errors.New("enrichment output must not overwrite the event file; use --enrich-in-place")
		}
	}
	if config.validate {
		validationOutput := validationOutputPath(config, input)
		if samePath(config.eventPath, validationOutput) {
			return errors.New("validation output must not overwrite the event file")
		}
		if enrichmentOutput != "" && !config.enrichInPlace && samePath(enrichmentOutput, validationOutput) {
			return errors.New("validation output must not overwrite the enriched event output")
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

func samePath(left string, right string) bool {
	if left == "" || right == "" || left == stdioPath || right == stdioPath {
		return false
	}
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func stdoutDestinationCount(config processConfig) int {
	return countSet(
		config.enrichOutput == stdioPath,
		config.validationOutput == stdioPath,
		config.summaryOutput == stdioPath,
		config.summaryJSONOutput == stdioPath,
	)
}

func processEvents(config processConfig, stdin io.Reader, stdout io.Writer) (processSummary, bool, error) {
	schema, err := eventschema.New(config.schemaPath)
	if err != nil {
		return processSummary{}, false, err
	}

	processes := make([]eventschema.EventProcess, 0, 2)
	if config.enrich {
		processes = append(processes, eventschema.NewEnrichment(
			eventschema.WithAddEnumSiblings(config.addEnumSiblings),
			eventschema.WithAddObservables(config.addObservables),
		))
	}
	// Keep validation last so it checks the event after all local processing,
	// including enrichment. Future event processors should be inserted before it.
	if config.validate {
		validationOptions := make([]eventschema.ValidationOption, 0, 1)
		if config.warnOnMissingRecommended {
			validationOptions = append(validationOptions, eventschema.WithWarnOnMissingRecommended())
		}
		processes = append(processes, eventschema.NewValidation(validationOptions...))
	}
	processor := schema.NewEventProcessor(processes...)

	inputs, err := collectInputs(config)
	if err != nil {
		return processSummary{}, false, err
	}

	summary := processSummary{
		SchemaPath: config.schemaPath,
		Files:      make([]fileSummary, 0, len(inputs)),
	}
	runtimeFailure := false
	for _, input := range inputs {
		fileResult := processOneEvent(config, processor, input, stdin, stdout)
		updateSummary(&summary, fileResult)
		if fileResult.ParseError != "" || fileResult.ProcessingError != "" ||
			fileResult.EnrichedEventWriteError != "" || fileResult.ValidationResultWriteError != "" {
			runtimeFailure = true
			break
		}
	}
	return summary, runtimeFailure, nil
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
	processor eventschema.EventProcessor,
	input inputEvent,
	stdin io.Reader,
	stdout io.Writer,
) fileSummary {
	fileResult := fileSummary{
		InputPath:    input.path,
		RelativePath: input.rel,
	}

	event, err := readInputEvent(input, stdin)
	if err != nil {
		fileResult.ParseError = err.Error()
		return fileResult
	}

	result, err := processor.ProcessEvent(event)
	if err != nil {
		fileResult.ProcessingError = err.Error()
		return fileResult
	}
	fileResult.Processed = true
	fileResult.ValidationErrorCount = len(result.Validation.Errors)
	fileResult.ValidationWarningCount = len(result.Validation.Warnings)

	if config.enrich {
		if err := writeEnrichmentOutput(config, input, event, result, &fileResult, stdout); err != nil {
			fileResult.EnrichedEventWriteError = err.Error()
			return fileResult
		}
	}
	if config.validate {
		if err := writeValidationOutput(config, input, result, &fileResult, stdout); err != nil {
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

func writeEnrichmentOutput(
	config processConfig,
	input inputEvent,
	event jsonish.Map,
	result eventschema.ProcessingResult,
	fileResult *fileSummary,
	stdout io.Writer,
) error {
	if config.skipInvalidOutput && len(result.Validation.Errors) > 0 {
		fileResult.EnrichedEventSkipped = true
		return nil
	}

	outputPath := enrichmentOutputPath(config, input)
	fileResult.EnrichedEventPath = outputPath
	writeOptions := config.writeOptions()
	if config.enrichInPlace {
		writeOptions.overwrite = true
	}
	if err := writeJSONDestination(outputPath, event, writeOptions, stdout); err != nil {
		return err
	}
	fileResult.EnrichedEventWritten = true
	return nil
}

func enrichmentOutputPath(config processConfig, input inputEvent) string {
	if config.enrichInPlace {
		return input.path
	}
	if config.eventPath != "" && config.enrichOutput != "" {
		return config.enrichOutput
	}
	return filepath.Join(selectedEnrichmentOutputDir(config), eventOutputRelativePath(input))
}

func writeValidationOutput(
	config processConfig,
	input inputEvent,
	result eventschema.ProcessingResult,
	fileResult *fileSummary,
	stdout io.Writer,
) error {
	output := validationOutput{
		InputPath:  input.path,
		Validation: result.Validation,
	}
	outputPath := validationOutputPath(config, input)
	if outputPath == "" {
		return nil
	}

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
	outputDir := selectedValidationOutputDir(config)
	if outputDir == "" {
		return ""
	}
	return filepath.Join(outputDir, validationRelativePath(eventOutputRelativePath(input)))
}

func selectedEnrichmentOutputDir(config processConfig) string {
	if config.outputDir != "" {
		return config.outputDir
	}
	return config.enrichOutputDir
}

func selectedValidationOutputDir(config processConfig) string {
	if config.outputDir != "" {
		return config.outputDir
	}
	return config.validationOutputDir
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
	dir := filepath.Dir(inputRel)
	base := filepath.Base(inputRel)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	validationName := name + "-validation.json"
	if dir == "." {
		return validationName
	}
	return filepath.Join(dir, validationName)
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
	if fileResult.EnrichedEventWriteError != "" {
		summary.EnrichedEventWriteFailures++
	}
	if fileResult.ValidationResultWriteError != "" {
		summary.ValidationResultWriteFailures++
	}
	if fileResult.EnrichedEventWritten {
		summary.EnrichedEventsWritten++
	}
	if fileResult.ValidationResultWritten {
		summary.ValidationResultsWritten++
	}
	if fileResult.EnrichedEventSkipped {
		summary.EnrichedEventsSkipped++
	}
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
