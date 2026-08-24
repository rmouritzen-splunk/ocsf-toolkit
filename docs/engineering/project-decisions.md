# Project design decisions

This engineering ledger records deliberate design, engineering, implementation, and derived-requirement choices that should inform long-lived tests and regression review. Unlike [Project and OCSF invariants](project-invariants.md), these entries need not follow directly from OCSF or an external public contract. They record why the toolkit chose one valid design among alternatives and what must remain stable unless that choice is deliberately revisited.

The ledger is evidence, not an independent source of authority. Each entry identifies its authority, derived requirements, test implications, and reasons to reconsider it. A historical review must apply only decisions established by that point in history; a later decision cannot retroactively prove an earlier change correct.

This document and [Project and OCSF invariants](project-invariants.md) are maintainer-facing engineering documentation. Keep durable decisions here rather than relying on a review thread or machine-local scratch files.

## Review use

- Establish the prior contract independently and trace a suspected regression to the current review head. Report an overall regression only when the incorrect behavior or documentation survives at that head. A historical defect that was later repaired is provenance rather than a current finding, and a temporary loss superseded by an intentional current contract does not require a current fix.
- Consult this ledger after the correctness-test and fixture diff and before classifying a production change as a regression, intentional contract change, or implementation-detail adaptation.
- Update this file when a review establishes, revises, challenges, or violates a recorded choice.
- Do not derive a decision from production code and its concurrently changed tests alone. Require an independent public contract, prior behavior, design documentation, explicit maintainer decision, or separately reviewed rationale.
- Treat long-lived tests as protection for the derived requirement, not necessarily the current mechanism. Prefer public black-box tests for user-visible consequences and engineering-invariant tests for architectural or operational requirements.
- Consult the maintainer before treating a debatable choice as settled, changing a decision, weakening a derived requirement, or changing a test that protects it.

## Decisions

### DECISION-API-001: deliberate pre-1.0 cleanup may break earlier public surfaces

Status: established project policy and explicitly confirmed by the maintainer on 2026-08-22.

Decision: before 1.0, the toolkit may deliberately make breaking changes to the CLI, Go API, report representation, and other public surfaces inherited from v0.8.0-era releases when doing so improves correctness, clarity, consistency, or long-term design. Compatibility with those earlier surfaces is not by itself an invariant during this cleanup period.

Rationale and authority: several intentional redesigns are being completed before the 1.0 contract is locked down. Retaining deprecated aliases or ambiguous behavior can make the eventual stable API harder to understand and maintain. The maintainer explicitly confirmed that these breaking changes are expected.

Derived requirements and test implications:

- A breaking change must still be deliberate, coherent, documented, and covered by tests that express the new contract.
- Breaking a CLI or public API surface does not authorize accidental loss of OCSF correctness, data preservation, path safety, deterministic behavior, concurrency safety, or another independently established invariant.
- Concurrently changed code, tests, and documentation still require review; this decision supplies compatibility authority only after the change is identified as intentional.
- Historical regression review should classify an intentional pre-1.0 surface redesign separately from an implementation regression and should still report stale documentation or incomplete migration.

Reconsider and replace this policy with an explicit compatibility and deprecation policy before releasing 1.0.

### DECISION-API-002: mutation options use one action model without compatibility aliases

Status: established during the pre-1.0 processing API redesign.

Decision: enum-sibling and observable mutation use `WithEnumSiblings`, `WithObservables`, and the shared `enrichment.Action` values. Do not restore the removed boolean `With*` options or `NewEnrichmentRemoval` as deprecated aliases. Passing `enrichment.None` to either component option is a legitimate no-op; only a processor whose final configuration leaves every component at `None` is invalid.

Rationale and authority: the former sticky `retain*` flags violated last-option-wins semantics, and positional booleans made transposed arguments easy to write and hard to notice. Maintaining the old and new surfaces together would preserve those hazards during the deliberate pre-1.0 cleanup authorized by `DECISION-API-001`. Explicit `None` must remain composable because callers such as the CLI can resolve an action from data and pass it through without conditionally omitting the option.

Derived requirements and test implications:

- Repeated component options must use the last selected action rather than retaining hidden state from an earlier option.
- `WithEnumSiblings(enrichment.None)` and `WithObservables(enrichment.None)` must succeed when another component remains active.
- Pipeline construction must reject the distinct case in which every component is `None` after all options have been applied.
- Public examples and tests must use the action-based API rather than preserving removed aliases.

