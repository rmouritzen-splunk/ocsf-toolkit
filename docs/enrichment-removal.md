# Enrichment Removal

Enrichment removal removes redundant enum siblings and observables while preserving content that cannot be proven redundant. It can reduce event size when downstream systems can reconstruct enrichment from the same schema.

Enum-sibling removal follows the shared recursive-object boundary described in [recursive object definitions in the Event Processing Model](event-processing.md#recursive-object-definitions). If an event reaches that boundary, descendants are not inspected or modified and the report contains the single top-level `issue_event_traversal_limited` processing issue for the event. Observable resolution applies the same boundary and retains an entry when proving it redundant would require deeper traversal.

This guide describes the language-neutral behavior. See [Event Processing Model](event-processing.md) for class resolution, profiles, null handling, encodings, in-memory representations, and operation ordering.

## Safe And Forced Removal

Safe removal is the default. It removes content only when the schema and event prove that the content duplicates another event value. Content that is malformed, non-standard, or different from the schema-derived value is retained and reported where appropriate.

Forced removal is explicitly destructive:

- forced enum sibling removal skips caption equality checks, but still retains integral enum ID 99 siblings because the OCSF convention requires them;
- forced observable removal deletes the top-level `observables` attribute without inspecting its entries or using the resolved class schema, but like every mutation it runs only after `class_uid` resolves successfully.

## Enum Siblings

Removal applies to direct `integer_t` and `long_t` enums whose declared sibling has direct type `string_t` and the same scalar or array shape. Named subtypes do not qualify. Invalid declarations are retained on the ordinary processing path; when the declaration belongs to an enum, the schema loader reports the precise source or target problem as a nonfatal initialization issue.

For each active supported enum attribute:

1. If the sibling has no logical value, ensure it is absent without counting a value as removed.
2. If the enum value or sibling has the wrong type, retain the sibling and report that requested removal could not be performed.
3. If an integral enum value is 99, always retain the sibling without a processing issue because OCSF requires a source-specific sibling for `Other`; string enum key `"99"` follows ordinary caption rules.
4. In safe mode, remove the sibling only when its string value exactly equals the schema caption for the enum value; remove an array sibling only when the arrays have equal lengths and every value matches the caption at the same index.
5. In forced mode, remove the sibling without comparing it to the caption.

The safe equality rule preserves non-standard source text attached to ordinary enum values. When an enum value is unknown or the sibling differs from the schema caption, safe removal retains the sibling and reports why it could not prove the content redundant. Validation may independently report the invalid final enum or sibling when validation is enabled.

Safe removal performs caption lookup and comparison work that force removal skips, particularly for arrays. See [Safe enum-sibling removal in the FAQ](FAQ.md#safe-enum-sibling-removal) for performance considerations.

## Observable Names

An observable `name` points to the value or object elsewhere in the event. Array traversal is found in several equivalent source conventions:

- `foo.bar.baz`: when `bar` is an array, search all elements;
- `foo.bar[].baz` or `foo.bar[*].baz`: explicitly search all elements;
- `foo.bar[0].baz`: search only the zero-based selected element.

Arrays can be nested inside other arrays of objects, and different array conventions can occur in the same path or event. A leading `$.` may identify the event root. Implementations should first check that a path is valid for the active compiled class, then resolve it against the actual event.

## Observable Matching

A scalar observable has a `value`, which is always a string in a valid OCSF event. Resolve every candidate value selected by `name` and compare it using the same stable scalar-to-string conversion used by enrichment. Remove the observable when at least one selected event value produces exactly the observable string.

An object observable has no logical `value`. An omitted or nil-valued map entry therefore has the same object-observable semantics. It is removable when `name` resolves to an object. The object's individual contents do not need to be compared.

Retain and report entries whose names are missing, malformed, undefined by the active schema, unresolved, or inconsistent with the event value. Structural validation may report the same underlying malformed shape when validation is also enabled.

Safe removal performs schema and event path resolution for every candidate, which can examine many values through array paths. See [Safe observable removal in the FAQ](FAQ.md#safe-observable-removal) for performance considerations.

## Filtering The Array

Enum-sibling work always completes before observable analysis, with no exception (see [ordering multiple operations in the Event Processing Model](event-processing.md#ordering-multiple-operations)); analyze observable entries against the event state that results, so an entry derived from an enum sibling that enum-sibling work has already removed cannot be verified and is retained. Mark redundant entries by their original array indexes. After analysis:

- delete the `observables` attribute when every entry is removable;
- otherwise, construct the retained array in original order and omit marked entries;
- ensure an `observables` attribute with no logical value or an empty array is absent without counting a logical entry as removed;
- leave a malformed non-array value intact in safe mode and report it.

This mark-then-filter approach avoids index changes during removal analysis. A later validation stage independently analyzes the filtered final array and reports paths using its final indexes.

## Language-Neutral Algorithm

```text
resolve the event class
if the class cannot be resolved, report the class problem and stop
determine active profiles

walk active class and nested object attributes:
    for each supported scalar or array enum sibling pair:
        delete a nil-valued sibling without counting a removed value
        in safe mode, remove the sibling only when the enum and sibling values prove it redundant
        in forced mode, remove the sibling without a caption comparison
        in either mode, retain the complete sibling when an integral enum value is 99

if forced observable removal is enabled:
    delete observables without analyzing its value
else:
    if observables has no logical value or is an empty array:
        delete it
    else if observables is a valid array:
        analyze each original entry against the post-enum-sibling event
        mark entries proven redundant
        filter marked entries after all analysis is complete
    else:
        retain it and report the malformed value

if an issue is configured as an error at any step, stop at that issue, return no processing result, and do not roll back earlier mutations
otherwise, return removed counts, retained counts, and warning-level issues
```

For immutable records, Parquet rows, or similar representations, “remove” can mean constructing an output projection without the redundant field or array entries. In-place mutation is not required for equivalent behavior.

## Reference Implementation

`internal/processing/enrichment_removal.go` is a fully functioning, tested example of this algorithm. `issue/code.go` lists every enrichment-removal issue code it can produce. Read it directly for anything this guide leaves out. The public API and internal design built on top of this algorithm are described in [Architecture](architecture.md), not here.
