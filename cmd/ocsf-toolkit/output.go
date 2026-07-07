package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type writeOptions struct {
	overwrite  bool
	prettyJSON bool
}

func writeJSON(w io.Writer, value any, pretty bool) error {
	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}
func writeJSONAtomic(path string, value any, options writeOptions) error {
	return writeFileAtomic(path, options.overwrite, func(w io.Writer) error {
		return writeJSON(w, value, options.prettyJSON)
	})
}

func writeJSONDestination(path string, value any, options writeOptions, stdout io.Writer) error {
	if path == stdioPath {
		return writeJSON(stdout, value, options.prettyJSON)
	}
	return writeJSONAtomic(path, value, options)
}

func writeTextAtomic(path string, text string, overwrite bool) error {
	return writeFileAtomic(path, overwrite, func(w io.Writer) error {
		if _, err := io.WriteString(w, text); err != nil {
			return fmt.Errorf("failed to write text: %w", err)
		}
		return nil
	})
}

func writeTextDestination(path string, text string, overwrite bool, stdout io.Writer) error {
	if path == stdioPath {
		if _, err := io.WriteString(stdout, text); err != nil {
			return fmt.Errorf("failed to write text to stdout: %w", err)
		}
		return nil
	}
	return writeTextAtomic(path, text, overwrite)
}

func writeFileAtomic(path string, overwrite bool, write func(io.Writer) error) error {
	if path == "" {
		return errors.New("output path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %q: %w", dir, err)
	}
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary output file for %q: %w", path, err)
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
		return fmt.Errorf("failed to write temporary output file %q: %w", tempPath, err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("failed to sync temporary output file %q: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary output file %q: %w", tempPath, err)
	}
	if overwrite {
		if err := os.Rename(tempPath, path); err != nil {
			return fmt.Errorf("failed to replace %q with temporary output file %q: %w", path, tempPath, err)
		}
	} else if err := os.Link(tempPath, path); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("output file %q already exists (use --overwrite to replace it)", path)
		}
		return fmt.Errorf("failed to create %q from temporary output file %q: %w", path, tempPath, err)
	} else {
		return nil
	}
	removeTemp = false
	return nil
}