Reconsider only through a deliberate public-API redesign; ordinary compatibility pressure before 1.0 is not sufficient reason to restore the ambiguous boolean surface.

### DECISION-API-003: pipeline construction and results remain additively extensible

Status: established by the pre-1.0 public API design.

Decision: keep `Schema.NewPipeline` extensible through sealed, toolkit-owned `PipelineOption` values and keep its resolved configuration private. A new processor family must own its action or mode types, finding-code types, and suppression policy rather than reuse types whose semantics belong to another family. Keep `ProcessingResult` an opaque concrete value with private value storage, adding a typed accessor for each new result family. This design reserves additive growth for the toolkit; it does not expose a third-party processor plug-in interface.

Rationale and authority: toolkit-owned options allow new configuration to be added without changing the `NewPipeline` signature or exposing internal processor configuration. Family-scoped types keep semantics such as `enrichment.Action`'s schema-verified removal distinct from unrelated operations. Private result storage permits additive typed accessors without exposing internal layout, requiring interface dispatch, or making callers construct a result representation that must anticipate future families.

Derived requirements and test implications:

- Public option interfaces and their family-specific nested option interfaces must remain sealed unless third-party processor extension becomes an explicit product requirement.
- A new processor family should add family-scoped options and types rather than a generic untyped configuration map or reuse of unrelated action and finding codes.
- A family that produces findings must define whether they are suppressible and, when suppression is supported, expose policy scoped to that family's code type rather than overload another family's suppression option.
- `ProcessingResult` must retain private value storage and expose new results through typed accessors; adding a processor must not replace it with an interface-backed or generic map representation merely for extensibility.
- Tests protecting this design should verify the public shape and observable allocation behavior rather than private field names or internal processor decomposition.

Reconsider if callers need to register third-party processors, if additive accessors become impractical at demonstrated scale, or if benchmark evidence supports a different representation without weakening type safety or hot-loop allocation behavior.

### DECISION-DATA-001: event-processing APIs use `jsonish.Map`

Status: established project API design.

Decision: retain `jsonish.Map` as the event-object type used by event-processing APIs. It provides useful domain vocabulary while remaining an alias for its underlying Go map type, with no conversion cost.

Rationale and authority: event-processing signatures benefit from distinguishing a JSON-like object from an arbitrary map without requiring wrappers, copying, or conversion at API boundaries. The name also keeps the accepted in-memory model explicit while allowing either supported JSON implementation to populate it.

Derived requirements and test implications:

- Public event-processing APIs should accept and return `jsonish.Map` where the value represents a JSON object.
- Processing must continue to mutate the caller's map in place where its public contract says it does; the alias must not introduce copying semantics.
- JSON decoding should continue to preserve `json.Number` where possible, as required independently by the numeric contract.

Reconsider if the event representation gains behavior that requires an opaque object, if a generic standard-library type acquires equally useful domain meaning, or if retaining the named type creates a measurable interoperability cost.

### DECISION-INPUT-001: local processing has no generic input-size limit

Status: established project robustness policy.

Decision: the current library and local-file CLI do not impose a generic event, schema, file, or directory input-size limit. Callers are responsible for applying limits appropriate to their environment.

Rationale and authority: a single universal threshold would reject legitimate local workloads without defining a meaningful protection boundary. The library and local CLI operate on caller-selected local data, while existing structural and output constraints address specific correctness and confinement requirements. A remotely reachable or long-lived service has a materially different resource-exhaustion boundary and must make an explicit limit decision.

Derived requirements and test implications:

- Do not introduce an arbitrary default size limit as an incidental parser, traversal, or CLI change.
- Specific configurable limits, such as finding counts or recursion boundaries, may be added when they protect a defined resource and preserve the documented default contract.
- Server, remote, and streaming interfaces must reconsider byte, record, depth, time, and aggregate resource limits before they are exposed.

Reconsider when the toolkit adds a server, remote input, streaming processing, or evidence that a bounded local interface needs an explicit resource policy.

### DECISION-CLI-001: composable processing actions use flags rather than subcommands

Status: established project behavior.

