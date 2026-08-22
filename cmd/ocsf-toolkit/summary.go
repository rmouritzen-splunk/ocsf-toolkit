package main

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/ocsf/ocsf-toolkit/schemaresult"
)

type processSummary struct {
	SchemaPath                            string
	InitializationIssues                  []schemaresult.InitializationIssue
	SuppressedInitializationIssues        int
	EventsProcessed                       int
	ParseFailures                         int
	ProcessingFailures                    int
	TotalValidationErrorCount             int
	TotalValidationWarningCount           int
	TotalSuppressedValidationErrorCount   int
	TotalSuppressedValidationWarningCount int
	EventsWithNoValidationFindings        int
	EventsWithValidationWarningsOnly      int
	EventsWithValidationErrorsOnly        int
	EventsWithValidationWarningsAndErrors int
	TotalEnumSiblingsAdded                int
	TotalObservablesAdded                 int
	TotalEnumSiblingsRemoved              int
	TotalEnumSiblingsRetained             int
	TotalObservablesRemoved               int
	TotalObservablesRetained              int
	TotalIssueCount                       int
	TotalSuppressedIssueCount             int
	EventWriteFailures                    int
	ReportWriteFailures                   int
	EventsWritten                         int
	ReportsWritten                        int
	EventsWithRetainedEnumSiblings        int
	EventsWithRetainedObservables         int
	Files                                 []fileSummary
}

const summaryVersion = 1

type summaryReport struct {
	SummaryVersion      int                             `json:"summary_version"`
	Metadata            summaryMetadataReport           `json:"metadata"`
	SchemaPath          string                          `json:"schema_path"`
	EventFilesProcessed int                             `json:"event_files_processed"`
	Initialization      *initializationSummaryReport    `json:"initialization,omitempty"`
	Validation          *validationSummaryReport        `json:"validation,omitempty"`
	Enrichment          *enrichmentSummaryReport        `json:"enrichment,omitempty"`
	EnrichmentRemoval   *enrichmentRemovalSummaryReport `json:"enrichment_removal,omitempty"`
	Issues              issueSummaryReport              `json:"issues"`
	Outputs             outputSummaryReport             `json:"outputs"`
	Files               []fileSummaryReport             `json:"files,omitempty"`
}

type initializationSummaryReport struct {
	Issues               []schemaresult.InitializationIssue `json:"issues,omitempty"`
	SuppressedIssueCount int                                `json:"suppressed_issue_count,omitempty"`
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
	EventsWithNoFindings        int `json:"events_with_no_errors_or_warnings"`
	EventsWithWarningsOnly      int `json:"events_with_warnings_only"`
	EventsWithErrorsOnly        int `json:"events_with_errors_only"`
	EventsWithWarningsAndErrors int `json:"events_with_warnings_and_errors"`
	TotalErrorCount             int `json:"total_error_count"`
	TotalWarningCount           int `json:"total_warning_count"`
	SuppressedErrorCount        int `json:"suppressed_error_count"`
	SuppressedWarningCount      int `json:"suppressed_warning_count"`
}

type enrichmentSummaryReport struct {
	EnumSiblingsAdded int `json:"enum_siblings_added"`
	ObservablesAdded  int `json:"observables_added"`
}

type enrichmentRemovalSummaryReport struct {
	EnumSiblingsRemoved            int `json:"enum_siblings_removed"`
	EnumSiblingsRetained           int `json:"enum_siblings_retained"`
	ObservablesRemoved             int `json:"observables_removed"`
	ObservablesRetained            int `json:"observables_retained"`
	EventsWithRetainedEnumSiblings int `json:"events_with_retained_enum_siblings"`
	EventsWithRetainedObservables  int `json:"events_with_retained_observables"`
}

type issueSummaryReport struct {
	ReportedCount   int `json:"reported_count"`
	SuppressedCount int `json:"suppressed_count"`
}

type outputSummaryReport struct {
	EventsWritten       int `json:"events_written"`
	ReportsWritten      int `json:"reports_written"`
	EventWriteFailures  int `json:"event_write_failures"`
	ReportWriteFailures int `json:"report_write_failures"`
}

