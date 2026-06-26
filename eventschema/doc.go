// Package eventschema loads compiled OCSF schemas and builds processors for OCSF events.
//
// A processor can enrich events, validate events, or do both in one pass. ProcessEvent mutates
// events in place when enrichment is enabled and reports OCSF validation failures in the returned
// ProcessingResult.
package eventschema
