// Package jsonio reads JSON objects into jsonish.Map values.
//
// The decoders in this package call json.Decoder.UseNumber so numeric values are preserved as
// json.Number for callers that need exact integer validation.
//
// The default encoding/json decoder accepts duplicate object member names, with later values
// replacing or merging into earlier values. Builds using GOEXPERIMENT=jsonv2 reject duplicate names by default.
// Applications requiring another policy should enforce it at their decoding boundary.
package jsonio
