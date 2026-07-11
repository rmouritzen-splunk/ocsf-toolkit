# Enrichment

Enrichment adds redundant, schema-derived information that makes OCSF events easier to read, search, report on, and use in cybersecurity detections. OCSF Toolkit can add enum siblings and observables independently.

This guide describes the language-neutral behavior. See [Event Processing Model](event-processing.md) for class resolution, profiles, null handling, encodings, in-memory representations, and schema-guided walking.

## Enum Siblings

An enum sibling is a string attribute associated with an integral enum ID attribute. For example:

```json
{
  "activity_id": 1,
  "activity_name": "Create"
}
```

For each active, scalar enum attribute with a schema-defined sibling:

1. Leave an existing non-null sibling unchanged.
2. Convert the enum ID to its canonical decimal string and find that ID in the attribute's enum definition.
3. If the enum entry has a usable caption, add the caption as the sibling value.
4. Otherwise, leave the sibling absent and report an enrichment issue.

This project enriches the standard scalar integral enum-ID and string-sibling form. It does not enrich legacy enum arrays or other non-standard enum forms.

Enum ID 99 means `Other`, and OCSF requires its string sibling. When the sibling is missing, enrichment adds the schema caption, normally `Other`, and reports that addition because the generic caption cannot recover a source-specific value that may have been lost upstream. Enrichment never replaces an existing source-specific sibling for ID 99.

## Observables

The top-level `observables` array is an index into values that already exist elsewhere in the event. It is not a general key-value store. Each observable names an event path and identifies the observable type. A scalar observable also contains a string `value`; an object observable omits `value` and names the object itself.

Observable definitions can come from:

- class-level observable path definitions;
- observable object types;
- attributes or their dictionary definitions with an observable type.

As the active event structure is walked, collect generated observables rather than repeatedly modifying the top-level array. For array values, generate an observable for each qualifying element while using the appropriate observable name form. Add the collected entries after the event walk is complete.

Scalar values are represented in the observable `value` string using a stable, JSON-compatible scalar conversion. Strings remain unchanged; booleans, signed integers, and floating-point values use their conventional textual forms. Do not generate scalar observables from objects, arrays, or values that cannot be represented safely.

## Existing Observables And Duplicates

If `observables` is absent, null, or an empty array, enrichment may start a new array. If it is a valid non-empty array, append generated entries while preserving existing entries.

Do not append an exact duplicate of an existing or already-generated observable. Duplicate identity includes the observable name, type, and the presence and value of `value`. A valueless object observable is therefore different from a scalar observable whose value is null.

Observable names are compared as supplied. Do not treat path spelling variations such as a bare array name and an explicit `[]` form as duplicates merely because they can resolve to the same event value. Normalizing those forms could discard intentionally distinct source entries.

Report each skipped duplicate as a non-fatal enrichment issue. If an existing `observables` value is malformed rather than an array, report the issue instead of replacing non-empty source content.

After enrichment, remove the `observables` attribute if it would otherwise contain an empty array. This avoids retaining an attribute that carries no information.

## Language-Neutral Algorithm

```text
resolve the event class and active profiles
if the class cannot be resolved, report the class problem and stop

prepare a set of existing observable identities
walk active class and nested object attributes:
    for a supported scalar enum:
        add its missing schema-caption sibling when possible
    for a schema-defined observable source:
        derive one or more observable entries
        retain entries whose identities have not already been seen

append retained generated observables to valid existing observables
remove observables if the final array is empty
return mutation counts and non-fatal issues
```

An implementation backed by structs, classes, or columnar data can perform the same steps through field metadata or nested-column access. It does not need to materialize the entire event as generic maps, provided its chosen destination representation can express added and removed fields and array entries.
