# Frequently asked questions

If OCSF Toolkit behavior does not meet your needs, open an issue at https://github.com/ocsf/ocsf-toolkit/issues with the relevant schema definitions and a minimal, non-sensitive example. This applies to the limitations and deliberate behavior described throughout this FAQ.

## Do diagnostics include event values?

No. Processing issues, validation warnings, validation errors, and returned Go errors identify a problem using stable codes, exact attribute paths when applicable, expected schema conditions, and other non-sensitive context without copying event values into their messages or detail maps. These results are natural candidates for logs or application-error events, so omitting event values reduces the risk that diagnostic handling unintentionally discloses sensitive event content. Investigating a problem may require correlating the diagnostic with the original event.

Processing-result messages preserve ordinary graphic Unicode but render control characters, non-graphic Unicode, and invalid UTF-8 bytes as visible escape sequences. This prevents schema-provided text from inserting new log lines or terminal control sequences when messages are logged without additional filtering.

`metadata.uid` is the one event-supplied value that may be used for correlation in a returned Go error. OCSF defines it as the event's unique identifier, and producers should treat it as an opaque identifier rather than placing sensitive content in it. Attribute paths and attribute names are also diagnostic identifiers and may appear in results. If either behavior is unsuitable for an environment, apply the appropriate logging controls or open an issue to discuss the use case.

## How does the toolkit represent an attribute with no value?

OCSF has no logical null value or null type, although carrier representations such as JSON and `jsonish.Map` can represent one. At an object-attribute boundary, the toolkit deliberately treats a carrier-level null as an absent OCSF attribute. In JSON input, an omitted property and a property whose value is `null` therefore have the same OCSF meaning. This representation punning is an encoding convenience for JSON, other encodings, and programmatic event producers for which explicitly representing an attribute without a value is easier than omitting it.

The Go implementation represents events with `jsonish.Map`. Processing reads an absent map key and a key whose value is `nil` identically, using a direct map lookup followed by a `value == nil` check. This is an implementation decision rather than a requirement for implementations using another in-memory representation. Removal processors may delete a physically present nil-valued map entry within their configured scope, but do not count it as a removed logical value; other processors do not normalize the representation merely because the value is nil.

An array position is different. A carrier representation can contain `[null]`, and `jsonish.Map` values can contain an array with a `nil` element, but that element has no type in the OCSF type system. It is an illegal OCSF element type rather than an omitted element, so validation reports `validation_attribute_wrong_type` at its indexed path.

## How do issue and validation error levels differ?

All processing issues default to `warning`. Warning-level issues are collected in the successful processing result. An ignorable issue may instead be omitted, while elevating an issue to `error` stops processing at the first matching issue and returns a `ProcessingIssueError`. As with any non-nil `ProcessEvent` error, the accompanying result is the zero value, although earlier in-place event mutations are not rolled back.

Validation findings are different. Their toolkit defaults vary by validation code, and warning-level and error-level findings are both accumulated in the validation result. An error-level validation finding does not make `ProcessEvent` return a Go error or stop validation at the first finding. This lets one pass report every validation problem it encounters. Library callers decide how to handle the collected levels; the CLI exits nonzero for collected error-level findings only when `--fail-on-validation-errors` is selected.

## What does `issue_event_traversal_limited` mean?

OCSF object definitions can be recursive, such as a file with a parent file or a person whose LDAP manager is another person. OCSF Toolkit permits useful recursive relationships, but stops descending when the current object attribute name already appears earlier on the active event path. The repeated object attribute is encountered and reported as the traversal boundary, but the object stored in that attribute is treated as opaque: none of its attributes are enriched, removed, or validated.

