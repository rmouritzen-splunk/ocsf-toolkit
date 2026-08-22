# Validation

Validation compares one logical OCSF event with a compiled schema and returns errors and warnings. It does not require JSON input or generic maps; it can operate on any encoding and in-memory representation that exposes the logical event operations described in [Event Processing Model](event-processing.md).

This guide describes the validation areas implemented by OCSF Toolkit. Detailed parity notes with OCSF Server are maintained in [OCSF Server Validation](ocsf-server-validation.md).

## Class And Profiles

Read `class_uid` as a signed integer and resolve the compiled class. Report a dedicated finding when it is missing, has the wrong type, or does not identify a class. These findings cannot be suppressed because every processor stops without a resolved class, but reporting policy may change their effective level. A separate mandatory processing issue records that processing stopped even when validation is disabled.

Validate `metadata.profiles`, resolve the active profile set, and apply profile filtering before validating attributes. A present non-null attribute defined by one or more inactive profiles is not unknown: report the profiles that can enable it and perform a best-effort shallow value check. Fully validate an ordinary primitive scalar, but check only the underlying primitive representation of an enum, the array shape without its elements, or the object shape and resolved object type without its contents. Report this outcome as `valid`, `shallowly_valid`, or `invalid` in `details.value_validation`, and state the scope of the check in the message. For an invalid value, include the captured validation result under `details.invalid_value`; do not also report those findings at the top level. The shallow check uses default schema validity rather than the event pipeline's finding suppression or level overrides. Enabling the profile makes the attribute part of the ordinary walk, which then performs complete validation and may reveal additional findings.

## Structure And Requirements

For every active schema attribute:

- treat missing and null as absent;
- report missing required attributes as errors;
- optionally report missing recommended attributes as warnings;
- check whether scalar and array shapes match the schema;
- recursively validate object values against their compiled object types;
- report present non-null attributes that are not defined by the compiled class or object;
- distinguish attributes defined by inactive profiles from attributes absent from the schema.

A direct use of the generic `object` object type is open-ended and allows arbitrary nested attributes. Other objects are closed, including derived objects with no attributes, and remain subject to their compiled profile-expanded definitions.

Validate class and object constraints after their attributes. The supported constraints include `at_least_one` and `just_one`; null values do not satisfy them.

## Primitive Types And Constraints

Validate values according to the resolved OCSF primitive type:

- `integer_t` and `long_t` use signed 64-bit integer semantics and accept floating-point representations only when their values are finite, in range, and mathematically integral;
- `float_t` accepts numeric representations convertible to the implementation's floating-point type, including integral values and, when the representation supports them, `NaN`, positive infinity, and negative infinity;
- `boolean_t` accepts booleans;
- `string_t` accepts strings;
- object attributes require objects of the compiled `object_type`.

Apply constraints declared by the type or its nearest applicable supertype:

- allowed values use normalized primitive equality rather than encoding-specific numeric spelling;
- numeric ranges are inclusive;
- maximum string length counts Unicode code points, not encoded bytes;
- regular-expression mismatches are warnings in this implementation;
- malformed schema regular expressions are reported as schema problems rather than event mismatches.

Report deprecated classes, attributes, objects, enum values, and types as warnings when the corresponding deprecated definition applies. For an event attribute whose schema attribute is deprecated, report only the attribute deprecation; otherwise, also report a deprecation on the schema attribute's declared dictionary type.

## Enums And Siblings

Validate a supported scalar or array enum and its same-shaped string sibling as one logical unit instead of validating either as an ordinary attribute. The source must have direct type `integer_t` or `long_t`; the non-enum target must have direct type `string_t` and the same `is_array` value. The enum is authoritative, so a sibling may exist only when its corresponding enum attribute exists. The pair is processed exactly once from the enum's ordered schema entry and reads both values from the post-enrichment event state. Invalid declarations are not paired, their attributes retain ordinary validation, and the enum declaration produces a specific schema initialization issue.

Report an unknown-enum finding only when the event value has a usable representation of the enum's resolved primitive type but no corresponding schema definition. When the value cannot be interpreted as that primitive type, skip the redundant unknown-enum finding and let primitive validation report the type error.

- a sibling present without its enum is an error;
- when the enum exists, an absent sibling is checked against the sibling's schema requirement and can be reported as an error or warning;
- by OCSF convention, integral enum ID 99 requires its sibling because the sibling carries the specific `Other` value; the rule applies independently at each position of an integral enum array, while string enum key `"99"` has no special meaning;
- a sibling whose value differs from the schema caption is retained as meaningful source content and can produce a diagnostic according to the enum case;
- enum arrays and their string-array siblings must have equal lengths and are validated as index-matched pairs; a length mismatch is one validation error, and paired values still receive their applicable element checks;

