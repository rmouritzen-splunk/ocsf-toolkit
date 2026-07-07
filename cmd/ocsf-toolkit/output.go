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

type claimedOutput struct {
	path         string
	description  string
	fileIdentity []os.FileInfo
}

// destinationWriter keeps command outputs distinct even when different path spellings
// identify the same file according to the host filesystem.
type destinationWriter struct {
	stdout  io.Writer
	options writeOptions
	claimed []claimedOutput
}

func newDestinationWriter(stdout io.Writer, options writeOptions) *destinationWriter {
	return &destinationWriter{stdout: stdout, options: options}
}

func (writer *destinationWriter) writeJSON(path, description string, value any) error {
	if path == stdioPath {
		return writeJSON(writer.stdout, value, writer.options.prettyJSON)
	}
	claim, err := writer.claim(path, description)
	if err != nil {
		return err
	}
	if err := writeJSONDestination(path, value, writer.options, writer.stdout); err != nil {
		return err
	}
	return writer.recordWrittenIdentity(claim)
}

// writeDerivedJSON skips destination claiming for directory outputs whose distinct
// paths are guaranteed by their validated input-relative path and output namespace.
func (writer *destinationWriter) writeDerivedJSON(path string, value any) error {
	return writeJSONDestination(path, value, writer.options, writer.stdout)
}

func (writer *destinationWriter) writeText(path, description, value string) error {
	if path == stdioPath {
		if _, err := io.WriteString(writer.stdout, value); err != nil {
			return fmt.Errorf("failed to write text to stdout: %w", err)
		}
		return nil
	}
	claim, err := writer.claim(path, description)
	if err != nil {
		return err
	}
	if err := writeTextDestination(path, value, writer.options.overwrite, writer.stdout); err != nil {
		return err
	}
	return writer.recordWrittenIdentity(claim)
}

func (writer *destinationWriter) claim(path, description string) (*claimedOutput, error) {
	identity, err := inspectOutputFile(path)
	if err != nil {
		return nil, err
	}
	for _, previous := range writer.claimed {
		if sameAbsolutePath(previous.path, path) || sameFileIdentity(identity.info, previous.fileIdentity) {
			return nil, fmt.Errorf(
				"output path %q is selected for both %s and %s",
				path,
				previous.description,
				description,
			)
		}
	}
	claim := claimedOutput{path: path, description: description}
	if identity.exists {
		claim.fileIdentity = append(claim.fileIdentity, identity.info)
	}
	writer.claimed = append(writer.claimed, claim)
	return &writer.claimed[len(writer.claimed)-1], nil
}

func (writer *destinationWriter) recordWrittenIdentity(claim *claimedOutput) error {
	if claim == nil {
		return nil
	}
	identity, err := inspectOutputFile(claim.path)
	if err != nil {
		return err
	}
	if identity.exists && !sameFileIdentity(identity.info, claim.fileIdentity) {
		claim.fileIdentity = append(claim.fileIdentity, identity.info)
	}
	return nil
}

type inspectedOutputFile struct {
	info   os.FileInfo
	exists bool
}

func inspectOutputFile(path string) (inspectedOutputFile, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return inspectedOutputFile{}, nil
	}
	if err != nil {
		return inspectedOutputFile{}, fmt.Errorf("failed to inspect output path %q: %w", path, err)
	}
	return inspectedOutputFile{info: info, exists: true}, nil
}

func sameFileIdentity(candidate os.FileInfo, identities []os.FileInfo) bool {
	if candidate == nil {
		return false
	}
	for _, identity := range identities {
		if os.SameFile(candidate, identity) {
			return true
		}
	}
	return false
}

func sameAbsolutePath(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbsolute) == filepath.Clean(rightAbsolute)
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
