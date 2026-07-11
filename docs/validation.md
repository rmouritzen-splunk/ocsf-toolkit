# Validation

Validation compares one logical OCSF event with a compiled schema and returns errors and warnings. It does not require JSON input or generic maps; it can operate on any encoding and in-memory representation that exposes the logical event operations described in [Event Processing Model](event-processing.md).

This guide describes the validation areas implemented by OCSF Toolkit. Detailed parity notes with OCSF Server are maintained in [OCSF Server Validation](ocsf-server-validation.md).

## Class And Profiles

Read `class_uid` as a signed integer and resolve the compiled class. Report an error when it is missing, has the wrong type, or does not identify a class. Without a class definition, schema-guided validation stops.

Validate `metadata.profiles`, resolve the active profile set, and apply profile filtering before validating attributes. An attribute defined only by an inactive profile is unknown for that event.

## Structure And Requirements

For every active schema attribute:

- treat missing and null as absent;
- report missing required attributes as errors;
- optionally report missing recommended attributes as warnings;
- check whether scalar and array shapes match the schema;
- recursively validate object values against their compiled object types;
- report present non-null attributes that are not defined by the active class or object.

A direct use of the generic `object` object type is open-ended and allows arbitrary nested attributes. Other objects are closed, including derived objects with no attributes, and remain subject to their compiled profile-expanded definitions.

Validate class and object constraints after their attributes. The supported constraints include `at_least_one` and `just_one`; null values do not satisfy them.

## Primitive Types And Constraints

Validate values according to the resolved OCSF primitive type:

- `integer_t` and `long_t` use signed 64-bit integer semantics;
- `float_t` accepts finite floating-point values according to the input representation;
- `boolean_t` accepts booleans;
- `string_t` accepts strings;
- object attributes require objects of the compiled `object_type`.

Apply constraints declared by the type or its nearest applicable supertype:

- allowed values use normalized primitive equality rather than encoding-specific numeric spelling;
- numeric ranges are inclusive;
- maximum string length counts Unicode code points, not encoded bytes;
- regular-expression mismatches are warnings in this implementation;
- malformed schema regular expressions are reported as schema problems rather than event mismatches.

Report deprecated classes, attributes, objects, enum values, and types as warnings when the corresponding deprecated definition applies.

## Enums And Siblings

Validate integral enum IDs against the attribute's enum definition. Validate the associated string sibling separately:

- an absent ordinary sibling can be reported as a warning;
- enum ID 99 requires its sibling because the sibling carries the specific `Other` value;
- a sibling whose value differs from the schema caption is retained as meaningful source content and can produce a diagnostic according to the enum case;
- legacy enum array forms are validated according to their defined shape but are not enriched or removed by the standard scalar sibling algorithms.

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

An explicit null observable value matches null or a missing branch at a schema-valid path because OCSF treats those states as equivalent. A wrong structural type encountered while following the path remains an error.

When safe enrichment removal precedes validation, validation should reuse or reproduce the pre-removal analysis but report only retained invalid entries. Entries proven redundant and removed should not later be diagnosed as invalid.

## Errors, Warnings, And Processing Failures

Validation errors mean the event does not conform to the applicable OCSF schema or cross-field rules. Warnings identify concerns such as deprecation, missing recommended content, earlier schema versions, or regular-expression mismatches. Both are successful validation results.

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
return ordered errors and warnings
```

Implementations may combine these checks into one traversal, use separate passes, validate columns in batches, or generate specialized code for concrete event classes. Compatibility depends on the observable rules and results, not on reproducing this project's internal architecture.

