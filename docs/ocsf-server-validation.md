# OCSF Server Validation Parity

Add matching tests to OCSF Server's `validator2.ex` implementation for the Go validation behavior listed here. Use the same event fixtures where practical and compare normalized issue codes and structured details rather than exact human-readable messages when language-specific formatting differs.

The local OCSF Server work has used the `fix-validator2` branch. The `long_t` range correction on that branch should remain aligned with OCSF's signed 8-byte integer definition.

## Event Processing

- Validation-only processing does not enrich or mutate events.
- Missing recommended attributes warn only when the warning option is enabled.
- Validation reports `type_uid` mismatches when `class_uid`, `activity_id`, and `type_uid` are integral.
- An event `metadata.version` that cannot be parsed reports the `version_invalid_format` error.

OCSF Server commit `27532f3` introduced `version_invalid_format`, but the explicit parse-error branch was lost during the precompiled-schema refactor in commit `64d6ab3`. Restore that branch in `validator2.ex`; `Schema.Utils.parse_version/1` still returns `{:error, "malformed", value}` for this case.

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

## Constraints

- `at_least_one` treats a dotted path such as `ball.green` as present when the nested value exists.
- `just_one` passes with exactly one present value.
- `just_one` fails with zero present values.
- `just_one` fails with more than one present value.
- Constraint paths treat explicit null values as missing.

## Type Constraints

- Numeric range constraints pass at both inclusive bounds.
- Numeric range constraints fail below and above their bounds.
- String `max_len` counts Unicode code points consistently.
- A string regular-expression mismatch produces a warning.
- Type `values` rejects values outside the allowed set.

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