OCSF provides a process-specific recommendation in [Representing Process Parentage](https://github.com/ocsf/ocsf-docs/blob/main/articles/representing-process-parentage.md): populate `process.parent_process` only on the top-level process object to avoid deep nesting, and use `process.ancestry` when ancestry beyond the immediate parent is needed. The toolkit follows the same effective processing depth: it fully processes `process.parent_process`, but if `process.parent_process.parent_process` is present, it reports that second `parent_process` attribute as the boundary and does not process the object stored there. An event following the OCSF recommendation does not contain that second `parent_process` and therefore does not reach the toolkit boundary through this relationship.

Recursion can also return to an object type through differently named relationships rather than an immediate self-reference. One real schema path is `person.ldap_person.manager.ldap_person`: an LDAP person's manager is another person, which can contain another `ldap_person`. Traversal reaches the second `ldap_person` attribute and reports that exact path as the boundary, but it does not process any attributes inside the second `ldap_person` object because the `ldap_person` attribute name already occurred on the active branch. This attribute-name rule covers both immediate and indirect recursive relationships without maintaining an object-type recursion graph.

This boundary is a performance and robustness safeguard. Recursive object relationships, particularly when combined with arrays or multiple recursive branches, can produce a geometrically growing nested structure. Treating the first repeated relationship as opaque prevents schema-guided processing from following that expansion indefinitely or spending disproportionate work below a recursion boundary. It does not impose a general event-size or array-breadth limit; callers handling untrusted input remain responsible for those limits as described below.

When an event reaches this boundary, the processing report contains at most one top-level issue for the event. The first affected indexed path is included in `details`:

```json
{
  "source": "processing",
  "code": "issue_event_traversal_limited",
  "message": "Event processing did not inspect content beyond at least one recursive object relationship because it reached the supported traversal limit; the first affected path was \"file.parent.parent\".",
  "details": {
    "attribute_path": "file.parent.parent",
    "attribute": "parent",
    "object_type": "file"
  }
}
```

This issue describes an internal processing limitation, not proof that the event is invalid. It therefore appears in top-level `issues`, including for a validation-only operation, and is not duplicated in `validation.warnings` or `validation.errors`. Enrichment, enrichment removal, and validation may all be incomplete below the reported boundary.

Consumers can route this condition by its stable `issue_event_traversal_limited` code. The toolkit does not allow its level to be set to ignored because it reports incomplete processing rather than a tolerable processor-specific condition.

See [Recursive objects](event-processing.md#recursive-objects) for the language-neutral traversal rule.

## How do I report missing recommended attributes?

The `validation_attribute_recommended_missing` code defaults to `ignored` because recommended attributes are optional. The validator skips this check unless policy sets the code, or an applicable all-code baseline, to `warning` or `error`. Set its level explicitly when an environment wants to report or reject their absence:

```sh
ocsf-toolkit \
  --schema ocsf-schema.json \
  --event event.json \
  --validate \
  --validation-level validation_attribute_recommended_missing=warning
```

The corresponding Go configuration is:

```go
eventpipeline.WithValidation(
	eventpipeline.WithValidationLevel(
		validation.AttributeRecommendedMissing,
		validation.LevelWarning,
	),
)
```

Use `error` instead of `warning` when absence should be an error-level validation finding. As with other validation errors, the CLI changes its exit status only when `--fail-on-validation-errors` is also selected.

## With the CLI, why do I see initialization issues when I use `--quiet`?

`--quiet` suppresses the default directory summary on stdout. It does not suppress diagnostics written to stderr. Initialization issues describe nonfatal problems found while preparing the compiled schema for event processing, before any event is read, so the CLI writes each non-ignored issue to stderr in both single-event and directory modes. Summary files also include these issues as durable run-level diagnostics, but individual event reports do not because the condition belongs to the schema rather than an event.

To silence an initialization issue, set its stable `issue_at_init_*` code to ignored with `--issue-level ISSUE_CODE=ignored`. Enum-sibling initialization distinguishes an ineligible source (`issue_at_init_schema_enum_sibling_source_not_integral`), a missing target (`issue_at_init_schema_enum_sibling_target_not_found`), an enum target (`issue_at_init_schema_enum_sibling_target_is_enum`), and a target without the required direct string type and matching scalar/array shape (`issue_at_init_schema_enum_sibling_target_not_string`). For example, `--issue-level issue_at_init_schema_enum_sibling_target_not_string=ignored` accepts that known processing limitation. Level policy is explicit because quiet output and accepting a known limitation are separate choices; setting the code to error instead aborts schema setup.

## Why does directory output use separate `events/` and `reports/` subdirectories?

The fixed subdirectories design around two filesystem problems. Processed events and processing reports can preserve the same input-relative path without competing for the same filename, and one `--output-dir` avoids separate event and report output directories whose trees could overlap or alias one another through links or platform-specific path behavior. Keeping the two output shapes in distinct namespaces makes collision and confinement checks simpler and more predictable.

This is structural protection rather than an exhaustive filesystem preflight. Reports also insert `.report` before the input extension for clarity, but the `events/` and `reports/` boundary is what ensures that the corresponding event and report occupy different directory trees.

## What happens if input or output directories change during processing?

Directory processing uses normal filesystem semantics. The input and output directory trees are expected to remain stable while the CLI is processing them. If another process concurrently adds, removes, renames, replaces, or otherwise modifies entries in those trees, processing may fail or may operate on the modified tree.

`--overwrite` is intended for rerunning the toolkit into a stable output tree created by an earlier toolkit run. In that ordinary use, it replaces the selected event and report files while leaving unrelated files alone. Manually modifying that tree, especially by adding symbolic links or hard links, can cause processing to fail or produce unexpected replacement behavior. Concurrent modification carries the same general risk without `--overwrite`; the flag changes whether selected existing destinations may be replaced, not whether the run is isolated from filesystem changes.

The CLI rejects unsafe path spellings and filesystem conditions it observes while preparing or traversing the selected trees, and it stops when a filesystem operation fails. These checks enforce the requested directory layout under ordinary use; they do not provide snapshot isolation or a security boundary against concurrent filesystem mutation.

Before processing, `--events-dir` must exist and name an actual directory rather than a symbolic link. Symbolic links found within that directory tree are ignored rather than followed. If the selected input directory is replaced by a file or symbolic link after preflight but before traversal observes its root, the walk processes no events and may complete successfully, just like an empty directory. If the path instead disappears or traversal encounters another filesystem error, processing fails with that error.

## Can two different input files produce the same output path?

In directory mode, each input file's output path is derived from its path relative to `--events-dir`. The CLI checks up front that the `--events-dir` and `--output-dir` trees themselves do not overlap (see [Output Behavior](../README.md#output-behavior) in the README), but it does not separately check whether two *distinct* input files end up deriving the *same* output path within a single run. This differs from single-event mode, where every explicitly selected destination (`--event-output`, `--report-output`, the schema path, summary files) receives a best-effort preflight comparison using the platform's path semantics and any filesystem identity already observable. A detected preflight collision leaves both outputs unwritten. Event and report outputs are also compared after the event write; if their shared identity becomes observable only then, the completed event remains and the report is suppressed. Case aliases can therefore fail during preflight on one platform and in flight on another without either output replacing the other.

A collision here requires an unusual setup: for example, an output filesystem that folds case when the events directory contains two regular files whose relative paths differ only by case, or an events directory made reachable through more than one name via filesystem links. With `--overwrite`, the file processed later in the walk silently replaces the earlier file's output; without `--overwrite`, the second write fails with an "already exists" error, since the first write already created that path earlier in the same run.

The toolkit intentionally does not add cross-file identity tracking to catch this. It is a local testing tool, not a filesystem-integrity checker, and cross-linking or case-colliding filenames inside an events directory is not an expected input shape. If this affects a real workflow, open an issue describing it.

## Does the recursive-object boundary limit all event-processing resource use?

No. The recursive-object boundary prevents unbounded descent through repeated object relationships, but it does not limit the number of elements in an array, the number of separate arrays in an event, or breadth across nested arrays. OCSF 1.9.0 permits schema paths containing as many as seven nested arrays. The number of values in such a structure can grow geometrically with the cardinality at each level, but every visited value must be present in the input event: toolkit processing remains approximately linear in the size of the actual event structure.

The toolkit does not currently impose a general event-size limit, a per-array limit, a limit on returned validation errors, warnings, or processing issues, or a limit on generated observables. A per-array limit alone would not bound total work across multiple or nested arrays. Callers processing untrusted data should enforce input limits appropriate to their environment. Configurable per-event processing and result limits are tracked as possible future work in the [roadmap](roadmap.md#validation-and-schema).

## Why does the toolkit interpret `integer_t` as a signed 64-bit integer?

The OCSF type definition describes `integer_t` as a signed integer but does not specify its bit width or range. It separately defines `long_t` as an 8-byte signed integer, giving `long_t` a signed 64-bit range. OCSF Toolkit uses one signed 64-bit representation and range for both types. This avoids platform-dependent Go `int` behavior, preserves every `long_t` value, and follows the conventional relationship that the range of an integer type should not exceed the range of its corresponding long type; the two ranges may be equal.

This is a toolkit interpretation where OCSF leaves `integer_t` width open, not a claim that OCSF requires every implementation to accept 64-bit `integer_t` values. A downstream system that chooses a signed 32-bit `integer_t` range may reject values outside that range. A possible pipeline option and CLI flag for selecting signed 32-bit or signed 64-bit `integer_t` bounds is tracked in the [roadmap](roadmap.md#validation-and-schema); signed 64-bit remains the current behavior.

## How does `max_len` measure string length?

OCSF Toolkit counts Unicode code points, represented as Go runes. It does not count UTF-8 bytes. This keeps `max_len` independent of the string's encoded byte length, consistent with OCSF being a logical schema.

A Unicode grapheme cluster—the closest technical unit to one user-perceived character—can contain multiple code points. For example, a precomposed `é` contains one code point, while `e` followed by a combining acute accent contains two code points even though both forms are normally perceived as one character. The toolkit therefore counts the first form as length 1 and the second as length 2.

The toolkit does not normalize strings before measuring them: it applies no Unicode Normalization Form, including NFC, NFD, NFKC, or NFKD. It counts the code points in the value exactly as supplied. The same preserve-as-supplied rule applies to exact string comparisons, including string allowed-value constraints, enum siblings and their schema captions, observable duplicate identity, and matching string-valued observables to event content. Canonically equivalent strings with different code-point sequences can therefore have different measured lengths and compare as distinct values. OCSF does not direct implementations to normalize these strings or select a normalization form, and implicit normalization could conflate values that a producer intended to distinguish. Producers or callers that require canonical equivalence must choose a normalization form appropriate to their data contract and apply it consistently before toolkit processing.

Counting grapheme clusters would more closely match user-perceived characters, but Go's standard library does not provide Unicode grapheme segmentation. The toolkit uses the standard library's rune count rather than adding a Unicode-segmentation dependency for this constraint. Code-point counting is conservative relative to grapheme-cluster counting: a grapheme containing multiple code points consumes multiple units and can therefore reach the limit sooner, but it cannot make an overlong value pass by undercounting its constituent code points. This provides a practical balance of validation strictness, performance, and implementation complexity. The OCSF metaschema defines `max_len` as a maximum length without specifying bytes, code points, or grapheme clusters, so this is an explicit toolkit interpretation that may be revisited if OCSF defines the unit more precisely or the Go standard library gains suitable support.

## Which Go numeric representations are supported?

Event processing accepts `json.Number`, `float32`, `float64`, and Go's signed integer types. It intentionally does not parse ordinary strings as numbers and does not accept unsigned integers, arbitrary-precision numeric types, or custom numeric types. Array attributes support slices and fixed-length arrays, including defined container types. Each element is inspected according to the same value rules regardless of whether the container is heterogeneous or homogeneous: an unsupported element produces an ordinary wrong-type validation finding rather than a processing error caused by its container representation. A type alias remains identical to the aliased element type, while a defined element type remains unsupported even when its underlying type is supported.

Numeric compatibility is based on the represented value rather than whether its carrier is conventionally called an integer or floating-point type. An OCSF `integer_t` or `long_t` accepts a floating-point representation only when its value is finite, mathematically integral, and within the signed 64-bit range. It never truncates a fraction or substitutes zero for an invalid conversion. An OCSF `float_t` accepts compatible integral representations as well as floating-point representations.

For `json.Number`, ordinary signed integer syntax takes the direct integer conversion path and is rejected if it falls outside the signed 64-bit range. Decimal and exponent spellings are normalized exactly and accepted for an OCSF `integer_t` or `long_t` attribute only when their original numeric value is mathematically integral and within the signed 64-bit range. This accepts equivalent spellings such as `1`, `1.0`, and `1e0` without allowing floating-point rounding or underflow to change the represented value.

A value already carried in `float32` or `float64` is checked as the value that type can represent. If an upstream system converted a large integer to floating point and lost precision, the toolkit cannot recover the original integer. Use `jsonio` to preserve JSON numbers as `json.Number` when exact signed 64-bit integer values matter.

## Should Go library callers use slices or fixed-length arrays for OCSF array values?

Prefer ordinary unnamed slices when choosing a representation. Slices and fixed-length arrays have the same logical processing behavior, but unnamed slices of the toolkit's supported element types use concrete access paths and are marginally more efficient. Defined slice and array containers use reflective fallback because a runtime type switch does not match a defined type as its underlying unnamed type; fixed-length arrays also have their length as part of their Go type, so `[2]string` and `[3]string` are distinct types. JSON decoders naturally represent arrays as unnamed slices, so this distinction primarily affects events constructed programmatically or decoded from another representation.

## How are `NaN` and infinite `float_t` values handled?

Programmatically constructed events and non-JSON encodings can represent `NaN`, positive infinity, and negative infinity. The toolkit treats all three as legal `float_t` values rather than type errors. Observable enrichment converts them to the strings `"NaN"`, `"+Inf"`, and `"-Inf"`.

A non-finite value cannot satisfy a finite schema range, so validation reports `validation_attribute_value_exceeds_range` when a range constraint applies. This is intentionally conservative because OCSF does not define how non-finite values interact with range constraints.

Standard JSON cannot encode these values. The CLI reads events from JSON and therefore does not ordinarily encounter them. Library callers using Go maps or another input encoding are responsible for selecting an output encoding that can represent them.
