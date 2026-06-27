# OCSF Toolkit

OCSF Toolkit provides a Go library and a command line tool for processing [OCSF](https://schema.ocsf.io/) events with a compiled OCSF schema.

The current processors support:

- Enrichment: add enum siblings and observables.
- Validation: validate a single event against a compiled schema.

Enrichment runs before validation when both are enabled, so validation checks the event after local processing has been applied.

## CLI Usage

### Install

Download an archive from the repository's GitHub Releases page:
<https://github.com/ocsf/ocsf-toolkit/releases>.

Release archives are named by version, operating system, and architecture:

```text
ocsf-toolkit_v0.1.0_darwin_arm64.tar.gz
ocsf-toolkit_v0.1.0_darwin_amd64.tar.gz
ocsf-toolkit_v0.1.0_linux_arm64.tar.gz
ocsf-toolkit_v0.1.0_linux_amd64.tar.gz
ocsf-toolkit_v0.1.0_windows_arm64.zip
ocsf-toolkit_v0.1.0_windows_amd64.zip
```

For macOS, choose the `darwin` OS archive. Modern Apple Silicon machines such as M1, M2, M3,
and newer use `arm64`. Older Intel Macs use `amd64`.

Extract the archive and check the binary:

```sh
tar -xzf ocsf-toolkit_v0.1.0_darwin_arm64.tar.gz
cd ocsf-toolkit_v0.1.0_darwin_arm64
./ocsf-toolkit --version
```

macOS may block downloaded unsigned binaries with a warning that Apple could not verify the tool is
free of malware. OCSF Toolkit does not currently provide signed or notarized macOS binaries. OCSF is
an unfunded project, and signing/notarization requires an Apple Developer account and CI secrets. To
run a downloaded macOS binary, remove the quarantine attribute:

```sh
xattr -d com.apple.quarantine ./ocsf-toolkit
```

The CLI can also be built locally from a source checkout. See [Development](#development).

### Quick Start

The CLI needs three inputs: a compiled OCSF schema, one or more event JSON files, and at least one operation.

Validate a single event and write the validation result to stdout:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event event.json \
  --validate \
  --validation-output -
```

The `--schema` argument must point to a compiled OCSF schema file. See [Compiled Schema](#appendix-compiled-schema).

General form:

```sh
ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --validate) [options]
```

### CLI Examples

Enrich and validate a single event, writing both outputs to one directory:

```sh
ocsf-toolkit -s ocsf-schema-v1.8.0.json -e event.json -E -V -o out
```

This writes:

- `out/event.json`
- `out/event-validation.json`

Enrich a single event without changing the input file:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event event.json \
  --enrich \
  --enrich-output enriched-event.json
```

Enrich an event in place:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event event.json \
  --enrich \
  --enrich-in-place
```

Validate in CI and fail the command when validation errors are found:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --events-dir events \
  --validate \
  --validation-output-dir validation-results \
  --fail-on-validation-errors
```

Enrich and validate a directory tree:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --events-dir events \
  --enrich \
  --validate \
  --output-dir out
```

Directory outputs preserve input-relative paths. For example:

```text
events/windows/windows_service_activity.json
```

becomes:

```text
out/windows/windows_service_activity.json
out/windows/windows_service_activity-validation.json
```

Use separate output trees when needed:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --events-dir events \
  --enrich \
  --enrich-output-dir enriched-events \
  --validate \
  --validation-output-dir validation-results
```

Read a single event from stdin and write enriched JSON to stdout:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event - \
  --enrich \
  --enrich-output -
```

### Output Behavior

The CLI requires an explicit output destination for each selected operation. Enrichment uses `--enrich-output`, `--enrich-output-dir`, `--output-dir`, or `--enrich-in-place`. Validation uses `--validation-output`, `--validation-output-dir`, or `--output-dir`.

Output directories are created if necessary. Output files are not replaced unless `--overwrite` is supplied, except that `--enrich-in-place` replaces the input event file for enrichment without requiring `--overwrite`.

`--output-dir` writes enriched events and validation results to one output tree. Use `--enrich-output-dir` and `--validation-output-dir` for separate trees. These output directories may be the same directory because validation files use `<base>-validation.json` names.

Validation outputs have this shape:

```json
{
  "input_path": "event.json",
  "validation": {
    "errors": [
      {
        "phase": "validation",
        "severity": "error",
        "code": "attribute_required_missing",
        "message": "Required attribute \"time\" is missing.",
        "attribute_path": "time",
        "attribute": "time"
      }
    ],
    "warnings": []
  }
}
```

Enrichment output is the processed event JSON. For example, if the schema defines `activity_id` with the `activity_name` enum sibling, enrichment can add the sibling field:

```json
{
  "activity_id": 1,
  "activity_name": "Create"
}
```

`--validation-output -`, `--enrich-output -`, `--summary-output -`, and `--summary-json-output -` write to stdout. At most one selected output may write to stdout.

By default, a terse human-readable summary is written to stderr. Use `--quiet` to suppress it. `--summary-output` writes a human-readable summary with tool metadata, and `--summary-json-output` writes the same summary information as JSON.

Path preservation differs slightly between directory and single-event processing. In directory mode,
the toolkit walks files under `--events-dir` and computes each output path relative to that input
root. In single-event mode, `--event` is supplied directly by the user, so preserving an absolute
path or a relative path containing `..` could place output outside the selected output directory.
For that reason, single-event output directories preserve only safe relative paths; unsafe paths use
the input file's basename.

Use `--skip-invalid-output` with `--enrich --validate` to avoid writing enriched events when validation errors are found.

### Exit Codes

- `0`: the command completed successfully.
- `1`: processing failed, writing output failed, or validation errors were found with `--fail-on-validation-errors`.
- `2`: command-line parsing or configuration failed.

Validation errors do not change the exit code by default. Use `--fail-on-validation-errors` when validation errors should fail a CI job or script.

Run full help:

```sh
ocsf-toolkit --help
```

## Library Usage

Import the event schema and JSON helpers:

```go
import (
	"fmt"

	"github.com/ocsf/ocsf-toolkit/eventschema"
	"github.com/ocsf/ocsf-toolkit/jsonio"
)
```

Load a compiled schema, build a processor, and process an event:

```go
schema, err := eventschema.New("ocsf-schema-v1.8.0.json")
if err != nil {
	return err
}

processor := schema.NewEventProcessor(
	eventschema.NewEnrichment(),
	eventschema.NewValidation(),
)

event, err := jsonio.ReadObject("event.json")
if err != nil {
	return err
}

result, err := processor.ProcessEvent(event)
if err != nil {
	return err
}

if len(result.Validation.Errors) > 0 {
	fmt.Printf("event has %d validation error(s)\n", len(result.Validation.Errors))
}
```

`Schema` and `EventProcessor` values are safe for concurrent use after construction when each `ProcessEvent` call receives a distinct event map. The event map and its nested maps or slices must not be accessed or mutated concurrently while processing is running.

`ProcessEvent` mutates the event in place when enrichment is enabled. Processing is not transactional: if `ProcessEvent` returns an error, the event may already be partially modified. Callers that need to preserve the original event should deep-copy it before processing.

Validation failures are reported in `ProcessingResult`; they do not normally return a Go `error`. The `error` return is for tooling failures or unusable input.

For JSON-encoded events, preserving numbers as `json.Number` is safer than decoding into `float64`, especially for OCSF integer values. The `jsonio` helpers do this for file input by using `json.Decoder.UseNumber()`. Events built from other sources can use normal Go values such as signed integer types, `float32`, `float64`, `bool`, `string`, slices, and nested `jsonish.Map` values.

### Processors

Create an enrichment processor:

```go
processor := schema.NewEventProcessor(eventschema.NewEnrichment())
```

Create a validation processor:

```go
processor := schema.NewEventProcessor(eventschema.NewValidation())
```

Create a processor that enriches and then validates:

```go
processor := schema.NewEventProcessor(
	eventschema.NewEnrichment(),
	eventschema.NewValidation(),
)
```

Options are applied to individual processors:

```go
processor := schema.NewEventProcessor(
	eventschema.NewEnrichment(
		eventschema.WithAddEnumSiblings(true),
		eventschema.WithAddObservables(false),
	),
	eventschema.NewValidation(
		eventschema.WithWarnOnMissingRecommended(),
	),
)
```

`NewEnrichment` adds enum siblings and observables by default. Use `WithAddEnumSiblings(false)` or `WithAddObservables(false)` to disable either enrichment.

`NewValidation` reports required validation errors by default. Use `WithWarnOnMissingRecommended()` to report missing recommended attributes as warnings.

### Result Model

`ProcessingResult` contains processor-specific results and any non-fatal processing issues:

```go
type ProcessingResult struct {
	Validation eventschema.ValidationResult
	Enrichment eventschema.EnrichmentResult
	Issues     []eventschema.ProcessingIssue
}
```

Validation issues are split by severity:

```go
result.Validation.Errors
result.Validation.Warnings
```

Enrichment counters report what was added:

```go
result.Enrichment.EnumSiblingsAdded
result.Enrichment.ObservablesAdded
```

For a complete working example of library usage, see the CLI implementation in `cmd/ocsf-toolkit`.

## Development

Local development requires a local checkout of this repository, Go 1.25.0 or newer, and `golangci-lint`.

Run the standard local verification target before submitting changes:

```sh
make verify
```

This lints, tests, and builds the CLI for the local platform. The development binary is written to:

```sh
build/ocsf-toolkit
```

See the `Makefile` for individual targets when you need to run one step directly.

## Appendix: Compiled Schema

The toolkit uses the compiled schema format produced by the [OCSF Schema Compiler](https://pypi.org/project/ocsf-schema-compiler/). It does not read the raw OCSF schema repository directly.

Set up a Python virtual environment and install the compiler:

```sh
python3 -m venv .venv
. .venv/bin/activate
pip install ocsf-schema-compiler
```

To compile a released version of the OCSF Schema, clone the schema repository at that version's tag:

```sh
branch=v1.8.0
git clone --single-branch --branch "$branch" https://github.com/ocsf/ocsf-schema.git "ocsf-schema-$branch"
```

Then compile it:

```sh
ocsf-schema-compiler ocsf-schema-v1.8.0 > ocsf-schema-v1.8.0.json
```

Use the generated JSON file as the schema input for both the library and CLI.
