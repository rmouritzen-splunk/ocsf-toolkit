# Roadmap

This file tracks unfinished or intentionally deferred work. Implemented architecture and design invariants are documented in [architecture.md](architecture.md).

## Validation and schema

### Item we definitely want to work on

- Support an immutable validation option and corresponding CLI control that specify which supported path styles are allowed for the observable `name` attribute. A resolvable observable name using no allowed style should produce a dedicated validation finding whose default level is warning; the existing validation suppression and level policies should remain applicable. This environment-specific restriction must not change which path styles the toolkit can parse or use for enrichment, and it must not apply to diagnostic paths or any event attribute other than observable `name`. A spelling compatible with more than one style, as paths without arrays can be, is allowed when any matching style is configured. Define how this allowed-style set composes with or replaces the existing single preferred-notation option before implementation.

### Items to consider

- Support loading compressed compiled schemas. JSON schema files are large and compress well; define the supported compression formats, whether detection uses file names or content signatures, how every schema input API exposes decompression, and how malformed or unexpectedly large decompressed input is reported without imposing an arbitrary library-wide schema-size limit.
- Consider a separate event bundle-processing API if library consumers need OCSF event bundle validation. `ProcessEvent` should remain a single-event operation. See [Bundling](https://github.com/ocsf/examples/tree/main/encodings/json#bundling).
- Consider an optional preflight utility for programmatically constructed events that detects cyclic maps and slices, unsupported Go values, and optionally excessive structural depth or node count before event processing. JSON-decoded events do not need the cycle and value-shape checks.
- Consider optional per-event limits on validation errors, validation warnings, processing issues, and generated observables. Limits should remain configurable rather than imposing arbitrary defaults, and should bound total event output rather than only individual arrays. See [Does the recursive-object boundary limit all event-processing resource use?](FAQ.md#does-the-recursive-object-boundary-limit-all-event-processing-resource-use) for the current behavior and rationale.
- Consider an immutable pipeline option and corresponding CLI flag that selects whether OCSF `integer_t` values use signed 32-bit or signed 64-bit bounds. Signed 64-bit should remain the default for backward compatibility and because OCSF does not normatively assign `integer_t` a narrower width; `long_t` remains signed 64-bit because the OCSF dictionary defines it as an 8-byte signed integer. Apply the selected `integer_t` policy consistently to class resolution, enums, primitive validation, constraints, schema-derived calculations, and every accepted Go or JSON numeric representation. Perform exact-integral conversion before the selected range check, and investigate how signed 32-bit mode should handle a compiled schema containing an `integer_t` class UID, enum key, range, or allowed value outside that mode's bounds.

## Multiple schema versions

Support processing a mixed-version event stream with multiple loaded compiled schemas in a future release. For example, one environment may contain events whose `metadata.version` values are `1.5.0` and `1.8.0`; selecting the corresponding compiled schema for each event should produce more accurate enrichment, enrichment removal, and validation than processing every event against one schema version.

The public API now constructs pipelines with `eventpipeline.NewPipeline` and supplies independently constructed, reusable schemas through `WithSchema`, inverting the earlier `Schema.NewPipeline` ownership model. The current implementation still selects one schema using last-option-wins semantics. To keep future support compatible throughout SemVer 1.x, `WithSchema` now reserves variadic `SchemaPipelineOption` values so a later release can add schema registration policy and multiple-schema dispatch without changing the function type or disturbing the single-schema use case.

Design and implementation requirements:

- Load and retain a bounded, immutable set of compiled schemas and construct reusable per-schema pipelines once rather than rebuilding a pipeline for each event.
- Select the schema from the event's `metadata.version` before schema-guided class resolution and processing. When `metadata.version` is missing, select the latest loaded schema by SemVer precedence using the toolkit's existing SemVer support. Define explicit error behavior for a null, malformed, unsupported, or unavailable explicit version rather than silently treating it as missing.
- Match schemas using both the core schema version and the versions of relevant extensions, not `metadata.version` alone. Compiled schemas describe their included extensions in the top-level `extensions` object. Events identify their extensions through `metadata.extensions` or the deprecated but still supported `metadata.extension`; inspect `metadata.extensions` first and, when it is absent, fall back to `metadata.extension`. Define exact matching and error behavior for missing, malformed, unsupported, unavailable, or contradictory extension metadata.
- Determine from the OCSF documentation and the history of `metadata.extensions` whether platform extensions must participate in schema identity. In particular, establish whether events are expected to report platform extensions when the core schema is compiled with them, or whether omitting them is the de facto event convention. Do not assume that platform extensions follow the same matching policy as non-platform extensions until this is resolved. Use `test/ocsf-schema-v1.9.0-aws-v1.1.0.json`, which combines OCSF 1.9.0 with AWS extension 1.1.0 and the core platform extensions, as an initial fixture.
- Decide whether selection requires an exact version match or may use a documented compatibility policy. Never silently select a schema whose compatibility with the event version has not been established.
- Reject duplicate or ambiguous schema registrations during pipeline construction. At minimum, passing the same schema more than once must fail; once schema identity is defined, independently loaded schemas with the same matching identity must also fail rather than making event dispatch depend on registration order.
- Make the selected schema version available in processing results and diagnostics so callers can explain which contract was applied.
- Define library and CLI loading interfaces for multiple schema files without making behavior depend on argument order, directory iteration order, or map iteration order. During initialization, compare loaded versions by SemVer precedence and retain a reference to the latest schema or its pipeline for the missing-version fallback; do not maintain a sorted schema collection when processing only requires exact lookup and that one latest reference.
- Preserve concurrency safety and hot-loop performance: version selection should use immutable precomputed indexes, and processing one event must not mutate schema or pipeline state shared with another version.
- Add black-box invariant tests covering mixed-version batches, unknown versions, identical class UIDs with version-specific definitions, deterministic selection, and concurrent processing across versions.
- Keep future bulk JSON Lines processing compatible with per-event schema selection rather than assigning one schema to an entire mixed-version input stream.

## Event processing performance

- Add a repeatable whole-pipeline benchmark that recursively represents every integral event value as an integer-spelled `json.Number`, an integral-float-spelled `json.Number`, an `int64`, and a `float64`. Verify that float-based variants preserve the original integer exactly and report or separate values outside exact `float64` representation instead of silently changing event semantics. Repeatedly execute a compact, synthetic, redistributable fixture set that preserves useful structural variety and numeric-density ranges without claiming that its event-shape frequencies represent a production stream or predict a particular user's throughput. Local non-redistributable event-shape sets may inform those shapes and local comparisons but must not contribute event content.
- After collecting enough release-baseline comparisons, decide whether stable runtime and bytes-per-operation regressions should become blocking CI failures. Allocation ceilings are already enforced by unit tests; runtime and retained schema/pipeline metadata are compared informationally against the newest reachable release tag on the same machine because absolute duration and heap thresholds are not portable across developer and CI hardware.
- Extend the event-processing benchmarks when new processors or materially different traversal patterns are added. Current coverage includes validation, enrichment, enrichment removal, combined processing, observable generation, nested arrays, malformed input, and repeated and rotating version strings.

## Testing and static analysis

- Evaluate Go statements not exercised by the unit tests and classify the uncovered paths. Add behavioral tests for meaningful ordinary and boundary behavior. For defensive paths that are impractical or unreliable to trigger locally—such as malformed compiled-schema states, uncommon operating-system failures, and similar exceptional conditions—use static control-flow analysis to verify that errors propagate to the appropriate public boundary and are reported with useful context. Record any path that cannot be verified confidently rather than adding artificial tests solely to increase the coverage percentage.

## JSON Lines support

Use [JSON Lines](https://jsonlines.org/) as the primary format name and contract. Toolkit output must also conform to the [NDJSON 1.0.0 specification](https://github.com/ndjson/ndjson-spec), and the parser must accept conforming NDJSON input. Use JSONL in CLI option names and documentation while accepting files regardless of whether their extension is `.jsonl` or `.ndjson`.

Add JSON Lines as an explicit input mode with separate homogeneous processed-event and processing-report output streams. Existing `--event` and `--events-dir` behavior must remain unchanged, and the input modes must be mutually exclusive.

Primary input option:

- `--events-jsonl FILE | -`

Output options:

- `--event-jsonl-output FILE | -`
- `--report-jsonl-output FILE | -`

Design constraints:

- File-based JSONL is a first-class use case for capturing events for testing, replay, and CI fixtures.
- `--events-jsonl -` reads from stdin. Input stdin and output stdout are independent, but at most one event, report, or summary output option may select stdout in one invocation.
- Input must be UTF-8 without a byte-order mark. Accept `\n` and `\r\n` line endings and a final record without a line terminator. Reject blank lines, comments, multiline values, and malformed JSON rather than silently skipping them.
- Each physical line must contain exactly one non-null JSON object, with ordinary JSON whitespace permitted around it. JSON Lines permits other JSON value types, but this application-level subset requires an object because each record is an OCSF event. Preserve JSON numbers as `json.Number`.
- When a line contains a valid non-object JSON value, report `Unexpected JSON value at line {line}; expect object, got {json_value_type}: {input_source}` using a one-based line number, a stable value-free type label, and the CLI's existing input-source display and quoting convention.
- Write compact UTF-8 JSON with one complete object followed by `\n` for every event and report record. `--pretty-json` does not apply to JSON Lines output.
- Event and report records use separate destinations; do not multiplex their different shapes into one positional stream.
- Machine-readable records must not use stderr.
- Stderr remains reserved for errors and failure diagnostics.
- Preserve the aggregate per-event report model when validation and enrichment removal are selected together.
- Before implementation, define fail-fast versus continue behavior for malformed or unprocessable events, whether file outputs are installed only after complete success or may retain a successful prefix, how event and report records correlate when one output cannot be produced, whether directory-style summaries apply, and whether a later directory-of-JSONL input mode is useful.

## Distribution

- Add source-built Homebrew formulae through a shared `ocsf/homebrew-tap`, starting with `ocsf-toolkit` and later adding `ocsf-schema-compiler`. See [homebrew.md](homebrew.md).
- Consider release bottles after the source-built Homebrew formula is stable.
- Consider Apple notarization and Windows code signing only if the project gains the necessary funding, identities, and secret-management infrastructure.
