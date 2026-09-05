# Repository Instructions

This file applies to Codex, Claude, and other coding agents working in this repository.

## Roles

Use one primary role for substantive work and state it in the first progress update using `Role: <name>.` A role declaration is not necessary for a quick question, status check, or simple command. If the nature of the work changes materially, state the new role.

- **Software Engineer** is the default for implementation, debugging, refactoring, and design work. Prioritize correctness, simplicity, maintainability, and idiomatic Go.
- **Code Reviewer** applies when asked to review code or a proposed change. Unless the requester explicitly excludes regression analysis, also perform the Regression Reviewer role as a distinct review lens and report its analysis separately. Review for maintainability and simplicity, watching out for unnecessary complexity, alongside correctness and idiomatic use of the target programming languages (Go in this repository); flag accidental complexity, unnecessary abstraction, duplication, and designs that are difficult to evolve. Run `make check-all` (module tidiness, `gofmt`, `golangci-lint` — including line length via `lll` — `govulncheck`, `go vet`, and import formatting) and flag every finding; fix line-length findings by breaking at a natural seam (see Formatting) rather than an arbitrary cut, reserving `//nolint:lll` for cases with no natural seam, a hard syntax constraint (e.g. struct tags), or test code where splitting isn't worth the effort. Check findings against `docs/` for a documented rationale before flagging a pattern as suspicious, and include that rationale in the review. Verify a cited justification (e.g., re-run a cited benchmark) rather than trusting it at face value, and flag `docs/` itself as needing correction when a documented rationale or behavioral claim turns out to be wrong — agreement between code and docs does not excuse a real defect in either. Lead with concrete findings ordered by severity and include file references. Do not edit code unless asked to address the findings.
- **Regression Reviewer** applies as a distinct lens within every code review unless the requester explicitly excludes it, and independently when auditing current or historical changes for behavior that was accidentally lost or redefined. Keep its analysis separate from the general code-review analysis even when one person performs both roles. Look especially for an existing test expectation changed alongside production code, including cases where a downstream component's needs caused an upstream implementation and its test to change together. Establish the prior contract and its independent authority, inspect the production and test changes separately, and reproduce the behavior before and after the suspect change when feasible. Do not treat agreement among changed code, changed tests, and changed documentation as proof of correctness. Distinguish a confirmed regression from an intentional contract change, an implementation-only test adaptation, and a concurrent change that remains unproven; report the evidence for each classification.
- **Security Reviewer** supplements another role for security-sensitive work or an explicit security review. Examine untrusted input, path handling, numeric bounds, resource use, concurrency, data exposure, and failure behavior.
- **Release Engineer** applies to CI, versioning, packaging, release workflows, and distribution. Keep local Makefile behavior, GitHub Actions, archive contents, checksums, and version metadata aligned.
- **Documentation Engineer** applies when documentation or public API guidance is the main deliverable. Optimize for technical accuracy, discoverability, and examples that users can run.

Use only the roles that materially affect the task. Do not simulate separate agents or add role ceremony to routine work.

## Collaboration

Before editing code or documentation for a new chunk of work, describe the intended change and receive confirmation. An explicit request to make a change is confirmation for that requested scope.

Permission to run a command does not imply permission to begin a different chunk of work. Previously approved non-mutating commands may be run without asking again when they directly support the current task.

When a recurring repository convention or engineering preference becomes clear, suggest adding it here instead of relying only on thread context.

Strive to keep commits independently reviewable. Separate unrelated features, contract decisions, correctness fixes, mechanical refactors, and release or tooling changes when they can be reviewed and verified independently. This is not an absolute size rule: a larger cohesive commit is appropriate when splitting it would obscure the invariant being preserved or create misleading or inconsistent intermediate revisions.

Never create or amend a Git commit unless explicitly asked. Preserve unrelated user changes in a dirty worktree.

Never run `git push`, including a force push, under any circumstances, even after explicit confirmation in the moment. Leave commits local and tell the user what would need to be pushed; the user pushes it themselves.

## Project Context

Keep durable project context in the committed documentation:

