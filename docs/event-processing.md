# Event Processing Model

This guide describes the common model used by OCSF Toolkit enrichment, enrichment removal, and validation. It is intended both for toolkit users and for developers implementing equivalent behavior in another language or data-processing environment.

The model is independent of the Go implementation. An implementation does not need a generic tree walker, processor pipeline, or visitor callbacks. It does need equivalent access to an event, its compiled OCSF schema, and the relationships between classes, objects, attributes, profiles, and dictionary types.

The operation-specific guides are:

- [Enrichment](enrichment.md)
- [Enrichment Removal](enrichment-removal.md)
- [Validation](validation.md)

## Logical Events, Encodings, And Representations

An OCSF event is a logical tree of named attributes, scalar values, objects, and arrays. JSON is one encoding of that tree, but these algorithms do not depend on JSON text. The same approach can be applied to data decoded from Parquet, Avro, a database row, a message protocol, or another encoding, provided the logical OCSF structure and scalar values are preserved.

The in-memory representation is also an implementation choice. Examples include:

- generic maps, arrays, and scalar values, as used by this project;
- Go structs and slices;
- Python dictionaries or classes;
- Java maps, records, or classes;
- rows and nested columns in a columnar processing system.

An implementation needs operations equivalent to:

- determine whether a named attribute is present and non-null;
- read, add, replace, and remove a named attribute;
- enumerate the attributes of an object when checking for unknown attributes;
- distinguish objects, arrays, and scalar values;
- enumerate and index arrays;
- preserve the OCSF scalar distinctions needed by validation, especially signed integers, floating-point values, booleans, and strings.

With concrete structs or classes, schema names may need to be mapped to fields or properties. With a columnar encoding, event paths may resolve to nested columns rather than object lookups. These differences affect the adapter, not the processing rules.

## Compiled Schema

Processing uses a compiled schema produced by `ocsf-schema-compiler`. The compiler has already resolved schema includes, inheritance, patches, dictionary attributes, and profile-expanded attributes. Implementations should use the compiled class and object definitions as authoritative instead of reconstructing the uncompiled schema.

Schema compilation owns semantic schema validation, including reference integrity. Event processors should still fail safely when a compiled schema is malformed or incompatible with the compiled format they support.

## Resolving The Event Class

The top-level `class_uid` identifies the event class. Resolve it as a signed integral value and look up the corresponding compiled class definition before performing schema-guided processing.

If `class_uid` is missing, malformed, or unknown, ordinary schema-guided walking cannot continue because the event shape is not known. Validation should report the class problem. Enrichment and safe enrichment removal should not guess a class. Operations that do not require a class, such as forced removal of the top-level `observables` attribute, may still proceed.

## Profiles

Read active profiles from `metadata.profiles`. A schema attribute with no profile restriction is active. A profile-restricted attribute is active when at least one of its profiles is active for the event.

Profile filtering affects traversal, enrichment, removal, requirements, and unknown-attribute validation. An attribute that exists only in an inactive profile is not part of the event's active schema.

The generic `object` object type is an open-ended OCSF object when used directly. Its contents are not restricted by profile-defined attributes. A derived object with no attributes is still a closed object and remains affected by profile definitions inherited into its compiled definition.

## Missing And Null Values

OCSF does not give a functional distinction to a missing attribute and an attribute with a null value. Treat both as absent for requirements, constraints, ordinary traversal, and unknown-attribute checks.

This rule does not make null a generally valid array element. OCSF attribute types do not define null array elements. Validation may therefore report a null element as an invalid value even though a null object attribute is treated as missing.

Observable matching has an additional consequence: an observable whose explicit `value` is null can match either an explicit null or a missing branch at a schema-valid event path. See [Enrichment Removal](enrichment-removal.md#observable-matching) and [Validation](validation.md#observables).

## Schema-Guided Walking

After resolving the class and profiles, walk the active class attributes defined by the compiled schema:

1. Determine whether the attribute is present and non-null.
2. Apply operation-specific handling for a missing or present attribute.
3. If the attribute is an array, process each element according to the element definition.
4. If it is an object attribute, resolve its compiled `object_type` and recursively process the object.
5. Apply completed-object or completed-class checks, such as structural constraints.

Walking schema attributes rather than only the attributes present in an event is important: requirements and missing enum siblings cannot otherwise be detected. Validation additionally enumerates actual object attributes to detect names that are not present in the active schema.

Paths used in reports should identify array elements with zero-based indexes, for example `network_endpoint[1].ip`. Observable names have their own path syntax described in the operation guides.

## Ordering Multiple Operations

Operations that mutate an event run before validation so validation observes the final event. When enrichment removal is enabled, observable redundancy is analyzed before enum siblings are removed; observable references are therefore evaluated against the original enriched content. Removable observables are then filtered before ordinary validation walks the event.

Adding and removing the same enrichment category in one operation is ambiguous and should be rejected. Adding observables while removing enum siblings, or adding enum siblings while removing observables, can be valid when the implementation defines and preserves the ordering.

Processing is not inherently transactional. A mutating implementation may leave an event partially changed if a later operation fails. Systems that require rollback should process a copy or use an encoding-specific transactional mechanism.

## Results And Diagnostics

Keep validation errors and warnings distinct from processing failures. An invalid OCSF event was successfully processed and produced validation results; an unusable schema, unsupported input representation, or internal failure prevented processing.

Enrichment and enrichment removal should report non-fatal issues that explain why requested content was not added or safely removed. Counts should describe actual mutations and retained content, while directory or stream summaries may aggregate those results per event.

