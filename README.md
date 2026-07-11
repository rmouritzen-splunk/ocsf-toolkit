# OCSF Toolkit

OCSF Toolkit provides a Go library and a command line tool for processing [OCSF](https://schema.ocsf.io/) events with a compiled OCSF schema.

The current processors support:

- Enrichment: add enum siblings and observables.
- Enrichment removal: safely or forcibly remove enum siblings and observables.
- Validation: validate a single event against a compiled schema.

Event mutations run before validation, so validation checks the final processed event.

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

Validate a single event and write its processing report to stdout:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event event.json \
  --validate \
  --report-output -
```

The `--schema` argument must point to a compiled OCSF schema file. See [Compiled Schema](#appendix-compiled-schema).

General form:

```sh
ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--validate] [options]
```

Select at least one processing action. Compatible actions may be combined.

### CLI Examples

Enrich and validate a single event, writing both outputs to one directory:

```sh
ocsf-toolkit -s ocsf-schema-v1.8.0.json -e event.json -E -V -o out
```

This writes:

- `out/events/event.json`
- `out/reports/event.json`

Enrich a single event without changing the input file:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event event.json \
  --enrich \
  --event-output enriched-event.json \
  --report-output enrichment-report.json
```

Validate in CI and fail the command when validation errors are found:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --events-dir events \
  --validate \
  --output-dir validation-results \
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
out/events/windows/windows_service_activity.json
out/reports/windows/windows_service_activity.json
```

Safely remove redundant enum siblings and observables, writing the processed event and processing report to one tree:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --events-dir events \
  --unenrich \
  --output-dir processed
```

This writes processed events beneath `processed/events/` and per-event processing reports beneath `processed/reports/`. Use `--force-remove-enum-siblings` or `--force-remove-observables` only when potentially non-redundant source content may be discarded. Forced observable removal deletes the entire `observables` attribute without inspecting its entries. `--retain-enum-siblings` and `--retain-observables` disable the corresponding removal.

Read a single event from stdin, write enriched JSON to stdout, and write its processing report to a file:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event - \
  --enrich \
  --event-output - \
  --report-output enrichment-report.json
```

### Output Behavior

The CLI never modifies input event files. Single-event processing, including `--event -` for stdin, does not choose an output destination implicitly. A mutated single event uses `--event-output` or `--output-dir`, and a single processing report uses `--report-output` or `--output-dir`. Directory mode requires `--output-dir`.

Output directories are created if necessary. Output files are not replaced unless `--overwrite` is supplied. In directory mode, an existing output directory must be empty unless `--overwrite` is supplied; unrelated existing files are left unchanged when overwrite is enabled.

Input and output directory trees must not overlap, including when symbolic links make differently written paths refer to the same location. The selected output directory itself may be a symbolic link, but the `events/` and `reports/` namespaces beneath it may contain only regular files and actual directories. Symbolic links, Windows junctions, and other special filesystem entries are rejected.

`--output-dir` writes processed events beneath `events/` and per-event processing reports beneath `reports/`. Both namespaces preserve the input-relative path, which prevents event filenames from colliding with report filenames.

- Enrichment and unenrichment create both `events/` and `reports/`.
- Validation-only processing creates `reports/`.
- Enrichment or unenrichment combined with validation creates both namespaces.
- `--skip-invalid-output` creates only the validation report for an invalid event.

Processing reports include the source event, the destination event when one was written, and the applicable processor results:

Enrichment reports include counts for added enum siblings and observables plus any issues explaining requested additions that could not be performed safely.

```json
{
  "event_source": "event.json",
  "event_destination": "out/events/event.json",
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
    ]
  }
}
```

When unenrichment is selected, the same report can contain removal counts and issues explaining why observable entries were retained:

```json
{
  "event_source": "event.json",
  "event_destination": "out/events/event.json",
  "enrichment_removal": {
    "enum_siblings_removed": 2,
    "enum_siblings_retained": 1,
    "observables_removed": 3,
    "observables_retained": 1
  },
  "issues": [
    {
      "phase": "enrichment_removal",
      "severity": "warning",
      "code": "observable_value_not_found",
      "message": "The observable value is not present at its name path."
    }
  ]
}
```

Event output is the processed event JSON. For example, if the schema defines `activity_id` with the `activity_name` enum sibling, enrichment can add the sibling field:

```json
{
  "activity_id": 1,
  "activity_name": "Create"
}
```

In single-event mode, `--event-output -` and `--report-output -` write to stdout. When both are selected, the processed event is written first and the processing report second. Compact JSON is therefore valid JSON Lines. With `--pretty-json`, the same values are written sequentially as a whitespace-separated JSON stream that can be read by a streaming JSON decoder. A skipped invalid event omits the processed event.

For example, this emits two compact JSON objects, one per line:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event event.json \
  --enrich \
  --validate \
  --event-output - \
  --report-output -
```

