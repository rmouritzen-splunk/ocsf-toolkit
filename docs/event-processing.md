# Event Processing Model

This guide describes the common model used by OCSF Toolkit enrichment, enrichment removal, and validation. It is intended both for toolkit users and for developers implementing equivalent behavior in another language or data-processing environment.

The model is independent of the Go implementation. An implementation does not need a generic tree walker, processor pipeline, or visitor callbacks. It does need equivalent access to an event, its compiled OCSF schema, and the relationships between classes, objects, attributes, profiles, and dictionary types.

The operation-specific guides are:

- [Enrichment](enrichment.md)
- [Enrichment Removal](enrichment-removal.md)
- [Validation](validation.md)

## Logical Events, Encodings, And Representations

An OCSF event is a logical tree of named attributes, scalar values, objects, and arrays. JSON is one encoding of that tree, but these algorithms do not depend on JSON text. The same approach can be applied to data decoded from Parquet, Avro, a database row, a message protocol, or another encoding, provided the logical OCSF structure and scalar values are preserved.

Encoding-specific ambiguity must be resolved before processing. For example, an in-memory event object has at most one value for an attribute, while a JSON parser determines whether duplicate object member names are rejected or which value is retained. Such parsing policy belongs at the decoding boundary rather than in schema-guided event processing.

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

If `class_uid` is missing, malformed, or unknown, event processing stops because the event does not identify a valid OCSF class. Report a mandatory processing issue for this termination whether or not validation is enabled. Validation should additionally report the class problem when enabled unless policy ignores that validation code. No mutation is performed, including force removal and other operations that would not otherwise require the class schema. Requiring class resolution before any mutation provides a minimal sanity check that the input is shaped as an OCSF event rather than arbitrary structured data.

## Profiles

Read active profiles from `metadata.profiles`. A schema attribute with no profile restriction is active. A profile-restricted attribute is active when at least one of its profiles is active for the event.

Profile filtering affects traversal, enrichment, removal, requirements, and validation. An attribute defined only by inactive profiles is not part of the event's active schema, so mutation processors do not receive it. Validation distinguishes a present inactive attribute from an undefined attribute, identifies every profile that can enable it, and performs a best-effort check of its top-level value shape. Ordinary primitive scalars receive complete value validation; enums, arrays, and objects receive only shallow validation because enabling the profile will place them in the ordinary complete walk and may reveal additional findings.

The generic `object` object type is an open-ended OCSF object when used directly. Its contents are not restricted by profile-defined attributes. A derived object with no attributes is still a closed object and remains affected by profile definitions inherited into its compiled definition.

## Missing And Null Values

OCSF has no logical null value. Treat an attribute represented without a value as absent for requirements, constraints, ordinary traversal, unknown-attribute checks, enrichment, removal, and observable resolution. In the Go API, a missing `jsonish.Map` key and a key whose value is `nil` both represent this logical absence.

This rule applies to map attributes, not array positions. An array position represented by a null or nil element remains an element, and validation reports it as invalid because OCSF attribute types do not define null array elements.

Processors do not distinguish missing and nil-valued map attributes in diagnostics or result counters. An enabled removal processor may delete a nil-valued attribute within its requested scope without counting a logical value as removed; other processors do not normalize the representation merely because the attribute has no logical value.

## Schema-Guided Walking

After resolving the class and profiles, walk the active class attributes defined by the compiled schema:

1. Determine whether the attribute is present and non-null.
2. Apply operation-specific handling for a missing or present attribute.
3. If the attribute is an array, process each element according to the element definition.
4. If it is an object attribute, resolve its compiled `object_type` and recursively process the object.
5. Apply completed-object or completed-class checks, such as structural constraints.

Walking schema attributes rather than only the attributes present in an event is important: requirements and missing enum siblings cannot otherwise be detected. The walker classifies present attributes excluded by profile filtering separately for validation. Validation additionally enumerates actual object attributes to detect names that are not present in the compiled schema definition.

A supported scalar or array enum and its same-shaped string sibling form one logical processing unit when both attributes are active. The enum must have direct type `integer_t` or `long_t`; the target must exist in the same concrete item, must not itself have an enum, and must have direct type `string_t` with the same `is_array` value. Named subtypes and attribute naming do not affect this decision. Array values are parallel: their lengths must match and each enum element is paired only with the sibling element at the same index. Dispatch that pair instead of dispatching either definition as an ordinary attribute; an operation that does not need pair-specific behavior may process either or both definitions using its ordinary attribute logic internally. The enum's ordered schema entry owns the pair dispatch, and the sibling's ordered entry is skipped, so the pair is processed exactly once whether neither, either, or both values are present in the event. Profile activation is evaluated independently for each attribute. If only one member is active, process it as an ordinary attribute and do not enrich, remove, or validate a sibling relationship through the inactive member. A present inactive member receives the normal profile-required validation treatment. An invalid declaration produces a specific nonfatal schema initialization issue, is not linked as a pair, and leaves its attributes on the ordinary processing path. A `sibling` property on a non-enum attribute is ignored without an issue.

