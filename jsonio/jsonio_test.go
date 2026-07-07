package jsonio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

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

func TestDecodeArrayOfObjectsRejectsTrailingJSONValue(t *testing.T) {
	assert := require.New(t)

	objects, err := DecodeArrayOfObjects(strings.NewReader(`[{"class_uid": 1}] {"extra": true}`))

	assert.Nil(objects)
	assert.ErrorContains(err, "unexpected trailing JSON value")
}

func TestDecodeArrayOfObjectsRejectsNull(t *testing.T) {
	objects, err := DecodeArrayOfObjects(strings.NewReader(`null`))

	require.Nil(t, objects)
	require.ErrorContains(t, err, "unexpected JSON null")
}

func TestDecodeArrayOfObjectsRejectsNullElement(t *testing.T) {
	objects, err := DecodeArrayOfObjects(strings.NewReader(`[{} , null]`))

	require.Nil(t, objects)
	require.ErrorContains(t, err, "element 1 is JSON null")
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
	assert.NoError(os.WriteFile(path, []byte(`{"class_uid":1}`), 0o644))

	object, err := ReadObject(path)

	assert.NoError(err)
	assert.Equal(json.Number("1"), object["class_uid"])

	_, err = ReadObject(filepath.Join(dir, "missing.json"))
	assert.ErrorContains(err, "failed to open JSON object file")
}

func TestReadObjectRejectsNullWithPath(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "event.json")
	assert.NoError(os.WriteFile(path, []byte(`null`), 0o644))

	object, err := ReadObject(path)

	assert.Nil(object)
	assert.ErrorContains(err, `failed to decode JSON object file "`+path+`"`)
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

func TestReadArrayOfObjects(t *testing.T) {
	assert := require.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "events.json")
	assert.NoError(os.WriteFile(path, []byte(`[{"class_uid":1}]`), 0o644))

	objects, err := ReadArrayOfObjects(path)

	assert.NoError(err)
	assert.Len(objects, 1)
	assert.Equal(json.Number("1"), objects[0]["class_uid"])

	_, err = ReadArrayOfObjects(filepath.Join(dir, "missing.json"))
	assert.ErrorContains(err, "failed to open JSON array of objects file")
}

func TestReadArrayOfObjectsFS(t *testing.T) {
	assert := require.New(t)
	files := fstest.MapFS{
		"events.json": &fstest.MapFile{Data: []byte(`[{"class_uid":1}]`)},
		"bad.json":    &fstest.MapFile{Data: []byte(`[] []`)},
	}

	objects, err := ReadArrayOfObjectsFS(files, "events.json")

	assert.NoError(err)
	assert.Len(objects, 1)
	assert.Equal(json.Number("1"), objects[0]["class_uid"])

	_, err = ReadArrayOfObjectsFS(files, "bad.json")
	assert.ErrorContains(err, `failed to decode JSON array of objects file "bad.json"`)
}