Decision: enrichment, enrichment removal, and validation are composable actions in one invocation, so the CLI models them as flags rather than mutually exclusive top-level subcommands. Mutation configuration exposes enum siblings and observables as independent components and rejects contradictory actions that select the same component.

Rationale and authority: a subcommand hierarchy would either prevent useful combinations or add an artificial nesting order unrelated to processing semantics. The architecture guide records this rationale, and the public CLI maps its flags to independent pipeline configuration.

Derived requirements and test implications:

- Tests should cover compatible actions together and reject only conflicts affecting the same component.
- The `--enrich`, `--unenrich`, and `--force-remove` shorthand flags must remain mutually exclusive with the per-component `--enum-siblings` and `--observables` flags. Partial combinations would require a per-component precedence rule whose ambiguity is not worth the convenience.
- Bare convenience actions and explicit component selections must resolve deterministically, independently of argument order except where an option is expressly single-use.
- Parser or help-formatting replacements must preserve processing configuration semantics even when option spellings deliberately change before 1.0.
- Processing order remains governed by `DECISION-ORDER-001`, not CLI flag order.

Reconsider if the CLI gains operations that are genuinely mutually exclusive workflows rather than composable event processors.

### DECISION-OUTPUT-001: directory processing uses bounded preflight and ordinary filesystem semantics

Status: established project behavior, refined by explicit maintainer decisions on 2026-08-22 and 2026-08-23.

Decision: the CLI is intended primarily as a local testing tool and follows ordinary command-line filesystem behavior. Directory processing performs useful, bounded preflight checks for paths and conflicts known before traversal, then walks and writes incrementally. When it observes a problem during preflight or in flight, it fails rather than attempting recovery or reconciliation. It does not first enumerate the complete input tree, reserve every derived destination, retain a same-run set of filesystem identities, monitor for concurrent filesystem modification, or provide snapshot or transaction isolation.

`--overwrite` is intended for repeated runs of this toolkit into a stable output layout. Semantically, it permits a selected event, report, or summary destination to replace a prior output of the same kind; it does not authorize conflicts between different output kinds or with inputs, the schema, or reserved namespaces. The CLI cannot prove the provenance or kind of an arbitrary existing file at a selected destination, so this is an operational contract for ordinary use rather than a content-inspection guarantee. The option does not protect a run from concurrent changes, manually introduced links, case-folding aliases, or other unusual filesystem identity changes.

Single-event processing first applies a best-effort preflight check to explicit event and report outputs using the target platform's path semantics and any filesystem identity already observable. Some platforms, including Windows, can identify differently cased nonexistent paths as overlapping and reject them before writing. After installing the processed event and immediately before writing the report, the CLI also compares the two now-existing destinations by filesystem identity. If they identify the same file, the CLI fails without writing the report, regardless of `--overwrite`, and leaves the completed event at its destination. Initially nonexistent case variants remain distinct outputs on a case-sensitive filesystem. On a case-insensitive filesystem whose lexical path operations do not recognize the alias before creation, the post-write check catches it in flight. The checks therefore prevent the same surprising replacement outcome across platforms without requiring their detection phase or partial-output state to be identical.

Rationale and authority: these are proportionate engineering choices for a local testing CLI, based on common command-line behavior rather than external OCSF requirements. Bounded preflight catches ordinary mistakes without turning the toolkit into a filesystem-integrity monitor. Directory mode separates event and report namespaces, while the explicit-output recheck handles aliases that become observable only in flight. Explicit maintainer decisions rejected exhaustive same-run identity detection, confirmed the repeated-run meaning of `--overwrite`, and confirmed that preflight and in-flight rejection are both valid best-effort outcomes. See the [FAQ](../FAQ.md) and [Architecture](../architecture.md).

Derived requirements and test implications:

