# Historical OCSF Server Validation Parity Notes

This document records validation differences and possible OCSF Server changes identified while OCSF Toolkit still pursued parity with `validator2.ex`. It is retained as historical technical reference, not as an active back-port plan. OCSF Toolkit validation has progressed beyond the server implementation, and this project does not plan to port these behaviors back to OCSF Server.

The earlier local OCSF Server work used the `fix-validator2` branch. The observations below describe that implementation at the time of review and may no longer reflect its current state.

## Robustness And Untrusted Input

`validator2.ex` must not create atoms from event-derived strings. Event attribute names, observable path segments, and enum values are untrusted and can contain an unbounded number of distinct values. Calling `String.to_atom/1` for them can exhaust the BEAM atom table and terminate the VM.

- Compare event attribute names with schema atom names without converting the event names to atoms.
- Resolve observable path segments without converting them to new atoms.
- Look up enum values without creating atoms. If the compiled enum map retains atom keys, use a bounded existing-atom lookup that treats an unknown value as an unknown enum rather than raising.

After structural type validation reports an issue, later validation stages must not assume that the malformed value has the expected type. In particular:

- Run enum-array validation only when the event value is an array.
- Convert scalar enum values only when their primitive type supports the conversion.
- Index an enum sibling array only when the sibling value is an array.
- Validate enum and sibling arrays as equal-length, index-matched pairs. Report one length-mismatch finding for either a shorter or longer sibling array, then validate every available pair.
- Apply the source-specific `Other` rule to integral enum ID 99 at scalar and array positions. A string enum key `"99"` has no equivalent special meaning.

Malformed values should produce validation issues and must not raise from `Enum.reduce/3`, `Enum.at/2`, `to_string/1`, or atom conversion.

OCSF Toolkit validation errors and warnings identify the affected location with `details.attribute_path` and omit event values from both messages and details. OCSF Server parity does not require retaining its original echoed value field or repeating values in messages; exact paths provide the machine-readable location without borrowing or duplicating event content.

## Event Processing

- Validation-only processing does not enrich or mutate events.
- Missing recommended attributes warn only when the warning option is enabled.
- Validation reports `type_uid` mismatches when `class_uid`, `activity_id`, and `type_uid` are integral.
- An event `metadata.version` that cannot be parsed reports the `version_invalid_format` error.

OCSF Server commit `27532f3` introduced `version_invalid_format`, but the explicit parse-error branch was lost during the precompiled-schema refactor in commit `64d6ab3`. Restore that branch in `validator2.ex`; `Schema.Utils.parse_version/1` still returns `{:error, "malformed", value}` for this case.

Throughout validation, treat an explicit JSON `null` as equivalent to a missing OCSF attribute. This includes required and recommended attributes, unknown attributes, profile-filtered attributes, enum siblings, constraints, and observable resolution. A null element inside an array remains malformed because OCSF does not define an array element type that accepts null.

## Objects And Profiles

- A direct `object_type: "object"` reference is open-ended and accepts arbitrary nested keys.
- Other object types and event classes remain closed when profile filtering leaves no active attributes.
- Flattened inherited profile attributes are accepted on a derived object only when their profile is active.
- Null inactive-profile attributes are treated as missing.
- A present inactive-profile attribute is defined by the schema rather than unknown. Report every profile that can enable it and whether its value is valid, shallowly valid, or invalid. Fully validate ordinary primitive scalars; check only the underlying primitive representation of enums, array shape, or object shape and resolved object type. Retain detected errors as an `invalid_value` validation result nested in the profile finding rather than independent findings.

`validator2.ex` currently treats every empty filtered attribute set as open-ended. It should instead carry the containing attribute's `object_type` into nested validation and allow unknown keys only for a direct `object_type: "object"` reference. The generic `object` convention must not be inferred from an empty filtered or unfiltered attribute collection.

## Enums

