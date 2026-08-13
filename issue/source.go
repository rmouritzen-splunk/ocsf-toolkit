package issue

import "github.com/ocsf/ocsf-toolkit/internal/coderegistry"

// Source identifies the part of event processing that reported an IssueCode.
type Source uint8

const (
	invalidSource Source = iota
	// SourceProcessing identifies an issue shared by the overall event-processing operation.
	SourceProcessing
	// SourceEnrichment identifies an issue reported while adding enrichment.
	SourceEnrichment
	// SourceEnrichmentRemoval identifies an issue reported while removing enrichment.
	SourceEnrichmentRemoval
	// SourceValidation identifies an issue reported while validating an event.
	SourceValidation
	sourceCount
)

var sourceInfos = [sourceCount]coderegistry.Info{
	SourceProcessing:        {Name: "processing"},
	SourceEnrichment:        {Name: "enrichment"},
	SourceEnrichmentRemoval: {Name: "enrichment_removal"},
	SourceValidation:        {Name: "validation"},
}

var sourceRegistry = coderegistry.New[Source]("issue source", sourceInfos[:])

// Sources returns every valid Source in processing order.
func Sources() []Source {
	return sourceRegistry.Codes()
}

// Valid reports whether source is defined by this toolkit version.
func (source Source) Valid() bool {
	return sourceRegistry.Valid(source)
}

// String returns the stable external representation of source, or an empty string for an invalid source.
func (source Source) String() string {
	return sourceRegistry.String(source)
}

// ParseSource resolves a stable external issue-source representation.
func ParseSource(value string) (Source, bool) {
	return sourceRegistry.Parse(value)
}

// MarshalText returns the stable external representation used by JSON encoders.
func (source Source) MarshalText() ([]byte, error) {
	return sourceRegistry.MarshalText(source)
}

// UnmarshalText resolves a stable external representation used by JSON decoders.
func (source *Source) UnmarshalText(text []byte) error {
	return sourceRegistry.UnmarshalText(text, source)
}
