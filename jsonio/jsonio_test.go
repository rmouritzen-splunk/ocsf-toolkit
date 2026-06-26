package jsonio

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeObjectRejectsTrailingJSONValue(t *testing.T) {
	assert := require.New(t)

	object, err := DecodeObject(strings.NewReader(`{"class_uid": 1} {"extra": true}`))

	assert.Nil(object)
	assert.ErrorContains(err, "unexpected trailing JSON value")
}

func TestDecodeArrayOfObjectsRejectsTrailingJSONValue(t *testing.T) {
	assert := require.New(t)

	objects, err := DecodeArrayOfObjects(strings.NewReader(`[{"class_uid": 1}] {"extra": true}`))

	assert.Nil(objects)
	assert.ErrorContains(err, "unexpected trailing JSON value")
}

func TestDecodeObjectPreservesNumbers(t *testing.T) {
	assert := require.New(t)

	object, err := DecodeObject(strings.NewReader(`{"class_uid": 1}`))

	assert.NoError(err)
	assert.Equal(json.Number("1"), object["class_uid"])
}
