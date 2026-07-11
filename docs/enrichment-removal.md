# Enrichment Removal

Enrichment removal removes redundant enum siblings and observables while preserving content that cannot be proven redundant. It can reduce event size when downstream systems can reconstruct enrichment from the same schema.

This guide describes the language-neutral behavior. See [Event Processing Model](event-processing.md) for class resolution, profiles, null handling, encodings, in-memory representations, and operation ordering.

## Safe And Forced Removal

Safe removal is the default. It removes content only when the schema and event prove that the content duplicates another event value. Content that is malformed, non-standard, or different from the schema-derived value is retained and reported where appropriate.

Forced removal is explicitly destructive:

- forced enum sibling removal skips caption equality checks, but still retains enum ID 99 siblings because OCSF requires them;
- forced observable removal deletes the top-level `observables` attribute without inspecting its entries and does not require a resolvable class.

## Enum Siblings

Removal applies only to the standard scalar integral enum-ID and string-sibling form. Array enum siblings and other legacy or non-standard forms are retained.

For each active supported enum attribute:

1. If the sibling is absent, do nothing.
2. If the sibling is null, remove it because null and missing are equivalent.
3. If the enum ID or sibling has the wrong type, retain the sibling.
4. If the enum ID is 99, always retain the sibling because OCSF requires a sibling for `Other`.
5. In safe mode, remove the sibling only when its string value exactly equals the schema caption for the enum ID.
6. In forced mode, remove the sibling without comparing it to the caption.

The safe equality rule preserves non-standard source text attached to ordinary enum IDs. It also avoids discarding a value when the schema does not provide a usable caption.

## Observable Names

An observable `name` points to the value or object elsewhere in the event. Array traversal is found in several equivalent source conventions:

- `foo.bar.baz`: when `bar` is an array, search all elements;
- `foo.bar[].baz` or `foo.bar[*].baz`: explicitly search all elements;
- `foo.bar[0].baz`: search only the zero-based selected element.

Arrays can be nested inside other arrays of objects, and different array conventions can occur in the same path or event. A leading `$.` may identify the event root. Implementations should first check that a path is valid for the active compiled class, then resolve it against the actual event.

## Observable Matching

A scalar observable has a `value`, which is always a string in a valid OCSF event. Resolve every candidate value selected by `name` and compare it using the same stable scalar-to-string conversion used by enrichment. Remove the observable when at least one selected event value produces exactly the observable string.

Because OCSF treats missing and null as equivalent, an explicit null observable value matches an explicit null or a missing branch at a schema-valid path. A branch that cannot be followed because an encountered value has the wrong structural type is not equivalent to missing.

An object observable omits `value`. It is removable when `name` resolves to an object. The object's individual contents do not need to be compared.

Retain and report entries whose names are missing, malformed, undefined by the active schema, unresolved, or inconsistent with the event value. Structural validation may report the same underlying malformed shape when validation is also enabled.

## Filtering The Array

Analyze all observable entries before removing enum siblings or otherwise changing referenced event content. Mark redundant entries by their original array indexes. After analysis:

- delete the `observables` attribute when every entry is removable;
- otherwise, construct the retained array in original order and omit marked entries;
- delete a null or empty `observables` attribute without further analysis;
- leave a malformed non-array value intact in safe mode and report it.

This mark-then-filter approach avoids index changes during analysis and supports shared observable analysis with a later validation stage.

## Language-Neutral Algorithm

```text
if forced observable removal is enabled:
    delete observables and continue with any requested enum removal

resolve the event class and active profiles
if observables is null or an empty array:
    delete it
else if observables is a valid array:
    analyze each original entry against the schema and event
    mark entries proven redundant
    filter marked entries after all analysis is complete
else:
    retain it and report the malformed value

walk active class and nested object attributes:
    safely or forcibly remove supported enum siblings

return removed counts, retained counts, and non-fatal issues
```

For immutable records, Parquet rows, or similar representations, “remove” can mean constructing an output projection without the redundant field or array entries. In-place mutation is not required for equivalent behavior.

