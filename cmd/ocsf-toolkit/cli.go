package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/ocsf/ocsf-toolkit/schemaresult"
)

const (
	cliUsage = "--schema COMPILED_SCHEMA_FILE" +
		" (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--force-remove] [--validate] [options]"
	defaultHelpWidth       = 78
	minimumHelpWidth       = 40
	terminalHelpMargin     = 2
	stdioPath              = "-"
	stdinEventRelativePath = "event.json"
)

var version = "dev"

func runWithIO(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	parser, options := newParser()
	err := parser.flags.Parse(args)
	remaining := parser.flags.Args()
	return handleParseResult(err, remaining, parser, options, stdin, stdout, stderr)
}

func handleParseResult(
	err error,
	remaining []string,
	parser *cliParser,
	options *cliOptions,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) int {
	if err != nil {
		writef(stderr, "error: %s\n", err)
		writeErrorUsage(stderr, parser)
		return 2
	}
	if len(remaining) != 0 {
		writef(stderr, "error: unexpected argument %q\n", remaining[0])
		writeErrorUsage(stderr, parser)
		return 2
	}
	if options.Help {
		writeHelp(stdout, parser)
		return 0
	}
	if options.General.Version {
		writef(stdout, "ocsf-toolkit %s\n", version)
		return 0
	}
	if options.ListIssueCodes || options.ListValidationCodes {
		width := helpOutputWidth(stdout)
		if options.ListIssueCodes {
			writeIssueCodes(stdout, width)
		}
		if options.ListIssueCodes && options.ListValidationCodes {
			writef(stdout, "\n")
		}
		if options.ListValidationCodes {
			writeValidationCodes(stdout, width)
		}
		return 0
	}

	config, err := options.toConfig()
	if err != nil {
		writef(stderr, "error: %s\n", err)
		writeErrorUsage(stderr, parser)
		return 2
	}
	exitCode := runProcessCommand(config, stdin, stdout, stderr)
	if exitCode == 2 {
		writeErrorUsage(stderr, parser)
	}
	return exitCode
}

func runProcessCommand(config processConfig, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	destinations, err := buildProcessingDestinations(config)
	if err != nil {
		writef(stderr, "error: %s\n", err)
		return 1
	}
	outputs := newDestinationWriter(stdout, config.writeOptions())
	pipeline, initializationIssues, suppressedInitializationIssues, err := newPipeline(config)
	if err != nil {
		writef(stderr, "error: %s\n", err)
		var configurationError *commandConfigurationError
		if errors.As(err, &configurationError) {
			return 2
		}
		return 1
	}
	writeInitializationIssues(stderr, initializationIssues, helpOutputWidth(stderr))

	summary, runtimeFailure, err := processEvents(
		config,
		pipeline,
		initializationIssues,
		suppressedInitializationIssues,
		destinations,
		stdin,
		outputs,
	)
	if err != nil {
		writef(stderr, "error: %s\n", err)
		writePartialCompletion(stderr, config, summary)
		var configurationError *commandConfigurationError
		if errors.As(err, &configurationError) {
			return 2
		}
		return 1
	}

	if runtimeFailure {
		writeFailureDetails(stderr, summary)
		writePartialCompletion(stderr, config, summary)
		return 1
	}

	if len(destinations.summaryFiles) > 0 || destinations.summaryJSONFile != nil {
		report := buildSummaryReport(config, summary)
		for _, summaryFile := range destinations.summaryFiles {
			path := summaryFile.path.display
			width := defaultHelpWidth
			if path == stdioPath {
				width = helpOutputWidth(stdout)
			}
			if err := outputs.writeText(path, humanSummaryWithMetadata(report, width)); err != nil {
				writef(stderr, "error: failed to write summary %q: %s\n", path, err)
				return 1
			}
		}
		if destinations.summaryJSONFile != nil {
			path := destinations.summaryJSONFile.path.display
			if err := outputs.writeJSON(path, report); err != nil {
				writef(stderr, "error: failed to write JSON summary %q: %s\n", path, err)
				return 1
			}
		}
	}

	if config.failOnValidationErrors &&
		summary.EventsWithValidationErrorsOnly+summary.EventsWithValidationWarningsAndErrors > 0 {
		return 1
	}
	return 0
}

func writeInitializationIssues(w io.Writer, issues []schemaresult.InitializationIssue, width int) {
	for _, found := range issues {
		prefix := fmt.Sprintf("initialization issue %s: ", found.Code)
		writef(w, "%s", prefix)
		writeWrappedHanging(w, found.Message, len(prefix), helpEntryIndent, width)
	}
}

func writePartialCompletion(w io.Writer, config processConfig, summary processSummary) {
	if config.eventsDir != "" && summary.EventsProcessed > 0 {
		writef(w, "Event files processed before error: %d\n", summary.EventsProcessed)
	}
}

func (config processConfig) writeOptions() writeOptions {
	return writeOptions{
		overwrite:  config.overwrite,
		prettyJSON: config.prettyJSON,
	}
}

func writef(w io.Writer, format string, args ...any) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return
	}
}
