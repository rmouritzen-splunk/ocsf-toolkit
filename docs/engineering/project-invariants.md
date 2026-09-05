# Project and OCSF invariants

This engineering ledger records established invariants for ongoing development and regression review. It is evidence, not an independent source of authority. Deliberate design choices and their derived requirements live in [Project design decisions](project-decisions.md) so they can inform stable tests without being mislabeled as OCSF or external invariants.

## Review use

- Keep the general Code Reviewer analysis separate from the Regression Reviewer analysis.
- Inspect correctness-test and fixture changes separately from production changes so the new implementation does not bias interpretation of the prior contract. Agreement among changed code, tests, and documentation is not independent evidence of correctness.
- Treat benchmark tests as performance evidence rather than correctness authority. Investigate them deeply only when a commit changes a protected performance or allocation budget, or when a performance-motivated production change could alter behavior.
- Consult [Project design decisions](project-decisions.md) for independently established engineering choices and the stable-test requirements derived from them.
- Treat invariants as time-aware. Current OCSF or project behavior does not prove that the same contract applied to an older commit; use the source revision or released schema version applicable at the time.
- Stop and consult the maintainer when an invariant is debatable, appears to be violated, or a regression is detected. Do not silently resolve or review past those checkpoints.
- Do not fix review findings unless the fix is separately requested and sequenced.

## Evidence hierarchy

Authority depends on the kind of claim being evaluated:

1. The version-appropriate `ocsf/ocsf-schema` metaschema is authoritative for valid OCSF schema-definition structure. Versioned schema definitions are authoritative for the concrete classes, objects, attributes, types, requirements, enums, profiles, constraints, and descriptions in that schema version.
2. The OCSF Schema Compiler format contract, source, and tests are authoritative for the normal compiled-schema representation that this toolkit consumes. Compiler behavior does not independently redefine event semantics.
3. The OCSF schema `CHANGELOG.md`, repository history, and linked merged decisions establish when schema concepts or definitions were introduced, changed, or corrected.
4. Explicit maintainer decisions and the toolkit's public contracts establish toolkit-specific behavior where OCSF leaves implementation choices open.
5. Prior working implementations, including `ocsf-translator-go`, `ses-translator`, OCSF Server `validator2.ex`, and relevant OCSF Java tooling, are historical and implementation evidence. Common ancestry reduces their independence, and disagreement with an authoritative source may expose a defect in the prior implementation.
6. Released toolkit behavior, protected tests, fixtures, and the local event corpus are compatibility and reproduction evidence. A test protects an invariant only when the requirement it expresses has independent authority.
7. `ocsf/ocsf-docs` is explanatory evidence intended for humans. It is useful for discovering intent and examples but may be vague, inaccurate, or stale; verify consequential claims against stronger sources.

## OCSF invariants

### OCSF-CLASS-001: `class_uid` identifies the event class

An OCSF event's `class_uid` is required and identifies its event class. A processor cannot perform class-guided behavior until that value resolves a compiled class.

Authority: `ocsf-schema/events/base_event.json`; OCSF Schema Compiler `docs/format.md` class `uid`; `ocsf-docs/overview/understanding-ocsf.md` as supporting explanation.

### OCSF-TYPE-001: `type_uid` combines class and activity

The required `type_uid` value is `class_uid * 100 + activity_id`.

Authority: `ocsf-schema/events/base_event.json`; `ocsf-docs/overview/understanding-ocsf.md` as supporting explanation; prior OCSF Server validation behavior as implementation evidence.

### OCSF-REQ-001: requirements belong to an attribute use

`required`, `recommended`, and `optional` describe an attribute in the context of a compiled class or object use. They are not intrinsic dictionary-attribute requirements. The compiler supplies a requirement for every compiled class and object attribute.

Authority: `ocsf-schema/metaschema/attribute.schema.json`; OCSF Schema Compiler `docs/format.md` and compiler tests.

### OCSF-CONSTRAINT-001: constraints relate present attributes

`at_least_one` requires at least one named attribute and `just_one` requires exactly one named attribute in the applicable class or object context.

Authority: `ocsf-schema/metaschema/common-event-object.schema.json`; OCSF Schema Compiler `docs/format.md`.

### OCSF-PROFILE-001: profiles are optional overlays

