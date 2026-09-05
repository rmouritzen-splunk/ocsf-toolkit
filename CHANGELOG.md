# Changelog

This changelog records user-visible changes by release, with the newest release first. OCSF Toolkit remains pre-1.0, so minor releases may intentionally change public APIs, defaults, and command-line behavior.

## [v0.9.0] - 2026-09-05

v0.9.0 contains significant intentional breaking changes to the Go library and CLI. The public library API has been reorganized and improved, with behavioral changes to processing issues, event validation, and observable enrichment. The CLI has a revised option model and simpler summary output. This release also substantially reduces memory allocation during event processing and includes numerous performance optimizations.

OCSF Toolkit follows [Semantic Versioning](https://semver.org/). While the project remains pre-1.0, minor releases may include intentional breaking changes as its public contracts mature. v0.9.0 uses that flexibility to establish APIs intended for v1.0 and reserves room for anticipated enhancements so they can generally be introduced compatibly. Before v1.0, the project will define its public API and compatibility surface; after that baseline is established, incompatible changes to the declared public API will require a major release. See the [version 1.0 compatibility gate](docs/roadmap.md#version-10-compatibility-gate).

### Migration guide

#### Library

In v0.8.0, callers created processors and passed them to a schema-owned pipeline:

```go
schema, err := eventschema.New("ocsf-schema.json")
if err != nil {
    return err
}

pipeline, err := schema.NewEventProcessorPipeline(
    eventschema.NewEnrichment(),
    eventschema.NewValidation(),
)
if err != nil {
    return err
}
```

In v0.9.0, callers create a schema, pass it to a top-level pipeline constructor, and select each mutation component explicitly:

```go
schema, initializationIssues, err := eventpipeline.NewSchema("ocsf-schema.json")
if err != nil {
    return err
}
_ = initializationIssues // Log or otherwise handle nonfatal initialization issues.

pipeline, err := eventpipeline.NewPipeline(
    eventpipeline.WithSchema(schema),
    eventpipeline.WithEnumSiblings(enrichment.Add),
    eventpipeline.WithObservables(enrichment.Add),
    eventpipeline.WithValidation(),
)
if err != nil {
    return err
}
```

Calls to `ProcessEvent` remain similar after pipeline construction. See [Library Usage](README.md#library-usage) for schema-loading alternatives, processing-issue levels, control of which event validation issues to check, observable selection, path notation, deduplication, and result handling.

#### CLI

The most common option migrations are:

| v0.8.0 | v0.9.0 |
|---|---|
| `--enrich --no-enum-siblings` | `--observables add` |
| `--enrich --no-observables` | `--enum-siblings add` |
| `--unenrich --retain-enum-siblings` | `--observables remove` |
| `--unenrich --retain-observables` | `--enum-siblings remove` |
| `--unenrich --force-remove-enum-siblings` | `--enum-siblings force-remove --observables remove` |
| `--unenrich --force-remove-observables` | `--enum-siblings remove --observables force-remove` |
| `--warn-on-missing-recommended` | `--validation-level validation_attribute_recommended_missing=warning` |
| `--summary-file FILE` | `--summary FILE` |
| `--summary-json-file FILE` | `--summary FILE --summary-format json` |
| Default stdout summary, disabled with `--quiet` | No summary by default; use `--summary -` |
| `--skip-invalid-output` | Removed; validation findings no longer suppress selected output |

`--enrich` and `--unenrich` remain as shorthands for applying the same action to enum siblings and observables, and `--force-remove` is the corresponding new force-removal shorthand. These shorthands cannot be combined with each other or with `--enum-siblings` and `--observables`. The short form of `--unenrich` changed from `-u` to `-U`.

#### OCSF Server v2 validator

Beyond replacing an HTTP POST request to the [OCSF Server v2 validator](https://schema.ocsf.io/api/v2/validate) with the Go library or CLI, migrations should account for the additional validations, deeper validation checks, and greater control over validation in OCSF Toolkit:

- Adds richer profile validation checks: distinguishes present attributes that require inactive profiles from unknown attributes, identifies the profiles that enable them, and performs an appropriate shallow value check.
- Distinguishes malformed event versions from valid earlier versions and reports overflow when deriving the expected `type_uid`.
- Reports when reaching the recursive-object boundary makes validation incomplete instead of presenting the partial validation as complete.
- Deepens observable validation by resolving names against the actual event and verifying scalar values or object references instead of checking only whether a path exists in the schema.
- Expands observable path handling across simple, empty-bracket, wildcard, indexed, and JSONPath styles; adds optional preferred-notation findings and duplicate-identity validation. See [Observable Path Notation in Library Usage](README.md#observable-path-notation).
- Deepens constraint validation by resolving nested paths and treating null values as absent.
- Unlike the v2 validator, which accepts only decoded integers for `integer_t` and `long_t` and only decoded floating-point values for `float_t`, OCSF Toolkit accepts a floating-point or decimal representation for an integral type when its value is finite, mathematically integral, and within the signed 64-bit range; for example, `1`, `1.0`, and `1e0` can all satisfy an integral type. It also accepts integral representations for `float_t` using its `float64` semantics, including the ordinary rounding of integer values that `float64` cannot represent exactly. The Toolkit applies signed 64-bit bounds to both integral types and measures string `max_len` in Unicode code points.
- Allows changing each validation check's level from its default to ignored, warning, or error. The OCSF Server v2 validator allows only enabling warnings for missing recommended attributes.
- Changes validation code names for consistency, including a common `validation_` prefix. Integrations that match the v2 validator's error or warning type strings must use the corresponding Toolkit code names.

See [Comparison with the OCSF Server v2 validator](docs/ocsf-server-v2-validator-comparison.md) for details and important behavioral qualifications.

### Changes since v0.8.0

#### New capabilities

- Added the `eventpipeline` API for constructing reusable schemas and pipelines, selecting enum-sibling and observable actions independently, filtering generated observables by type ID, choosing observable path notation, and processing distinct events concurrently through one immutable pipeline. Schemas can be loaded from a file, an `fs.FS`, bytes, or an `io.Reader`; loaders return typed nonfatal initialization issues separately from fatal errors. See [Library Usage](README.md#library-usage) and [Architecture](docs/architecture.md).
- Added typed `enrichment`, `issue`, `validation`, `eventresult`, `schemaresult`, and `pathstyle` packages. Per-code ignored, warning, and error policies now control processing issues and validation findings, with CLI discovery through `--list-issue-codes` and `--list-validation-codes`.
- Expanded schema-guided processing for OCSF profiles, scalar and array enum siblings, class-, object-, attribute-, dictionary-, and type-derived observables, all supported observable path notations, defined Go slice and array containers, exact signed 64-bit integral values, compiled-schema extensions, and the shared recursive-object traversal boundary. The repository examples and fixtures now use OCSF 1.9.0.
- Added safer CLI output planning, explicit text or JSON directory summaries, versioned machine-readable reports, aggregate configuration diagnostics, and stricter confinement and collision checks for directory and single-file output.

#### Go API and default changes

- Replaced the `eventschema` package with `eventpipeline`. The former `eventschema.New`, `Schema.NewEventProcessorPipeline`, `NewEnrichment`, `NewEnrichmentRemoval`, `NewValidation`, processor interfaces, and processor-specific option interfaces are removed. Create a concrete schema with `eventpipeline.NewSchema`, `NewSchemaFromFS`, `NewSchemaFromBytes`, or `NewSchemaFromReader`; then pass exactly one `eventpipeline.WithSchema` option to `eventpipeline.NewPipeline` and configure enum siblings and observables independently with `enrichment.Action` values. Omitted component options select `enrichment.None` rather than implicitly adding or removing both components, and zero `Schema` and `Pipeline` values are uninitialized handles that return errors when used.
- Pipeline construction now rejects repeated single-valued pipeline and nested validation options instead of letting the last option win. It returns the first configuration problem in deterministic order, with structured `eventpipeline.PipelineOptionError` implementations for repeated options and invalid issue- or validation-level rule ordering.
- Replaced the former public result structs with an opaque `eventpipeline.ProcessingResult` and typed accessors returning structs from `eventresult`. The result remains JSON-marshalable as a processor-section object but no longer supports JSON unmarshaling. Validation results now use one `findings` array whose entries carry typed levels and codes; processing issues use typed `source`, `code`, `message`, and `details` fields rather than the former phase/severity and value-bearing shape. Every non-nil `ProcessEvent` error is accompanied by a zero processing result; an issue configured at `issue.LevelError` is returned as an `*eventpipeline.ProcessingIssueError`.
- Replaced the opt-in `WithWarnOnMissingRecommended` validation option with the general `WithValidationLevel` and `WithAllValidationLevels` policy. `validation.LevelIgnored` is now a public level, missing-recommended and observable-duplicate validation default to ignored, and every validation code can be ignored. Processing issues use the parallel `WithIssueLevel` and `WithAllIssueLevels` policy; mandatory class-resolution and traversal-limit issues can be warning or error but cannot be ignored.
- Removed `jsonio.ReadArrayOfObjects`, `ReadArrayOfObjectsFS`, and `DecodeArrayOfObjects`. `jsonio.NewDecoder` remains the general number-preserving entry point for JSON shapes other than one object.
- Changed observable duplicate handling. v0.8.0 automatically made deduplication part of ordinary enrichment and reported each skipped generated duplicate. v0.9.0 appends every generated candidate by default, and `issue_observable_duplicate` and `validation_observable_duplicate` both default to `ignored`. Opt in to generated-only mutation with `eventpipeline.WithObservableDeduplication(enrichment.ObservableDeduplicationGenerated)` or `--deduplicate-observables generated`: it retains the first generated identity and silently omits only later generated matches. It never removes existing entries and never suppresses a generated candidate merely because an existing observable has the same identity. Duplicate reporting is configured independently; the issue can detect existing-existing, generated-existing, and generated-generated matches. See [existing observables and duplicates in Enrichment](docs/enrichment.md#existing-observables-and-duplicates).

#### CLI changes

- Replaced `--no-enum-siblings`, `--no-observables`, `--retain-enum-siblings`, `--retain-observables`, `--force-remove-enum-siblings`, and `--force-remove-observables` with `--enum-siblings [ACTION]` and `--observables [ACTION]`. Retained `--enrich` and `--unenrich` as whole-operation shorthands and added `--force-remove`; shorthands cannot be combined with each other or with component action flags. The short form of `--unenrich` changed from `-u` to `-U`, and `--version` gained `-v`.
- Added repeatable `--observable-id ID`, `--observable-path-notation STYLE`, and `--deduplicate-observables MODE` controls. Repeatable scalar selections use one value per occurrence rather than comma-separated lists, while flag values generally accept both `--flag value` and `--flag=value` spellings.
- Replaced `--warn-on-missing-recommended` with repeatable `--validation-level VALIDATION_CODE=LEVEL`, and added the parallel `--issue-level ISSUE_CODE=LEVEL`. Both accept `ignored`, `warning`, or `error` and an `all` rule that must precede specific codes. Issue policy applies to nonfatal schema initialization issues as well as event-processing issues, and an initialization issue promoted to error aborts setup. `--list-issue-codes` and `--list-validation-codes` print the available codes and defaults without loading a schema; the issue list also marks mandatory codes.
- Removed `--skip-invalid-output`. Validation findings no longer suppress requested event or report output; `--fail-on-validation-errors` affects only the final exit status after selected outputs have been written.
- Replaced `--summary-file`, `--summary-json-file`, and the default stdout summary controlled by `--quiet` with one opt-in `--summary FILE`; `--summary-format text|json` selects its representation. Directory processing is quiet by default, at most one of `--event-output`, `--report-output`, and `--summary` may use stdout, and a fatal directory-processing error does not write a normal or partial summary.
- Replaced the v0.8.0 machine-readable output shapes. Processing reports now carry `report_version`, `event_source`, optional `event_destination`, typed processor sections, and the new finding and issue shapes. JSON directory summaries carry `summary_version` and aggregate processor, issue, and output counts; they no longer include per-file result records. Successfully parsed invocations report independent configuration problems together and exit with status 2, while parsing, filesystem, schema-loading, layout, and processing failures remain fail-fast.

#### Behavioral corrections

- Treats a missing map attribute and a nil-valued map attribute as the same logical absence throughout class resolution, profiles, enrichment, removal, observable resolution, constraints, and validation. Nil array elements remain present invalid values.
- Requires successful `class_uid` resolution before every mutation, including force removal, and reports mandatory processing issues for class-resolution failure or limited recursive traversal even when validation is disabled.
- Runs enum-sibling mutation before observable mutation and validation after all mutation, independently of option order. Error-level processing issues stop processing and return a zero result without rolling back earlier in-place mutations.
- Correctly handles exact integral numeric representations, profile-restricted attributes, enum sibling pairing and ID 99 behavior, malformed observable destinations, observable array paths, and validation of the final generated-observable suffix.

#### Performance and memory improvements

- Reduced processing time and transient allocation in repository microbenchmarks using synthetic schemas and events to isolate enrichment, enrichment removal, validation, combined processing, and disabled-work paths.
- Reduced memory retained by loaded schemas and reusable validation metadata.
- Expanded microbenchmark and allocation-regression coverage, including concurrent processing and release-to-release comparisons.

#### Operational and compatibility notes

- The main module continues to require Go 1.25; complete development and release verification use Go 1.27 and also test the supported Go 1.25 baseline. Release builds use `GOEXPERIMENT=jsonv2`. With JSON v2, duplicate names in one JSON object are rejected; ordinary source builds without that experiment use the active standard decoder's duplicate-name behavior.
- Release verification now runs formatting, lint, vulnerability, vet, import-formatting, compatibility, coverage, race, release-tag-selection, benchmark-argument, Make-variable-safety, and cross-platform build checks. Archives are built for every platform owned by the cross-platform build script and accompanied by `SHA256SUMS`.
- OCSF Toolkit remains pre-1.0. This release does not promise source, CLI, serialized-output, or behavioral compatibility with v0.8.0; the planned gate is tracked in [version 1.0 compatibility in the Roadmap](docs/roadmap.md#version-10-compatibility-gate).

## [v0.8.0] - 2026-07-11

- Initial public release.
- Added the `eventschema` Go API and `ocsf-toolkit` CLI for enriching, removing enrichment from, and validating OCSF events with a compiled schema.
- Added reusable, concurrency-safe schema pipelines with in-place event mutation, combined processor execution, and structured processing results.
- Added single-event, standard-input, and directory-tree processing with separate event and report outputs, text and JSON summaries, overwrite controls, path preservation, and output-tree safety checks.
- Added the `jsonish.Map` event representation and `jsonio` helpers that preserve JSON numbers as `json.Number` for exact OCSF integer validation.
- Established Go 1.25 as the minimum supported Go version and published release archives for macOS, Linux, and Windows on AMD64 and ARM64.
