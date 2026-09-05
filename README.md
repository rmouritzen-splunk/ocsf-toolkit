# OCSF Toolkit

OCSF Toolkit provides a Go library and a command line tool for processing [OCSF](https://schema.ocsf.io/) events with a compiled OCSF schema.

The current processors support:

- Enrichment: add enum siblings and observables.
- Enrichment removal: safely or forcibly remove enum siblings and observables.
- Validation: validate a single event against a compiled schema.

Event mutations run before validation, so validation checks the final processed event.

## Processing Behavior

The processing algorithms are documented independently of this project's Go implementation. The same logical behavior can be implemented for generic maps, concrete structs or classes, columnar rows, or other in-memory forms, and for encodings such as JSON, Parquet, or Avro when the logical OCSF values are preserved:

- [Event processing model](docs/event-processing.md): shared schema traversal, profiles, null semantics, encoding independence, and operation ordering.
- [Enrichment](docs/enrichment.md): adding enum siblings and observables.
- [Enrichment removal](docs/enrichment-removal.md): safe and forced removal of redundant enrichment.
- [Validation](docs/validation.md): structural, type, constraint, metadata, and observable validation.
- [Frequently asked questions](docs/FAQ.md): operational behavior, limitations, and report codes such as `issue_event_traversal_limited`.

These guides are intended both for toolkit users and for developers implementing compatible processing in another language or software ecosystem.

## CLI Usage

### Install

Download an archive from the repository's GitHub Releases page:
<https://github.com/ocsf/ocsf-toolkit/releases>.

Release archives are named by version, operating system, and architecture:

```text
ocsf-toolkit_v0.9.0_darwin_arm64.tar.gz
ocsf-toolkit_v0.9.0_darwin_amd64.tar.gz
ocsf-toolkit_v0.9.0_linux_arm64.tar.gz
ocsf-toolkit_v0.9.0_linux_amd64.tar.gz
ocsf-toolkit_v0.9.0_windows_arm64.zip
ocsf-toolkit_v0.9.0_windows_amd64.zip
```

For macOS, choose the `darwin` OS archive. Modern Apple Silicon machines such as M1, M2, M3,
and newer use `arm64`. Older Intel Macs use `amd64`.

Extract the archive and check the binary:

```sh
tar -xzf ocsf-toolkit_v0.9.0_darwin_arm64.tar.gz
cd ocsf-toolkit_v0.9.0_darwin_arm64
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

The CLI decodes event files with Go's standard `encoding/json` package. In its default mode, duplicate names in one JSON object are accepted and later values replace or merge into earlier values according to that decoder's rules. Builds made with `GOEXPERIMENT=jsonv2` reject duplicate object names by default. An in-memory event map, including one populated from another encoding such as Parquet or Avro, inherently has at most one value for each attribute name.

Validate a single event and write its processing report to stdout:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --event event.json \
  --validate \
  --report-output -
```

The `--schema` argument must point to a compiled OCSF schema file. The CLI follows symbolic links and requires the resulting path to be a regular file that can be opened for reading before schema loading begins. See [Compiled Schema](#appendix-compiled-schema).

General form:

```sh
ocsf-toolkit --schema COMPILED_SCHEMA_FILE (--event FILE | --events-dir DIR) [--enrich] [--unenrich] [--force-remove] [--validate] [options]
```

Select at least one processing action. Compatible actions may be combined.

The mutation actions operate on two independent components: `enum-siblings` and `observables`. `--enum-siblings ACTION` and `--observables ACTION` set one component's action directly, where `ACTION` is `add`, `remove`, or `force-remove` (bare `--enum-siblings` or `--observables` means `add`). Attached forms such as `--enum-siblings=remove` are also accepted. `--enrich`, `--unenrich`, and `--force-remove` are shorthand that set both components at once to `add`, `remove`, or `force-remove` respectively. The shorthand flags cannot be combined with each other or with `--enum-siblings`/`--observables`; select either one shorthand flag or the per-component flags. Enum-sibling work always runs before observable work, regardless of flag order.

### CLI Examples

Enrich and validate a single event, writing both outputs to one directory:

```sh
ocsf-toolkit -s ocsf-schema-v1.9.0.json -e event.json -E -V -o out
```

This writes:

- `out/events/event.json`
- `out/reports/event.report.json`

Enrich a single event without changing the input file:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --event event.json \
  --enrich \
  --event-output enriched-event.json \
  --report-output enrichment-report.json
```

Validate in CI and fail the command when validation errors are found:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --events-dir events \
  --validate \
  --output-dir validation-results \
  --fail-on-validation-errors
```

Enrich and validate a directory tree:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --events-dir events \
  --enrich \
  --validate \
  --output-dir out
```

Select the notation used for generated observable names and warn during validation when existing names use another supported notation:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --events-dir events \
  --enrich \
  --validate \
  --observable-path-notation indexed \
  --output-dir out
```

Supported styles are `simple`, `brackets`, `wildcard`, `indexed`, and `jsonpath`. The option requires observable enrichment or validation. Observable resolution accepts every supported notation regardless of this preference.

Add only selected observable types by repeating their numeric IDs:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --events-dir events \
  --enrich \
  --observable-id 1 \
  --observable-id 2 \
  --observable-id 4 \
  --output-dir out
```

Omitting `--observable-id` adds every schema-declared observable type. Repeat the option to select multiple types. Each selected ID, including `0`, must exist in the loaded schema; duplicate IDs are accepted and pipeline construction deduplicates them, while reporting all unknown IDs together.

Set the handling level for a processing issue code, or use `all` to set the level for every issue:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --events-dir events \
  --enrich \
  --issue-level issue_enrichment_observable_duplicate_skipped=ignored \
  --output-dir out
```

Repeat `--issue-level ISSUE_CODE=LEVEL` to configure multiple codes. Every issue code defaults to `warning`, as reported by `issue.Code.DefaultLevel()`. Levels are `ignored`, `warning`, and `error`; `error` stops at the first matching issue. Use `all=LEVEL` once before specific codes; each specific code occurs once. Mandatory diagnostics reporting class-resolution failure or limited event traversal cannot be ignored. The same policy applies to schema initialization issues in the CLI. `ocsf-toolkit --list-issue-codes` prints every issue code and exits, noting which codes are mandatory.

Validation finding levels use the same repeated key-value form:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --events-dir events \
  --validate \
  --validation-level all=error \
  --validation-level validation_attribute_unknown=ignored \
  --output-dir out
```

Repeat `--validation-level VALIDATION_CODE=LEVEL` to configure multiple codes; it requires `--validate`. Use `all=LEVEL` once before specific codes; each specific code occurs once. Every validation finding may be ignored, including findings for a missing, wrong-type, or unknown `class_uid`; the corresponding processing issues remain mandatory. Enabling validation while resolving every code to `ignored` is a configuration error. `--fail-on-validation-errors` uses effective levels after policy is applied. `ocsf-toolkit --list-validation-codes` prints every code with its description and toolkit default level.

CLI help canonically displays flag values with a space, such as `--event my_event.json`. The parser also accepts an attached flag value, such as `--event=my_event.json`.

The `validation_attribute_recommended_missing` code defaults to `ignored`. Set it to `warning` or `error` with `--validation-level` when missing recommended attributes should produce findings.

Directory outputs preserve input-relative paths. For example:

```text
events/windows/windows_service_activity.json
```

becomes:

```text
out/events/windows/windows_service_activity.json
out/reports/windows/windows_service_activity.report.json
```

Safely remove redundant enum siblings and observables, writing the processed event and processing report to one tree:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --events-dir events \
  --unenrich \
  --output-dir processed
```

This writes processed events beneath `processed/events/` and per-event processing reports beneath `processed/reports/`. Use `--enum-siblings remove` or `--observables remove` alone to select one safe-removal component. Use `--force-remove`, or `--enum-siblings force-remove`/`--observables force-remove` for one component, only when potentially non-redundant source content may be discarded. Forced observable removal deletes the entire `observables` attribute without inspecting its entries. Forced enum sibling removal still preserves siblings required for integral enum ID 99, including an array sibling when any paired enum element is 99.

Read a single event from stdin, write enriched JSON to stdout, and write its processing report to a file:

```sh
ocsf-toolkit \
  --schema ocsf-schema-v1.9.0.json \
  --event - \
  --enrich \
  --event-output - \
  --report-output enrichment-report.json
```

### Output Behavior

The CLI never modifies input event files. Single-event processing, including `--event -` for stdin, does not choose an output destination implicitly. A mutated single event uses `--event-output` or `--output-dir`, and a single processing report uses `--report-output` or `--output-dir`. Directory mode requires `--output-dir`.

Output directories are created if necessary. Output files are not replaced unless `--overwrite` is supplied. In directory mode, an existing output directory must be empty unless `--overwrite` is supplied. When overwrite is enabled, files selected as output destinations are replaced while preserving their existing permission bits on a best-effort basis; other existing files are left unchanged.

Input and output directory trees must not overlap, including when symbolic links make differently written paths refer to the same location. The selected output directory itself may be a symbolic link, but the `events/` and `reports/` namespaces beneath it may contain only regular files and actual directories. Symbolic links, Windows junctions, and other special filesystem entries are rejected.

`--events-dir` must exist and name an actual directory rather than a symbolic link. Directory traversal does not follow symbolic links found within the input tree, so linked files and directories are ignored.

While processing `--events-dir`, other activity modifying the filesystem is handled on a best-effort basis. See [What happens if input or output directories change during processing?](docs/FAQ.md#what-happens-if-input-or-output-directories-change-during-processing) in the FAQ for details.

Overlap checking applies to the `--events-dir`/`--output-dir` trees as a whole, not to individual derived output paths. Two distinct input files that derive the same output path within one run (for example, via case folding or unusual filesystem links) are not detected against each other; see [Can two different input files produce the same output path?](docs/FAQ.md#can-two-different-input-files-produce-the-same-output-path) in the FAQ.

`--output-dir` writes processed events beneath `events/` and per-event processing reports beneath `reports/`. Both namespaces preserve input-relative directories. Report filenames insert `.report` before the input filename extension, which prevents a report from colliding with an event that has the same relative path.

- Enrichment, safe removal, and forced removal create both `events/` and `reports/`.
- Validation-only processing creates `reports/`.
- Any mutation action combined with validation creates both namespaces.

Processing reports include the source event, the destination event when one was written, and the applicable processor results:

Enrichment reports include counts for added enum siblings and observables plus any issues explaining requested additions that could not be performed safely.

```json
{
  "report_version": 1,
  "event_source": "event.json",
  "event_destination": "out/events/event.json",
  "validation": {
    "findings": [
      {
        "level": "error",
        "code": "validation_attribute_required_missing",
        "message": "Required attribute \"time\" is missing.",
        "details": {
          "attribute_path": "time",
          "attribute": "time"
        }
      }
    ]
  }
}
```

When unenrichment is selected, the same report can contain removal counts and issues explaining why supported enum siblings or observable entries could not be safely removed:

```json
{
  "report_version": 1,
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
      "source": "enrichment_removal",
      "code": "issue_observable_value_not_found",
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

In single-event mode, either `--event-output -` or `--report-output -` writes its selected JSON document to stdout. At most one output option among `--event-output`, `--report-output`, `--summary`, and `--summary-json` may use `-` in one invocation. Send every other selected output to a file or `--output-dir`; stdout therefore contains one output representation rather than a heterogeneous stream.

`--pretty-json` pretty-prints every selected JSON destination, including stdout. It does not affect human-readable summaries.

Successfully completed directory processing writes a human-readable summary with tool metadata to stdout by default. Use `--quiet` to suppress this default. `--summary` and `--summary-json` add explicit human-readable and JSON summary destinations; they may be used together, with or without `--quiet`, subject to the one-output-on-stdout rule. Summary options apply only to directory processing. JSON processing reports carry `report_version: 1`; JSON directory summaries carry `summary_version: 1` and group initialization issues, per-file results, and aggregate validation, enrichment, enrichment-removal, issue, and output counts. If directory processing stops on a fatal error, normal summaries are not written; stderr reports the failure and, when at least one event completed, only the number of event files processed before the error.

stderr is reserved for errors, failure diagnostics, and nonfatal schema initialization issues. When successfully parsed command-line options contain multiple independent configuration problems, each problem is printed on its own `error: ...` line followed by terse usage once. Parsing, filesystem operations, schema loading, and processing remain fail-fast. Before processing, output paths selected for different command-wide artifacts are checked and must identify different files, including when existing filesystem aliases make differently written paths refer to the same file.

Path preservation differs slightly between directory and single-event processing. In directory mode, the toolkit walks files under `--events-dir` and computes each output path relative to that input root. In single-event mode, `--event` is supplied directly by the user. Paths that remain within the current directory tree after lexical cleaning are preserved beneath the selected output directory. Absolute paths, relative paths that would escape the current directory tree, and other non-local paths use a safe basename instead.

### Exit Codes

- `0`: the command completed successfully.
- `1`: processing failed, writing output failed, or validation errors were found with `--fail-on-validation-errors`.
- `2`: command-line parsing or configuration failed.

Validation errors do not change the exit code by default. Use `--fail-on-validation-errors` when effective validation errors after policy is applied should fail a CI job or script.

Run full help:

```sh
ocsf-toolkit --help
```

## Library Usage

Import the event schema, enrichment action type, validation result types, and JSON helpers:

```go
import (
	"fmt"
	"log"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventpipeline"
	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/validation"
)
```

Load a compiled schema, build a pipeline, and process an event:

```go
schema, initializationIssues, err := eventpipeline.NewSchema("ocsf-schema-v1.9.0.json")
if err != nil {
	return err
}
for _, issue := range initializationIssues {
	log.Printf("%s: %s", issue.Code, issue.Message)
}

pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithEnumSiblings(enrichment.Add),
	eventpipeline.WithObservables(enrichment.Add),
	eventpipeline.WithValidation(),
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

if count := result.Validation().Count(validation.LevelError); count > 0 {
	fmt.Printf("event has %d validation error(s)\n", count)
}
```

Use `NewSchemaFromFS` when the schema is in an `fs.FS`; its path must satisfy `fs.ValidPath`. `NewSchemaFromBytes` avoids copying schemas that are already available as byte slices, including embedded schemas. An embedded `[]byte` can instead be wrapped in a reader and passed to `NewSchemaFromReader`, but `NewSchemaFromBytes` is more memory-efficient because the reader path requires an additional input-sized buffer. Use `NewSchemaFromReader` when the schema is exposed only as an `io.Reader`, such as a decompressor or network response body; the caller remains responsible for closing the source. For example, embed a compiled schema while continuing to use `jsonio` for number-preserving event input:

```go
import (
	_ "embed"

	"github.com/ocsf/ocsf-toolkit/eventpipeline"
	"github.com/ocsf/ocsf-toolkit/jsonio"
)

//go:embed ocsf-schema-v1.9.0.json
var compiledSchema []byte

schema, initializationIssues, err := eventpipeline.NewSchemaFromBytes(compiledSchema)
if err != nil {
	return err
}
_ = initializationIssues // Log, suppress, or otherwise handle nonfatal initialization issues.

event, err := jsonio.ReadObject("event.json")
if err != nil {
	return err
}
```

`Schema` and `Pipeline` are concrete handles with private state. Create schemas with `eventpipeline.NewSchema`, `eventpipeline.NewSchemaFromFS`, `eventpipeline.NewSchemaFromBytes`, or `eventpipeline.NewSchemaFromReader`, and create pipelines with `eventpipeline.NewPipeline` and `eventpipeline.WithSchema`; their zero values return initialization errors. `NewSchemaFromBytes` does not retain its input. `NewSchemaFromReader` consumes the reader through EOF but does not close it. Constructed values are safe for concurrent use when each `ProcessEvent` call receives a distinct event map. The event map and its nested maps or slices must not be accessed or mutated concurrently while processing is running.

**NOTE:** Use of `GOEXPERIMENT=jsonv2` is recommended for faster schema loading (see below).

```sh
GOEXPERIMENT=jsonv2 go build ./...
GOEXPERIMENT=jsonv2 go test ./...
```

Schema loading is dominated by JSON decoding. With the OCSF 1.9 test schema, JSON v2 reduced median loading time by approximately 45% and allocation count by approximately 35%. It requires no newer toolchain than the module's existing Go 1.25 baseline. The default decoder remains appropriate when schema initialization is not performance-sensitive. The setting applies to the complete build, so test with it as well; the resulting binary does not need the environment variable at runtime. JSON v2 remains experimental and outside the Go 1 compatibility guarantee. See the [Go 1.25 release notes](https://go.dev/doc/go1.25).

`ProcessEvent` mutates the event in place when enrichment or enrichment removal is enabled. Processing is not transactional: if `ProcessEvent` returns an error, the event may already be partially modified. Callers that need to preserve the original event should deep-copy it before processing.

Validation failures are reported in `eventpipeline.ProcessingResult`; they do not normally return a Go `error`. The `error` return is for tooling failures or unusable input.

For JSON-encoded events, preserving numbers as `json.Number` is safer than decoding into `float64`, especially for OCSF integer values. The `jsonio` file and object helpers do this automatically. Use `jsonio.NewDecoder` when decoding another JSON shape, such as a typed structure, with the same number-preserving behavior. Events built from other sources can use normal Go values such as signed integer types, `float32`, `float64`, `bool`, `string`, slices, and nested `jsonish.Map` values.

The JSON helpers use Go's active standard JSON implementation. The default `encoding/json` decoder accepts duplicate object member names, with later values replacing or merging into earlier values according to its rules; `GOEXPERIMENT=jsonv2` rejects duplicates by default. Applications that require a different duplicate-name policy should enforce it in their decoding boundary before calling `ProcessEvent`.

Array attributes may use JSON-native `[]any` values or typed Go slices such as `[]int64`, `[]float64`, and `[]jsonish.Map`, as well as fixed-length arrays. Defined container types are accepted and traversed like their unnamed equivalents. Every element is validated using the same scalar and object rules regardless of its container representation; defined element types remain unsupported, while type aliases remain identical to their aliased representations.

### Pipeline Options

`eventpipeline.NewPipeline` takes a list of `eventpipeline.PipelineOption` values. Configure its schema with `eventpipeline.WithSchema`. Each single-valued option, including `WithSchema`, `WithEnumSiblings`, `WithObservables`, `WithEnrichmentObservablePathNotation`, and `WithValidation`, may be passed once; repeating one is a configuration error rather than an override. The same rule applies to `WithValidationObservablePathNotation` within `WithValidation`.

Add enum siblings and observables:

```go
pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithEnumSiblings(enrichment.Add),
	eventpipeline.WithObservables(enrichment.Add),
)
```

Validate only:

```go
pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithValidation(),
)
```

Safely remove enum siblings and observables:

```go
pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithEnumSiblings(enrichment.Remove),
	eventpipeline.WithObservables(enrichment.Remove),
)
```

Safe removal preserves enum siblings and observables that cannot be proven redundant. Force removal is explicit, and each component can select a different action:

```go
pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithEnumSiblings(enrichment.ForceRemove),
	eventpipeline.WithObservables(enrichment.ForceRemove),
)
```

Build a pipeline that enriches and then validates; enum-sibling work always runs before observable work, and mutation always runs before validation, regardless of option order:

```go
pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithEnumSiblings(enrichment.Add),
	eventpipeline.WithObservables(enrichment.Add),
	eventpipeline.WithValidation(),
)
```

`WithValidation` takes its own nested `ValidationOption` values:

```go
pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithEnumSiblings(enrichment.Add),
	eventpipeline.WithObservables(enrichment.Add, 1, 2, 4),
	eventpipeline.WithEnrichmentObservablePathNotation(pathstyle.ArrayIndexed),
	eventpipeline.WithValidation(
		eventpipeline.WithValidationLevel(
			validation.AttributeRecommendedMissing,
			validation.LevelWarning,
		),
		eventpipeline.WithValidationObservablePathNotation(pathstyle.ArrayIndexed),
	),
)
```

`WithEnumSiblings` takes an `enrichment.Action`: `enrichment.Add` adds enum siblings, `enrichment.Remove` or `enrichment.ForceRemove` removes them, and `enrichment.None` (the default when the option is omitted) leaves them alone. `WithObservables` works the same way for observables, and when its action is `enrichment.Add`, an optional list of observable type IDs restricts generation to those types; an empty list means all types, duplicate IDs are harmless, and pipeline construction reports every selected ID absent from the schema. Supplying IDs when the action is not `enrichment.Add` is invalid. Use `WithEnrichmentObservablePathNotation` with a `pathstyle.Style` value to select generated observable name notation; it has no effect unless observables are added. Generated enum sibling arrays are parallel to their enum arrays, with one caption at each matching index. When integral enum ID 99 has no sibling value, including at an integral enum-array position, enrichment adds the schema caption, typically `Other`, and reports that synthesized value as an enrichment issue so a corresponding validation warning has clear provenance. String enum key `"99"` has no special meaning.

`eventpipeline.NewPipeline` returns the first detected problem in deterministic validation order. It first checks structural option errors such as repeated single-valued options and invalid level-rule ordering, then requires an initialized schema selected through exactly one `WithSchema`, and finally validates the resolved processing configuration, including empty or no-op configurations, invalid actions, observable path notation or type IDs configured without adding observables, invalid path notation, and invalid issue or validation level rules. CLI flag validation reports equivalent conflicts using the relevant flag names.

Across event processing, an object attribute whose value is null is treated as missing. Null array elements remain invalid because no OCSF array element type permits null.

Enrichment preserves existing observable entries and appends generated entries that are not duplicates. Scalar values, including empty strings, produce a string `value`; object observables omit `value`. Structured content found where the schema declares an observable scalar is skipped and reported as a nonfatal enrichment issue. Duplicate identity uses the exact `name`, the integral `type_id`, and whether `value` is omitted, null, or an exact string; the derived `type` caption and unrelated fields do not affect identity. Each skipped generated duplicate is reported as a nonfatal enrichment issue and is not included in `ObservablesAdded`. After the event class resolves and observable enrichment or removal runs, an empty `observables` array is removed. Other malformed structure that prevents requested enrichment is also reported by `eventpipeline.ProcessingResult.Issues()`; enrichment does not attempt to duplicate general validation.

Safe removal (`enrichment.Remove`) removes supported scalar and array enum siblings whose source has direct type `integer_t` or `long_t` and whose same-shaped target has direct type `string_t`, plus redundant observables that can be proven safe. Enum and sibling arrays must have equal lengths, and safe removal compares every value with the caption at the same index before removing the sibling array. Validation reports unequal enum/sibling array lengths with `validation_attribute_enum_array_sibling_length_mismatch`. Observable names support bare, `[]`, `[*]`, numeric index, and `$`-rooted path forms. Scalar observable values are matched using the toolkit's stable scalar-to-string formatting, and an explicit null value matches either null or missing event content. Object observables without values are removed only when their path resolves to a JSON object.

`WithValidation` reports findings at each code's toolkit default level. Missing recommended attributes are not checked while their code remains at its `validation.LevelIgnored` default; configure `validation.AttributeRecommendedMissing` as warning or error to enable that validation. Use `WithValidationObservablePathNotation` to report when a valid observable name does not use the preferred notation; this preference never prevents resolution of another supported notation.

Validation policy is immutable pipeline configuration. `WithValidationLevel(code, level)` sets one code to `validation.LevelIgnored`, `validation.LevelWarning`, or `validation.LevelError`; `WithAllValidationLevels(level)` sets the level for every code. The `all` setting may appear once and must precede specific settings; each specific code may appear once. Every validation code may be ignored, including class-resolution findings; mandatory processing issues independently report class-resolution failures. `NewPipeline` rejects `WithValidation` configurations whose resolved policy ignores every validation code.

```go
pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithValidation(
		eventpipeline.WithAllValidationLevels(validation.LevelError),
		eventpipeline.WithValidationLevel(validation.AttributeUnknown, validation.LevelIgnored),
	),
)
```

Enrichment and enrichment removal report processing conditions according to immutable pipeline issue policy. `WithIssueLevel(code, level)` sets one code to `issue.LevelIgnored`, `issue.LevelWarning`, or `issue.LevelError`; `WithAllIssueLevels(level)` sets the level for every issue. The `all` setting may appear once and must precede specific settings; each specific code may appear once. Mandatory diagnostics reporting class-resolution failure or limited event traversal cannot be ignored. An error-level issue stops processing and returns an `eventpipeline.ProcessingIssueError`; ignored and warning-level issues do not change the mutation that led to the condition.

```go
pipeline, err := eventpipeline.NewPipeline(
	eventpipeline.WithSchema(schema),
	eventpipeline.WithEnumSiblings(enrichment.Add),
	eventpipeline.WithObservables(enrichment.Add),
	eventpipeline.WithIssueLevel(issue.EnrichmentObservableDuplicateSkipped, issue.LevelIgnored),
)
```

### Result Model

`eventpipeline.ProcessingResult` is an opaque concrete value with typed accessors for processor-specific results and warning-level processing issues:

```go
result.Validation()
result.Enrichment()
result.EnrichmentRemoval()
result.Issues()
```

The private value representation lets future toolkit releases add processor families through new accessor methods without changing the public structure. Compiler diagnostics confirm that the supported Go toolchain inlines every simple accessor across package boundaries, reducing calls to direct private-state access and making the abstraction zero-cost without interface boxing or an inherently necessary allocation. The zero value is a valid empty result. `ProcessingResult` preserves its processor-section JSON representation when marshaled and can be unmarshaled from the same representation. The processor-specific structs in `eventresult` remain field-oriented and support keyed literals, but intentionally reject positional literals so future releases can add fields compatibly.

Validation findings have a severity-neutral stable `validation.Code`, an explicit effective `validation.Level`, a human-readable message, and code-specific structured details. Codes use the `validation_` prefix. `Code.Description()` returns a short description and `Code.DefaultLevel()` reports the toolkit's default level independently of the effective level recorded on a finding. Findings remain in reporting order in one slice:

```go
result.Validation().Findings
result.Validation().Count(validation.LevelError)
result.Validation().Count(validation.LevelWarning)
```

Enrichment counters report what was added:

```go
result.Enrichment().EnumSiblingsAdded
result.Enrichment().ObservablesAdded
```

Enrichment-removal counters report what was removed or retained:

```go
result.EnrichmentRemoval().EnumSiblingsRemoved
result.EnrichmentRemoval().EnumSiblingsRetained
result.EnrichmentRemoval().ObservablesRemoved
result.EnrichmentRemoval().ObservablesRetained
```

`ProcessingResult.Issues()` returns warning-level processing diagnostics with a typed `issue.Source` identifying the broad part of processing that reported the issue and a stable `issue.Code` whose string begins with `issue_` identifying the precise condition. They are separate from OCSF validation findings and include enrichment and enrichment-removal problems as well as shared processing limitations, so an issue can be reported even by a validation-only pipeline. Ignored issues are omitted without a count. An error-level issue makes `ProcessEvent` return a zero result and an `*eventpipeline.ProcessingIssueError`; callers can use `errors.As` to recover its structured `eventresult.ProcessingIssue`. Validation findings appear only in the `Findings` slice returned by `Validation()`.

For a complete working example of library usage, see the CLI implementation in `cmd/ocsf-toolkit`.

## Development

Local development, the separate development-tools module, and complete verification use a security-patched Go 1.27 toolchain, matching the primary Linux CI build and the toolchain used to produce release binaries. The main library and CLI module remains compatible with Go 1.25.13. A dedicated CI job runs `go test ./...` with the latest Go 1.25 patch release and `GOTOOLCHAIN=local`, which prevents automatic switching to a newer toolchain from concealing an unintended dependency on a newer language or standard library. Go 1.25 alone cannot run `make`, `make check-all`, or `make all`. A local checkout also requires `golangci-lint`, `govulncheck`, and `goimports`. Install each with Homebrew:

```sh
brew install golangci-lint govulncheck goimports
```

or with `go install`:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
go install golang.org/x/tools/cmd/goimports@latest
```