Profiles overlay event classes and objects with attributes, requirements, and constraints. Compiled attribute profile metadata determines whether an attribute use depends on one or more active profiles.

Authority: `ocsf-schema/metaschema/profile.schema.json`, `common-event-object.schema.json`, and versioned profile definitions; OCSF Schema Compiler output contract and tests. `ocsf-docs` profile articles are supporting explanation only.

### OCSF-OBJECT-001: only the generic `object` type is open-ended

An attribute that directly uses `object_type: "object"` may contain arbitrary nested keys because the OCSF `object` definition is the generic object. Event classes and every concrete object type remain closed against their active compiled attributes. A concrete object does not become open-ended merely because profile filtering leaves no active attributes, and flattened inherited profile attributes are accepted only when their profiles are active.

Authority: `ocsf-schema/objects/object.json`; compiled class and object definitions; OCSF Schema Compiler profile-flattening behavior; toolkit validation tests.

### OCSF-ENUM-001: an enum sibling carries the human-readable value

A schema `sibling` pairs an enum source attribute with a string attribute. For a defined enum value, the sibling normally carries the enum label; when the source value is `Other`, it carries a source-specific label.

Authority: `ocsf-schema/metaschema/dictionary-attribute.schema.json` and `attribute.schema.json`; concrete versioned enum and sibling definitions.

### OCSF-ENUM-002: integral enum ID 99 means `Other`

For conventional integral OCSF enums, ID 99 represents `Other`, and its sibling must preserve a source-specific value rather than merely repeating the generic `Other` caption. A string enum key spelled `"99"` does not acquire integer semantics merely from its spelling.

Authority: concrete versioned enum definitions and sibling descriptions; the metaschema's enum-convention and sibling semantics. `ocsf-docs/overview/understanding-ocsf.md` is supporting explanation.

### OCSF-ENUM-003: enum-sibling relationships originate in the dictionary

An enum attribute's string sibling relationship is established in the attribute dictionary. Profiles elevate that relationship, and the schema compiler normally carries the logical pair into concrete class and object attributes. Attribute properties can be modified at the class and object level, so consumers process the relationship declared by each concrete item rather than reconstructing it from the dictionary.

Authority: the OCSF attribute-dictionary and compilation model; concrete versioned dictionary, profile, class, and object definitions; explicit maintainer and OCSF Schema Compiler author confirmation on 2026-08-23.

### OCSF-OBS-001: observables reference event content

The top-level `observables` array surfaces information already present elsewhere in the event. An observable identifies its type and event path; it is not a general store for unrelated new data.

Authority: `ocsf-schema/events/base_event.json`, `objects/observable.json`, and observable definitions allowed by the metaschema.

### OCSF-OBS-002: scalar and object observables have different value semantics

A scalar observable has a string `value` derived from the referenced scalar. An object observable names the object and omits `value`. An array of primitive observable sources produces one observable value per element; an array of objects produces object observables without values.

Authority: `ocsf-schema/objects/observable.json`; `ocsf-schema/metaschema/observable.schema.json`; `ocsf-docs/articles/defining-and-using-observables.md` as detailed but non-authoritative explanation; prior enrichment implementations as corroboration.

### OCSF-NUM-001: `long_t` is a signed 8-byte integer

The versioned OCSF dictionary defines `long_t` as an 8-byte signed integer, giving it signed 64-bit range. OCSF does not normatively assign a bit width to `integer_t` in the metaschema or its current dictionary description.

Authority: `ocsf-schema/dictionary.json`. OCSF Server's signed-128-bit `long_t` validation is a historical implementation divergence, not authority for this invariant.

## Compiled-schema invariants

### COMPILED-001: the toolkit consumes normal compile-version 1 output

The toolkit consumes the normal, non-browser OCSF Schema Compiler format with `compile_version: 1`. Browser metadata and legacy output are not part of the required processing contract.

Authority: OCSF Schema Compiler `README.md`, `docs/format.md`, source, and tests; toolkit public documentation.

### COMPILED-002: compiled class and object attributes are complete

Normal compiled class and object attributes have already incorporated dictionary definitions, includes, inheritance, patches, and profiles. A consumer must not reconstruct those source-level operations or replace the compiled result with independently derived definitions.

Authority: OCSF Schema Compiler `docs/format.md`, source, and tests.

