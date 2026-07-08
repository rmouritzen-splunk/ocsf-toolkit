//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateOutputNamespacesRejectsSpecialFilesystemEntries(t *testing.T) {
	outputDir := t.TempDir()
	eventsDir := filepath.Join(outputDir, eventsOutputDirectory)
	require.NoError(t, os.Mkdir(eventsDir, 0o755))
	require.NoError(t, syscall.Mkfifo(filepath.Join(eventsDir, "redirect"), 0o600))
	outputRoot, err := newFilesystemPath(outputDir)
	require.NoError(t, err)

	err = validateOutputNamespaces(processConfig{addEnumSiblings: true}, outputRoot)

	require.ErrorContains(t, err, "unsupported filesystem entry")
}
