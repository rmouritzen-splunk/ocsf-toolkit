package jsonio

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/ocsf/ocsf-processor/jsonish"
)

func ReadObject(path string) (jsonish.Map, error) {
	var err error
	var f *os.File
	if f, err = os.Open(path); err != nil {
		return nil, fmt.Errorf("failed to open JSON object file %q: %w", path, err)
	}
	defer func(f fs.File) { _ = f.Close() }(f)
	var m jsonish.Map
	if m, err = DecodeObject(f); err != nil {
		return nil, fmt.Errorf("failed to decode JSON object file %q: %w", path, err)
	} else {
		return m, f.Close()
	}
}

func ReadObjectFS(dirFS fs.FS, path string) (jsonish.Map, error) {
	var f fs.File
	var err error
	if f, err = dirFS.Open(path); err != nil {
		return nil, fmt.Errorf("failed to open JSON object file %q: %w", path, err)
	}
	defer func(f fs.File) { _ = f.Close() }(f)
	var m jsonish.Map
	if m, err = DecodeObject(f); err != nil {
		return nil, fmt.Errorf("failed to decode JSON object file %q: %w", path, err)
	} else {
		return m, f.Close()
	}
}

func ReadArrayOfObjects(path string) ([]jsonish.Map, error) {
	var err error
	var f *os.File
	if f, err = os.Open(path); err != nil {
		return nil, fmt.Errorf("failed to open JSON array of objects file %q: %w", path, err)
	}
	defer func(f fs.File) { _ = f.Close() }(f)
	var a []jsonish.Map
	if a, err = DecodeArrayOfObjects(f); err != nil {
		return nil, fmt.Errorf("failed to decode JSON array of objects file %q: %w", path, err)
	} else {
		return a, f.Close()
	}
}

func ReadArrayOfObjectsFS(dirFS fs.FS, path string) ([]jsonish.Map, error) {
	var f fs.File
	var err error
	if f, err = dirFS.Open(path); err != nil {
		return nil, fmt.Errorf("failed to open JSON array of objects file %q: %w", path, err)
	}
	defer func(f fs.File) { _ = f.Close() }(f)
	var a []jsonish.Map
	if a, err = DecodeArrayOfObjects(f); err != nil {
		return nil, fmt.Errorf("failed to decode JSON array of objects file %q: %w", path, err)
	} else {
		return a, f.Close()
	}
}

func DecodeObject(r io.Reader) (jsonish.Map, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	object := jsonish.Map{}
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("failed to decode JSON object: %w", err)
	}
	return object, nil
}

func DecodeArrayOfObjects(r io.Reader) ([]jsonish.Map, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	var objects []jsonish.Map
	if err := decoder.Decode(&objects); err != nil {
		return nil, fmt.Errorf("failed to decode JSON array of objects: %w", err)
	}
	return objects, nil
}