### COMPILED-003: compiled enum keys preserve their logical primitive type through schema context

JSON object keys in an `enum` are strings in the compiled representation. Their logical meaning depends on the resolved OCSF primitive type; numeric spelling alone must not turn a string enum into an integral enum.

Authority: OCSF Schema Compiler `docs/format.md`; compiled attribute and dictionary type definitions.

### COMPILED-004: a compiled schema identifies its OCSF schema version

Normal compiled output includes the source OCSF schema `version`. The toolkit requires that value and rejects a compiled schema whose version is absent or cannot be parsed as the supported semantic-version form before constructing event-processing pipelines.

Authority: OCSF Schema Compiler `docs/format.md`; toolkit loader and architecture contracts.

### COMPILED-005: concrete enum-sibling relationships are locally unambiguous

Within each compiled class and object, every relationship accepted for enum-sibling processing is one-to-one: an enum has one declared string sibling and a string sibling belongs to only one accepted enum relationship. Classes and objects are independent processing scopes, and their concrete relationships need not be compared with the dictionary or with one another. Ineligible relationships are ignored with specific initialization issues rather than linked into traversal metadata; a shared target among otherwise-eligible relationships remains an invalid compiled schema.

Authority: `DECISION-ENUM-001`; concrete class and object definitions consumed by event processing; explicit maintainer confirmation on 2026-08-23.

## Toolkit external invariants

### TOOLKIT-CLASS-001: failed class resolution stops before mutation

If `class_uid` is missing, has the wrong type, or does not resolve a class, processing reports the mandatory class-resolution issue and stops before every mutation, including forced removal. Validation additionally reports its class finding when enabled unless policy ignores that validation code. The validation finding is optional; the processing issue cannot be ignored.

Authority: `docs/event-processing.md`, `docs/architecture.md`, the public processing contract, and explicit maintainer confirmation on 2026-08-22 that force removal must not run without `class_uid` resolution.

### TOOLKIT-NUM-001: OCSF integral values use one signed-64-bit carrier

The toolkit accepts signed 64-bit values for both `integer_t` and `long_t`. This is an explicit toolkit compatibility policy where OCSF leaves `integer_t` width unspecified: it preserves the expected `integer_t`-within-`long_t` relationship, avoids platform-dependent Go `int` behavior, and represents the authoritative signed-64-bit `long_t` range without loss. It does not claim that every downstream implementation accepts the entire signed-64-bit `integer_t` range.

Authority: explicit maintainer decision on 2026-08-22; `AGENTS.md`; `docs/architecture.md`.

### TOOLKIT-NUM-002: integral conversion is exact

Class IDs, enum values, constraints, and other integral semantics accept a representation only when its mathematical value is finite, in signed-64-bit range, and exactly integral. Decimal or exponent spellings must not be rounded, truncated, underflowed, or overflowed into a different integer. JSON input should retain `json.Number` when possible.

Authority: toolkit numeric contract; `eventpipeline/invariant_test.go`; `TestInvariantScalarConversionsPreserveEquivalentValues`; and `TestInvariantValueConstraintsUseTypedEquality`.

### TOOLKIT-STRING-001: `max_len` counts Unicode code points

String `max_len` constraints count Unicode code points, represented as Go runes, rather than encoded bytes or grapheme clusters. Canonically equivalent strings can therefore have different lengths when one uses a precomposed code point and the other uses a combining sequence.

Authority: explicit maintainer decision on 2026-08-22; `docs/validation.md`; `docs/FAQ.md`; `TestProcessEventValidationMaxLenCountsUnicodeCodePoints`. The OCSF metaschema requires a nonnegative maximum length but does not specify its measurement unit, so this is a toolkit invariant rather than an OCSF invariant.

### TOOLKIT-NULL-001: missing and null object attributes are equivalent

OCSF has no logical null value. A missing event-map key and a key represented without a value are the same absent attribute for validation, enrichment, removal, observable resolution, diagnostics, and result counting. Observable `value` has no exception: omitted and nil-valued map entries both denote a valueless object observable. Array positions represented without a value remain elements and are invalid because OCSF array element types do not include null. A removal processor may delete a nil-valued map entry within its configured scope without counting a logical value as removed; other processors do not normalize it.

Authority: explicit maintainer decision on 2026-09-04; `docs/event-processing.md`; `docs/architecture.md`; public processing behavior.