Paths used in reports identify array elements with zero-based indexes, for example `network_endpoint[1].ip`. This canonical diagnostic form is independent of the selected observable name notation. Observable names have their own path syntax described in the operation guides.

### Recursive object definitions

OCSF object definitions may be recursive. The shared attribute dictionary gives recursive relationships consistent attribute names, so processing uses repeated object-attribute names as the recursion boundary. When an object-valued attribute has the same name as an earlier attribute on the active traversal branch, encounter and report that repeated attribute normally but treat its object value as opaque: do not process any attributes inside the repeated object. Array positions do not participate in this comparison.

For example, processing may walk from a file into its parent file, but a second parent attribute is only reported as the boundary; the second parent object is not processed. Similarly, traversal proceeds through `person.ldap_person.manager` because each relationship name is distinct, but a subsequent `ldap_person` attribute is the boundary and the second LDAP person object is not processed. The same attribute name on a separate sibling branch remains eligible for normal traversal.

This depth agrees with OCSF's process-specific [guidance for representing process parentage](https://github.com/ocsf/ocsf-docs/blob/main/articles/representing-process-parentage.md), which recommends populating `process.parent_process` only on the top-level process object and using `process.ancestry` for ancestry beyond the immediate parent. The toolkit applies its attribute-name boundary generally because recursive relationships also exist outside process parentage.

The boundary also protects processing performance and robustness. Recursive object relationships combined with arrays or multiple recursive branches can create a geometrically growing nested structure. Treating the first repeated relationship as opaque bounds schema-guided recursion before processing follows that expansion indefinitely or performs disproportionate work below the boundary. This rule does not impose a general event-size or array-breadth limit.

Content below this recursion boundary is retained but is not enriched, unenriched, or validated by schema-guided processing. Reaching the boundary does not make the event invalid, but it means processing is incomplete. Report at most one top-level `issue_event_traversal_limited` processing issue per event, using the first affected indexed path. Do not report the limitation as a validation warning or error.

## Ordering Multiple Operations

Operations that mutate an event run before validation so validation observes the final event. Enum-sibling work (adding, safely removing, or force-removing) always completes before observable work, with no exception, regardless of which actions are combined; observable analysis and generation therefore see the event state after any enum-sibling change has already been applied. An observable derived from an enum sibling that enum-sibling work has already removed cannot be verified and is retained, not removed. Removable observables are then filtered, and validation independently analyzes the retained observables and the rest of the final event.

Adding and removing the same enrichment category in one operation is ambiguous and should be rejected.

Processing is not inherently transactional. A mutating implementation may leave an event partially changed if a later operation fails. Systems that require rollback should process a copy or use an encoding-specific transactional mechanism.

## Results And Diagnostics

Keep validation errors and warnings distinct from processing failures. An invalid OCSF event was successfully processed and produced validation results; an unusable schema, unsupported input representation, or internal failure prevented processing.

Enrichment and enrichment removal should report issues that explain why requested content was not added or safely removed. Counts should describe actual mutations and retained content, while directory or stream summaries may aggregate those results per event. A duplicate-observable issue is detection rather than an explanation that content was skipped: duplicate reporting and generated-observable deduplication are independent controls.

An implementation may let callers assign ignored, warning, or error levels to processing issue codes. Ignored handling should avoid constructing paths, messages, and details for omitted issues. Warning handling should collect the issue in the successful result. Error handling should stop at the first matching issue and return it through the processing-failure channel rather than returning a meaningful partial result. Level policy must not change the mutation that led to an issue, and an event may therefore be partially mutated when an error-level issue stops processing. When the same duplicate condition has both an issue and a validation representation, an implementation may designate the enabled issue as the sole diagnostic owner to avoid scanning and reporting twice. Issues that disclose incomplete processing, including class-resolution failure and the recursive-object traversal limitation, remain mandatory and cannot be ignored.

## Reference Implementation

`internal/processing` is a fully functioning, tested example of this model: `process.go` implements schema-guided walking and the recursive-object boundary, and `pipeline.go` and `config.go` implement operation ordering. `issue.Code` (`issue/code.go`) lists every processing issue this implementation can produce. Read the code directly for anything this guide leaves out. The public API and internal design built on top of this model are described in [Architecture](architecture.md), not here.