When enrichment runs before validation, a schema-derived sibling may satisfy structural requirements. Enrichment reports notable ID 99 additions so the resulting validation can be interpreted in context.

## Event Metadata And Type UID

Validate `metadata.version` as a supported semantic-version form and compare it with the compiled schema version. Exact versions are compatible. Earlier stable versions may produce a warning; incompatible initial-development, prerelease, malformed, or later versions produce errors according to the implemented compatibility rules.

When `class_uid`, `activity_id`, and `type_uid` are valid signed integers, check:

```text
type_uid = class_uid * 100 + activity_id
```

Detect arithmetic overflow rather than wrapping the expected value.

## Observables

The `observables` attribute must have the schema-defined array and element structure. Each entry must have a valid string `name`, a valid observable type, and either:

- a string `value` that matches a scalar at the named event path after stable scalar-to-string conversion; or
- no `value`, in which case the name must resolve to an object.

Validate observable names against both the active schema and the actual event. Support bare array traversal, `[]`, `[*]`, numeric indexes, nested arrays, and an optional root marker as described in [Enrichment Removal](enrichment-removal.md#observable-names).

Validation may be configured with a preferred observable path notation. A valid name using another supported notation remains resolvable and does not become a validation error, but produces a warning identifying the preferred style. For paths without arrays, the simple, bracket, wildcard, and indexed styles have the same spelling; root-relative JSONPath remains distinct because of its `$` prefix.

An explicit null observable value matches null or a missing branch at a schema-valid path because OCSF treats those states as equivalent. A wrong structural type encountered while following the path remains an error.

When safe enrichment removal precedes validation, validation independently analyzes the retained observables in the final event. Entries proven redundant and removed are no longer present, and paths for retained entries use their final array indexes.

When observable enrichment and validation run in the same operation, generated entries appended after the existing observable prefix may be trusted as semantically valid by construction. Validation must still analyze the existing prefix and enforce the requested preferred notation for every final name. Implementations using this optimization should independently test enrichment output by validating it in a separate operation.

Reaching the shared recursive-object traversal boundary means validation is incomplete but does not establish that the event is invalid. Report this as the top-level `issue_event_traversal_limited` processing issue described in [Event Processing](event-processing.md#recursive-objects), not as a validation warning or error.

## Finding Codes, Levels, And Processing Failures

Each validation condition has a stable code, description, and default level. The code is independent of its reporting level: errors ordinarily mean the event does not conform to the applicable OCSF schema or cross-field rules, while warnings ordinarily identify concerns such as deprecation, missing recommended content, earlier schema versions, or regular-expression mismatches. A finding records its effective level explicitly. Both levels are successful validation results.

A processing environment may suppress selected suppressible validation codes independently of level or set any validation code's effective level to warning or error. Findings for a missing, wrong-type, or unknown `class_uid` cannot be suppressed because they coincide with mandatory termination of event processing; explicitly selecting one for suppression is invalid, while a suppress-all policy retains it. An explicit level remains meaningful even when it equals the implementation's current default, because it records environment policy independently of a future default change. Conflicting actions for one code are invalid. Suppression counts are grouped by the suppressed code's default level so remapping policy does not make the accounting ambiguous.

Each validation finding identifies the exact affected event location through `details.attribute_path` when an attribute path applies. Messages and details do not repeat event values. This keeps the result independent of mutable event maps and slices, avoids retaining event subtrees, gives all validation levels one consistent result shape, and reduces the risk of disclosing sensitive event content when diagnostics are logged or embedded in application-error events. Investigating a problem may require correlating the diagnostic with the original event.

A processing error is different: the validator could not perform its work because the schema, input representation, or implementation was unusable. APIs and command-line tools should keep that failure channel separate from ordinary validation findings.

## Language-Neutral Algorithm

```text
resolve class_uid
if it is missing, malformed, or unknown:
    report the class error and stop schema-guided validation

validate metadata.profiles and determine active attributes
walk the active class and nested objects:
    validate requirements and value shapes
    validate primitive values, enum values, and siblings
    validate applicable type constraints and deprecations
    validate completed-object and completed-class constraints
    report non-null attributes outside the active schema

validate event version and type_uid relationships
validate retained observable entries against schema paths and event values
return ordered findings with effective levels and suppression counts
```

Implementations may combine these checks into one traversal, use separate passes, validate columns in batches, or generate specialized code for concrete event classes. Compatibility depends on the observable rules and results, not on reproducing this project's internal architecture.

## Reference Implementation

`internal/processing/validation.go` is a fully functioning, tested example of this algorithm. `validation/code.go` lists every error and warning code it can produce, including the exact stable name behind each one. Read both directly for anything this guide leaves out. The public API and internal design built on top of this algorithm are described in [architecture.md](architecture.md), not here.
