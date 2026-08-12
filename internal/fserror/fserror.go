// Package fserror provides safe formatting for filesystem errors.
package fserror

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
)

// QuotePaths returns an error that quotes paths carried by standard filesystem errors.
// It preserves the original error chain for errors.Is and errors.As.
func QuotePaths(err error) error {
	if err == nil {
		return nil
	}
	var quoted quotedPathsError
	if errors.As(err, &quoted) {
		return err
	}
	var linkError *os.LinkError
	if errors.As(err, &linkError) {
		return quotedPathsError{err: err, linkError: linkError}
	}
	var pathError *fs.PathError
	if errors.As(err, &pathError) {
		return quotedPathsError{err: err, pathError: pathError}
	}
	return err
}

type quotedPathsError struct {
	err       error
	pathError *fs.PathError
	linkError *os.LinkError
}

func (e quotedPathsError) Error() string {
	if err := e.linkError; err != nil {
		return err.Op + " " + strconv.Quote(err.Old) + " " + strconv.Quote(err.New) + ": " +
			errorText(err.Err)
	}
	if err := e.pathError; err != nil {
		return err.Op + " " + strconv.Quote(err.Path) + ": " + errorText(err.Err)
	}
	return e.err.Error()
}

func errorText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return QuotePaths(err).Error()
}

func (e quotedPathsError) Unwrap() error {
	return e.err
}
