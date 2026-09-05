package eventpipeline

import (
	"io"
	"io/fs"

	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/schemaresult"
)

// Schema is a loaded compiled OCSF schema.
//
// Schema values must be created with NewSchema, NewSchemaFromBytes, NewSchemaFromReader, or NewSchemaFromFS and are
// safe for concurrent use.
type Schema struct {
	// Keep the compiled schema private so its representation can evolve without expanding the public API. This
	// pointer is used during pipeline construction; each constructed internal pipeline retains the same schema.
	compiled *schema.Compiled
}

// NewSchema creates a Schema from the compiled OCSF schema at path.
//
// The file must be in the compiled schema format produced by the OCSF Schema Compiler. NewSchema returns nonfatal
// initialization issues when the schema can be loaded but contains forms the toolkit cannot fully process.
func NewSchema(path string) (*Schema, []schemaresult.InitializationIssue, error) {
	compiled, issues, err := schema.Load(path)
	if err != nil {
		return nil, nil, err
	}
	return newSchema(compiled), issues, nil
}

// NewSchemaFromBytes creates a Schema from a compiled OCSF schema in data.
//
// The data must be in the compiled schema format produced by the OCSF Schema Compiler. NewSchemaFromBytes returns
// nonfatal initialization issues when the schema can be loaded but contains forms the toolkit cannot fully process.
// NewSchemaFromBytes avoids the additional input-sized buffer required by NewSchemaFromReader, making it useful for
// embedded schemas and small schemas used in unit tests. It does not retain data after returning.
func NewSchemaFromBytes(data []byte) (*Schema, []schemaresult.InitializationIssue, error) {
	compiled, issues, err := schema.LoadBytes(data)
	if err != nil {
		return nil, nil, err
	}
	return newSchema(compiled), issues, nil
}

// NewSchemaFromFS creates a Schema from the compiled OCSF schema at path in fsys.
//
// The file must be in the compiled schema format produced by the OCSF Schema Compiler. NewSchemaFromFS returns
// nonfatal initialization issues when the schema can be loaded but contains forms the toolkit cannot fully process.
// Path must satisfy the requirements of fs.ValidPath.
func NewSchemaFromFS(fsys fs.FS, path string) (*Schema, []schemaresult.InitializationIssue, error) {
	compiled, issues, err := schema.LoadFS(fsys, path)
	if err != nil {
		return nil, nil, err
	}
	return newSchema(compiled), issues, nil
}

// NewSchemaFromReader creates a Schema by reading a compiled OCSF schema from reader.
//
// The reader must be in the compiled schema format produced by the OCSF Schema Compiler. NewSchemaFromReader returns
// nonfatal initialization issues when the schema can be loaded but contains forms the toolkit cannot fully process.
// NewSchemaFromReader reads through EOF and does not close reader. When the schema is already available as a byte
// slice, use NewSchemaFromBytes to avoid an additional input-sized buffer. NewSchemaFromReader is generally less
// efficient than NewSchema, NewSchemaFromFS, and NewSchemaFromBytes; prefer those functions when the source is
// already available in one of their native forms.
func NewSchemaFromReader(reader io.Reader) (*Schema, []schemaresult.InitializationIssue, error) {
	compiled, issues, err := schema.LoadReader(reader)
	if err != nil {
		return nil, nil, err
	}
	return newSchema(compiled), issues, nil
}

func newSchema(compiled *schema.Compiled) *Schema {
	return &Schema{compiled: compiled}
}
