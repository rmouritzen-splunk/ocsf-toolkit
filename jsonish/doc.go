// Package jsonish defines the shared in-memory representation for JSON objects.
//
// Event values are nested structures composed of map[string]any objects, slices, fixed arrays,
// and supported primitive values. Map is a domain-oriented alias for map[string]any; using the
// alias is helpful but not required. Supported primitives are nil, bool, string, json.Number,
// Go signed integer types, float32, and float64. OCSF integer_t and long_t values have signed 64-bit
// semantics. Unsigned integers, structs, other Go map types, and other non-JSON values are not
// supported. When source data is JSON, the jsonio decoding helpers are preferred because they
// preserve numbers as json.Number rather than converting them to float64. Callers that need
// struct-backed event processing should file a project issue describing the required use case.
package jsonish