type fileSummary struct {
	InputPath                        string
	RelativePath                     string
	ProcessingCompleted              bool
	ParseError                       string
	ProcessingError                  string
	EventPath                        string
	EventWriteError                  string
	ReportPath                       string
	ReportWriteError                 string
	ValidationErrorCount             int
	ValidationWarningCount           int
	SuppressedValidationErrorCount   int
	SuppressedValidationWarningCount int
	EnumSiblingsAdded                int
	ObservablesAdded                 int
	EnumSiblingsRemoved              int
	EnumSiblingsRetained             int
	ObservablesRemoved               int
	ObservablesRetained              int
	IssueCount                       int
	SuppressedIssueCount             int
	EventWritten                     bool
	ReportWritten                    bool
}

type fileSummaryReport struct {
	EventSource       string                              `json:"event_source"`
	RelativePath      string                              `json:"relative_path,omitempty"`
	Processed         bool                                `json:"processed"`
	Validation        *fileValidationSummaryReport        `json:"validation,omitempty"`
	Enrichment        *enrichmentSummaryReport            `json:"enrichment,omitempty"`
	EnrichmentRemoval *fileEnrichmentRemovalSummaryReport `json:"enrichment_removal,omitempty"`
	Issues            issueSummaryReport                  `json:"issues"`
	Outputs           fileOutputSummaryReport             `json:"outputs"`
}

type fileValidationSummaryReport struct {
	ErrorCount             int `json:"error_count"`
	WarningCount           int `json:"warning_count"`
	SuppressedErrorCount   int `json:"suppressed_error_count"`
	SuppressedWarningCount int `json:"suppressed_warning_count"`
}

type fileEnrichmentRemovalSummaryReport struct {
	EnumSiblingsRemoved  int `json:"enum_siblings_removed"`
	EnumSiblingsRetained int `json:"enum_siblings_retained"`
	ObservablesRemoved   int `json:"observables_removed"`
	ObservablesRetained  int `json:"observables_retained"`
}

type fileOutputSummaryReport struct {
	EventDestination  string `json:"event_destination,omitempty"`
	ReportDestination string `json:"report_destination,omitempty"`
	EventWritten      bool   `json:"event_written"`
	ReportWritten     bool   `json:"report_written"`
}

func buildSummaryReport(config processConfig, summary processSummary) summaryReport {
	report := summaryReport{
		SummaryVersion:      summaryVersion,
		Metadata:            buildSummaryMetadata(),
		SchemaPath:          summary.SchemaPath,
		EventFilesProcessed: summary.EventsProcessed,
		Issues: issueSummaryReport{
			ReportedCount:   summary.TotalIssueCount,
			SuppressedCount: summary.TotalSuppressedIssueCount,
		},
		Outputs: outputSummaryReport{
			EventsWritten:       summary.EventsWritten,
			ReportsWritten:      summary.ReportsWritten,
			EventWriteFailures:  summary.EventWriteFailures,
			ReportWriteFailures: summary.ReportWriteFailures,
		},
	}
	report.Files = make([]fileSummaryReport, len(summary.Files))
	for index, file := range summary.Files {
		report.Files[index] = buildFileSummaryReport(config, file)
	}
	if len(summary.InitializationIssues) != 0 || summary.SuppressedInitializationIssues != 0 {
		report.Initialization = &initializationSummaryReport{
			Issues:               summary.InitializationIssues,
			SuppressedIssueCount: summary.SuppressedInitializationIssues,
		}
	}
	if config.validate {
		report.Validation = &validationSummaryReport{
			EventsWithNoFindings:        summary.EventsWithNoValidationFindings,
			EventsWithWarningsOnly:      summary.EventsWithValidationWarningsOnly,
			EventsWithErrorsOnly:        summary.EventsWithValidationErrorsOnly,
			EventsWithWarningsAndErrors: summary.EventsWithValidationWarningsAndErrors,
			TotalErrorCount:             summary.TotalValidationErrorCount,
			TotalWarningCount:           summary.TotalValidationWarningCount,
			SuppressedErrorCount:        summary.TotalSuppressedValidationErrorCount,
			SuppressedWarningCount:      summary.TotalSuppressedValidationWarningCount,
		}
	}
	if config.enrich {
		report.Enrichment = &enrichmentSummaryReport{
			EnumSiblingsAdded: summary.TotalEnumSiblingsAdded,
			ObservablesAdded:  summary.TotalObservablesAdded,
		}
	}
	if config.enrichmentRemoval {
		report.EnrichmentRemoval = &enrichmentRemovalSummaryReport{
			EnumSiblingsRemoved:            summary.TotalEnumSiblingsRemoved,
			EnumSiblingsRetained:           summary.TotalEnumSiblingsRetained,
			ObservablesRemoved:             summary.TotalObservablesRemoved,
			ObservablesRetained:            summary.TotalObservablesRetained,
			EventsWithRetainedEnumSiblings: summary.EventsWithRetainedEnumSiblings,
			EventsWithRetainedObservables:  summary.EventsWithRetainedObservables,
		}
	}
	return report
}

