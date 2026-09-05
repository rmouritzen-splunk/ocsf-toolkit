package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/ocsf/ocsf-toolkit/internal/fserror"
	"github.com/ocsf/ocsf-toolkit/jsonio"
	"github.com/ocsf/ocsf-toolkit/schemaresult"
)

// Load decodes and indexes a compiled OCSF schema file.
func Load(name string) (*Compiled, []schemaresult.InitializationIssue, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open schema file %q: %w", name, fserror.QuotePaths(err))
	}
	return load(data, name)
}

// LoadBytes decodes and indexes a compiled OCSF schema from data.
func LoadBytes(data []byte) (*Compiled, []schemaresult.InitializationIssue, error) {
	return load(data, "")
}

// LoadFS decodes and indexes a compiled OCSF schema file from fsys.
func LoadFS(fsys fs.FS, name string) (*Compiled, []schemaresult.InitializationIssue, error) {
	if fsys == nil {
		return nil, nil, errors.New("failed to open schema file: filesystem is nil")
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open schema file %q: %w", name, fserror.QuotePaths(err))
	}
	return load(data, name)
}

// LoadReader decodes and indexes a compiled OCSF schema from reader.
func LoadReader(reader io.Reader) (*Compiled, []schemaresult.InitializationIssue, error) {
	if reader == nil {
		return nil, nil, errors.New("failed to read schema from reader: reader is nil")
	}
	definition, err := decodeReader(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode schema from reader: %w", fserror.QuotePaths(err))
	}
	return loadDefinition(definition, "")
}

// load decodes and compiles data, describing the source as name in error messages, or generically if name is
// empty (LoadReader and LoadBytes have no file name to report).
func load(data []byte, name string) (*Compiled, []schemaresult.InitializationIssue, error) {
	definition, err := decode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode schema%s: %w", namedSuffix(name), err)
	}
	return loadDefinition(definition, name)
}

func loadDefinition(
	definition *Definition,
	name string,
) (*Compiled, []schemaresult.InitializationIssue, error) {
	compiled, err := New(definition)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load schema%s: %w", namedSuffix(name), err)
	}
	return compiled, compiled.InitializationIssues(), nil
}

func namedSuffix(name string) string {
	if name == "" {
		return ""
	}
	return fmt.Sprintf(" file %q", name)
}

func decode(data []byte) (*Definition, error) {
	var definition Definition
	if err := json.Unmarshal(data, &definition); err != nil {
		return nil, err
	}
	return &definition, nil
}

func decodeReader(reader io.Reader) (*Definition, error) {
	var definition Definition
	decoder := jsonio.NewDecoder(reader)
	if err := decoder.Decode(&definition); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return &definition, nil
	} else if err != nil {
		return nil, err
	}
	return nil, errors.New("unexpected trailing JSON value")
}
