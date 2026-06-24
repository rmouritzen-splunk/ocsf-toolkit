package jsonish

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetMap(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
		"d": nil,
	}
	a, err := GetMap(m, "a")
	assert.NoError(err)
	assert.NotNil(a)
	assert.IsType(Map{}, a)
	assert.Equal(Map{"x": 1, "y": 2, "z": 3}, a)

	b, err := GetMap(m, "b")
	assert.Nil(b)
	assert.Error(err)

	c, err := GetMap(m, "c")
	assert.Nil(c)
	assert.Error(err)

	d, err := GetMap(m, "d")
	assert.Nil(d)
	assert.Error(err)

	e, err := GetMap(m, "e")
	assert.Nil(e)
	assert.Error(err)
}

func TestGetMapNil(t *testing.T) {
	assert := require.New(t)
	n, err := GetMap(nil, "a")
	assert.Nil(n)
	assert.Error(err)
}

func TestGetMapBlankKeys(t *testing.T) {
	assert := require.New(t)

	n1, err := GetMap(nil, "")
	assert.Nil(n1)
	assert.Error(err)

	n2, err := GetMap(Map{}, "")
	assert.Nil(n2)
	assert.Error(err)

	blank, err := GetMap(Map{"": Map{"a": 1}}, "")
	assert.NoError(err)
	assert.NotNil(blank)
	assert.Equal(Map{"a": 1}, blank)
}

func TestGetOptionalMap(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
		"d": nil,
	}
	a, err := GetOptionalMap(m, "a")
	assert.NoError(err)
	assert.NotNil(a)
	assert.IsType(Map{}, a)
	assert.Equal(Map{"x": 1, "y": 2, "z": 3}, a)

	b, err := GetOptionalMap(m, "b")
	assert.Nil(b)
	assert.Error(err)

	c, err := GetOptionalMap(m, "c")
	assert.Nil(c)
	assert.Error(err)

	d, err := GetOptionalMap(m, "d")
	assert.Nil(d)
	assert.NoError(err)

	e, err := GetOptionalMap(m, "e")
	assert.Nil(e)
	assert.NoError(err)
}

func TestGetOptionalMapNil(t *testing.T) {
	assert := require.New(t)
	n, err := GetOptionalMap(nil, "a")
	assert.Nil(n)
	assert.Error(err)
}

func TestGetOptionalMapBlankKeys(t *testing.T) {
	assert := require.New(t)

	n1, err := GetOptionalMap(nil, "")
	assert.Nil(n1)
	assert.Error(err)

	n2, err := GetOptionalMap(Map{}, "")
	assert.Nil(n2)
	assert.NoError(err)

	blank, err := GetOptionalMap(Map{"": Map{"a": 1}}, "")
	assert.NoError(err)
	assert.NotNil(blank)
	assert.Equal(Map{"a": 1}, blank)
}

func TestGetSliceOfMaps(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": []Map{
			{"x": 1},
			{"y": 2},
			{"z": 3},
		},
		"b": Map{"x": "whatever"},
		"c": "whatever",
		"d": 42,
	}
	a, err := GetSliceOfMaps(m, "a")
	assert.NotNil(a)
	assert.NoError(err)
	assert.IsType([]Map{}, a)
	assert.Equal([]Map{{"x": 1}, {"y": 2}, {"z": 3}}, a)

	b, err := GetSliceOfMaps(m, "b")
	assert.Nil(b)
	assert.Error(err)

	c, err := GetSliceOfMaps(m, "c")
	assert.Nil(c)
	assert.Error(err)

	d, err := GetSliceOfMaps(m, "d")
	assert.Nil(d)
	assert.Error(err)

	e, err := GetSliceOfMaps(m, "e")
	assert.Nil(e)
	assert.Error(err)
}

func TestGetOptionalSliceOfMaps(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": []Map{
			{"x": 1},
			{"y": 2},
			{"z": 3},
		},
		"b": Map{"x": "whatever"},
		"c": "whatever",
		"d": 42,
		"e": nil,
	}
	a, err := GetOptionalSliceOfMaps(m, "a")
	assert.NotNil(a)
	assert.NoError(err)
	assert.IsType([]Map{}, a)
	assert.Equal([]Map{{"x": 1}, {"y": 2}, {"z": 3}}, a)

	b, err := GetOptionalSliceOfMaps(m, "b")
	assert.Nil(b)
	assert.Error(err)

	c, err := GetOptionalSliceOfMaps(m, "c")
	assert.Nil(c)
	assert.Error(err)

	d, err := GetOptionalSliceOfMaps(m, "d")
	assert.Nil(d)
	assert.Error(err)

	e, err := GetOptionalSliceOfMaps(m, "e")
	assert.Nil(e)
	assert.NoError(err)

	f, err := GetOptionalSliceOfMaps(m, "f")
	assert.Nil(f)
	assert.NoError(err)
}

