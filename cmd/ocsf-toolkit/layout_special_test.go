//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/stretchr/testify/require"
)

func TestValidateOutputNamespacesRejectsSpecialFilesystemEntries(t *testing.T) {
	outputDir := t.TempDir()
	eventsDir := filepath.Join(outputDir, eventsOutputDirectory)
	require.NoError(t, os.Mkdir(eventsDir, 0o750))
	require.NoError(t, syscall.Mkfifo(filepath.Join(eventsDir, "redirect"), 0o600))
	outputRoot, err := newFilesystemPath(outputDir)
	require.NoError(t, err)

	err = validateOutputNamespaces(processConfig{enumSiblingsAction: enrichment.Add}, outputRoot)

	require.ErrorContains(t, err, "unsupported filesystem entry")
}

// Engineering invariant test: schema preflight must reject a FIFO without blocking while opening it for reading.
func TestEngineeringInvariantPreflightSchemaFileRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.json")
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	result := make(chan error, 1)

	go func() {
		result <- preflightSchemaFile(path)
	}()

	select {
	case err := <-result:
		require.ErrorContains(t, err, "must name a regular file")
	case <-time.After(time.Second):
		t.Fatal("schema preflight blocked while opening a FIFO")
	}
}
