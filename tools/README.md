# Development tools

This directory is a separate Go module for development tools that should not become dependencies of the `ocsf-toolkit` library or CLI.

The module currently pins `golang.org/x/perf/cmd/benchstat`. The `scripts/benchmark-compare.sh` script runs it with `go tool benchstat`.

The Go `tool` directive records the executable in `tools/go.mod`, while ordinary module requirements and `tools/go.sum` pin its dependencies. This provides a reproducible tool version without requiring a globally installed executable.

Use `make gotidy-check` to verify the tool module and `make gotidy` after changing its tools or dependencies.
