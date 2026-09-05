# Project design decisions

This document records the toolkit's current deliberate design choices and the requirements they impose. It is a normative description of the design at `HEAD`, not a change log. When a decision changes, replace the obsolete text; Git history already records earlier designs.

[Project and OCSF invariants](project-invariants.md) records requirements derived from specifications, public contracts, and stable project behavior. This document records choices among otherwise valid designs. Use both as independent authority when reviewing production code and tests; do not infer a decision solely from code and tests changed together.

## Review use

- Check proposed behavior against the current decisions and derived requirements below.
- Update this document when the design changes so it continues to describe only the current contract.
- Protect user-visible consequences with public black-box tests and architectural or operational requirements with engineering-invariant tests.
- Consult the maintainer before changing a decision, weakening a derived requirement, or changing a test that protects one.

## Decisions

### DECISION-API-001: deliberate pre-1.0 cleanup may change public surfaces

Decision: before 1.0, the toolkit may deliberately make breaking changes to the CLI, Go API, report representation, and other public surfaces when doing so improves correctness, clarity, consistency, or long-term design. Compatibility with earlier pre-1.0 surfaces is not by itself an invariant.

Rationale: retaining deprecated aliases or ambiguous behavior can make the eventual stable API harder to understand and maintain.

Derived requirements and test implications:

- A breaking change must be deliberate, coherent, documented, and covered by tests that express the new contract.
- Breaking a CLI or public API surface does not authorize accidental loss of OCSF correctness, data preservation, path safety, deterministic behavior, concurrency safety, or another independently established invariant.
- Concurrently changed code, tests, and documentation still require independent contract review.

Replace this decision with an explicit compatibility and deprecation policy before releasing 1.0.

### DECISION-API-002: mutation options use one action model and reject duplicate component configuration

Decision: enum-sibling and observable mutation use `WithEnumSiblings`, `WithObservables`, and the shared `enrichment.Action` values. Each component option may occur once; repeating it is a configuration error rather than an override. Passing `enrichment.None` to either component option is a legitimate no-op; a processor whose configuration leaves every component at `None` is invalid.

Rationale: one action-valued option per component makes the selected mutation explicit. Rejecting repeated single-valued directives exposes contradictory configuration instead of silently choosing one by order. Explicit `None` remains composable so callers can resolve an action from data and pass it through without conditionally omitting the option.

Derived requirements and test implications:

- Repeated component options must fail pipeline construction with a structured duplicate-option error.
- `WithEnumSiblings(enrichment.None)` and `WithObservables(enrichment.None)` must succeed when another component remains active.
- Pipeline construction must reject a configuration in which every mutation component is `None`.
- Public examples and tests must use the action-based component options.

Reconsider only through a deliberate public API redesign.

### DECISION-API-003: pipeline construction and results remain additively extensible

Decision: keep `eventpipeline.NewPipeline` extensible through sealed, toolkit-owned `PipelineOption` values, including `WithSchema`, and keep its resolved configuration private. A processor family owns its action or mode types, finding-code types, and reporting policy rather than reusing types whose semantics belong to another family. Keep `ProcessingResult` an opaque concrete value with private value storage and a typed accessor for each result family. This design reserves additive growth for the toolkit; it does not expose a third-party processor plug-in interface.

Rationale: toolkit-owned options allow configuration to grow without changing the `NewPipeline` signature or exposing internal processor configuration. Family-scoped types keep unrelated semantics distinct. Private result storage permits additive typed accessors without exposing internal layout, requiring interface dispatch, or making callers construct a result representation that anticipates future families.

Derived requirements and test implications:

- Public option interfaces and family-specific nested option interfaces remain sealed unless third-party processor extension becomes an explicit product requirement.
- A new processor family must add family-scoped options and types rather than a generic untyped configuration map or reuse of unrelated actions and finding codes.
- A family that produces findings must define whether they are ignorable and expose policy scoped to that family's code type.
- `ProcessingResult` retains private value storage and exposes results through typed accessors.
- Tests should protect the public shape and observable allocation behavior rather than private field names or internal processor decomposition.

Reconsider if callers need to register third-party processors, additive accessors become impractical at demonstrated scale, or benchmark evidence supports a different representation without weakening type safety or hot-loop allocation behavior.

