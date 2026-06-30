# Architecture

OCSF Toolkit provides a Go library and CLI for processing events with the lean compiled schema format produced by the OCSF Schema Compiler. The current event processes are enrichment and validation.

## Public Packages

- `eventschema` loads compiled schemas, configures event processes, and processes events.
- `jsonio` reads JSON objects while preserving numbers as `json.Number` and rejecting trailing JSON values.
- `jsonish` defines `Map`, the common JSON-object type used by the public API.

Implementation details that library consumers do not need belong under `internal/`.

## Compiled Schema

`eventschema.New` accepts compiler format version 1. It expects the default lean output from `ocsf-schema-compiler`, not browser-mode output or the uncompiled schema repository format.

The compiler has already resolved includes, inheritance, patches, dictionary attribute details, and profile-expanded attributes. The loader treats compiled class and object attributes as authoritative rather than reconstructing the uncompiled schema. Optional dictionary sections are normalized so lean schemas can omit them safely.

Classes are indexed by their signed 64-bit `class_uid`, and observable type captions are indexed when the schema is loaded. OCSF `int_t` and `long_t` values are treated as signed 64-bit integers. Type constraints and regular expressions are resolved when the corresponding values are validated.

Loaded `Schema` values are immutable and safe for concurrent use.

## Event Processor Construction

Callers construct process descriptions with `NewEnrichment` and `NewValidation`, then pass them to `Schema.NewEventProcessor`:

```go
processor := schema.NewEventProcessor(
	eventschema.NewEnrichment(),
	eventschema.NewValidation(),
)
```

Options belong to the process they configure. Enrichment adds enum siblings and observables by default; callers can disable either behavior with `WithAddEnumSiblings(false)` or `WithAddObservables(false)`. Validation reports missing recommended attributes only when `WithWarnOnMissingRecommended()` is supplied.

`EventProcess` and its option interfaces are intentionally sealed. They provide a small construction API without exposing the internal visitor protocol.

Validation factories are always placed after mutating process factories, regardless of the order passed to `NewEventProcessor`. This guarantees that validation observes enrichment and any future event mutation. New mutating processes must preserve validation as the final phase.

Constructed `EventProcessor` values contain immutable process configuration and are safe for concurrent use when each call receives a distinct event map.

## Single-Pass Visitor

Each `ProcessEvent` call creates a fresh `processingContext` and performs one recursive schema-guided walk. Internal visitors receive hooks at the class, object, attribute, completed-item, and completed-event levels.

The shared walker owns traversal, profile filtering, object lookup, array handling, and path construction. Individual visitors perform process-specific work:

- Enrichment adds missing enum siblings, gathers schema-defined observables, and writes generated observables at the end of the event.
- Validation checks requirements, unknown attributes, types, enum values and siblings, deprecations, constraints, schema version, `type_uid`, profiles, and observable references.

A visitor can inspect more deeply when its behavior requires it, but traversal remains centralized. This avoids separate full enrichment and validation walks while keeping their logic in separate source files.

If `class_uid` is missing, has the wrong type, or does not identify a compiled class, validation records the corresponding issue and the processing context stops before class-scoped traversal. Recoverable validation failures are accumulated rather than stopping processing.

## Mutation And Results

`ProcessEvent` mutates its `jsonish.Map` argument in place when enrichment or another mutating process is enabled. Processing is not transactional. If it returns a Go error, the map may already be partially modified; callers that need the original event must deep-copy it first.

OCSF validation failures are returned in `ProcessingResult.Validation`, not as Go errors. A Go error means the processor could not operate on the supplied input. `ProcessingIssue.Code` is a stable machine-readable identifier intended for searching, grouping, metrics, and structured logs; `Message` is human-readable.

The event map and its nested maps and slices must not be accessed concurrently during processing. Separate events may be processed concurrently by the same processor.

## Numeric Values

Validation accepts normal Go numeric values from non-JSON sources. For JSON, `json.Number` is preferred because decoding directly to `float64` can lose integer precision. The `jsonio` package enables `json.Decoder.UseNumber()` for this reason.

Integral validation rejects non-integral values and applies signed 64-bit bounds where required. Numeric range constraints are inclusive.

## CLI Boundary

`cmd/ocsf-toolkit` owns filesystem and command-line concerns: selecting input files, mapping output trees, preventing unintended overwrites, atomic in-place enrichment, summary formatting, and exit codes. These policies do not belong in the event-processing library.

The CLI may process one file or walk a directory tree, but each JSON object is still passed independently to `ProcessEvent`. Directory outputs preserve safe paths relative to the input root. A single input path that is absolute or contains `..` is reduced to its basename when written under an output directory so it cannot escape that directory.

Enrichment and validation complete before output decisions are made for an event. `--skip-invalid-output` can therefore suppress non-validation output for events with validation errors without changing library semantics.

## Design Invariants

- Validation remains the final event-processing phase.
- Enrichment remains in-place and non-transactional unless the public contract is deliberately changed.
- OCSF integer types use signed 64-bit semantics.
- `jsonish.Map` remains the public JSON-object type.
- Validation issues remain data; only processing failures become Go errors.
- Filesystem overwrite and output-path policy remain in the CLI layer.
- The library and CLI do not impose a general input-size limit. Callers and deployment environments may impose their own limits.