func TestGetSliceOfMapsNil(t *testing.T) {
	assert := require.New(t)
	n, err := GetSliceOfMaps(nil, "a")
	assert.Nil(n)
	assert.Error(err)
}

func TestGetOptionalSliceOfMapsNil(t *testing.T) {
	assert := require.New(t)
	n, err := GetOptionalSliceOfMaps(nil, "a")
	assert.Nil(n)
	assert.Error(err)
}

func TestGetSliceOfMapsFromJSON(t *testing.T) {
	assert := require.New(t)

	ruleJSON := `
{
    "dest": "Whatever rule",
    "when": "event_type == 'whatever'",
    "rules": [
        {
            "class_uid": {
                "desc": "Whatever",
                "@value": 9001
            }
        },
        {
            "activity_id": {
                "desc": "Whatever (1)",
                "@value": 1
            }
        },
        {
            "event_type": {
                "@move": {
                    "name": "metadata.event_code"
                }
            }
        }
    ]
}`

	rule := make(Map)
	err := json.Unmarshal([]byte(ruleJSON), &rule)
	assert.NoError(err)

	rules, err := GetSliceOfMaps(rule, "rules")
	assert.NoError(err)
	assert.NotNil(rules)
	assert.Equal(3, len(rules))
}

func TestGetSliceOfMapsFromJSONFail(t *testing.T) {
	assert := require.New(t)

	// First element of the rules attribute array isn't an object
	ruleJSON := `
{
    "dest": "Whatever rule",
    "when": "event_type == 'whatever'",
    "rules": [
        "whatever",
        {
            "event_type": {
                "@move": {
                    "name": "metadata.event_code"
                }
            }
        }
    ]
}`

	rule := make(Map)
	err := json.Unmarshal([]byte(ruleJSON), &rule)
	assert.NoError(err)

	rules, err := GetSliceOfMaps(rule, "rules")
	assert.Nil(rules)
	assert.Error(err)
}

func TestGetSliceOfMapsFailNotMaps(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": []any{1, 2, 3},
	}
	a, err := GetSliceOfMaps(m, "a")
	assert.Nil(a)
	assert.Error(err)
}

func TestGetOptionalSliceOfMapsFailNotMaps(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": []any{1, 2, 3},
	}
	a, err := GetOptionalSliceOfMaps(m, "a")
	assert.Nil(a)
	assert.Error(err)
}

func TestGetString(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
	}
	a, err := GetString(m, "a")
	assert.Equal("", a)
	assert.Error(err)

	b, err := GetString(m, "b")
	assert.NoError(err)
	assert.NotNil(b)
	assert.Equal("whatever", b)

	c, err := GetString(m, "c")
	assert.Equal("", c)
	assert.Error(err)

	d, err := GetString(m, "d")
	assert.Equal("", d)
	assert.Error(err)
}

func TestGetStringBlankKeys(t *testing.T) {
	assert := require.New(t)

	n1, err := GetString(nil, "")
	assert.Equal("", n1)
	assert.Error(err)

	n2, err := GetString(Map{}, "")
	assert.Equal("", n2)
	assert.Error(err)

	n3, err := GetString(map[string]any{}, "")
	assert.Equal("", n3)
	assert.Error(err)

	blankBad, err := GetString(Map{"": Map{"a": 1}}, "")
	assert.Equal("", blankBad)
	assert.Error(err)

	blankBad1, err := GetString(Map{"": Map{"a": 1}}, "")
	assert.Equal("", blankBad1)
	assert.Error(err)

	blankBad2, err := GetString(map[string]any{"": map[string]any{"a": 1}}, "")
	assert.Equal("", blankBad2)
	assert.Error(err)

	blank, err := GetString(Map{"": "whatever"}, "")
	assert.NoError(err)
	assert.Equal("whatever", blank)
}