- Preflight must reject conflicts among explicitly selected input, output, schema, event, report, and summary paths before writing.
- Preflight and in-flight checks should cover ordinary, observable conflicts without attempting exhaustive alias detection or protection against concurrent filesystem mutation. Processing must fail when a relevant problem is detected.
- Directory traversal must preserve only safe relative paths beneath the selected output root. Absolute paths and paths containing `..` must never determine a destination outside that root, and traversal must reject observed symbolic links, unsafe path spellings, reserved output namespaces, and unsupported entry types according to the documented platform rules.
- Without `--overwrite`, a derived destination that already exists, including one written earlier in the same run, must fail rather than replace it.
- `--overwrite` must not relax conflict checks between different output kinds or between an output and an input, schema, or reserved namespace. Its intended use is replacing the corresponding output from an earlier toolkit run.
- With `--overwrite`, a later derived path may replace an earlier output when unusual case folding or aliasing reaches the same file; tests must not require an exhaustive same-run identity registry.
- Explicit file-valued event and report outputs must be checked during best-effort preflight and compared again by filesystem identity after the event write. A preflight collision must leave both outputs unwritten. A collision that becomes observable only after the event write must preserve the event and suppress the report. Both outcomes apply regardless of overwrite mode.
- Tests should separately cover preflight, in-flight traversal checks, confinement, overwrite replacement, and failures caused by observed filesystem changes. They should not imply snapshot isolation.

Reconsider if the CLI becomes a server or privileged service, directory processing gains an explicit transactional mode, real workflows produce derived-path aliases, or a security boundary requires protection against an actively hostile concurrent filesystem.

### DECISION-OUTPUT-002: validation findings do not suppress selected mutation output

Status: established project behavior.

Decision: all selected mutation and validation processing completes before output, and the CLI writes the selected processed event and complete processing report even when validation finds errors. Validation conformance may affect the exit status, but it does not silently discard mutation output or remove non-validation sections from the report.

Rationale and authority: validation findings are successful processing results rather than processing failures. Keeping full output makes mutation results and diagnostics inspectable and avoids a second report shape controlled by event validity. See [Architecture](../architecture.md) and [Event processing](../event-processing.md).

Derived requirements and test implications:

- Combined mutation and validation tests must retain the processed event and every selected report section when validation errors are present.
- `--fail-on-validation-errors` changes command success policy after processing; it must not change the event or report content selected for writing.
- Summaries count written events and validation findings without an `events_skipped` state derived solely from invalidity.

Reconsider if a future output policy explicitly introduces quarantined destinations or caller-selected filtering while retaining complete processing results.

### DECISION-OUTPUT-003: one output option owns stdout

Status: established by explicit maintainer decision on 2026-08-22 during pre-1.0 CLI cleanup.

Decision: at most one of `--event-output`, `--report-output`, `--summary`, and `--summary-json` may select stdout in one invocation. The default directory summary owns stdout unless `--quiet` suppresses it or an explicit summary option selects stdout. Other selected artifacts must use files or `--output-dir`.

Rationale and authority: event, processing-report, human-summary, and JSON-summary documents have different shapes. Concatenating them makes stdout a heterogeneous stream that consumers must interpret by position, and pretty JSON would not be one JSON document. The maintainer explicitly replaced the earlier event-then-report stdout ordering with one unambiguous output representation per invocation.

Derived requirements and test implications:

- CLI configuration must reject multiple explicit stdout output selections before processing begins.
- Each individual output option must continue to support stdout where its processing mode permits it.
- Help and user documentation must not imply that event and report documents can share stdout.
- Future streaming designs must either preserve single ownership or define one homogeneous record envelope with an explicit discriminator.

Reconsider if a future streaming protocol defines a stable, self-describing envelope that deliberately carries multiple output record types.

### DECISION-CLASS-001: every mutation is gated by class resolution

Status: established project behavior confirmed by the maintainer on 2026-08-22.

Decision: every event mutation requires `class_uid` to resolve successfully, including force-removal operations that could mechanically delete content without schema data.

Rationale and authority: successful class resolution is the minimum sanity check that the input can be processed as an OCSF event. Destructive mode changes how much proof is required before removing content; it does not bypass event identity. See [Event processing](../event-processing.md), [Enrichment removal](../enrichment-removal.md), [Architecture](../architecture.md), and `TOOLKIT-CLASS-001` in [Project and OCSF invariants](project-invariants.md).

Derived requirements and test implications:

- A missing, malformed, non-integral, out-of-range, or unknown `class_uid` must leave the event unchanged for every enrichment and removal action.
- Force-removal tests must exercise the same class-resolution gate as safe removal and enrichment.
- Validation may additionally report its class finding, but mutation must already have stopped independently of whether validation is enabled.

Reconsider only through an explicit contract decision that defines a non-OCSF structural editing mode distinct from event processing.

