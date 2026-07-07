package main

import (
	"fmt"
	"io"
	"runtime"
	"strings"
)

type processSummary struct {
	SchemaPath                       string        `json:"schema_path"`
	Processed                        int           `json:"processed"`
	ParseFailures                    int           `json:"parse_failures"`
	ProcessingFailures               int           `json:"processing_failures"`
	TotalValidationErrorCount        int           `json:"total_validation_error_count"`
	TotalValidationWarningCount      int           `json:"total_validation_warning_count"`
	EventWriteFailures               int           `json:"event_write_failures"`
	ReportWriteFailures              int           `json:"report_write_failures"`
	EventsWithValidationErrors       int           `json:"events_with_validation_errors"`
	EventsWithValidationWarningsOnly int           `json:"events_with_validation_warnings_only"`
	EventsWritten                    int           `json:"events_written"`
	ReportsWritten                   int           `json:"reports_written"`
	EventsSkipped                    int           `json:"events_skipped"`
	EventsWithRetainedEnumSiblings   int           `json:"events_with_retained_enum_siblings"`
	EventsWithRetainedObservables    int           `json:"events_with_retained_observables"`
	Files                            []fileSummary `json:"files"`
}

type summaryReport struct {
	Metadata            summaryMetadataReport           `json:"metadata"`
	SchemaPath          string                          `json:"schema_path"`
	EventFilesProcessed *int                            `json:"event_files_processed"`
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
	EventsWithErrors       *int `json:"events_with_errors,omitempty"`
	EventsWithWarningsOnly *int `json:"events_with_warnings_only,omitempty"`
	TotalErrorCount        *int `json:"total_error_count,omitempty"`
	TotalWarningCount      *int `json:"total_warning_count,omitempty"`
}

type eventProcessingSummaryReport struct {
	EventsWritten *int `json:"events_written,omitempty"`
	EventsSkipped *int `json:"events_skipped,omitempty"`
}

type enrichmentRemovalSummaryReport struct {
	EventsWithRetainedEnumSiblings *int `json:"events_with_retained_enum_siblings,omitempty"`
	EventsWithRetainedObservables  *int `json:"events_with_retained_observables,omitempty"`
}

type fileSummary struct {
	InputPath              string `json:"event_source"`
	RelativePath           string `json:"relative_path,omitempty"`
	Processed              bool   `json:"processed"`
	ParseError             string `json:"parse_error,omitempty"`
	ProcessingError        string `json:"processing_error,omitempty"`
	EventPath              string `json:"event_destination,omitempty"`
	EventWriteError        string `json:"event_write_error,omitempty"`
	ReportPath             string `json:"report_path,omitempty"`
	ReportWriteError       string `json:"report_write_error,omitempty"`
	ValidationErrorCount   int    `json:"validation_error_count"`
	ValidationWarningCount int    `json:"validation_warning_count"`
	EventWritten           bool   `json:"event_written"`
	ReportWritten          bool   `json:"report_written"`
	EventSkipped           bool   `json:"event_skipped"`
	EnumSiblingsRemoved    int    `json:"enum_siblings_removed"`
	EnumSiblingsRetained   int    `json:"enum_siblings_retained"`
	ObservablesRemoved     int    `json:"observables_removed"`
	ObservablesRetained    int    `json:"observables_retained"`
}

func buildSummaryReport(config processConfig, summary processSummary) summaryReport {
	report := summaryReport{
		Metadata:            buildSummaryMetadata(),
		SchemaPath:          summary.SchemaPath,
		EventFilesProcessed: ptrTo(summary.Processed),
		Files:               summary.Files,
	}
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
		if file.ReportWriteError != "" {
			writef(w, "%s: report write error: %s\n", file.InputPath, file.ReportWriteError)
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
	if fileResult.ReportWriteError != "" {
		summary.ReportWriteFailures++
	}
	if fileResult.EventWritten {
		summary.EventsWritten++
	}
	if fileResult.ReportWritten {
		summary.ReportsWritten++
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
