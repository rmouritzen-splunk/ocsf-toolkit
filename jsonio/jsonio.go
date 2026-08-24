package jsonio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/ocsf/ocsf-toolkit/internal/fserror"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// NewDecoder returns a JSON decoder that preserves numbers as json.Number values.
func NewDecoder(r io.Reader) *json.Decoder {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	return decoder
}

// ReadObject reads a JSON object file from path.
//
// Numbers are decoded as json.Number values.
func ReadObject(path string) (jsonish.Map, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON object file %q: %w", path, fserror.QuotePaths(err))
	}
	defer func() { _ = f.Close() }()
	m, err := DecodeObject(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON object file %q: %w", path, fserror.QuotePaths(err))
	}
	return m, nil
}

// ReadObjectFS reads a JSON object file from fsys at path.
//
// Numbers are decoded as json.Number values.
func ReadObjectFS(fsys fs.FS, path string) (jsonish.Map, error) {
	if fsys == nil {
		return nil, errors.New("failed to open JSON object file: filesystem is nil")
	}
	f, err := fsys.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON object file %q: %w", path, fserror.QuotePaths(err))
	}
	defer func() { _ = f.Close() }()
	m, err := DecodeObject(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON object file %q: %w", path, fserror.QuotePaths(err))
	}
	return m, nil
}

// DecodeObject decodes one non-null JSON object from r and rejects trailing JSON values.
//
// Numbers are decoded as json.Number values.
func DecodeObject(r io.Reader) (jsonish.Map, error) {
	decoder := NewDecoder(r)
	object := jsonish.Map{}
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("failed to decode JSON object: %w", fserror.QuotePaths(err))
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("failed to decode JSON object: %w", fserror.QuotePaths(err))
	}
	if object == nil {
		return nil, errors.New("unexpected JSON null; expected a JSON object")
	}
	return object, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}
