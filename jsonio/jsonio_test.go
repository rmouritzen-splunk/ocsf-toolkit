package jsonio

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeObjectRejectsTrailingJSONValue(t *testing.T) {
	assert := require.New(t)

	object, err := DecodeObject(strings.NewReader(`{"class_uid": 1} {"extra": true}`))

	assert.Nil(object)
	assert.ErrorContains(err, "unexpected trailing JSON value")
}

func TestDecodeObjectRejectsNull(t *testing.T) {
	object, err := DecodeObject(strings.NewReader(`null`))

	require.Nil(t, object)
	require.ErrorContains(t, err, "unexpected JSON null")
}

func TestDecodeObjectPreservesNumbers(t *testing.T) {
	assert := require.New(t)

	object, err := DecodeObject(strings.NewReader(`{"class_uid": 1}`))

	assert.NoError(err)
	assert.Equal(json.Number("1"), object["class_uid"])
}

func TestNewDecoderPreservesNumbers(t *testing.T) {
	assert := require.New(t)
	var value any

	decoder := NewDecoder(strings.NewReader(`9007199254740993`))
	err := decoder.Decode(&value)

	assert.NoError(err)
	assert.Equal(json.Number("9007199254740993"), value)
}

func TestReadObject(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "event.json")
	assert.NoError(os.WriteFile(path, []byte(`{"class_uid":1}`), 0o600))

	object, err := ReadObject(path)

	assert.NoError(err)
	assert.Equal(json.Number("1"), object["class_uid"])

	_, err = ReadObject(filepath.Join(dir, "missing.json"))
	assert.ErrorContains(err, "failed to open JSON object file")
}

func TestReadObjectQuotesFilesystemErrorPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing event.json")
	_, err := ReadObject(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), strconv.Quote(path))
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestReadObjectRejectsNullWithPath(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "event.json")
	assert.NoError(os.WriteFile(path, []byte(`null`), 0o600))

	object, err := ReadObject(path)

	assert.Nil(object)
	assert.ErrorContains(err, fmt.Sprintf("failed to decode JSON object file %q", path))
	assert.ErrorContains(err, "unexpected JSON null")
}

func TestReadObjectFS(t *testing.T) {
	assert := require.New(t)
	files := fstest.MapFS{
		"event.json": &fstest.MapFile{Data: []byte(`{"class_uid":1}`)},
		"bad.json":   &fstest.MapFile{Data: []byte(`{} {}`)},
	}

	object, err := ReadObjectFS(files, "event.json")

	assert.NoError(err)
	assert.Equal(json.Number("1"), object["class_uid"])

	_, err = ReadObjectFS(files, "bad.json")
	assert.ErrorContains(err, `failed to decode JSON object file "bad.json"`)
}

func TestReadObjectFSRejectsNilFilesystem(t *testing.T) {
	object, err := ReadObjectFS(nil, "event.json")

	require.Nil(t, object)
	require.EqualError(t, err, "failed to open JSON object file: filesystem is nil")
}