### DECISION-NUMERIC-001: OCSF integral types use an exact signed-64-bit carrier

Status: established toolkit policy clarified by explicit maintainer decision on 2026-08-22.

Decision: the toolkit interprets both OCSF `integer_t` and `long_t` as signed 64-bit integers. Every accepted numeric representation must denote an exact mathematical integer within that range; conversion must not round, truncate, underflow, or overflow into a different value.

Rationale and authority: OCSF defines `long_t` as a signed 8-byte integer but does not specify an `integer_t` bit width or range. Using one signed-64-bit carrier is platform-independent, preserves the conventional `integer_t`-within-`long_t` range relationship, and avoids loss for `long_t`. See the [FAQ](../FAQ.md), [Roadmap](../roadmap.md), and `TOOLKIT-NUM-001` and `TOOLKIT-NUM-002` in [Project and OCSF invariants](project-invariants.md).

Derived requirements and test implications:

- Boundary, decimal, and exponent tests must derive expectations from the represented mathematical value rather than `float64` conversion behavior.
- Class resolution, enum lookup, constraints, and validation must share the same exact signed-64-bit interpretation.
- JSON decoding should preserve `json.Number` where possible so the original numeric value remains available for exact conversion.
- A future 32-bit `integer_t` compatibility option must be explicit, retain signed-64-bit `long_t`, and apply consistently across all processing paths.

Reconsider when implementing the roadmap option, when OCSF normatively defines an `integer_t` range, or when a public API needs a different numeric carrier.

### DECISION-STRING-001: `max_len` uses the standard-library rune count

Status: established toolkit policy, confirmed by explicit maintainer decision on 2026-08-22.

Decision: measure string `max_len` constraints in Unicode code points using Go's standard-library rune count. Do not measure encoded bytes. Do not add a third-party Unicode grapheme-segmentation dependency solely to approximate user-perceived characters.

Rationale and authority: OCSF is a logical schema, so encoded byte length is inappropriate. Grapheme clusters more closely match user-perceived characters, but Go's standard library does not provide grapheme segmentation. Code-point counting is conservative relative to grapheme-cluster counting: multi-code-point graphemes consume multiple units, so validation can reject them sooner but cannot become looser by undercounting their constituent code points. The OCSF metaschema says only “maximum length” and does not distinguish code points from grapheme clusters. The maintainer selected the standard-library rune count as the project's explicit balance of validation strictness, performance, implementation complexity, and dependency cost.

Derived requirements and test implications:

- `max_len` validation must use Unicode code-point count, not UTF-8 byte length.
- Tests must distinguish code points from bytes and should retain a combining-sequence case documenting that code-point count is not grapheme-cluster count.
- Public documentation must call this a toolkit interpretation rather than an explicit OCSF requirement.
- Canonical Unicode normalization is caller-owned; processing preserves strings as supplied and does not normalize them before measuring or comparing them.

Reconsider if OCSF normatively defines the unit, Go's standard library adds Unicode grapheme segmentation, or real interoperability requirements justify a deliberate dependency and contract change.

### DECISION-ORDER-001: mutation phases have semantic order independent of option order

Status: established by the enrichment pipeline design and documented public behavior.

Decision: enum-sibling mutation runs before observable mutation, and all mutation runs before validation. Configuration-option or CLI-flag order does not change this processing order.

Rationale and authority: observables can depend on enum siblings, and validation must assess the final mutated event. Central pipeline ordering prevents option parsing or processor registration order from changing semantics. See [Event processing](../event-processing.md), [Architecture](../architecture.md), and `TOOLKIT-ORDER-001` in [Project and OCSF invariants](project-invariants.md).

Derived requirements and test implications:

- Adding enum siblings and observables in one pipeline must allow observable generation from a sibling added during that call.
- Removing enum siblings must affect later observable analysis according to the documented safe-removal rules.
- Validation results must describe the post-mutation event.
- Tests should vary configuration-option order and require identical behavior while avoiding assertions about incidental internal collection layout.
- Before adding a processor family, establish and document its semantic phase relative to every existing family. Mutation must remain ahead of validation unless an explicit contract decision revises that ordering.

Reconsider only if a new processor has a documented semantic dependency that requires extending the fixed phase model.

### DECISION-ENUM-001: schema loading defensively validates concrete enum siblings

