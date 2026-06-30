# Roadmap

This file tracks unfinished or intentionally deferred work. Implemented architecture and design invariants are documented in [architecture.md](architecture.md).

## Validation And Schema

- Continue parity testing against OCSF Server's `validator2.ex` using the same event fixtures where practical. See [ocsf-server-validation.md](ocsf-server-validation.md).
- Consider validating schema references at load time, including referenced object names, dictionary type names, and observable type IDs. Any load-time validation must continue to accept lean compiled schemas that legitimately omit optional sections.
- Consider a separate bundle-processing API if library consumers need OCSF bundle validation. `ProcessEvent` should remain a single-event operation.

## JSON Lines

Treat JSONL as a separate streaming mode rather than another encoding option for the current single-event and directory-file modes.

Input options to consider:

- `--events-jsonl FILE | -`
- `--events-jsonl-dir DIR`

Output options to consider:

- `--enriched-jsonl-output FILE | -`
- `--enriched-jsonl-output-dir DIR`
- `--validation-jsonl-output FILE | -`
- `--validation-jsonl-output-dir DIR`

Design constraints:

- File-based JSONL is a first-class use case for capturing events for testing, replay, and CI fixtures.
- Streaming should default to stdin and stdout only when the destinations are unambiguous.
- Enriched events and validation records require distinct destinations because they have different shapes.
- At most one machine-readable output may use stdout. Machine-readable records must not use stderr.
- Stderr remains for diagnostics and status and should remain suppressible with `--quiet`.
- JSONL output is compact, one record per line; `--pretty-json` does not apply.
- Decide whether requesting both processes requires explicit separate destinations.
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