### TOOLKIT-JSON-001: object decoders require one non-null object

Public JSON object decoding accepts exactly one non-null JSON object, rejects trailing JSON values and top-level null, and preserves JSON numbers as `json.Number`. File and filesystem wrappers retain this behavior while adding path context to errors.

Authority: the public `jsonio` API documentation and tests; the toolkit's exact numeric contract.

### TOOLKIT-MUTATION-001: mutation is in-place and non-transactional

Event enrichment and enrichment removal mutate the supplied `jsonish.Map` in place. A later failure may leave earlier mutations in place.

Authority: `AGENTS.md`, `docs/architecture.md`, `docs/event-processing.md`, and public API documentation.

### TOOLKIT-ORDER-001: mutation precedes validation in a fixed order

Enum-sibling mutation completes before observable generation or removal, and every mutating operation completes before validation. Validation therefore observes the final event state regardless of option order.

Authority: `AGENTS.md`, `docs/event-processing.md`, and `docs/architecture.md`.

### TOOLKIT-ENUM-001: a supported enum and sibling are one exactly-once unit

A supported scalar or array enum and its same-shaped string sibling are paired and processed exactly once from the enum's schema entry when both attributes are active. Eligibility follows `DECISION-ENUM-001`; it is a defensive toolkit contract rather than a normative OCSF invariant. Array elements pair only by matching index. Profiles activate each attribute independently. When only one member is active, process that member as an ordinary attribute and do not perform pair-specific enrichment, removal, or validation through the inactive member.

Authority: `docs/event-processing.md`, `docs/enrichment.md`, `docs/enrichment-removal.md`, and `docs/validation.md`.

### TOOLKIT-ENRICH-001: enrichment preserves existing non-null siblings

Enum enrichment fills a missing or null supported sibling from the schema caption but does not replace an existing non-null sibling, including a source-specific ID 99 value.

Authority: `docs/enrichment.md` and public enrichment behavior; prior translator enrichment implementations as historical evidence.

### TOOLKIT-REMOVE-001: safe removal requires proof of redundancy

Safe enrichment removal deletes only content proven redundant with schema-derived or referenced event content. After successful class resolution, forced observable removal deletes the entire top-level `observables` attribute without inspecting its entries or reporting per-entry removal issues. Forced removal is explicitly destructive but retains the OCSF-required source-specific sibling for integral enum ID 99.

Authority: `docs/enrichment-removal.md`, public enrichment-removal behavior, and explicit maintainer confirmation on 2026-08-22.

### TOOLKIT-OBS-001: observable analysis is schema-valid and event-aware

Observable names must be valid for the active compiled class and resolve consistently against actual event content. Supported array notations are semantic alternatives, while exact supplied spelling remains relevant to duplicate identity and notation diagnostics.

Authority: `docs/enrichment.md`, `docs/enrichment-removal.md`, and `docs/validation.md`; OCSF observable schema and documentation for the underlying path concept.

### TOOLKIT-OBS-002: observable type selection filters generation only

An omitted observable type selection generates every schema-declared observable type. A nonempty selection is validated completely against the loaded schema before events are processed and applies consistently to class paths, object types, attributes, dictionary attributes, and dictionary types. Excluded declarations generate neither observables nor malformed-source enrichment issues. Selection does not filter existing observables or change enrichment removal or validation.

Authority: `README.md`, `docs/enrichment.md`, `docs/architecture.md`, and public pipeline construction behavior.

### TOOLKIT-OBS-003: generated observables merge without replacing existing entries

Observable enrichment preserves every existing entry and appends generated entries in deterministic traversal order. Deduplication is disabled by default. Generated mode silently suppresses only a later generated candidate that duplicates an earlier generated candidate; it never scans, suppresses, or removes an existing entry, and a generated candidate matching only an existing entry is retained. Duplicate identity uses the exact observable name, integral-equivalent `type_id`, and optional exact string value; omitted and nil-valued map entries both represent no logical value. Derived type captions and unrelated fields do not affect identity. A suppressed generated candidate is excluded from the added count.

Authority: explicit maintainer confirmation on 2026-09-04, `docs/enrichment.md`, `docs/architecture.md`, and public observable enrichment behavior.