Status: established by explicit maintainer and OCSF Schema Compiler author decision on 2026-08-23.

Decision: the toolkit validates enum-sibling relationships independently within every concrete class and object when loading a compiled schema. It treats the relationship declared by each concrete item as authoritative and does not compare it with the dictionary or with other items. A supported source enum has the direct type `integer_t` or `long_t`; a named subtype does not qualify. The source may be scalar or an array, and eligibility does not depend on `_id`, `_ids`, or another attribute-name pattern. A supported target exists in the same concrete item, is not itself an enum, has the direct type `string_t`, and has the same `is_array` value as the source. A named `string_t` subtype does not qualify.

Rationale and authority: OCSF documentation describes integral enums, string siblings, and parallel scalar or array forms but does not clearly state whether named subtypes qualify or how a consumer should handle malformed compiled relationships. The current OCSF schema consistently uses direct `integer_t` sources and direct `string_t` targets for declared enum siblings; `long_t` is included as the other OCSF integral carrier supported by the toolkit. The direct-type and failure-handling rules are therefore explicit toolkit decisions confirmed by the maintainer on 2026-08-23, not claims of a normative OCSF invariant. Event processing represents each accepted enum and sibling as one bidirectionally linked, exactly-once unit, so accepting an enum as another enum's sibling can skip the target's ordinary enum validation. Defensive initialization must prevent invalid links before the event hot loop. See `OCSF-ENUM-003`, `COMPILED-002`, `COMPILED-005`, and `TOOLKIT-ENUM-001` in [Project and OCSF invariants](project-invariants.md).

Derived requirements and test implications:

- Schema loading must build enum-to-sibling and sibling-to-enum relationships separately for every concrete class and object, linking only relationships that satisfy the direct-type, target, and shape rules above.
- A `sibling` property on a non-enum attribute is irrelevant and ignored. An enum without `sibling` remains an ordinary enum without an initialization issue, regardless of whether it would qualify as a sibling source.
- An enum with `sibling` but without direct type `integer_t` or `long_t` produces `issue_at_init_schema_enum_sibling_source_not_integral`; the relationship is ignored.
- A missing target produces `issue_at_init_schema_enum_sibling_target_not_found`. A target that is itself an enum produces `issue_at_init_schema_enum_sibling_target_is_enum`, which takes precedence over the target's type or shape. Any other target without direct type `string_t` or without the source's scalar/array shape produces `issue_at_init_schema_enum_sibling_target_not_string`. Every such relationship is ignored, and each affected attribute continues through ordinary processing.
- Loading must reject a concrete item that assigns one otherwise-supported sibling target to multiple enums. Self-links, chains, and cycles through enum targets are instead ignored with the target-is-enum initialization issue, so an ignored target enum retains its ordinary validation and evaluates its own declaration independently.
- A class or object may declare a relationship that differs from the dictionary or another item; validation must not impose cross-scope consistency that event processing does not require.
- The toolkit must not reconstruct includes, inheritance, patches, or profile expansion.
- Profiles activate the source and target independently. The toolkit uses pair-specific processing only when both are active and otherwise processes an active member ordinarily, without diagnosing why the counterpart is inactive. OCSF Schema Compiler issue 22 proposes enforcing equal profiles as a stricter authoring rule, but that compiler enforcement remains TBD and is not a prerequisite for correct toolkit behavior.
- Scalar pairs operate on their two values, while array pairs require equal lengths and associate values by index.
- Eligibility and links must be resolved once during schema initialization. During an event walk, the visitor dispatches an accepted linked pair instead of visiting either member as an ordinary attribute; processors may reuse their ordinary attribute logic internally when pair-specific behavior is unnecessary.
- Tests should cover both classes and objects, direct and subtype declarations, scalar and array shapes, missing and enumerated targets, issue precedence, different valid relationships across processing scopes, shared siblings, chains and cycles, profile combinations, and hand-edited compiled schemas.
- These checks run during schema loading and must not add lookup, allocation, or graph-validation work to the per-event traversal path.

Reconsider only if event processing stops resolving enum siblings from concrete class and object definitions or supports an ambiguous relationship graph explicitly.

## Debatable or unresolved choices

No unresolved choice currently blocks review. Add a candidate here rather than treating observed implementation behavior as a settled decision.
