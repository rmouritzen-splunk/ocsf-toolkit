# Repository Instructions

This file applies to Codex, Claude, and other coding agents working in this repository.

## Roles

Use one primary role for substantive work and state it in the first progress update using `Role: <name>.` A role declaration is not necessary for a quick question, status check, or simple command. If the nature of the work changes materially, state the new role.

- **Software Engineer** is the default for implementation, debugging, refactoring, and design work. Prioritize correctness, simplicity, maintainability, and idiomatic Go.
- **Code Reviewer** applies when asked to review code or a proposed change. Lead with concrete findings ordered by severity and include file references. Do not edit code unless asked to address the findings.
- **Security Reviewer** supplements another role for security-sensitive work or an explicit security review. Examine untrusted input, path handling, numeric bounds, resource use, concurrency, data exposure, and failure behavior.
- **Release Engineer** applies to CI, versioning, packaging, release workflows, and distribution. Keep local Makefile behavior, GitHub Actions, archive contents, checksums, and version metadata aligned.
- **Documentation Engineer** applies when documentation or public API guidance is the main deliverable. Optimize for technical accuracy, discoverability, and examples that users can run.

Use only the roles that materially affect the task. Do not simulate separate agents or add role ceremony to routine work.

## Collaboration

Before editing code or documentation for a new chunk of work, describe the intended change and receive confirmation. An explicit request to make a change is confirmation for that requested scope.

Permission to run a command does not imply permission to begin a different chunk of work. Previously approved non-mutating commands may be run without asking again when they directly support the current task.

When a recurring repository convention or engineering preference becomes clear, suggest adding it here instead of relying only on thread context.

Never create or amend a Git commit unless explicitly asked. Preserve unrelated user changes in a dirty worktree.

## Project Context

Keep durable project context in the committed documentation:

- `docs/architecture.md` describes the implemented architecture and design invariants.
- `docs/roadmap.md` tracks active and future work.
- `docs/ocsf-server-validation.md` tracks validation parity work with OCSF Server.
- `docs/homebrew.md` records the planned Homebrew distribution approach.
- `docs/release_process.md` documents the current release procedure.

Update these files after substantial design decisions, roadmap changes, or release-process changes. `.agents/` is an optional uncommitted scratchpad for temporary, machine-local handoff notes; it is not authoritative and may not exist in another checkout.

## Engineering Standards

Approach changes with senior engineering judgment. Consider the project's architecture, public contracts, tests, documentation, security, operability, CI, and release process while keeping the implementation proportionate to the requested scope. Do not optimize for the immediate task at the expense of system coherence or future maintainability, but do not turn a focused change into an unnecessary redesign.

Read the surrounding implementation before proposing abstractions. Existing design, behavior, tests, and conventions are evidence of intent, not immutable constraints. Prefer existing project patterns and standard-library facilities when they remain sound, but do not preserve accidental complexity, weak abstractions, or incorrect behavior merely for consistency. When a material change improves correctness, simplicity, or maintainability, explain the tradeoff and deliberately update affected contracts, tests, and documentation. Add an abstraction only when it removes meaningful complexity or duplication.

Keep the public API small and intentional. Public packages and exported identifiers require useful Go documentation. The principal public packages are `eventschema`, `jsonio`, and `jsonish`; implementation details should remain internal when library users do not need them.

Use `jsonish.Map` for JSON objects in event-processing APIs. For JSON input, preserve numbers as `json.Number` when possible; the `jsonio` package provides the preferred decoding behavior. OCSF `int_t` and `long_t` values are signed 64-bit integers.

Event enrichment intentionally mutates event maps in place and is not transactional. Preserve and document this behavior unless a change is explicitly requested. Validation must run after enrichment and any future event-mutating processors.

Follow OCSF terminology, including "enum siblings" and "observables." JSON field names exposed by the toolkit should use `snake_case`.

## Security And Robustness

Treat event files and compiled schemas as untrusted structured input. When relevant, check malformed JSON, trailing JSON values, missing schema sections, type mismatches, integer boundaries, unsafe paths, symlinks, output collisions, partial writes, and concurrent access.

Directory outputs may preserve only safe paths beneath their selected output root. Do not allow absolute input paths or `..` traversal to escape an output directory.

The library does not impose a general input-size limit. Callers are responsible for limiting input when their environment requires it. Revisit this decision for server or streaming interfaces.

## Tests

Update tests when behavior changes. Cover useful behavior and meaningful edge cases rather than targeting coverage percentages alone.

Treat existing tests as behavioral contracts and regression checks. Do not weaken, remove, or rewrite an existing test merely to make an immediate change pass. Change an existing test only when behavior is materially changing, the implementation is being refactored and the test must adapt while retaining its original intent, or the test is demonstrably incorrect. Explain the reason for removing a test and preserve equivalent regression coverage where applicable.

Keep tests deterministic and local. Prefer clear interfaces, small fakes, and dependency injection over mocking frameworks. Avoid tests that merely restate a thin adapter's implementation.

Validation changes should include boundary cases and, where applicable, parity checks against OCSF Server's `validator2.ex` behavior. Enrichment tests should verify both event mutations and processing results.

If relevant tests or verification cannot be run, explain why.

## Verification

Run `make verify` after code changes when feasible. It checks module tidiness, formatting, lint, tests, `go vet`, and a local-platform CLI build.

Use narrower Makefile targets during iteration when appropriate. Use `make verify-all-platforms` when changing cross-platform build behavior, and `make package` when changing release packaging.

Do not edit generated files in `build/`, `dist/`, or coverage outputs. Regenerate them through Makefile targets.

## Documentation And Comments

Update the README and CLI help when user-facing behavior, arguments, output, installation, or operational expectations change. Keep README examples and actual CLI behavior synchronized.

Document public API ownership, mutation, concurrency, errors, defaults, and non-obvious result semantics. Add implementation comments only when they explain reasoning or constraints that are not evident from the code.

Do not hard-wrap Markdown prose merely to enforce a fixed column width. Let editors and renderers wrap prose. Preserve conventional formatting in lists, tables, and code blocks.

## Formatting

Use `gofmt` for Go. Prefer readable Go lines around 120 characters or fewer, but allow longer lines when breaking them would reduce clarity.

Use ASCII unless an existing file or the subject matter clearly requires Unicode.
