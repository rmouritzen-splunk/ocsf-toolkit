// Package eventschema loads compiled OCSF schemas and builds processors for OCSF events.
//
// A Pipeline enriches, removes enrichment from, or validates events in one pass, configured by
// options passed to Schema.NewPipeline. Schema and Pipeline values are safe for concurrent use
// after construction, but each ProcessEvent call must receive an event map that is not being
// accessed or mutated concurrently.
//
// ProcessEvent mutates events in place when enrichment, enrichment removal, or another mutating
// processor is enabled and reports OCSF validation failures in the returned ProcessingResult.
// Processing is not transactional: if ProcessEvent returns an error, the event may have already
// been partially modified. Callers that need to preserve the original event should deep-copy it
// before processing.
package eventschema
