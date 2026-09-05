package main

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"

	"github.com/ocsf/ocsf-toolkit/eventpipeline"
	"github.com/ocsf/ocsf-toolkit/schemaresult"
	"github.com/ocsf/ocsf-toolkit/validation"
)

const (
	summaryVersion    = 1
	summaryFormatText = "text"
	summaryFormatJSON = "json"
)

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
	Output              outputSummaryReport             `json:"output"`
}

type initializationSummaryReport struct {
	Issues []schemaresult.InitializationIssue `json:"issues,omitempty"`
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
	WarningOnlyEvents int `json:"warning_only_events"`
	ErrorEvents       int `json:"error_events"`
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
	ReportedCount int `json:"reported_count"`
}

type outputSummaryReport struct {
	EventsWritten  int `json:"events_written"`
	ReportsWritten int `json:"reports_written"`
}

func newSummaryReport(
	config processConfig,
	initializationIssues []schemaresult.InitializationIssue,
) *summaryReport {
	report := &summaryReport{
		SummaryVersion: summaryVersion,
		Metadata:       buildSummaryMetadata(),
		SchemaPath:     config.schemaPath,
	}
	if len(initializationIssues) != 0 {
		report.Initialization = &initializationSummaryReport{
			Issues: initializationIssues,
		}
	}
	if config.validate {
		report.Validation = &validationSummaryReport{}
	}
	if config.enrich {
		report.Enrichment = &enrichmentSummaryReport{}
	}
	if config.enrichmentRemoval {
		report.EnrichmentRemoval = &enrichmentRemovalSummaryReport{}
	}
	return report
}

func (report *summaryReport) addProcessingResult(result eventpipeline.ProcessingResult) bool {
	report.EventFilesProcessed++
	report.Issues.ReportedCount += len(result.Issues())

	validationErrorCount := 0
	if report.Validation != nil {
		validationResult := result.Validation()
		validationErrorCount = validationResult.Count(validation.LevelError)
		switch {
		case validationErrorCount > 0:
			report.Validation.ErrorEvents++
		case validationResult.Count(validation.LevelWarning) > 0:
			report.Validation.WarningOnlyEvents++
		}
	}
	if report.Enrichment != nil {
		enrichmentResult := result.Enrichment()
		report.Enrichment.EnumSiblingsAdded += enrichmentResult.EnumSiblingsAdded
		report.Enrichment.ObservablesAdded += enrichmentResult.ObservablesAdded
	}
	if report.EnrichmentRemoval != nil {
		removalResult := result.EnrichmentRemoval()
		report.EnrichmentRemoval.EnumSiblingsRemoved += removalResult.EnumSiblingsRemoved
		report.EnrichmentRemoval.EnumSiblingsRetained += removalResult.EnumSiblingsRetained
		report.EnrichmentRemoval.ObservablesRemoved += removalResult.ObservablesRemoved
		report.EnrichmentRemoval.ObservablesRetained += removalResult.ObservablesRetained
		if removalResult.EnumSiblingsRetained > 0 {
			report.EnrichmentRemoval.EventsWithRetainedEnumSiblings++
		}
		if removalResult.ObservablesRetained > 0 {
			report.EnrichmentRemoval.EventsWithRetainedObservables++
		}
	}
	return validationErrorCount > 0
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
		)
	}
	if report.Validation != nil {
		lines = append(lines,
			"Validation:",
			summaryEntry("Events with warnings only: %d", report.Validation.WarningOnlyEvents),
			summaryEntry("Events with errors: %d", report.Validation.ErrorEvents),
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
		"Output:",
		summaryEntry("Processed events written: %d", report.Output.EventsWritten),
		summaryEntry("Processing reports written: %d", report.Output.ReportsWritten),
	)
	return strings.Join(lines, "\n") + "\n"
}
