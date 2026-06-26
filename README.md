# OCSF Toolkit

OCSF Toolkit provides a Go library and a command line tool for processing [OCSF](https://schema.ocsf.io/) events with a compiled OCSF schema.

The current processors support:

- Enrichment: add enum sibling captions and observables.
- Validation: validate a single event against a compiled schema.

Enrichment runs before validation when both are enabled, so validation checks the event after local processing has been applied.

## CLI Usage

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

General form:

```sh
ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) (--enrich | --validate) [options]
```

The `--schema` argument must point to a compiled OCSF schema file. See [Compiled Schema](#appendix-compiled-schema).

Common single-event processing:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.8.0.json \
  --event event.json \
  --enrich \
  --validate \
  --output-dir out
```

Equivalent with common short options:

```sh
ocsf-toolkit -s ocsf-schema-v1.8.0.json -e event.json -E -V -o out
```

This writes:

- `out/event.json`
- `out/event-validation.json`

Process a directory tree:

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

### CLI Notes

- Output directories are created if necessary.
- Directory outputs preserve safe input-relative paths.
  - With `--events-dir`, paths are relative to that input directory.
  - With `--event`, safe relative paths are preserved as supplied. Absolute paths and paths with `..` use only the basename.
- Output files are not replaced unless `--overwrite` is supplied.
- `--enrich-in-place` replaces input event files and does not require `--overwrite` for enrichment.
- `--validation-output -` writes validation results to stdout.
- `--enrich-output -` writes enriched event JSON to stdout.
- At most one selected output may write to stdout.
- `--fail-on-validation-errors` exits non-zero when one or more events has validation errors.
- `--skip-invalid-output` prevents non-validation outputs from being written for events with validation errors.

Path preservation differs slightly between directory and single-event processing. In directory mode,
the toolkit walks files under `--events-dir` and computes each output path relative to that input
root. In single-event mode, `--event` is supplied directly by the user, so preserving an absolute
path or a relative path containing `..` could place output outside the selected output directory.
For that reason, single-event output directories preserve only safe relative paths; unsafe paths use
the input file's basename.

Run full help:

```sh
ocsf-toolkit --help
```

## Library Usage

Import the event schema and JSON helpers:

```go
import (
	"fmt"

	"github.com/ocsf/ocsf-processor/eventschema"
	"github.com/ocsf/ocsf-processor/jsonio"
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

`ProcessEvent` mutates the event in place when enrichment is enabled. Validation failures are reported in `ProcessingResult`; they do not normally return a Go `error`. The `error` return is for tooling failures or unusable input.

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

Local development requires Go 1.25.0 or newer and `golangci-lint`.

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