func buildFileSummaryReport(config processConfig, file fileSummary) fileSummaryReport {
	report := fileSummaryReport{
		EventSource:  file.InputPath,
		RelativePath: file.RelativePath,
		Processed:    file.ProcessingCompleted,
		Issues: issueSummaryReport{
			ReportedCount:   file.IssueCount,
			SuppressedCount: file.SuppressedIssueCount,
		},
		Outputs: fileOutputSummaryReport{
			EventDestination:  file.EventPath,
			ReportDestination: file.ReportPath,
			EventWritten:      file.EventWritten,
			ReportWritten:     file.ReportWritten,
		},
	}
	if config.validate {
		report.Validation = &fileValidationSummaryReport{
			ErrorCount:             file.ValidationErrorCount,
			WarningCount:           file.ValidationWarningCount,
			SuppressedErrorCount:   file.SuppressedValidationErrorCount,
			SuppressedWarningCount: file.SuppressedValidationWarningCount,
		}
	}
	if config.enrich {
		report.Enrichment = &enrichmentSummaryReport{
			EnumSiblingsAdded: file.EnumSiblingsAdded,
			ObservablesAdded:  file.ObservablesAdded,
		}
	}
	if config.enrichmentRemoval {
		report.EnrichmentRemoval = &fileEnrichmentRemovalSummaryReport{
			EnumSiblingsRemoved:  file.EnumSiblingsRemoved,
			EnumSiblingsRetained: file.EnumSiblingsRetained,
			ObservablesRemoved:   file.ObservablesRemoved,
			ObservablesRetained:  file.ObservablesRetained,
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

func humanSummaryWithMetadata(report summaryReport, width int) string {
	tool := report.Metadata.Tool
	value := fmt.Sprintf("%s %s %s/%s %s\n\n%s",
		tool.Name,
		tool.Version,
		tool.Platform.OS,
		tool.Platform.Architecture,
		tool.GoVersion,
		strings.TrimSuffix(humanSummary(report), "\n"),
	)
	var output bytes.Buffer
	writeWrappedLines(&output, value, width)
	return output.String()
}

// summaryEntry indents a summary statistic line at helpEntryIndent, the same top-level indent the help
// system uses for its entries, so the two never drift apart.
func summaryEntry(format string, args ...any) string {
	return strings.Repeat(" ", helpEntryIndent) + fmt.Sprintf(format, args...)
}

func humanSummary(report summaryReport) string {
	lines := []string{fmt.Sprintf("Event files processed: %d", report.EventFilesProcessed)}
	if report.Initialization != nil {
		lines = append(lines, "Initialization issues:")
		for _, found := range report.Initialization.Issues {
			lines = append(lines, summaryEntry("%s: %s", found.Code, found.Message))
		}
		lines = append(lines,
			summaryEntry("Reported: %d", len(report.Initialization.Issues)),
			summaryEntry("Suppressed: %d", report.Initialization.SuppressedIssueCount),
		)
	}
	if report.Validation != nil {
		lines = append(lines,
			"Validation:",
			summaryEntry("Events with no errors or warnings: %d", report.Validation.EventsWithNoFindings),
			summaryEntry("Events with warnings only: %d", report.Validation.EventsWithWarningsOnly),
			summaryEntry("Events with errors only: %d", report.Validation.EventsWithErrorsOnly),
			summaryEntry("Events with warnings and errors: %d", report.Validation.EventsWithWarningsAndErrors),
			summaryEntry("Errors reported: %d", report.Validation.TotalErrorCount),
			summaryEntry("Warnings reported: %d", report.Validation.TotalWarningCount),
			summaryEntry("Errors suppressed: %d", report.Validation.SuppressedErrorCount),
			summaryEntry("Warnings suppressed: %d", report.Validation.SuppressedWarningCount),
		)
	}
	if report.Enrichment != nil {
		lines = append(lines,
			"Enrichment:",
			summaryEntry("Enum siblings added: %d", report.Enrichment.EnumSiblingsAdded),
			summaryEntry("Observables added: %d", report.Enrichment.ObservablesAdded),
		)
	}
	if report.EnrichmentRemoval != nil {
		lines = append(lines,
			"Enrichment removal:",
			summaryEntry("Enum siblings removed: %d", report.EnrichmentRemoval.EnumSiblingsRemoved),
			summaryEntry("Enum siblings retained: %d", report.EnrichmentRemoval.EnumSiblingsRetained),
			summaryEntry("Observables removed: %d", report.EnrichmentRemoval.ObservablesRemoved),
			summaryEntry("Observables retained: %d", report.EnrichmentRemoval.ObservablesRetained),
			summaryEntry(
				"Events with retained enum siblings: %d", report.EnrichmentRemoval.EventsWithRetainedEnumSiblings,
			),
			summaryEntry(
				"Events with retained observables: %d", report.EnrichmentRemoval.EventsWithRetainedObservables,
			),
		)
	}
	lines = append(lines,
		"Processing issues:",
		summaryEntry("Reported: %d", report.Issues.ReportedCount),
		summaryEntry("Suppressed: %d", report.Issues.SuppressedCount),
		"Outputs:",
		summaryEntry("Processed events written: %d", report.Outputs.EventsWritten),
		summaryEntry("Processing reports written: %d", report.Outputs.ReportsWritten),
	)
	return strings.Join(lines, "\n") + "\n"
}

func writeFailureDetails(w io.Writer, summary processSummary) {
	for _, file := range summary.Files {
		if file.ParseError != "" {
			writef(w, "%q: parse error: %s\n", file.InputPath, file.ParseError)
		}
		if file.ProcessingError != "" {
			writef(w, "%q: processing error: %s\n", file.InputPath, file.ProcessingError)
		}
		if file.EventWriteError != "" {
			writef(w, "%q: event write error: %s\n", file.InputPath, file.EventWriteError)
		}
		if file.ReportWriteError != "" {
			writef(w, "%q: report write error: %s\n", file.InputPath, file.ReportWriteError)
		}
	}
}

func updateSummary(summary *processSummary, fileResult fileSummary, retainFileSummary bool) {
	if retainFileSummary || fileResult.failed() {
		summary.Files = append(summary.Files, fileResult)
	}
	if fileResult.ParseError != "" {
		summary.ParseFailures++
		return
	}
	if fileResult.ProcessingError != "" {
		summary.ProcessingFailures++
		return
	}
	if fileResult.ProcessingCompleted {
		summary.EventsProcessed++
	}
	summary.TotalValidationErrorCount += fileResult.ValidationErrorCount
	summary.TotalValidationWarningCount += fileResult.ValidationWarningCount
	summary.TotalSuppressedValidationErrorCount += fileResult.SuppressedValidationErrorCount
	summary.TotalSuppressedValidationWarningCount += fileResult.SuppressedValidationWarningCount
	switch {
	case fileResult.ValidationErrorCount > 0 && fileResult.ValidationWarningCount > 0:
		summary.EventsWithValidationWarningsAndErrors++
	case fileResult.ValidationErrorCount > 0:
		summary.EventsWithValidationErrorsOnly++
	case fileResult.ValidationWarningCount > 0:
		summary.EventsWithValidationWarningsOnly++
	default:
		summary.EventsWithNoValidationFindings++
	}
	summary.TotalEnumSiblingsAdded += fileResult.EnumSiblingsAdded
	summary.TotalObservablesAdded += fileResult.ObservablesAdded
	summary.TotalEnumSiblingsRemoved += fileResult.EnumSiblingsRemoved
	summary.TotalEnumSiblingsRetained += fileResult.EnumSiblingsRetained
	summary.TotalObservablesRemoved += fileResult.ObservablesRemoved
	summary.TotalObservablesRetained += fileResult.ObservablesRetained
	summary.TotalIssueCount += fileResult.IssueCount
	summary.TotalSuppressedIssueCount += fileResult.SuppressedIssueCount
	if fileResult.EventWriteError != "" {
		summary.EventWriteFailures++
	}
	if fileResult.ReportWriteError != "" {
		summary.ReportWriteFailures++
	}
	if fileResult.EventWritten {
		summary.EventsWritten++
	}
	if fileResult.ReportWritten {
		summary.ReportsWritten++
	}
	if fileResult.EnumSiblingsRetained > 0 {
		summary.EventsWithRetainedEnumSiblings++
	}
	if fileResult.ObservablesRetained > 0 {
		summary.EventsWithRetainedObservables++
	}
}