func TestGetIn(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
	}

	a := GetIn(m, "a")
	assert.NotNil(a)
	assert.IsType(Map{}, a)
	assert.Equal(Map{"x": 1, "y": 2, "z": 3}, a)

	ax := GetIn(m, "a", "x")
	assert.NotNil(ax)
	assert.Equal(1, ax)

	b := GetIn(m, "b")
	assert.NotNil(b)
	assert.Equal("whatever", b)

	c := GetIn(m, "c")
	assert.NotNil(c)
	assert.Equal(42, c)

	assert.Nil(GetIn(m, "d"))
	assert.Nil(GetIn(m, "foo"))
	assert.Nil(GetIn(m, "foo", "bar"))
	assert.Nil(GetIn(m, "a", "foo"))
	assert.Nil(GetIn(m, "a", "x", "foo"))
	assert.Nil(GetIn(m))
}

func TestGetInNilMap(t *testing.T) {
	assert := require.New(t)
	assert.Nil(GetIn(nil))
	assert.Nil(GetIn(nil, "foo"))
	assert.Nil(GetIn(nil, "foo", "bar"))
}

func TestFormatStringArray(t *testing.T) {
	// some cases not easily covered by cases below
	assert := require.New(t)

	assert.Equal(`nil`, formatStringArray(nil))
	assert.Equal(`[]`, formatStringArray([]string{}))
	assert.Equal(`["one"]`, formatStringArray([]string{"one"}))
	assert.Equal(`["one", "two"]`, formatStringArray([]string{"one", "two"}))
}

func TestGetInPath(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
	}

	a := GetInPath(m, "a")
	assert.NotNil(a)
	assert.IsType(Map{}, a)
	assert.Equal(Map{"x": 1, "y": 2, "z": 3}, a)

	ax := GetInPath(m, "a.x")
	assert.NotNil(ax)
	assert.Equal(1, ax)
}

func TestGetInPathDots(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a.x": "alpha",
		"b":   "whatever",
		"c":   42,
	}
	ax := GetInPath(m, "a.x")
	assert.Equal("alpha", ax)
}

func TestRemoveIn(t *testing.T) {
	assert := require.New(t)

	var m Map
	var ok bool

	m = Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
	}

	a := RemoveIn(m, "a")
	assert.NotNil(a)
	assert.IsType(Map{}, a)
	assert.Equal(Map{"x": 1, "y": 2, "z": 3}, a)
	_, ok = m["a"]
	assert.False(ok)

	m = Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
	}

	ax := RemoveIn(m, "a", "x")
	assert.NotNil(ax)
	assert.Equal(1, ax)
	a = GetIn(m, "a")
	assert.IsType(Map{}, a)
	aMap := a.(Map)
	_, ok = aMap["x"]
	assert.False(ok)

	b := RemoveIn(m, "b")
	assert.NotNil(b)
	assert.Equal("whatever", b)
	_, ok = m["b"]
	assert.False(ok)

	c := RemoveIn(m, "c")
	assert.NotNil(c)
	assert.Equal(42, c)
	_, ok = m["c"]
	assert.False(ok)

	assert.Nil(GetIn(m, "d"))
	assert.Nil(GetIn(m, "foo"))
	assert.Nil(GetIn(m, "foo", "bar"))
	assert.Nil(GetIn(m, "a", "foo"))
	assert.Nil(GetIn(m, "a", "x", "foo"))
	assert.Nil(GetIn(m))
}

func TestRemoveInNilMap(t *testing.T) {
	assert := require.New(t)
	assert.Nil(RemoveIn(nil))
	assert.Nil(RemoveIn(nil, "foo"))
	assert.Nil(RemoveIn(nil, "foo", "bar"))
}

func TestRemoveInPath(t *testing.T) {
	assert := require.New(t)

	var m Map
	var ok bool

	m = Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
	}

	a := RemoveInPath(m, "a")
	assert.NotNil(a)
	assert.IsType(Map{}, a)
	assert.Equal(Map{"x": 1, "y": 2, "z": 3}, a)
	_, ok = m["a"]
	assert.False(ok)

	m = Map{
		"a": Map{
			"x": 1,
			"y": 2,
			"z": 3,
		},
		"b": "whatever",
		"c": 42,
	}

	ax := RemoveInPath(m, "a.x")
	assert.NotNil(ax)
	assert.Equal(1, ax)
	a = GetIn(m, "a")
	assert.IsType(Map{}, a)
	aMap := a.(Map)
	_, ok = aMap["x"]
	assert.False(ok)
}