### DECISION-API-004: pipelines own schema use while schemas remain reusable

Decision: construct each logical schema independently with `eventpipeline.NewSchema`, then supply it to `eventpipeline.NewPipeline` through `WithSchema`. A `Schema` owns reusable compiled-schema data; a `Pipeline` owns processor configuration and how it uses its supplied schema. `WithSchema` accepts sealed, toolkit-owned `SchemaPipelineOption` values. These nested options configure the schema-to-pipeline relationship rather than changing the reusable schema itself.

Rationale: this ownership model makes a loaded schema reusable across pipelines and allows future pipeline-level schema selection without making a schema own processing policy. Reserving the variadic `SchemaPipelineOption` parameter preserves the exact `WithSchema` function type for SemVer 1.x callers.

Derived requirements and test implications:

- A constructed `Schema` remains immutable, safe for concurrent use, and reusable by more than one pipeline; schema-derived caches remain schema-owned rather than being rebuilt for each pipeline.
- Pipeline-specific schema selection, dispatch, fallback, and compatibility policy belong to pipeline construction rather than `NewSchema` options or mutable schema state.
- `SchemaPipelineOption` remains distinct from schema-loading options and top-level `PipelineOption` values. It remains sealed unless third-party extension becomes an explicit requirement.
- The current implementation selects exactly one schema and rejects repeated `WithSchema` calls. Multiple registration, ambiguity detection, default selection, and extension-version policy require separate decisions.
- Public API tests protect the `func(*Schema, ...SchemaPipelineOption) PipelineOption` shape even while the nested option set is empty.

Reconsider the nested option name before 1.0 if the first association-level option shows that it does not describe the abstraction. Reconsider the sealed boundary only if callers need to define schema-to-pipeline behavior outside the toolkit.

### DECISION-API-005: diagnostic policy uses unified per-code levels

Decision: configure processing issues and validation findings through family-specific ignored, warning, and error levels rather than separate suppression and severity-remapping options. `WithIssueLevel` and `WithValidationLevel` set one typed code; `WithAllIssueLevels` and `WithAllValidationLevels` set the level for every code. The `all` setting may occur once and must precede specific code settings; each specific code may occur once. Mandatory processing issue codes may be warning or error but cannot be ignored; an `all=ignored` setting retains their toolkit defaults. Every validation code may be ignored, but enabling validation with every code resolved to ignored is a configuration error.

`validation_attribute_recommended_missing`, `validation_observable_duplicate`, and `issue_observable_duplicate` default to ignored. Environments enable those diagnostics through the same level policy used by every other code. Processors skip the corresponding separable work while its effective level is ignored.

For validation, warning and error are effective finding levels and both remain successful `ProcessEvent` results. For processing issues, warning adds the issue to the result, ignored omits it, and error stops at the first matching issue and returns a `ProcessingIssueError`. Every non-nil `ProcessEvent` error is accompanied by the zero `ProcessingResult`; prior in-place event mutations are not rolled back.

The CLI exposes the same model through repeatable `--issue-level ISSUE_CODE=LEVEL` and `--validation-level VALIDATION_CODE=LEVEL` flags, with `all` accepted in the code position. It applies issue policy to schema initialization issues as well as event-processing issues. Repeatable scalar selections use a singular flag, including `--observable-id ID`; comma-separated lists are not a separate CLI grammar. Help and documentation use a space between a flag and its metavariable. Attached values remain accepted and may be illustrated with forms such as `--event=my_event.json`.

Rationale: one level vocabulary models ignoring, reporting, and escalation without contradictory option combinations. Typed code and level arguments preserve compile-time safety in the Go API. Requiring an all-code baseline before specific exceptions makes precedence explicit, while rejecting repeated baselines and codes exposes contradictory configuration.

Derived requirements and test implications:

- Issue and validation levels have the stable text forms `ignored`, `warning`, and `error`.
- Unknown codes, unknown levels, and explicit attempts to ignore mandatory codes make pipeline construction fail.
- Ignored diagnostics are omitted without maintaining ignored counts. Independently separable validation checks are skipped when their effective level is ignored.
- Error-level processing issues stop callbacks and processor dispatch promptly, return a structured error usable through `errors.As`, and return no meaningful partial processing result.
- CLI level flags are repeatable; `all` occurs at most once before specific code settings, each specific code occurs at most once, and help displays value-taking flags consistently as `--flag VALUE`.

