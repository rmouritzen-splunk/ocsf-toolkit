# Architecture

OCSF Toolkit provides a Go library and CLI for processing events with the lean compiled schema format produced by the OCSF Schema Compiler. The current event processors perform enrichment, enrichment removal, and validation.

## Public Packages

- `eventschema` loads compiled schemas, configures event processor pipelines, and processes events.
- `jsonio` reads JSON objects while preserving numbers as `json.Number` and rejecting trailing JSON values.
- `jsonish` defines `Map`, the common JSON-object type used by the public API.

Implementation details that library consumers do not need belong under `internal/`.

## Compiled Schema

`eventschema.New` accepts compiler format version 1. It expects the default lean output from `ocsf-schema-compiler`, not browser-mode output or the uncompiled schema repository format.

The compiler has already resolved includes, inheritance, patches, dictionary attribute details, and profile-expanded attributes. The loader treats compiled class and object attributes as authoritative rather than reconstructing the uncompiled schema. Optional dictionary sections are normalized so lean schemas can omit them safely. The compiled schema version is required and must use the supported semantic-version form before event processing can be configured.

Classes are indexed by their signed 64-bit `class_uid`, and observable type captions are indexed when the schema is loaded. OCSF `int_t` and `long_t` values are treated as signed 64-bit integers. Type constraints and regular expressions are resolved when the corresponding values are validated.

Loaded `Schema` values are immutable and safe for concurrent use.

## Event Processor Construction

Callers construct processors with `NewEnrichment`, `NewEnrichmentRemoval`, and `NewValidation`, then pass them to `Schema.NewEventProcessorPipeline`:

```go
pipeline, err := schema.NewEventProcessorPipeline(
	eventschema.NewEnrichment(),
	eventschema.NewValidation(),
)
```

Options belong to the processor they configure. Enrichment adds enum siblings and observables by default; callers can disable either behavior with `WithAddEnumSiblings(false)` or `WithAddObservables(false)`. Validation reports missing recommended attributes only when `WithWarnOnMissingRecommended()` is supplied.

Enrichment removal safely removes scalar integral enum siblings and observables by default. Safe removal preserves values that cannot be proven redundant. Callers can disable either category with `WithRemoveEnumSiblings(false)` or `WithRemoveObservables(false)`, or explicitly request destructive behavior with `WithForceRemoveEnumSiblings()` and `WithForceRemoveObservables()`. Forced observable removal deletes the entire `observables` attribute without analyzing individual entries. Enum sibling arrays are legacy forms and remain untouched.

`EventProcessor` and its option interfaces are intentionally sealed. They provide a small construction API without exposing the internal visitor protocol.

`NewEventProcessorPipeline` is the authoritative validator for processing semantics. Each processor retains an ordered, typed pipeline registration so validation can report processor-specific and cross-processor problems without losing attribution. Construction returns an aggregate error containing every detected empty or no-op configuration, duplicate processor, retain/force conflict, and attempt to add and remove the same enrichment category. The CLI additionally validates flag relationships so it can identify invalid flags directly; path, output, overwrite, and traversal rules remain CLI concerns.

Validation factories are always placed after mutating processor factories, regardless of the order passed to `NewEventProcessorPipeline`. This guarantees that validation observes enrichment and any future event mutation. New mutating processors must preserve validation as the final phase.

Constructed `EventProcessorPipeline` values contain immutable processor configuration and are safe for concurrent use when each call receives a distinct event map.

## Single-Pass Visitor

Each `ProcessEvent` call creates a fresh `processingContext` and performs one recursive schema-guided walk. Internal visitors receive hooks at the class, object, attribute, completed-item, and completed-event levels.

The shared walker owns traversal, profile filtering, object lookup, array handling, and path construction. Individual visitors perform processor-specific work:

- Enrichment adds missing enum siblings, gathers schema-defined observables, and writes generated observables at the end of the event.
- Enrichment removal analyzes and filters observables before class attribute traversal, then removes supported enum siblings before each class or object item is traversed. This ensures validation observes only the final retained content.
- Validation checks requirements, unknown attributes, types, enum values and siblings, deprecations, constraints, schema version, `type_uid`, profiles, and observable references.

A visitor can inspect more deeply when its behavior requires it, but traversal remains centralized. This avoids separate full enrichment and validation walks while keeping their logic in separate source files.

Observable reference analysis is shared by enrichment removal and validation and cached in the per-event processing context. The analyzer parses bare, `[]`, `[*]`, numeric index, and `$`-rooted paths; checks the reference against the active compiled class; resolves it against actual event content; compares scalar values after JSON-compatible string conversion; and distinguishes value-bearing scalar observables from valueless object observables. An explicit null observable value matches either an explicit null or a missing branch at its schema-valid event path, reflecting OCSF's equivalence of null and missing values. Enrichment removal uses the analysis to decide which entries are redundant, while validation reports invalid references and values. When both processors are enabled, analysis occurs before mutation and validation reuses the cached result.

An attribute used directly with `object_type: "object"` is the OCSF convention for an open-ended JSON-like object, so unknown nested keys are allowed. All event classes and concrete object types remain closed against their active compiled attributes, including when profile filtering leaves no active attributes. A concrete object may still gain inherited attributes when their profiles are active because the compiler flattens those profile annotations onto the derived object.

Mutating processors use a shared internal diagnostic representation when malformed event content prevents requested work. Each processor maps that diagnostic into a phase-specific `ProcessingIssue`; validation separately maps retained invalid content into validation errors or warnings. Enrichment diagnostics are intentionally narrow: they cover enum siblings or observables that could not be added, enum ID 99 siblings synthesized from the schema caption, and generated observable duplicates that were skipped, while unrelated event validity remains the validator's responsibility. Generated observables preserve existing entries and append in traversal order after semantic duplicate suppression. Duplicate identity uses the exact observable name, an integral-equivalent type ID, and the distinction among an omitted, null, or exact string value; derived type captions and unrelated fields are ignored. After class resolution succeeds, observable enrichment and removal delete the `observables` attribute when their work leaves it as an empty array.