func TestRemoveInPathDots(t *testing.T) {
	assert := require.New(t)
	m := Map{
		"a.x": "alpha",
		"b":   "whatever",
		"c":   42,
	}
	ax := RemoveInPath(m, "a.x")
	assert.Equal("alpha", ax)
	_, ok := m["a.x"]
	assert.False(ok)
}

func TestPutInPathOverwriteFalse(t *testing.T) {
	assert := require.New(t)

	var inMap, outMap Map
	var err error

	inMap = nil
	outMap, err = PutInPath(inMap, "type_id", 100, false)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"type_id": 100}, outMap)
	assert.Nil(inMap, "inMap is still nil; outMap is a new map instance")

	inMap = nil
	outMap, err = PutInPath(inMap, "", 100, false)
	assert.NoError(err)
	assert.Nil(outMap)

	inMap = Map{}
	outMap, err = PutInPath(inMap, "", 100, false)
	assert.NoError(err)
	assert.NotNil(outMap)

	inMap = Map{}
	outMap, err = PutInPath(inMap, "type_id", 100, false)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"type_id": 100}, outMap)
	assert.Equal(outMap, inMap, "inMap is modified in place")

	inMap = Map{}
	outMap, err = PutInPath(inMap, "type_id", nil, false)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Contains(outMap, "type_id")
	_, present := outMap["type_id"]
	assert.True(present)
	assert.Equal(Map{"type_id": nil}, outMap)
	assert.Equal(outMap, inMap, "inMap is modified in place")

	inMap = nil
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, false)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Nil(inMap, "inMap is still nil; outMap is a new map instance")

	inMap = Map{}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, false)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Equal(outMap, inMap)

	inMap = Map{"metadata": Map{"version": "1.0"}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, false)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Equal(outMap, inMap)

	inMap = Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": Map{"name": "RickOS"}}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, false)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(
		Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": Map{"name": "RickOS", "type_id": 100}}},
		outMap)
	assert.Equal(outMap, inMap)

	inMap = Map{"metadata": Map{"version": "1.0"}}
	outMap, err = PutInPath(inMap, "metadata.uid", "unique", false)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"metadata": Map{"version": "1.0", "uid": "unique"}}, outMap)
	assert.Equal(outMap, inMap)

	// Overwrite tests
	inMap = Map{"type_id": 1}
	outMap, err = PutInPath(inMap, "type_id", 100, false)
	assert.Nil(outMap)
	assert.Error(err)
	assert.ErrorIs(err, ErrExistingAssociation)

	inMap = Map{"device": Map{"os": "whatever"}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, false)
	assert.Nil(outMap)
	assert.Error(err)
	assert.ErrorIs(err, ErrExistingAssociation)

	inMap = Map{"device": Map{"os": Map{"type_id": 1}}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, false)
	assert.Nil(outMap)
	assert.Error(err)
	assert.ErrorIs(err, ErrExistingAssociation)

	inMap = Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": "whatever"}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, false)
	assert.Nil(outMap)
	assert.Error(err)
	assert.ErrorIs(err, ErrExistingAssociation)

	inMap = Map{"metadata": Map{"version": "1.0"}}
	outMap, err = PutInPath(inMap, "metadata", Map{"uid": "unique"}, false)
	assert.Nil(outMap)
	assert.Error(err)
	assert.ErrorIs(err, ErrExistingAssociation)
}