Reconsider only if a demonstrated use case requires an additional informational level or a policy dimension that cannot be represented as one handling level.

### DECISION-OBS-001: generated deduplication and duplicate diagnostics are independent

Decision: observable generation defaults to appending every generated candidate. `WithObservableDeduplication` accepts ignored or generated mode; the CLI exposes the same selection as `--deduplicate-observables disabled|generated`. Generated mode silently removes only later generated candidates that duplicate earlier generated candidates. It does not inspect existing observables, remove existing duplicates, or remove a generated candidate that matches only an existing entry. No all-observables mode is currently supported.

Duplicate diagnostics are separate opt-in work. The issue detects existing-existing, generated-existing, and generated-generated duplicates during observable addition. The validation detects duplicate identities in the final observable array. Both codes default to ignored because each requires an identity scan that is unnecessary for ordinary processing. When both codes are enabled during observable addition, the issue owns the condition and validation omits its duplicate scan and findings.

Rationale: generated-to-generated deduplication is a mutation optimization, not evidence that the event is invalid or that a processing operation failed. Separating it from diagnostics makes mutation predictable, allows the common path to avoid identity hashing and existing-array scans, and leaves room for a future explicitly designed all-observables mutation mode. One shared duplicate analyzer prevents semantic drift between issue and validation behavior, while single diagnostic ownership avoids duplicate work when both policies are enabled.

Derived requirements and test implications:

- Ignored or omitted deduplication appends all generated candidates and performs no identity bookkeeping unless duplicate diagnostics independently require it.
- Generated mode requires observable addition and rejects unsupported values, including a prospective all mode.
- Generated mode never changes existing entries and never compares a candidate with the existing prefix.
- Duplicate diagnostics use the same semantic identity across all origin pairs and include indexes; the issue additionally identifies existing or generated origin.
- Error-level duplicate issues are evaluated after traversal but before the generated suffix is appended. Earlier in-place mutations remain.
- Enabling both issue and validation duplicate codes during observable addition produces only issue diagnostics and performs one duplicate analysis.

Reconsider an all-observables mutation mode only with a separate contract for existing-entry ownership, ordering, counts, and diagnostics.

### DECISION-API-006: library pipeline construction fails fast

Decision: `eventpipeline.NewPipeline` and the internal processing configuration validator return the first problem found in their deterministic validation order. They do not aggregate independent configuration problems or expose them through `Unwrap() []error`. This library contract is separate from CLI configuration reporting, which may collect independent flag problems.

Rationale: fail-fast construction gives callers a stable, actionable initialization error without requiring aggregate-error traversal. Structural option errors are detected while resolving the supplied option list. Schema state is checked before translating or compiling the resolved internal processing configuration, avoiding work that cannot produce a pipeline. The CLI has different user-facing diagnostic needs.

Derived requirements and test implications:

- Every `NewPipeline` failure path returns exactly one error, including missing or uninitialized schemas and invalid internal processing configuration.
- Structural option errors precede schema-state errors, which precede resolved internal processing-configuration errors.
- Internal processing validation retains its precedence order across action, mutation, issue-policy, validation-path, and validation-policy checks.
- Policy helpers stop at the first invalid rule and do not return or unwrap later policy problems.
- Public typed `PipelineOptionError` values remain available for structural option errors.
- Tests verify the first error and that later configuration problems are absent from the message and `Unwrap() []error`.

Reconsider only through an explicit library contract change; CLI diagnostic accumulation does not alter this decision.

### DECISION-DATA-001: event-processing APIs use `jsonish.Map`

Decision: retain `jsonish.Map` as the event-object type used by event-processing APIs. It provides domain vocabulary while remaining an alias for its underlying Go map type, with no conversion cost.

Rationale: event-processing signatures benefit from distinguishing a JSON-like object from an arbitrary map without wrappers, copying, or conversion at API boundaries. The name keeps the accepted in-memory model explicit while allowing either supported JSON implementation to populate it.

Derived requirements and test implications:

- Public event-processing APIs accept and return `jsonish.Map` where the value represents a JSON object.
- Processing mutates the caller's map in place where its public contract says it does; the alias does not introduce copying semantics.
- JSON decoding preserves `json.Number` where possible, as required by the numeric contract.

