# Roadmap

This file tracks unfinished or intentionally deferred work. Implemented architecture and design invariants are documented in [architecture.md](architecture.md).

## Validation And Schema

- Continue parity testing against OCSF Server's `validator2.ex` using the same event fixtures where practical. See [ocsf-server-validation.md](ocsf-server-validation.md).
- Consider validating schema references at load time, including referenced object names, dictionary type names, and observable type IDs. Any load-time validation must continue to accept lean compiled schemas that legitimately omit optional sections.
- Consider a separate bundle-processing API if library consumers need OCSF bundle validation. `ProcessEvent` should remain a single-event operation.

## Event Processing Performance

- Add representative benchmarks and allocation profiles for validation-only, enrichment-only, combined processing, and observable-heavy events. Include valid events and malformed-input cases so optimizations do not merely move cost into failure handling.
- Compile type regular expressions when a validation-enabled `EventProcessorPipeline` is constructed instead of calling `regexp.Compile` for every matching string value. Store the compiled expressions and compilation errors as immutable processor-owned data so concurrent event processing needs no synchronization, enrichment-only processors do no unused work, and invalid expressions still produce useful schema-error diagnostics.
- Precompute immutable schema traversal metadata where practical, including sorted attribute and constraint names and parsed constraint paths. The current traversal filters attributes into a new map and sorts schema-defined names for each event and nested object.
- Avoid collecting and sorting every key merely to detect unknown attributes. Collect only unknown keys and sort that smaller set when deterministic diagnostic ordering is required.
- Benchmark observable path parsing and resolution. If repeated observable names are material, prefer allocation-reduced parsing or a bounded concurrency-safe cache owned by the processor; never use an unbounded cache for attacker-controlled names.
- Benchmark scalar allowed-value comparisons and path construction. Consider replacing general `reflect.DeepEqual` and repeated path-string allocation only where profiles show a meaningful pipeline cost.

## Bulk JSON Lines

The single-event CLI already allows JSON-valued event and report outputs to share stdout in deterministic order. Compact output is JSON Lines, while `--pretty-json` produces a whitespace-separated JSON stream. This section concerns a future bulk JSONL input and output mode rather than that existing stdout multiplexing.

Input options to consider:

- `--events-jsonl FILE | -`
- `--events-jsonl-dir DIR`

Output options to consider:

- `--event-jsonl-output FILE | -`
- A common JSONL output directory or bundle convention for processed events and processor reports.

Design constraints:

- File-based JSONL is a first-class use case for capturing events for testing, replay, and CI fixtures.
- Streaming should default to stdin and stdout only when the destinations are unambiguous.
- Bulk output must define how event and aggregate report records are identified while preserving the existing event-then-report ordering.
- Machine-readable records must not use stderr.
- Stderr remains reserved for errors and failure diagnostics.
- JSONL output is compact, one record per line; `--pretty-json` does not apply.
- Preserve the aggregate per-event report model when validation and enrichment removal are selected together.
- Define malformed-line behavior, line-number reporting, fail-fast versus continue behavior, and summary counts before implementation.

## Distribution

- Add source-built Homebrew formulae through a shared `ocsf/homebrew-tap`, starting with `ocsf-toolkit` and later adding `ocsf-schema-compiler`. See [homebrew.md](homebrew.md).
- Consider release bottles after the source-built Homebrew formula is stable.
- Consider Apple notarization and Windows code signing only if the project gains the necessary funding, identities, and secret-management infrastructure.
- Add a `package-release` Makefile layer only if future release packaging needs behavior beyond the existing `package` and `package-dist` targets.

## Decisions To Preserve

- Keep `jsonish.Map`; it provides useful domain vocabulary with no conversion cost.
- Do not add generic input-size limits to the current library or local-file CLI. Reconsider limits for server, remote, or streaming modes.
- Preserve only safe relative paths beneath output directories. Absolute paths and paths containing `..` must not determine output paths outside the selected root.