func TestPutInPathOverwriteTrue(t *testing.T) {
	assert := require.New(t)

	var inMap, outMap Map
	var err error

	inMap = nil
	outMap, err = PutInPath(inMap, "type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"type_id": 100}, outMap)
	assert.Nil(inMap, "inMap is still nil; outMap is a new map instance")

	inMap = nil
	outMap, err = PutInPath(inMap, "", 100, true)
	assert.NoError(err)
	assert.Nil(outMap)

	inMap = Map{}
	outMap, err = PutInPath(inMap, "", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)

	inMap = Map{}
	outMap, err = PutInPath(inMap, "type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"type_id": 100}, outMap)
	assert.Equal(outMap, inMap, "inMap is modified in place")

	inMap = Map{}
	outMap, err = PutInPath(inMap, "type_id", nil, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Contains(outMap, "type_id")
	_, present := outMap["type_id"]
	assert.True(present)
	assert.Equal(Map{"type_id": nil}, outMap)
	assert.Equal(outMap, inMap, "inMap is modified in place")

	inMap = nil
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Nil(inMap, "inMap is still nil; outMap is a new map instance")

	inMap = Map{}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Equal(outMap, inMap)

	inMap = Map{"metadata": Map{"version": "1.0"}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Equal(outMap, inMap)

	inMap = Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": Map{"name": "RickOS"}}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(
		Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": Map{"name": "RickOS", "type_id": 100}}},
		outMap)
	assert.Equal(outMap, inMap)

	// Overwrite tests
	inMap = Map{"type_id": 1}
	outMap, err = PutInPath(inMap, "type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"type_id": 100}, outMap)
	assert.Equal(outMap, inMap, "inMap is modified in place")

	inMap = Map{"device": Map{"os": "whatever"}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Equal(outMap, inMap, "inMap is modified in place")

	inMap = Map{"device": Map{"os": Map{"type_id": 1}}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Equal(outMap, inMap, "inMap is modified in place")

	inMap = Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": "whatever"}}
	outMap, err = PutInPath(inMap, "device.os.type_id", 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)
	assert.Equal(Map{"metadata": Map{"version": "1.0"}, "device": Map{"os": Map{"type_id": 100}}}, outMap)
	assert.Equal(outMap, inMap, "inMap is modified in place")
}

func TestPut(t *testing.T) {
	// check cases that PutInPath doesn't cover
	assert := require.New(t)

	var inMap, outMap Map
	var err error

	inMap = nil
	outMap, err = Put(inMap, "foo", 100, false)
	assert.NoError(err)
	assert.Nil(inMap)
	assert.NotNil(outMap)
	assert.Equal(Map{"foo": 100}, outMap)

	inMap = nil
	outMap, err = Put(inMap, "foo", 100, true)
	assert.NoError(err)
	assert.Nil(inMap)
	assert.NotNil(outMap)
	assert.Equal(Map{"foo": 100}, outMap)

	inMap = Map{"foo": 999}
	outMap, err = Put(inMap, "foo", 100, false)
	assert.Error(err)
	assert.NotNil(inMap)
	assert.Nil(outMap)
	assert.Equal(Map{"foo": 999}, inMap, "not overwritten")

	inMap = Map{"foo": 999}
	outMap, err = Put(inMap, "foo", 100, true)
	assert.NoError(err)
	assert.NotNil(inMap)
	assert.NotNil(outMap)
	assert.Equal(outMap, inMap)
	assert.Equal(Map{"foo": 100}, inMap, "overwritten")
}

func TestPutIn(t *testing.T) {
	// check cases that PutInPath doesn't cover
	assert := require.New(t)

	var inMap, outMap Map
	var err error

	inMap = nil
	outMap, err = PutIn(inMap, nil, 100, false)
	assert.NoError(err)
	assert.Nil(outMap)

	inMap = nil
	outMap, err = PutIn(inMap, nil, 100, true)
	assert.NoError(err)
	assert.Nil(outMap)

	inMap = Map{}
	outMap, err = PutIn(inMap, []string{}, 100, false)
	assert.NoError(err)
	assert.NotNil(outMap)

	inMap = Map{}
	outMap, err = PutIn(inMap, []string{}, 100, true)
	assert.NoError(err)
	assert.NotNil(outMap)

	inMap = nil
	outMap, err = PutIn(inMap, []string{"foo", "bar"}, 100, false)
	assert.NoError(err)
	assert.Nil(inMap)
	assert.NotNil(outMap)
	assert.Equal(Map{"foo": Map{"bar": 100}}, outMap)

	inMap = nil
	outMap, err = PutIn(inMap, []string{"foo", "bar"}, 100, true)
	assert.NoError(err)
	assert.Nil(inMap)
	assert.NotNil(outMap)
	assert.Equal(Map{"foo": Map{"bar": 100}}, outMap)

	inMap = Map{"foo": Map{"bar": 999}}
	outMap, err = PutIn(inMap, []string{"foo", "bar"}, 100, false)
	assert.Error(err)
	assert.NotNil(inMap)
	assert.Nil(outMap)
	assert.Equal(Map{"foo": Map{"bar": 999}}, inMap, "not overwritten")

	inMap = Map{"foo": Map{"bar": 999}}
	outMap, err = PutIn(inMap, []string{"foo", "bar"}, 100, true)
	assert.NoError(err)
	assert.NotNil(inMap)
	assert.NotNil(outMap)
	assert.Equal(outMap, inMap)
	assert.Equal(Map{"foo": Map{"bar": 100}}, inMap, "overwritten")
}
