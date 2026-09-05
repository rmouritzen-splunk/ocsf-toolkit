// Package eventresult defines processor-specific results and diagnostics exposed by eventpipeline.ProcessingResult.
// Its exported structs support keyed literals for tests and other caller-owned values but intentionally reject
// positional literals so fields can be added compatibly in future releases.
package eventresult
