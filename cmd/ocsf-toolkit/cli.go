package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	flags "github.com/jessevdk/go-flags"
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
	layout, err := buildProcessingLayout(config)
	if err != nil {
		writef(stderr, "error: %s\n", err)
		return 1
	}

	summary, runtimeFailure, err := processEvents(config, layout, stdin, stdout)
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
	if layout.summaryJSONOutput != nil {
		path := layout.summaryJSONOutput.path.display
		if err := writeJSONDestination(path, report, config.writeOptions(), stdout); err != nil {
			writef(stderr, "error: failed to write JSON summary %q: %s\n", path, err)
			return 1
		}
	}
	if layout.summaryOutput != nil {
		path := layout.summaryOutput.path.display
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

func stdoutDestinationCount(config processConfig) int {
	return countSet(
		config.eventOutput == stdioPath,
		config.validationOutput == stdioPath,
		config.unenrichIssuesOutput == stdioPath,
		config.summaryOutput == stdioPath,
		config.summaryJSONOutput == stdioPath,
	)
}
func (config processConfig) mutatesEvent() bool {
	return config.addEnumSiblings || config.addObservables || config.removeEnumSiblings || config.removeObservables
}