Directory processing writes a human-readable summary with tool metadata to stdout by default. Use `--quiet` to suppress this default. `--summary-file` and `--summary-json-file` add explicit human-readable and JSON summary destinations; they may be used together, with or without `--quiet`, and either may use `-` for stdout. When both use stdout, the human-readable summary is written first. Summary options apply only to directory processing.

stderr is reserved for errors and failure diagnostics. Output paths selected for different artifacts must identify different files, including when filesystem aliases make differently written paths refer to the same file.

Path preservation differs slightly between directory and single-event processing. In directory mode,
the toolkit walks files under `--events-dir` and computes each output path relative to that input
root. In single-event mode, `--event` is supplied directly by the user, so preserving an absolute
path or a relative path containing `..` could place output outside the selected output directory.
For that reason, single-event output directories preserve only safe relative paths; unsafe paths use
the input file's basename.

Use `--skip-invalid-output` with a mutating operation and `--validate` to write only the validation report when an event has validation errors. The processed event and non-validation report sections are omitted for that event.

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

Load a compiled schema, build a processor pipeline, and process an event:

```go
schema, err := eventschema.New("ocsf-schema-v1.8.0.json")
if err != nil {
	return err
}

pipeline, err := schema.NewEventProcessorPipeline(
	eventschema.NewEnrichment(),
	eventschema.NewValidation(),
)
if err != nil {
	return err
}

event, err := jsonio.ReadObject("event.json")
if err != nil {
	return err
}

result, err := pipeline.ProcessEvent(event)
if err != nil {
	return err
}

if len(result.Validation.Errors) > 0 {
	fmt.Printf("event has %d validation error(s)\n", len(result.Validation.Errors))
}
```

`Schema` and `EventProcessorPipeline` values are safe for concurrent use after construction when each `ProcessEvent` call receives a distinct event map. The event map and its nested maps or slices must not be accessed or mutated concurrently while processing is running.

`ProcessEvent` mutates the event in place when enrichment or enrichment removal is enabled. Processing is not transactional: if `ProcessEvent` returns an error, the event may already be partially modified. Callers that need to preserve the original event should deep-copy it before processing.

Validation failures are reported in `ProcessingResult`; they do not normally return a Go `error`. The `error` return is for tooling failures or unusable input.

For JSON-encoded events, preserving numbers as `json.Number` is safer than decoding into `float64`, especially for OCSF integer values. The `jsonio` file and object helpers do this automatically. Use `jsonio.NewDecoder` when decoding another JSON shape, such as a typed structure, with the same number-preserving behavior. Events built from other sources can use normal Go values such as signed integer types, `float32`, `float64`, `bool`, `string`, slices, and nested `jsonish.Map` values.

Array attributes may use JSON-native `[]any` values or typed Go slices such as `[]int64`, `[]float64`, and `[]jsonish.Map`. Elements are validated using the same scalar and object rules as JSON-decoded array elements.

### Processors

Create an enrichment processor:

```go
pipeline, err := schema.NewEventProcessorPipeline(eventschema.NewEnrichment())
```

Create a validation processor:

```go
pipeline, err := schema.NewEventProcessorPipeline(eventschema.NewValidation())
```

Create a safe enrichment-removal processor:

```go
pipeline, err := schema.NewEventProcessorPipeline(eventschema.NewEnrichmentRemoval())
```

Safe removal preserves enum siblings and observables that cannot be proven redundant. Force removal is explicit:

```go
pipeline, err := schema.NewEventProcessorPipeline(
	eventschema.NewEnrichmentRemoval(
		eventschema.WithForceRemoveEnumSiblings(),
		eventschema.WithForceRemoveObservables(),
	),
)
```

