package enrichment

// ObservableDeduplication selects which generated observables are deduplicated during enrichment.
type ObservableDeduplication string

const (
	// ObservableDeduplicationIgnored disables observable deduplication.
	ObservableDeduplicationIgnored ObservableDeduplication = "ignored"
	// ObservableDeduplicationGenerated deduplicates generated observables against earlier generated observables.
	ObservableDeduplicationGenerated ObservableDeduplication = "generated"
)

// Valid reports whether the observable deduplication mode is supported.
func (mode ObservableDeduplication) Valid() bool {
	switch mode {
	case ObservableDeduplicationIgnored, ObservableDeduplicationGenerated:
		return true
	default:
		return false
	}
}