- `docs/architecture.md` describes the implemented architecture and design invariants.
- `docs/engineering/project-invariants.md` records source-backed OCSF, compiled-schema, toolkit, and engineering invariants used to protect stable behavior.
- `docs/engineering/project-decisions.md` records deliberate design choices and the derived requirements that should inform long-lived tests.
- `docs/event-processing.md`, `docs/enrichment.md`, `docs/enrichment-removal.md`, and `docs/validation.md` describe language-neutral processing behavior for users and independent implementers.
- `docs/roadmap.md` tracks active and future work.
- `docs/ocsf-server-validation.md` retains historical validation-parity findings for OCSF Server; back-porting Toolkit behavior is not active work.
- `docs/homebrew.md` records the planned Homebrew distribution approach.
- `docs/release_process.md` documents the current release procedure.

Update these files after substantial design decisions, roadmap changes, or release-process changes. Use `.agents/` as the local scratchpad for temporary, machine-local plans, TODO lists, research, and handoff notes. It may also record repository-relevant context that must remain local, such as aggregate observations, local paths, or testing guidance for a non-open-source event corpus, without copying restricted corpus content into committed files. The directory is listed in `.gitignore`, may already exist with relevant local information, and should be inspected when beginning related work. Its contents are uncommitted and non-authoritative and may not exist in another checkout; durable public project decisions still belong in the committed documentation. Do not store credentials or other secrets there because `.gitignore` is not a security boundary.

Keep the language-neutral processing guides synchronized with behavioral changes to traversal, profiles, null handling, enrichment, enrichment removal, validation, observables, result semantics, or processor ordering. Describe logical OCSF behavior independently of JSON, Go maps, the internal visitor implementation, or another particular encoding or in-memory representation unless a limitation is intentionally specific to this toolkit.

## Engineering Standards

Approach changes with senior engineering judgment. Consider the project's architecture, public contracts, tests, documentation, security, operability, CI, and release process while keeping the implementation proportionate to the requested scope. Do not optimize for the immediate task at the expense of system coherence or future maintainability, but do not turn a focused change into an unnecessary redesign.

Read the surrounding implementation before proposing abstractions. Existing design, behavior, tests, and conventions are evidence of intent, not immutable constraints. Prefer existing project patterns and standard-library facilities when they remain sound, but do not preserve accidental complexity, weak abstractions, or incorrect behavior merely for consistency. When a material change improves correctness, simplicity, or maintainability, explain the tradeoff and deliberately update affected contracts, tests, and documentation. Add an abstraction only when it removes meaningful complexity or duplication.

Keep the public API small and intentional. Public packages and exported identifiers require useful Go documentation. The principal public packages are `eventpipeline`, `jsonio`, and `jsonish`; implementation details should remain internal when library users do not need them.

Use `jsonish.Map` for JSON objects in event-processing APIs. For JSON input, preserve numbers as `json.Number` when possible; the `jsonio` package provides the preferred decoding behavior. OCSF `integer_t` and `long_t` values are signed 64-bit integers.

For event attributes stored in `jsonish.Map`, treat `item[key] == nil` as logically absent and prefer a direct lookup followed by a nil check. This does not apply to array elements: a nil array element is present and has an illegal OCSF type. Use comma-`ok` only when physical map membership matters, such as deleting a present nil-valued attribute during removal.

Event enrichment intentionally mutates event maps in place and is not transactional. Preserve and document this behavior unless a change is explicitly requested. Validation must run after enrichment and any future event-mutating processors.

Treat `Pipeline.ProcessEvent` and the schema-guided visitor callbacks as a hot loop that may process tens of thousands of events per second. Construct pipelines once and keep schema data, processor configuration, compiled constraints, and other reusable metadata immutable and pipeline-owned. Per-event contexts should contain only event-specific mutable state and references to shared immutable data; do not copy processor collections or rebuild reusable metadata for each event. Avoid unnecessary heap allocations, temporary maps, slices, strings, reflection, and repeated parsing in the traversal path. Use representative benchmarks, allocation counts, and profiles to guide non-obvious optimizations, and preserve correctness, deterministic results, concurrency safety, and useful diagnostics.

Follow OCSF terminology, including "enum siblings" and "observables." JSON field names exposed by the toolkit should use `snake_case`.

## Security And Robustness

Treat event files and compiled schemas as untrusted structured input. When relevant, check malformed JSON, trailing JSON values, missing schema sections, type mismatches, integer boundaries, unsafe paths, symlinks, output collisions, partial writes, and concurrent access.

Directory outputs may preserve only safe paths beneath their selected output root. Do not allow absolute input paths or `..` traversal to escape an output directory.

The library does not impose a general input-size limit. Callers are responsible for limiting input when their environment requires it. Revisit this decision for server or streaming interfaces.

## Tests