Run the complete checks, tests, and cross-platform build before submitting changes:

```sh
make all
```

For ordinary local development, run `make` or `make dev`. This runs the standard checks, tests with the current Go toolchain and JSON v2, and builds the CLI for the local platform. The development binary is written to:

```sh
build/ocsf-toolkit
```

`make all` additionally checks import formatting, runs compatibility tests with Go 1.27 and the minimum supported Go 1.25 toolchain using both JSON implementations, runs race and coverage tests, tests release-tag selection, benchmark-comparison arguments, and safe Make-variable handling, and builds every supported target platform. Linux CI runs this complete verification with Go 1.27 on every pull request and push. A separate minimum-version job runs the main module's tests once with Go 1.25 and automatic toolchain switching disabled. CI also compares benchmarks using the latest Go toolchain and runs native tests on macOS and Windows.

See the `Makefile` for individual targets when you need to run one step directly.

Run `make check-all` for the complete set of static checks without tests or builds. It includes module tidiness, `gofmt`, `golangci-lint`, `govulncheck`, `go vet`, and import formatting.

Run `make lint-audit` periodically for the normal golangci-lint checks plus exhaustive enum-switch, cognitive-complexity, and maintainability analysis. This intentionally judgment-based audit is not part of `check`, `check-all`, or CI; some findings identify deliberate classifiers or complexity inherent in the problem and do not warrant source-level suppression markers.

