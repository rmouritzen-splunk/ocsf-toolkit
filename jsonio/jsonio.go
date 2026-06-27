package jsonio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// ReadObject reads a JSON object file from path.
//
// Numbers are decoded as json.Number values.
func ReadObject(path string) (jsonish.Map, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON object file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	m, err := DecodeObject(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON object file %q: %w", path, err)
	}
	return m, nil
}

// ReadObjectFS reads a JSON object file from dirFS at path.
//
// Numbers are decoded as json.Number values.
func ReadObjectFS(dirFS fs.FS, path string) (jsonish.Map, error) {
	f, err := dirFS.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON object file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	m, err := DecodeObject(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON object file %q: %w", path, err)
	}
	return m, nil
}

// ReadArrayOfObjects reads a JSON file containing an array of objects from path.
//
// Numbers are decoded as json.Number values.
func ReadArrayOfObjects(path string) ([]jsonish.Map, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON array of objects file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	a, err := DecodeArrayOfObjects(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON array of objects file %q: %w", path, err)
	}
	return a, nil
}

// ReadArrayOfObjectsFS reads a JSON file containing an array of objects from dirFS at path.
//
// Numbers are decoded as json.Number values.
func ReadArrayOfObjectsFS(dirFS fs.FS, path string) ([]jsonish.Map, error) {
	f, err := dirFS.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON array of objects file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	a, err := DecodeArrayOfObjects(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON array of objects file %q: %w", path, err)
	}
	return a, nil
}

// DecodeObject decodes one JSON object from r and rejects trailing JSON values.
//
// Numbers are decoded as json.Number values.
func DecodeObject(r io.Reader) (jsonish.Map, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	object := jsonish.Map{}
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("failed to decode JSON object: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("failed to decode JSON object: %w", err)
	}
	return object, nil
}

// DecodeArrayOfObjects decodes one JSON array of objects from r and rejects trailing JSON values.
//
// Numbers are decoded as json.Number values.
func DecodeArrayOfObjects(r io.Reader) ([]jsonish.Map, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	var objects []jsonish.Map
	if err := decoder.Decode(&objects); err != nil {
		return nil, fmt.Errorf("failed to decode JSON array of objects: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("failed to decode JSON array of objects: %w", err)
	}
	return objects, nil
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