Reconsider if the event representation needs an opaque object, a generic standard-library type acquires equally useful domain meaning, or the named type creates a measurable interoperability cost.

### DECISION-INPUT-001: local processing has no generic input-size limit

Decision: the current library and local-file CLI do not impose a generic event, schema, file, or directory input-size limit. Callers apply limits appropriate to their environment.

Rationale: a universal threshold would reject legitimate local workloads without defining a meaningful protection boundary. A remotely reachable or long-lived service has a materially different resource-exhaustion boundary and must make an explicit limit decision.

Derived requirements and test implications:

- Do not introduce an arbitrary default size limit as an incidental parser, traversal, or CLI change.
- Specific configurable limits may be added when they protect a defined resource and preserve the documented default contract.
- Server, remote, and streaming interfaces must decide byte, record, depth, time, and aggregate resource limits before they are exposed.

Reconsider when the toolkit adds a server, remote input, streaming processing, or evidence that a bounded local interface needs an explicit resource policy.

### DECISION-CLI-001: composable processing actions use flags rather than subcommands

Decision: enrichment, enrichment removal, and validation are composable actions in one invocation, so the CLI models them as flags rather than mutually exclusive top-level subcommands. Mutation configuration exposes enum siblings and observables as independent components and rejects contradictory actions that select the same component.

Rationale: a subcommand hierarchy would either prevent useful combinations or add an artificial nesting order unrelated to processing semantics.

Derived requirements and test implications:

- Tests cover compatible actions together and reject only conflicts affecting the same component.
- The `--enrich`, `--unenrich`, and `--force-remove` shorthand flags remain mutually exclusive with the per-component `--enum-siblings` and `--observables` flags. Partial combinations would require an ambiguous per-component precedence rule.
- Bare convenience actions and explicit component selections resolve deterministically, independently of argument order except where an option is expressly single-use.
- Parser or help-formatting replacements preserve processing configuration semantics even when option spellings deliberately change before 1.0.
- Processing order remains governed by `DECISION-ORDER-001`, not CLI flag order.

Reconsider if the CLI gains operations that are genuinely mutually exclusive workflows rather than composable event processors.

### DECISION-CLI-002: parsed configuration reports independent problems together

Decision: after command-line parsing and mutation-action resolution succeed, the CLI reports every independent configuration problem it can determine safely. Each problem is printed on its own `error: ...` line and terse usage is printed once. The CLI owns the level-rule order, duplicate, issue-policy, and validation-policy checks needed for this behavior; focused parity tests protect their agreement with authoritative library semantics. Parser failures, mutation-selection conflicts, filesystem operations, schema loading, layout preflight, and runtime processing remain fail-fast.

Configuration checks suppress diagnostics derived from an unresolved mutation selection, input mode, or validation enablement. Before schema loading, the CLI opens `--schema` for reading and verifies from the opened descriptor that its target is a regular file. Ordinary opening follows symbolic links. Schema-format and content checks remain in schema loading and pipeline construction.

Rationale: command-line users can correct unrelated flag mistakes in one edit-and-run cycle, while library callers retain the stable fail-fast construction contract in `DECISION-API-006`. Keeping collection local to the CLI avoids weakening that contract or introducing a shared validation abstraction whose error model would serve neither boundary cleanly. Schema preflight rejects an observed special file before opening because a read-only open can block on a FIFO, then checks the opened descriptor again to verify the target used by the command.

Derived requirements and test implications:

- Successfully parsed independent flag problems are emitted in deterministic order with one error prefix per problem and one terse usage block.
- CLI-local level and policy checks have focused parity coverage for ordering, duplicates, mandatory issue handling, and all-ignored validation policy.
- Parser, filesystem-operation, schema-loading, layout, and runtime failures remain singular.
- Dependent output and policy diagnostics are not emitted when their prerequisite selection is unresolved.
- Schema preflight accepts a symbolic link to a readable regular file, rejects non-regular or unreadable targets, and leaves schema content validation to schema loading.

Reconsider if CLI configuration becomes programmatically consumed through a structured diagnostics API or another frontend demonstrates enough identical validation needs to justify a shared boundary-specific component.

### DECISION-OUTPUT-001: output preflight is bounded and follows local filesystem semantics