The test targets can also be run independently: `make test` uses the current Go toolchain with JSON v2, `make test-compatibility` runs all current/minimum-toolchain and default/JSON-v2 combinations, `make test-coverage` runs the current toolchain and default JSON implementation with coverage, and `make test-all` combines compatibility, coverage, race, release-tag-selection, and Make-variable-safety tests.

Run `make build-all-platforms` to rebuild every supported platform under `build/`. The build script owns the supported-platform list and produces one self-contained directory per platform. `make package VERSION=vX.Y.Z` first runs `make all`, then packages each platform directory as-is with the common license and documentation files and writes the archives and `SHA256SUMS` to `dist/`.

Go code should keep line lengths within 120 columns (tabs counted as 4) where a natural break exists (a sentence boundary, a semicolon-separated clause, a regex's logical groups), but longer lines are fine when breaking them would produce non-idiomatic Go.

Run the event-processing benchmark suite with:

```sh
go test ./eventpipeline -run '^$' -bench '.' -benchmem -benchtime 500ms -count 10
```

Compare the current checkout with the newest reachable release tag using the same `v` followed by a digit and no `+` sanity checks as the release workflow:

```sh
scripts/benchmark-compare.sh
```

Prerelease tags participate in this baseline. Override the selected tag when necessary:

```sh
scripts/benchmark-compare.sh --base v0.8.0
```

Use `--pattern REGEXP` to focus the suite and `--count N` or `--time DURATION` when the defaults do not provide enough statistical confidence. Run `scripts/benchmark-compare.sh --help` for the complete argument summary.

The pull-request workflow uses five 250 ms samples for a lightweight comparison. Use the script's default ten 500 ms samples for deliberate local regression analysis, increasing either setting when the results need more statistical confidence.

The comparison runs both revisions on the same machine and reports statistically evaluated runtime, bytes, and allocation differences through `benchstat`. The event-processing suite includes numeric enum coverage for integer-spelled and integral-float-spelled `json.Number` values, `int64`, and `float64`. A benchmark introduced after the selected release appears only in the current column until a later release contains the same benchmark.

Project design and maintenance documentation:

- [Frequently asked questions](docs/FAQ.md)
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
branch=v1.9.0
git clone --single-branch --branch "$branch" https://github.com/ocsf/ocsf-schema.git "ocsf-schema-$branch"
```

Then compile it:

```sh
ocsf-schema-compiler ocsf-schema-v1.9.0 > ocsf-schema-v1.9.0.json
```

Use the generated JSON file as the schema input for both the library and CLI.
