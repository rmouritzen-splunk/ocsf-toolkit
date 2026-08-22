package eventschema

import (
	"io"
	"io/fs"

	"github.com/ocsf/ocsf-toolkit/internal/processing"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/schemaresult"
)

// Schema is a loaded compiled OCSF schema.
//
// Schema values must be created with Load, LoadBytes, LoadReader, or LoadFS and are safe for concurrent use.
type Schema struct {
	// Keep the compiled schema and processing caches behind an internal handle so their representations can
	// evolve without expanding the public API. This pointer is used during pipeline construction, not event processing.
	pipelineFactory *processing.PipelineFactory
}

// Load loads a compiled OCSF schema from path.
//
// The file must be in the compiled schema format produced by the OCSF Schema Compiler. Load returns nonfatal
// initialization issues when the schema can be loaded but contains forms the toolkit cannot fully process.
func Load(path string) (*Schema, []schemaresult.InitializationIssue, error) {
	compiled, issues, err := schema.Load(path)
	if err != nil {
		return nil, nil, err
	}
	return newSchema(compiled), issues, nil
}

// LoadReader loads a compiled OCSF schema from reader.
//
// The reader must be in the compiled schema format produced by the OCSF Schema Compiler. LoadReader returns nonfatal
// initialization issues when the schema can be loaded but contains forms the toolkit cannot fully process.
// LoadReader reads through EOF and does not close reader. When the schema is already available
// as a byte slice, use LoadBytes to avoid an additional input-sized buffer.
// LoadReader is generally less efficient than Load, LoadFS, and LoadBytes; prefer those functions when the source is
// already available in one of their native forms.
func LoadReader(reader io.Reader) (*Schema, []schemaresult.InitializationIssue, error) {
	compiled, issues, err := schema.LoadReader(reader)
	if err != nil {
		return nil, nil, err
	}
	return newSchema(compiled), issues, nil
}

// LoadBytes loads a compiled OCSF schema from data.
//
// The data must be in the compiled schema format produced by the OCSF Schema Compiler. LoadBytes returns nonfatal
// initialization issues when the schema can be loaded but contains forms the toolkit cannot fully process.
// LoadBytes avoids the additional input-sized buffer required by LoadReader, making it useful for
// embedded schemas and small schemas used in unit tests. It does not retain data after returning.
func LoadBytes(data []byte) (*Schema, []schemaresult.InitializationIssue, error) {
	compiled, issues, err := schema.LoadBytes(data)
	if err != nil {
		return nil, nil, err
	}
	return newSchema(compiled), issues, nil
}

// LoadFS loads a compiled OCSF schema from path in fsys.
//
// The file must be in the compiled schema format produced by the OCSF Schema Compiler. LoadFS returns nonfatal
// initialization issues when the schema can be loaded but contains forms the toolkit cannot fully process. Path must
// satisfy the requirements of fs.ValidPath.
func LoadFS(fsys fs.FS, path string) (*Schema, []schemaresult.InitializationIssue, error) {
	compiled, issues, err := schema.LoadFS(fsys, path)
	if err != nil {
		return nil, nil, err
	}
	return newSchema(compiled), issues, nil
}

func newSchema(compiled *schema.Compiled) *Schema {
	return &Schema{pipelineFactory: processing.NewPipelineFactory(compiled)}
}