### TOOLKIT-OBS-004: duplicate diagnostics are independent and avoid duplicate ownership

The default-ignored observable duplicate issue detects existing-existing, generated-existing, and generated-generated identity collisions during observable addition independently of generated deduplication. The default-ignored observable duplicate validation detects collisions in the final observable array. If both are enabled during observable addition, the issue is the sole diagnostic owner so the same condition is neither scanned nor reported twice. Generated deduplication itself emits no issue or validation finding.

Authority: explicit maintainer confirmation on 2026-09-04, `docs/enrichment.md`, `docs/validation.md`, and public issue and validation code contracts.

### TOOLKIT-RESULT-001: findings are data and processing failures are errors

Validation errors and warnings are successful processing results. Go errors represent inability to process. Nonfatal mutation and traversal issues are reported separately from validation findings.

Authority: `docs/event-processing.md`, `docs/architecture.md`, and public result APIs.

### TOOLKIT-DIAGNOSTIC-001: diagnostics do not repeat event values

Processing issues, validation findings, and Go errors identify paths and safe schema context without copying event values into messages or details.

Authority: `docs/architecture.md`, `docs/validation.md`, and the documented security and retention rationale.

### TOOLKIT-CODE-001: diagnostic codes are typed machine-readable identities

Processing issues and validation findings carry typed codes with stable string encodings distinct from human-readable messages. Processing issue strings use the `issue_` namespace. Validation code strings use the `validation_` namespace, while effective warning or error level is separate policy rather than part of code identity. Within a stable major version, exported code names and ordinals, string encodings, default levels, and ignorable or mandatory classifications remain stable. A code that stops being emitted remains exported at its existing ordinal and is marked deprecated for the rest of that major version. New codes append rather than shifting existing ordinals. Human-readable descriptions may be clarified without changing code identity.

Authority: `README.md`, `docs/architecture.md`, `docs/validation.md`, and the public `issue` and `validation` packages.

### TOOLKIT-CONCURRENCY-001: schemas and pipelines are reusable immutable state

Loaded schemas and constructed pipelines are immutable and safe for concurrent use when each call receives a distinct event map. One event map and its nested storage must not be accessed concurrently during processing.

Authority: `docs/architecture.md` and public API documentation.

## Toolkit engineering invariants

### ENGINEERING-HOTLOOP-001: reusable work stays outside the event loop

Schema metadata, processor configuration, compiled constraints, caches, and other reusable state are immutable and pipeline-owned. Per-event contexts contain event-specific mutable state and references to shared data; processing does not rebuild reusable metadata or copy processor collections for each event.

Authority: `AGENTS.md`, `docs/architecture.md`, allocation tests, and benchmark evidence.

### ENGINEERING-TRAVERSAL-001: recursive object traversal is bounded and reported

The shared repeated-object-attribute boundary prevents unbounded schema-guided recursion. Content beneath the boundary is retained and processing reports at most one mandatory traversal-limited issue using the first affected path.

Authority: `docs/event-processing.md`, `docs/architecture.md`, and engineering tests.

### ENGINEERING-SCHEMA-001: cyclic type inheritance fails safely

Schema and validation-cache initialization reject cyclic dictionary type inheritance independently of map iteration or which type is resolved first.

Authority: `docs/architecture.md` and `TestEngineeringInvariantBuildRejectsCyclicTypeDefinitions`.

### ENGINEERING-SCHEMA-002: malformed index inputs fail safely

When compiled-schema content required for runtime indexing is malformed, schema construction returns a contextual error rather than panicking or silently building an ambiguous index. This defensive boundary does not make the toolkit responsible for reproducing the OCSF Schema Compiler's complete semantic validation.

Authority: the repository's untrusted-schema robustness requirements, `docs/architecture.md`, and schema-construction tests.

### ENGINEERING-RELEASE-001: destructive release cleanup is confined

Release build and packaging automation may destructively clear only the repository-owned scratch directories selected by the scripts. Supported platform targets must be validated before cleanup begins, so an invalid target cannot erase an otherwise usable build or distribution tree.

Authority: current release build and packaging scripts, their tests, and the repository's untrusted-path robustness requirements.

## Debatable or unresolved candidates

No unresolved candidate currently blocks review. Add candidates here rather than promoting them silently when evidence conflicts or the contract requires a maintainer decision.