Decision: the CLI is a local testing tool and follows ordinary command-line filesystem behavior. Directory processing performs bounded preflight checks for paths and conflicts known before traversal, then walks and writes incrementally. When it observes a problem during preflight or in flight, it fails rather than attempting recovery or reconciliation. It does not enumerate the complete input tree before processing, reserve every derived destination, retain a same-run set of filesystem identities, monitor concurrent filesystem modification, or provide snapshot or transaction isolation.

`--overwrite` permits a selected event, report, or summary destination to replace a prior output of the same kind. It does not authorize conflicts between different output kinds or with inputs, the schema, or reserved namespaces. The CLI cannot prove the provenance or kind of every existing destination, so this is an operational contract for ordinary use rather than a content-inspection guarantee. It does not protect a run from concurrent changes, manually introduced links, case-folding aliases, or other unusual filesystem identity changes.

Single-event processing applies best-effort preflight checks to explicit event and report outputs using the target platform's path semantics and observable filesystem identity. After installing the processed event and immediately before writing the report, it compares the now-existing destinations by filesystem identity. If they identify the same file, processing fails without writing the report, regardless of `--overwrite`, and leaves the completed event at its destination. Initially nonexistent case variants remain distinct on a case-sensitive filesystem. On a case-insensitive filesystem where the alias becomes observable only after creation, the post-write check catches it in flight.

Rationale: bounded preflight catches ordinary mistakes without turning the toolkit into a filesystem-integrity monitor. Directory mode separates event and report namespaces, while the explicit-output recheck handles aliases that become observable only in flight. See the [FAQ](../FAQ.md) and [Architecture](../architecture.md).

Derived requirements and test implications:

- Preflight rejects conflicts among explicitly selected input, output, schema, event, report, and summary paths before writing.
- Preflight and in-flight checks cover ordinary observable conflicts without attempting exhaustive alias detection or protection against concurrent filesystem mutation.
- Directory traversal preserves only safe relative paths beneath the selected output root. Absolute paths and paths containing `..` never determine a destination outside that root, and traversal rejects observed symbolic links, unsafe path spellings, reserved output namespaces, and unsupported entry types according to documented platform rules.
- Without `--overwrite`, a derived destination that already exists, including one written earlier in the same run, fails rather than being replaced.
- `--overwrite` does not relax conflicts between different output kinds or between an output and an input, schema, or reserved namespace.
- With `--overwrite`, a later derived path may replace an earlier output when unusual case folding or aliasing reaches the same file; tests do not require an exhaustive same-run identity registry.
- Explicit event and report outputs are checked during preflight and compared again by filesystem identity after the event write. A preflight collision leaves both outputs unwritten. A collision observable only after the event write preserves the event and suppresses the report.
- Tests cover preflight, in-flight traversal checks, confinement, overwrite replacement, and observed filesystem changes separately without implying snapshot isolation.

Reconsider if the CLI becomes a server or privileged service, directory processing gains a transactional mode, real workflows produce derived-path aliases, or a security boundary requires protection against an actively hostile concurrent filesystem.

### DECISION-OUTPUT-002: validation findings do not suppress selected mutation output

Decision: all selected mutation and validation processing completes before output, and the CLI writes the selected processed event and complete processing report even when validation finds errors. Validation conformance may affect the exit status, but it does not discard mutation output or remove non-validation report sections.

Rationale: validation findings are successful processing results rather than processing failures. Keeping full output makes mutation results and diagnostics inspectable and avoids a second report shape controlled by event validity. See [Architecture](../architecture.md) and [Event processing](../event-processing.md).

Derived requirements and test implications:

- Combined mutation and validation tests retain the processed event and every selected report section when validation errors are present.
- `--fail-on-validation-errors` changes command success policy after processing; it does not change selected event or report content.
- Summaries count written events and validation findings without an `events_skipped` state derived solely from invalidity.

Reconsider if an explicit output policy introduces quarantined destinations or caller-selected filtering while retaining complete processing results.

### DECISION-OUTPUT-003: one output option owns stdout

Decision: at most one of `--event-output`, `--report-output`, `--summary`, and `--summary-json` may select stdout in one invocation. The default directory summary owns stdout unless `--quiet` suppresses it or an explicit summary option selects stdout. Other selected artifacts use files or `--output-dir`.