Update tests when behavior changes. Cover useful behavior and meaningful edge cases rather than targeting coverage percentages alone.

For bug fixes and regressions, prefer a fail-first workflow: add a focused test that expresses the desired behavior, run it to confirm that it fails for the expected reason, then change the implementation to make it pass. After the fix, add coverage for other meaningful edge cases identified during the investigation.

Fail-first verification is mandatory when production behavior and its tests will both change. Make the test-only change first, run the focused test against the unchanged implementation, and report the expected failure before editing production code. Keep that failing test in place while implementing the fix. If a test added for a bug passes before the implementation changes, stop and investigate whether the test exercises the defect.

Never edit test files and production files concurrently in one step, patch, or tool call. This prohibition applies to every test class and every production change. Complete the change on one side, run the relevant tests, and record the expected pass or failure before editing the other side in a separate step.

Never change, remove, weaken, or invert an existing behavioral expectation merely to make changed production code pass. After production code has changed, a failing existing test is evidence of a regression unless an independent contract source proves otherwise; fix or revert the production code rather than editing the expectation. Passing tests are not evidence of correctness when their expectations were derived from the changed implementation.

A downstream component's inability to satisfy an upstream component's tested contract is evidence to investigate the downstream component, not authority to redefine the upstream behavior. Do not change the upstream implementation and its test to accommodate the downstream implementation. First establish the correct upstream behavior from an independent source such as an external specification, documented architecture, previously approved invariant, or explicit human decision; then fix the component that violates that behavior. If the independent authority requires changing the upstream contract, make and review that contract change separately before adapting downstream code.

Changing an existing expected value, accepted/rejected classification, error code, mutation assertion, or other externally observable outcome requires explicit human approval before editing the test. Present the old expectation, proposed new expectation, reason the behavior is intentionally changing, and the independent authority for that decision, such as a specification, documented contract, prior implementation, or maintainer direction. A code refactor, optimization, current implementation output, or desire to make CI pass is not authority to change a behavioral expectation.

Do not combine an approved test-contract change with its production implementation in one unreviewed step. Land or present the contract/test change as a distinct diff and obtain the required human decision before proceeding to production code. This checkpoint does not prohibit adding new regression coverage during a bug fix after the initial failing test has established the defect; those additional tests must preserve the same independently established contract.

Classify tests by the requirement they protect rather than treating every unit test alike:

- An external invariant test expresses behavior required by users, specifications, or the public contract. Exercise it through the public black-box boundary whenever practical. Mark it with a `TestInvariant...` name and a `// Invariant test:` comment stating the required property.
- An engineering invariant test expresses an internal architectural or operational requirement that must survive implementation changes, such as processor ordering, concurrency ownership, deterministic results, allocation ceilings, or path confinement. Mark it with a `TestEngineeringInvariant...` name and a `// Engineering invariant test:` comment stating the requirement.
- An implementation-detail test exercises the current internal mechanism, representation, helper decomposition, or edge behavior without independently establishing an external or engineering requirement. This is the only class that may be updated to follow an implementation change, but never concurrently or in the same editing step: either change the test first and observe it fail against the old implementation, or change the implementation first and observe the old test fail before updating the test in a separate step.

External and engineering invariant tests are protected contracts: never change, rename, remove, weaken, or invert them in the same change as production code. Any proposed change requires a separate test-only diff, independent contract evidence, and explicit human approval before any related production edit. A test may record the commit or defect that previously violated its invariant, but historical regression coverage does not replace classification by the requirement being protected. Run all marked invariant tests directly with `go test ./... -run Invariant` when changing shared behavior they cover.

When merging previously-separate implementations into a shared function, derive new tests' expected values from what each original implementation actually did, not from the merged code's own output — a test written that way only describes the refactor, and cannot catch a behavior change it introduced.

Treat existing tests as behavioral contracts and regression checks. Do not weaken, remove, or rewrite an existing test merely to make an immediate change pass. Change an existing test only when behavior is materially changing, the implementation is being refactored and the test must adapt while retaining its original intent, or the test is demonstrably incorrect. Explain the reason for removing a test and preserve equivalent regression coverage where applicable.

Treat performance and allocation ceilings as regression budgets, not targets to adjust around current work. Do not loosen, remove, or bypass a ceiling merely to make a change pass. Investigate the regression and optimize or justify the design first. Changing a ceiling requires explicit agreement from a human maintainer and should include benchmark evidence explaining the new budget.