Create a pipeline that enriches and then validates:

```go
pipeline, err := schema.NewEventProcessorPipeline(
	eventschema.NewEnrichment(),
	eventschema.NewValidation(),
)
```

Options are applied to individual processors:

```go
pipeline, err := schema.NewEventProcessorPipeline(
	eventschema.NewEnrichment(
		eventschema.WithAddEnumSiblings(true),
		eventschema.WithAddObservables(false),
	),
	eventschema.NewValidation(
		eventschema.WithWarnOnMissingRecommended(),
	),
)
```

`NewEnrichment` adds enum siblings and observables by default. Use `WithAddEnumSiblings(false)` or `WithAddObservables(false)` to disable either enrichment. When enum ID 99 has no sibling value, enrichment adds the schema caption, typically `Other`, and reports that synthesized value as an enrichment issue so a corresponding validation warning has clear provenance.

`NewEventProcessorPipeline` validates the complete processing configuration. It returns an aggregate error containing all detected problems with an empty or no-op configuration, duplicate processors, retain/force conflicts, or a configuration that adds and removes the same category. CLI flag validation reports equivalent conflicts using the relevant flag names.

Across event processing, an object attribute whose value is null is treated as missing. Null array elements remain invalid because no OCSF array element type permits null.

Enrichment preserves existing observable entries and appends generated entries that are not duplicates. Duplicate identity uses the exact `name`, the integral `type_id`, and whether `value` is omitted, null, or an exact string; the derived `type` caption and unrelated fields do not affect identity. Each skipped generated duplicate is reported as a nonfatal enrichment issue and is not included in `ObservablesAdded`. After the event class resolves and observable enrichment or removal runs, an empty `observables` array is removed. Other malformed structure that prevents requested enrichment is also reported in `ProcessingResult.Issues`; enrichment does not attempt to duplicate general validation.

`NewEnrichmentRemoval` safely removes supported scalar integral enum siblings and redundant observables by default. Use `WithRemoveEnumSiblings(false)` or `WithRemoveObservables(false)` to retain either category. Legacy enum arrays remain untouched. Observable names support bare, `[]`, `[*]`, numeric index, and `$`-rooted path forms. Scalar observable values are matched using OCSF-compatible string conversion, and an explicit null value matches either null or missing event content. Object observables without values are removed only when their path resolves to a JSON object.

`NewValidation` reports required validation errors by default. Use `WithWarnOnMissingRecommended()` to report missing recommended attributes as warnings.

### Result Model

`ProcessingResult` contains processor-specific results and any non-fatal processing issues:

```go
type ProcessingResult struct {
	Validation        eventschema.ValidationResult
	Enrichment        eventschema.EnrichmentResult
	EnrichmentRemoval eventschema.EnrichmentRemovalResult
	Issues            []eventschema.ProcessingIssue
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

Enrichment-removal counters report what was removed or retained:

```go
result.EnrichmentRemoval.EnumSiblingsRemoved
result.EnrichmentRemoval.EnumSiblingsRetained
result.EnrichmentRemoval.ObservablesRemoved
result.EnrichmentRemoval.ObservablesRetained
```

`ProcessingResult.Issues` aggregates phase-specific processor diagnostics. Validation issues also appear in `Validation.Errors` or `Validation.Warnings`; enrichment and enrichment-removal issues explain requested mutations that could not be performed safely.

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

Run the event-processing benchmark suite with:

```sh
make benchmark
```

Compare the current checkout with the newest reachable release tag using the same `v` followed by
a digit and no `+` sanity checks as the release workflow:

```sh
make benchmark-compare
```

Prerelease tags participate in this baseline. Override the selected tag when necessary:

```sh
make benchmark-compare BENCHMARK_BASE=v0.1.0
```

The comparison runs both revisions on the same machine and reports statistically evaluated runtime,
bytes, and allocation differences through `benchstat`. A benchmark introduced after the selected
release appears only in the current column until a later release contains the same benchmark.

Project design and maintenance documentation:

- [Architecture](docs/architecture.md)
- [Roadmap](docs/roadmap.md)
- [Release process](docs/release_process.md)

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