Rationale: event, processing-report, human-summary, and JSON-summary documents have different shapes. Concatenating them makes stdout a heterogeneous stream and prevents pretty JSON from being one JSON document.

Derived requirements and test implications:

- CLI configuration rejects multiple explicit stdout output selections before processing begins.
- Each individual output option continues to support stdout where its processing mode permits it.
- Help and user documentation do not imply that event and report documents can share stdout.
- Future streaming designs either preserve single ownership or define one homogeneous record envelope with an explicit discriminator.

Reconsider if a streaming protocol defines a stable, self-describing envelope that deliberately carries multiple output record types.

### DECISION-CLASS-001: every mutation is gated by class resolution

Decision: every event mutation requires `class_uid` to resolve successfully, including force-removal operations that could mechanically delete content without schema data.

Rationale: successful class resolution is the minimum sanity check that the input can be processed as an OCSF event. Destructive mode changes how much proof is required before removing content; it does not bypass event identity. See [Event processing](../event-processing.md), [Enrichment removal](../enrichment-removal.md), [Architecture](../architecture.md), and `TOOLKIT-CLASS-001` in [Project and OCSF invariants](project-invariants.md).

Derived requirements and test implications:

- A missing, malformed, non-integral, out-of-range, or unknown `class_uid` leaves the event unchanged for every enrichment and removal action.
- Force-removal tests exercise the same class-resolution gate as safe removal and enrichment.
- Validation may additionally report its class finding, but mutation stops independently of whether validation is enabled.

Reconsider only through an explicit contract decision that defines a non-OCSF structural editing mode distinct from event processing.

### DECISION-NUMERIC-001: OCSF integral types use an exact signed-64-bit carrier

Decision: the toolkit interprets both OCSF `integer_t` and `long_t` as signed 64-bit integers. Every accepted numeric representation denotes an exact mathematical integer within that range; conversion does not round, truncate, underflow, or overflow into a different value.

Rationale: OCSF defines `long_t` as a signed 8-byte integer but does not specify an `integer_t` bit width or range. One signed-64-bit carrier is platform-independent, preserves the conventional `integer_t`-within-`long_t` relationship, and avoids loss for `long_t`. See the [FAQ](../FAQ.md), [Roadmap](../roadmap.md), and `TOOLKIT-NUM-001` and `TOOLKIT-NUM-002` in [Project and OCSF invariants](project-invariants.md).

Derived requirements and test implications:

- Boundary, decimal, and exponent tests derive expectations from the represented mathematical value rather than `float64` conversion behavior.
- Class resolution, enum lookup, constraints, and validation share the same exact signed-64-bit interpretation.
- JSON decoding preserves `json.Number` where possible so the original numeric value remains available for exact conversion.
- A future 32-bit `integer_t` compatibility option must be explicit, retain signed-64-bit `long_t`, and apply consistently across all processing paths.

Reconsider when implementing that compatibility option, when OCSF normatively defines an `integer_t` range, or when a public API needs a different numeric carrier.

### DECISION-STRING-001: `max_len` uses the standard-library rune count

Decision: measure string `max_len` constraints in Unicode code points using Go's standard-library rune count. Do not measure encoded bytes or add a third-party Unicode grapheme-segmentation dependency solely to approximate user-perceived characters.

Rationale: OCSF is a logical schema, so encoded byte length is inappropriate. Grapheme clusters more closely match user-perceived characters, but Go's standard library does not provide grapheme segmentation. Code-point counting is conservative relative to grapheme-cluster counting. The OCSF metaschema says only "maximum length" and does not distinguish code points from grapheme clusters.

Derived requirements and test implications:

- `max_len` validation uses Unicode code-point count, not UTF-8 byte length.
- Tests distinguish code points from bytes and retain a combining-sequence case documenting that code-point count is not grapheme-cluster count.
- Public documentation calls this a toolkit interpretation rather than an explicit OCSF requirement.
- Canonical Unicode normalization is caller-owned; processing preserves strings as supplied and does not normalize them before measuring or comparing them.

Reconsider if OCSF normatively defines the unit, Go's standard library adds Unicode grapheme segmentation, or interoperability requirements justify a deliberate dependency and contract change.

### DECISION-ORDER-001: mutation phases have semantic order independent of option order

