//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeOutputRelativePathRejectsUnsafeWindowsPaths(t *testing.T) {
	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "current volume rooted", path: `\tmp\event.json`, want: "event.json"},
		{name: "drive relative", path: `C:event.json`, want: "event.json"},
		{name: "reserved device name", path: `NUL`, want: stdinEventRelativePath},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.want, safeOutputRelativePath(testCase.path))
		})
	}
}

func TestValidateOutputNamespacesRejectsWindowsDirectoryJunction(t *testing.T) {
	outputDir := t.TempDir()
	eventsDir := filepath.Join(outputDir, eventsOutputDirectory)
	targetDir := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.Mkdir(eventsDir, 0o755))
	require.NoError(t, os.Mkdir(targetDir, 0o755))
	junctionPath := filepath.Join(eventsDir, "redirect")
	output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junctionPath, targetDir).CombinedOutput()
	require.NoError(t, err, string(output))
	outputRoot, err := newFilesystemPath(outputDir)
	require.NoError(t, err)

	err = validateOutputNamespaces(processConfig{addEnumSiblings: true}, outputRoot)

	require.ErrorContains(t, err, "unsupported filesystem entry")
}
