# Enrichment

Enrichment adds redundant, schema-derived information that makes OCSF events easier to read, search, report on, and use in cybersecurity detections. OCSF Toolkit can add enum siblings and observables independently.

Enrichment follows the shared recursive-object boundary described in [Event Processing](event-processing.md#recursive-objects). If an event reaches that boundary, descendants are not enriched and the report contains the single top-level `issue_event_traversal_limited` processing issue for the event.

This guide describes the language-neutral behavior. See [Event Processing Model](event-processing.md) for class resolution, profiles, null handling, encodings, in-memory representations, and schema-guided walking.

## Enum Siblings

An enum sibling is a descriptive string attribute associated with an enum attribute. For example:

```json
{
  "activity_id": 1,
  "activity_name": "Create"
}
```

For each active scalar or array enum attribute with a supported same-shaped schema-defined string sibling:

1. Leave an existing non-null sibling unchanged.
2. Resolve the enum value to its schema key and find that key in the attribute's enum definition.
3. If the enum entry has a usable caption, add the caption as the sibling value.
4. Otherwise, leave the sibling absent and report an enrichment issue.

This project supports direct `integer_t` and `long_t` enums whose declared sibling has direct type `string_t` and the same scalar or array shape. Named subtypes do not qualify. A missing target, an enum target, a target of another direct type, or a scalar/array mismatch produces a specific schema initialization issue; an ineligible source with a declared sibling also produces a specific issue. The ignored relationship is not mutated. For a supported array, enrichment adds the complete caption array only when every enum value has a usable schema caption. The generated sibling array has exactly one caption at the matching index for every enum array element. An existing non-null sibling array remains unchanged; validation reports it if it is not a valid parallel array.

By OCSF convention, integral enum ID 99 means `Other` and requires its string sibling. This rule applies to a scalar integral enum and to each index of an integral enum array. When the sibling is missing, enrichment adds the schema caption, normally `Other`, and reports that addition because the generic caption cannot recover a source-specific value that may have been lost upstream. Enrichment never replaces an existing source-specific sibling for ID 99. String enums have no equivalent special value; a string enum key `"99"` follows ordinary caption rules.

## Observables

The top-level `observables` array is an index into values that already exist elsewhere in the event. It is not a general key-value store. Each observable names an event path and identifies the observable type. A scalar observable also contains a string `value`; an object observable omits `value` and names the object itself.

Observable definitions can come from:

- class-level observable path definitions;
- observable object types;
- attributes or their dictionary definitions with an observable type.

Observable generation may be restricted to a selected set of observable type IDs. Validate the complete selection against the loaded schema before processing events and report all unknown IDs together. An omitted or empty selection means all observable types. Apply a nonempty selection consistently to class-level paths, object types, attributes, dictionary attributes, and dictionary types. Excluded declarations produce neither observables nor malformed-source enrichment issues. Selection affects only newly generated observables; it does not filter existing observables or alter enrichment removal and validation.

As the active event structure is walked, collect generated observables rather than repeatedly modifying the top-level array. For array values, generate an observable for each qualifying element while using the appropriate observable name form. Generated-observable deduplication is disabled by default. When generated mode is selected, retain only the first generated candidate of each identity. This optimization compares generated candidates only with earlier generated candidates and never compares them with existing observable entries. Add the retained entries only after the event walk is complete.

Generated observable names may use simple array traversal (`resources.uid`), empty brackets (`resources[].uid`), a wildcard (`resources[*].uid`), concrete indexes (`resources[1].uid`), or root-relative JSONPath (`$.resources[1].uid`). The notation changes only the path spelling, not which values are observable.

Scalar values are represented in the observable `value` string using stable scalar-to-string formatting. Strings remain unchanged, including the empty string; booleans, signed integers, and floating-point values use their conventional textual forms. Do not generate scalar observables from objects, arrays, or values that cannot be represented safely. If structured content occurs where the schema declares an observable scalar, leave it unchanged, skip that observable, and report an enrichment issue.

Treat `json_t` values as opaque and do not generate observables from them, including when a private extension declares one as an observable source. The type has historically represented both arbitrary in-memory values and JSON-encoded strings, so an implementation cannot reliably select scalar or object observable semantics. Leave the value unchanged and report an enrichment issue directing users with a concrete use case to https://github.com/ocsf/ocsf-toolkit/issues.

## Existing Observables And Duplicates

If `observables` has no logical value or is an empty array, enrichment may start a new array. If it is a valid non-empty array, append generated entries while preserving existing entries. When enrichment has nothing to add, it does not normalize a nil-valued map entry by deleting it.

Generated entries form a suffix after any existing entries. When enrichment and validation run in one operation, an implementation may treat that suffix as semantically valid by construction and avoid resolving it again. Existing entries must still be validated, and preferred-notation warnings still apply to generated names. Independently test generated events with validation performed in a separate operation so this optimization cannot hide a generator defect.

Duplicate identity includes the observable name, type, and optional string `value`. A missing or nil-valued map entry has no logical value and identifies the same valueless object observable. With deduplication disabled, enrichment appends every generated candidate. With generated deduplication enabled, a later generated candidate that duplicates an earlier generated candidate is omitted and does not contribute to the added count. A generated candidate that duplicates an existing entry is still appended, and existing entries are never removed or deduplicated by this option.

Observable names are compared as supplied. Do not treat path spelling variations such as a bare array name and an explicit `[]` form as duplicates merely because they can resolve to the same event value. Normalizing those forms could discard intentionally distinct source entries.

Duplicate reporting is independent of generated-observable deduplication. When enabled, the duplicate issue detects existing-existing, generated-existing, and generated-generated identities and identifies the origin and index of both the duplicate and the first occurrence. It defaults to ignored. Generated deduplication itself is silent whether or not reporting is enabled. If duplicate reporting is elevated to error, report the first duplicate after event traversal and before appending any generated observables; earlier in-place mutations are not rolled back. If an existing `observables` value is malformed rather than an array, report the issue instead of replacing non-empty source content.

After enrichment, remove the `observables` attribute if it would otherwise contain an empty array. This avoids retaining an attribute that carries no information.

## Language-Neutral Algorithm

```text
resolve the event class and active profiles
if the class cannot be resolved, report the class problem and stop

classify the existing observable destination without mutating it
walk active class and nested object attributes:
    for a supported scalar or array enum with a string sibling:
        add its missing schema-caption sibling when possible
    for a schema-defined observable source selected for generation:
        derive one or more observable entries
        if generated deduplication is enabled, retain only the first generated occurrence of each identity

if duplicate issue reporting is enabled, detect duplicates across existing and generated observables
emit duplicate issues in observable order
append retained generated observables to valid existing observables
remove observables if the final array is empty
return mutation counts and warning-level issues, or stop with the first error-level issue
```

An implementation backed by structs, classes, or columnar data can perform the same steps through field metadata or nested-column access. It does not need to materialize the entire event as generic maps, provided its chosen destination representation can express added and removed fields and array entries.

## Reference Implementation

`internal/processing/enrichment.go`, `class_observables.go`, and `observable_type_selector.go` are a fully functioning, tested example of this algorithm. `issue/code.go` lists every enrichment issue code it can produce. Read them directly for anything this guide leaves out. The public API and internal design built on top of this algorithm are described in [architecture.md](architecture.md), not here.