Decision: enum-sibling mutation runs before observable mutation, and all mutation runs before validation. Configuration-option or CLI-flag order does not change this processing order.

Rationale: observables can depend on enum siblings, and validation must assess the final mutated event. Central pipeline ordering prevents option parsing or processor registration order from changing semantics. See [Event processing](../event-processing.md), [Architecture](../architecture.md), and `TOOLKIT-ORDER-001` in [Project and OCSF invariants](project-invariants.md).

Derived requirements and test implications:

- Adding enum siblings and observables in one pipeline allows observable generation from a sibling added during that call.
- Removing enum siblings affects later observable analysis according to documented safe-removal rules.
- Validation results describe the post-mutation event.
- Tests vary configuration-option order and require identical behavior without asserting incidental internal collection layout.
- Before adding a processor family, establish and document its semantic phase relative to every existing family. Mutation remains ahead of validation unless an explicit contract decision revises that order.

Reconsider only if a new processor has a documented semantic dependency that requires extending the fixed phase model.

### DECISION-ENUM-001: schema loading defensively validates concrete enum siblings

Decision: the toolkit validates enum-sibling relationships independently within every concrete class and object when loading a compiled schema. It treats the relationship declared by each concrete item as authoritative and does not compare it with the dictionary or other items. A supported source enum has the direct type `integer_t` or `long_t`; a named subtype does not qualify. The source may be scalar or an array, and eligibility does not depend on its attribute name. A supported target exists in the same concrete item, is not itself an enum, has the direct type `string_t`, and has the same `is_array` value as the source. A named `string_t` subtype does not qualify.

Rationale: OCSF documentation describes integral enums, string siblings, and parallel scalar or array forms but does not specify named subtype eligibility or malformed compiled relationships. Event processing represents each accepted enum and sibling as one bidirectionally linked unit, so initialization must prevent invalid links before the event hot loop. See `OCSF-ENUM-003`, `COMPILED-002`, `COMPILED-005`, and `TOOLKIT-ENUM-001` in [Project and OCSF invariants](project-invariants.md).

Derived requirements and test implications:

- Schema loading builds enum-to-sibling and sibling-to-enum relationships separately for every concrete class and object, linking only relationships that satisfy the direct-type, target, and shape rules.
- A `sibling` property on a non-enum attribute is ignored. An enum without `sibling` remains an ordinary enum without an initialization issue.
- An enum with `sibling` but without direct type `integer_t` or `long_t` produces `issue_at_init_schema_enum_sibling_source_not_integral`; the relationship is ignored.
- A missing target produces `issue_at_init_schema_enum_sibling_target_not_found`. A target that is itself an enum produces `issue_at_init_schema_enum_sibling_target_is_enum`, which takes precedence over target type or shape. Any other target without direct type `string_t` or without matching scalar or array shape produces `issue_at_init_schema_enum_sibling_target_not_string`. Each invalid relationship is ignored, and affected attributes continue through ordinary processing.
- Loading rejects a concrete item that assigns one otherwise-supported sibling target to multiple enums. Self-links, chains, and cycles through enum targets are ignored with the target-is-enum initialization issue, so an ignored target enum retains ordinary validation and evaluates its own declaration independently.
- A class or object may declare a relationship that differs from the dictionary or another item; validation does not impose cross-scope consistency that event processing does not require.
- The toolkit does not reconstruct includes, inheritance, patches, or profile expansion.
- Profiles activate the source and target independently. Pair-specific processing applies only when both are active; otherwise an active member is processed ordinarily without diagnosing why the counterpart is inactive.
- Scalar pairs operate on their two values. Array pairs require equal lengths and associate values by index.
- Eligibility and links are resolved once during schema initialization. During an event walk, the visitor dispatches an accepted linked pair instead of visiting either member as an ordinary attribute.
- Tests cover classes and objects, direct and subtype declarations, scalar and array shapes, missing and enumerated targets, issue precedence, different valid relationships across scopes, shared siblings, chains and cycles, profile combinations, and hand-edited compiled schemas.
- These checks run during schema loading and add no lookup, allocation, or graph-validation work to per-event traversal.

Reconsider only if event processing stops resolving enum siblings from concrete class and object definitions or explicitly supports an ambiguous relationship graph.
