package fserror

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotePathsPathError(t *testing.T) {
	original := &fs.PathError{Op: "open", Path: "missing\n\"event.json", Err: fs.ErrNotExist}
	err := QuotePaths(original)

	assert.Equal(t, "open \"missing\\n\\\"event.json\": file does not exist", err.Error())
	assert.False(t, strings.ContainsRune(err.Error(), '\n'))
	assert.ErrorIs(t, err, fs.ErrNotExist)
	var pathError *fs.PathError
	require.ErrorAs(t, err, &pathError)
	assert.Same(t, original, pathError)
}

func TestQuotePathsLinkError(t *testing.T) {
	original := &os.LinkError{Op: "rename", Old: "old\nfile", New: "new\tfile", Err: fs.ErrPermission}
	err := QuotePaths(original)

	assert.Equal(t, "rename \"old\\nfile\" \"new\\tfile\": permission denied", err.Error())
	assert.False(t, strings.ContainsAny(err.Error(), "\n\t"))
	assert.ErrorIs(t, err, fs.ErrPermission)
	var linkError *os.LinkError
	require.ErrorAs(t, err, &linkError)
	assert.Same(t, original, linkError)
}

func TestQuotePathsLeavesOtherErrorsUnchanged(t *testing.T) {
	original := errors.New("ordinary error")
	assert.Same(t, original, QuotePaths(original))
	assert.NoError(t, QuotePaths(nil))
}