- A scalar enum sibling mismatch produces the sibling-incorrect warning.
- Enum value `99` with a sibling matching `Other` produces the suspicious-Other warning.
- Enum arrays report missing sibling elements.
- Enum arrays report incorrect sibling elements.
- Unknown enum array values produce the array-specific unknown-enum issue.
- An enum value with an unusable representation of the enum's resolved primitive type produces the ordinary type finding without a redundant unknown-enum finding.

Enum validation currently assumes structurally valid event values and performs unbounded event-string-to-atom conversion. Apply the robustness changes above before looking up enum definitions or sibling array elements. Structural validation can report the original type issue; enum-specific validation should then skip values it cannot safely interpret.

## Constraints

- `at_least_one` treats a dotted path such as `ball.green` as present when the nested value exists.
- `just_one` passes with exactly one present value.
- `just_one` fails with zero present values.
- `just_one` fails with more than one present value.
- Constraint paths treat explicit null values as missing.

`validator2.ex` currently uses `Map.has_key?/2`, which treats null as present and interprets a dotted constraint path as one literal map key. Resolve dotted paths through nested objects while retaining support for a literal key when the compiled schema actually defines one. Also retain the result of `Map.put(extra, :value_count, count)` so a failed `just_one` issue includes the calculated count.

## Type Constraints

- Numeric range constraints pass at both inclusive bounds.
- Numeric range constraints fail below and above their bounds.
- String `max_len` counts Unicode code points consistently.
- A string regular-expression mismatch produces a warning.
- Type `values` rejects values outside the allowed set.

The inherited-regex branch in `validator2.ex` checks that the supertype has a regex but then reads the regex from the derived type. Compile and report the supertype's regex in that branch.

For parity with OCSF Toolkit's documented interpretation, `max_len` is measured in Unicode code points. Elixir's `String.length/1` counts grapheme clusters, so `validator2.ex` would instead need to count `String.codepoints/1` and include a combining-character parity test. Go's `utf8.RuneCountInString` provides the toolkit's code-point count. The OCSF metaschema specifies a maximum length but does not define whether its unit is bytes, code points, or grapheme clusters.

## Numeric Bounds

- `integer_t` accepts signed `int64` minimum and maximum values.
- `integer_t` rejects values below signed `int64` minimum and above signed `int64` maximum.
- `long_t` accepts signed `int64` minimum and maximum values.
- `long_t` rejects values below signed `int64` minimum and above signed `int64` maximum.
- Integral fields accept native floating-point values and decimal or exponent representations when their values are finite, in range, and mathematically integral.
- Integral fields reject numeric values with decimal or exponent representations when the represented value is not integral.
- `float_t` fields accept integral numeric representations that can be converted to the implementation's floating-point type.

`validator2.ex` currently relies on Elixir's disjoint `is_integer/1` and `is_float/1` runtime predicates. Normalize compatible numeric representations before applying the OCSF primitive type and constraint checks so validation does not depend on whether an encoding or decoder represented an integral value as an integer or a float.

## Deprecations

- Deprecated classes produce warnings.
- Deprecated objects produce warnings when visited.
- Deprecated attributes produce warnings when present.
- A present non-deprecated attribute whose declared dictionary type is deprecated produces a type-deprecation warning. A deprecated attribute produces only its attribute-deprecation warning rather than a redundant warning for its deprecated type.
- Deprecated enum values produce warnings when used.

## Observables

- Observable `name` references accept valid direct paths.
- Observable `name` references accept valid array paths using `[]`.
- Observable `name` references accept valid array paths using indexes such as `[0]`.
- Observable `name` references reject paths that do not exist in the active schema.

`validator2.ex` currently checks only whether an observable name refers to an active schema attribute. Extend it to resolve the path against the event and validate the observable itself:

- Support bare array names, `[]`, `[*]`, and zero-based indexes, including arrays nested inside arrays of objects.
- For a scalar observable with a `value`, require at least one resolved event value to have the same best-effort string representation.
- For an object observable without a `value`, require the resolved event value to be an object.
- Treat a missing path and an explicit null value as equivalent when the observable value is null.
- Report malformed path syntax, an inactive or undefined schema reference, an unresolved event path, a wrong observable value type, and a value mismatch without raising.
