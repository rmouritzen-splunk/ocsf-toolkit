// Package jsonio reads JSON objects into jsonish.Map values.
//
// The decoders in this package call json.Decoder.UseNumber so numeric values are preserved as
// json.Number for callers that need exact integer validation.
package jsonio
