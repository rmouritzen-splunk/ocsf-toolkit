# OCSF Server Validation Parity

Add matching tests to OCSF Server's `validator2.ex` implementation for the Go validation behavior listed here. Use the same event fixtures where practical and compare normalized issue codes and structured details rather than exact human-readable messages when language-specific formatting differs.

The local OCSF Server work has used the `fix-validator2` branch. The `long_t` range correction on that branch should remain aligned with OCSF's signed 8-byte integer definition.

## Robustness And Untrusted Input

`validator2.ex` must not create atoms from event-derived strings. Event attribute names, observable path segments, and enum values are untrusted and can contain an unbounded number of distinct values. Calling `String.to_atom/1` for them can exhaust the BEAM atom table and terminate the VM.

- Compare event attribute names with schema atom names without converting the event names to atoms.
- Resolve observable path segments without converting them to new atoms.
- Look up enum values without creating atoms. If the compiled enum map retains atom keys, use a bounded existing-atom lookup that treats an unknown value as an unknown enum rather than raising.

After structural type validation reports an issue, later validation stages must not assume that the malformed value has the expected type. In particular:

- Run enum-array validation only when the event value is an array.
- Convert scalar enum values only when their primitive type supports the conversion.
- Index an enum sibling array only when the sibling value is an array.

Malformed values should produce validation issues and must not raise from `Enum.reduce/3`, `Enum.at/2`, `to_string/1`, or atom conversion.

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

`validator2.ex` currently treats every empty filtered attribute set as open-ended. It should instead carry the containing attribute's `object_type` into nested validation and allow unknown keys only for a direct `object_type: "object"` reference. The generic `object` convention must not be inferred from an empty filtered or unfiltered attribute collection.

## Enums

- A scalar enum sibling mismatch produces the sibling-incorrect warning.
- Enum value `99` with a sibling matching `Other` produces the suspicious-Other warning.
- Enum arrays report missing sibling elements.
- Enum arrays report incorrect sibling elements.
- Unknown enum array values produce the array-specific unknown-enum issue.

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

OCSF `max_len` is measured in Unicode code points. Elixir's `String.length/1` counts grapheme clusters, so `validator2.ex` should instead count `String.codepoints/1` and include a combining-character parity test. Go's `utf8.RuneCountInString` already provides the required code-point count.

## Numeric Bounds

- `integer_t` accepts signed `int64` minimum and maximum values.
- `integer_t` rejects values below signed `int64` minimum and above signed `int64` maximum.
- `long_t` accepts signed `int64` minimum and maximum values.
- `long_t` rejects values below signed `int64` minimum and above signed `int64` maximum.
- Integral fields reject numeric values with decimal or exponent representations when the represented value is not integral.

## Deprecations

- Deprecated classes produce warnings.
- Deprecated objects produce warnings when visited.
- Deprecated attributes produce warnings when present.
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