If `class_uid` is missing, has the wrong type, or does not identify a compiled class, validation records the corresponding issue and the processing context stops all schema-dependent processing before class-scoped traversal or completed-event callbacks. Forced observable removal is the deliberate exception because deleting the complete top-level attribute requires no class schema or entry analysis. Recoverable validation failures after class resolution are accumulated rather than stopping processing.

## Mutation And Results

`ProcessEvent` mutates its `jsonish.Map` argument in place when enrichment or another mutating processor is enabled. Processing is not transactional. If it returns a Go error, the map may already be partially modified; callers that need the original event must deep-copy it first.

OCSF object attributes with an explicit null value are treated as missing throughout validation, enrichment, and enrichment removal. This applies to requirements, constraints, unknown-attribute checks, enum siblings, and the top-level `observables` attribute. A null array element is not missing and fails validation because no OCSF array element type permits null. Observable entries are the format-driven exception: an omitted `value` denotes an object observable, while an explicit null `value` denotes a scalar observable referring to null or missing event content.

OCSF validation failures are returned in `ProcessingResult.Validation`, not as Go errors. A Go error means the processor could not operate on the supplied input. `ProcessingIssue.Code` is a stable machine-readable identifier intended for searching, grouping, metrics, and structured logs; `Message` is human-readable.

The event map and its nested maps and slices must not be accessed concurrently during processing. Separate events may be processed concurrently by the same processor.

## Numeric Values

Validation accepts normal Go numeric values from non-JSON sources. For JSON, `json.Number` is preferred because decoding directly to `float64` can lose integer precision. The `jsonio` package enables `json.Decoder.UseNumber()` for this reason.

Array attributes accept JSON-native `[]any` values as well as typed Go slices from programmatic and non-JSON event sources. JSON-native forms retain their fast paths; other accepted array containers are adapted only when encountered, and each element still undergoes ordinary schema validation.

Integral validation rejects non-integral values and applies signed 64-bit bounds where required. Numeric range constraints are inclusive.

## CLI Boundary

`cmd/ocsf-toolkit` owns filesystem and command-line concerns: selecting input files, mapping output trees, preventing unintended overwrites, summary formatting, and exit codes. These policies do not belong in the event-processing library. Input event files are never modified.

All directory-mode outputs share one `--output-dir` root with fixed `events/` and `reports/` namespaces. Both preserve input-relative paths. Every processing action produces a report so enrichment issues are not discarded. Each report aggregates the selected processor results for one source event and records `event_source` plus `event_destination` when a processed event was written. Single-event mode may instead select explicit event and report files and never chooses implicit destinations, including when stdin is the event source. This prevents event/report filename collisions without multiplying directory controls as processors are added.

Before processing begins, the CLI resolves and validates command-wide input and output roots plus explicit single-event destinations. Directory processing then streams files directly from `filepath.WalkDir`; it does not retain a full list of event paths or calculated destinations. Output paths are derived from each safe input-relative path beneath the fixed output namespaces.

Input and output directory trees must be disjoint after symlink resolution, including when the selected output directory does not exist yet. Existing paths and ancestors are also compared by filesystem identity, allowing aliases to be rejected without detecting or assuming filesystem case-sensitivity rules. The selected output root itself may be a symlink because the user chose it explicitly, but existing output namespaces may contain only regular files and actual directories; symbolic links, Windows junctions, and other special filesystem entries are rejected. Without `--overwrite`, a directory-mode output root must be nonexistent or empty. Output is staged in a temporary file and installed by rename or hard link when the filesystem supports it. A filesystem without hard-link support uses an exclusive-create copy fallback that preserves the no-overwrite guarantee but cannot make the completed file visible atomically. Replacement fails unless `--overwrite` is selected. Command outputs are also claimed as they are written so two path spellings that identify the same filesystem file cannot overwrite one another.

The CLI may process one file or walk a directory tree, but each JSON object is still passed independently to `ProcessEvent`. Directory outputs preserve paths that `filepath.IsLocal` identifies as safe relative paths. A single input path that is absolute, rooted, drive-relative, reserved by the host filesystem, or contains `..` is reduced to a safe filename when written under an output directory so it cannot escape that directory or select a special device.

Single-event JSON outputs may share stdout. The processed event is written first when present, followed by its processing report. Compact output is JSON Lines; pretty output is a sequence of whitespace-separated JSON values suitable for a streaming decoder. `--pretty-json` applies to every JSON destination.

Summaries apply only to directory processing. A human-readable summary is written to stdout by default and may be suppressed with `--quiet`. Explicit human-readable and JSON summary destinations are additive, may be selected together with or without `--quiet`, and may include stdout; the human-readable summary is written first. stderr is reserved for errors and failure diagnostics.

All mutations and validation complete before output decisions are made for an event. `--skip-invalid-output` therefore writes only the validation report for an invalid event, suppressing both the processed event and non-validation report sections without changing library semantics.

## Design Invariants

- Validation remains the final event-processing phase.
- Event mutation remains in-place and non-transactional unless the public contract is deliberately changed.
- OCSF integer types use signed 64-bit semantics.
- `jsonish.Map` remains the public JSON-object type.
- Validation issues remain data; only processing failures become Go errors.
- Filesystem overwrite and output-path policy remain in the CLI layer.
- The library and CLI do not impose a general input-size limit. Callers and deployment environments may impose their own limits.