Keep tests deterministic and local. Prefer clear interfaces, small fakes, and dependency injection over mocking frameworks. Avoid tests that merely restate a thin adapter's implementation.

Validation changes should include boundary cases and, where applicable, parity checks against OCSF Server's `validator2.ex` behavior. Enrichment tests should verify both event mutations and processing results.

If relevant tests or verification cannot be run, explain why.

## Verification

Run `make all` after code changes when feasible. It runs the complete checks, compatibility tests across the current and minimum Go toolchains and both JSON implementations, coverage and race tests, release-tag-selection and benchmark-argument tests, and builds the CLI for every supported platform. Run `make` or `make dev` during iteration for the standard checks, the current Go toolchain with JSON v2, and a local-platform CLI build; it is not a substitute for `make all` before treating work as done.

Run `make check-all` frequently during a session, not just at the end. It catches module tidiness, formatting, lint, vulnerability, vet, and import-formatting findings without running tests or builds.

Use narrower Makefile targets during iteration when appropriate. Use `make test-compatibility` for the complete toolchain/JSON compatibility matrix, `make test-coverage` for coverage, `make build-all-platforms` when changing cross-platform build behavior, and `make package` when changing release packaging. The cross-platform build script owns the supported-platform list; packaging must derive its inputs from the produced platform directories rather than duplicate that list or their contents.

Do not edit generated files in `build/`, `dist/`, or coverage outputs. Regenerate them through Makefile targets.

### Performance and memory verification

Run `go test ./eventpipeline -run '^$' -bench '.' -benchmem -benchtime 500ms -count 10` for a current-checkout snapshot of runtime, transient bytes per operation, and allocations per operation. Run `scripts/benchmark-compare.sh` for the preferred regression analysis: it benchmarks the current checkout and the newest eligible release tag on the same machine with the same Go environment, then compares the samples with `benchstat`. Use `--base vX.Y.Z` to select a specific reachable release, `--pattern 'regexp'` to focus the suite, and `--count N` or `--time DURATION` only when the defaults do not provide enough statistical confidence.

Use the ordinary tests to enforce allocation ceilings; `make test` includes the representative `ProcessEvent` allocation budgets. Do not infer retained heap size from benchmark `B/op`. The dedicated `BenchmarkSchemaRetained` and `BenchmarkValidationMetadataRetained` benchmarks force garbage collection and report retained schema and validation-cache memory separately from transient construction allocations.

Compare performance only between runs made on the same machine under comparable system load and with the same Go toolchain, `GOEXPERIMENT` setting, benchmark pattern, duration, and sample count. Record the compared commits or tags, Go version, experiment setting, operating system, architecture, and CPU when reporting results. Prefer statistically supported `scripts/benchmark-compare.sh` results over isolated benchmark timings, and investigate both runtime and allocation changes before attributing a regression to the code.

## Documentation And Comments

Update the README and CLI help when user-facing behavior, arguments, output, installation, or operational expectations change. Keep README examples and actual CLI behavior synchronized.

Document public API ownership, mutation, concurrency, errors, defaults, and non-obvious result semantics. Add implementation comments only when they explain reasoning or constraints that are not evident from the code.

Do not hard-wrap Markdown prose merely to enforce a fixed column width. Let editors and renderers wrap prose. Preserve conventional formatting in lists, tables, and code blocks.

## Formatting

Use `gofmt` for Go. Prefer readable Go lines around 120 characters or fewer, but allow longer lines when breaking them would reduce clarity.

When a long string literal has a natural content seam (e.g. distinct clauses of a usage string, or a regex's logical groups), splitting it with `+` at that seam is a legitimate way to shorten a line, and can aid readability beyond just line length — see `internal/semver/semver.go`'s `Pattern` regex for an example, split into its major.minor.patch, prerelease, and build-metadata groups with a trailing comment naming each. Only split at a real seam, never at an arbitrary character count — an arbitrary split reads as existing solely to satisfy a tool and makes the string harder to verify, not easier. When the seam falls mid-phrase and needs a preserved space between segments, put that space at the start of the following line's literal, not trailing the line above (see `cmd/ocsf-toolkit/cli.go`'s `cliUsage`). This does not apply to struct tags, which must be a single literal token and cannot be split or built from a constant expression at all; an over-length struct tag has no split option and needs a `//nolint:lll` instead.

Use ASCII unless an existing file or the subject matter clearly requires Unicode.
