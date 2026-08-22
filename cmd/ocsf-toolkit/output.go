package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ocsf/ocsf-toolkit/internal/fserror"
)

const temporaryOutputPattern = ".ocsf-toolkit-output-*"

type writeOptions struct {
	overwrite  bool
	prettyJSON bool
}

var createOutputHardLink = os.Link

type destinationWriter struct {
	stdout  io.Writer
	options writeOptions
}

func newDestinationWriter(stdout io.Writer, options writeOptions) *destinationWriter {
	return &destinationWriter{stdout: stdout, options: options}
}

func (writer *destinationWriter) writeJSON(path string, value any) error {
	return writeJSONDestination(path, value, writer.options, writer.stdout)
}

func (writer *destinationWriter) writeText(path, value string) error {
	return writeTextDestination(path, value, writer.options.overwrite, writer.stdout)
}

func writeJSON(w io.Writer, value any, pretty bool) error {
	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", fserror.QuotePaths(err))
	}
	return nil
}

func writeJSONDestination(path string, value any, options writeOptions, stdout io.Writer) error {
	if path == stdioPath {
		return writeJSON(stdout, value, options.prettyJSON)
	}
	return writeOutputFile(path, options.overwrite, func(w io.Writer) error {
		return writeJSON(w, value, options.prettyJSON)
	})
}

func writeTextOutputFile(path string, text string, overwrite bool) error {
	return writeOutputFile(path, overwrite, func(w io.Writer) error {
		if _, err := io.WriteString(w, text); err != nil {
			return fmt.Errorf("failed to write text: %w", fserror.QuotePaths(err))
		}
		return nil
	})
}

func writeTextDestination(path string, text string, overwrite bool, stdout io.Writer) error {
	if path == stdioPath {
		if _, err := io.WriteString(stdout, text); err != nil {
			return fmt.Errorf("failed to write text to stdout: %w", fserror.QuotePaths(err))
		}
		return nil
	}
	return writeTextOutputFile(path, text, overwrite)
}

// writeOutputFile stages output in a temporary file and installs it at path only once writing succeeds, so an
// ordinary encoding or write failure does not leave a partial file at the real destination. Installation atomicity
// depends on operating-system and filesystem semantics; staging minimizes the exposure window when an atomic rename
// or hard link is unavailable. This is the fail-fast half of the CLI's collision handling:
// resolveDistinctDestination in layout.go rejects known collisions up front, and
// this function assumes that check already passed. It does not re-verify path's identity, retry, or reconcile with
// whatever else may be touching the filesystem; if a collision or error still occurs here (e.g. the exclusive-create
// fallback in installTemporaryFileWithoutOverwrite finds path now exists), it is reported as an ordinary error like
// any other filesystem failure. See the preflight comment on resolveDistinctDestination and "CLI Boundary" in
// docs/architecture.md.
func writeOutputFile(path string, overwrite bool, write func(io.Writer) error) error {
	if path == "" {
		return errors.New("output path is empty")
	}
	dir := filepath.Dir(path)
	//nolint:gosec // G301: CLI output directories use conventional searchable permissions.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %q: %w", dir, fserror.QuotePaths(err))
	}
	tempFile, err := os.CreateTemp(dir, temporaryOutputPattern)
	if err != nil {
		return fmt.Errorf("failed to create temporary output file for %q: %w", path, fserror.QuotePaths(err))
	}
	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := write(tempFile); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to write temporary output file %q: %w", tempPath, fserror.QuotePaths(err))
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to sync temporary output file %q: %w", tempPath, fserror.QuotePaths(err))
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary output file %q: %w", tempPath, fserror.QuotePaths(err))
	}
	if overwrite {
		info, err := os.Stat(path)
		if err == nil {
			if err := os.Chmod(tempPath, info.Mode().Perm()); err != nil {
				return fmt.Errorf(
					"failed to preserve permissions for output file %q: %w", path, fserror.QuotePaths(err),
				)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect permissions for output file %q: %w", path, fserror.QuotePaths(err))
		}
		if err := os.Rename(tempPath, path); err != nil {
			return fmt.Errorf(
				"failed to replace %q with temporary output file %q: %w", path, tempPath, fserror.QuotePaths(err),
			)
		}
	} else {
		return installTemporaryFileWithoutOverwrite(tempPath, path)
	}
	removeTemp = false
	return nil
}

// installTemporaryFileWithoutOverwrite uses a hard link when the filesystem supports it, which
// makes the completed temporary file visible at its destination in one operation. The exclusive-
// create fallback preserves the no-overwrite guarantee on filesystems without hard-link support.
func installTemporaryFileWithoutOverwrite(tempPath, path string) error {
	linkErr := createOutputHardLink(tempPath, path)
	if linkErr == nil {
		return nil
	}
	if os.IsExist(linkErr) {
		return outputAlreadyExistsError(path)
	}

	source, err := os.Open(tempPath)
	if err != nil {
		return fmt.Errorf("failed to reopen temporary output file %q: %w", tempPath, fserror.QuotePaths(err))
	}
	defer func() { _ = source.Close() }()

	destination, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return outputAlreadyExistsError(path)
		}
		return fmt.Errorf("failed to create output file %q using exclusive fallback: %w", path, fserror.QuotePaths(err))
	}
	installed := false
	defer func() {
		_ = destination.Close()
		if !installed {
			_ = os.Remove(path)
		}
	}()

	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf("failed to copy temporary output file %q to %q: %w", tempPath, path, fserror.QuotePaths(err))
	}
	if err := destination.Sync(); err != nil {
		return fmt.Errorf("failed to sync output file %q: %w", path, fserror.QuotePaths(err))
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("failed to close output file %q: %w", path, fserror.QuotePaths(err))
	}
	installed = true
	return nil
}

func outputAlreadyExistsError(path string) error {
	return fmt.Errorf("output file %q already exists (use --overwrite to replace it)", path)
}
