package observablepath

import (
	"encoding/json"
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/stretchr/testify/require"
)

func TestParseAcceptsSupportedNotationVariations(t *testing.T) {
	for _, test := range []struct {
		name  string
		first string
	}{
		{name: "actor.user.name", first: "actor"},
		{name: "actors[].user.name", first: "actors"},
		{name: "actors[*].user.name", first: "actors"},
		{name: "actors[0].user.name", first: "actors"},
		{name: "$.actors[0].user.name", first: "actors"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, err := Parse(test.name)
			require.NoError(t, err)
			require.Equal(t, test.first, path.FirstAttribute())
		})
	}
}

func TestParseRejectsMalformedPaths(t *testing.T) {
	tests := map[string]string{
		"":              "path is empty",
		"$":             "root marker must be followed by an attribute",
		"$actor.name":   "root marker must be followed by a dot",
		"actor..name":   "empty attribute",
		"[0].name":      "selector has no attribute",
		"actors[-1]":    "not a non-negative index",
		"actors[0]name": "unexpected text",
	}
	for name, wantError := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(name)
			require.ErrorContains(t, err, wantError)
		})
	}
}

func TestResolveTraversesAllSupportedArraySelectors(t *testing.T) {
	event := jsonish.Map{
		"actors": []jsonish.Map{
			{"ports": []any{json.Number("80"), json.Number("443")}},
			{"ports": []any{json.Number("8080")}},
		},
	}

	tests := []struct {
		name    string
		value   string
		found   bool
		missing bool
	}{
		{name: "actors.ports", value: "443", found: true},
		{name: "actors[].ports[]", value: "8080", found: true},
		{name: "actors[*].ports[0]", value: "8080", found: true},
		{name: "$.actors[0].ports[1]", value: "443", found: true},
		{name: "actors[3].ports", value: "443", missing: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := Parse(test.name)
			require.NoError(t, err)
			resolution := path.ResolveString(event, test.value)
			require.Equal(t, test.found, resolution.Matched)
			require.Equal(t, test.missing, resolution.Missing)
		})
	}
}

func TestResolutionMatchesObjectsAndScalarStrings(t *testing.T) {
	event := jsonish.Map{
		"values": []any{jsonish.Map{"name": "actor"}, nil, json.Number("443")},
	}
	path, err := Parse("values")
	require.NoError(t, err)

	require.True(t, path.ResolveObject(event).Matched)
	require.True(t, path.ResolveString(event, "443").Matched)
	require.False(t, path.ResolveString(event, "80").Matched)
}

// Invariant test: path resolution treats a nil-valued event-map attribute as missing.
func TestInvariantResolutionTreatsNilMapValueAsMissing(t *testing.T) {
	path, err := Parse("value")
	require.NoError(t, err)

	for _, resolution := range []Resolution{
		path.ResolveObject(jsonish.Map{"value": nil}),
		path.ResolveString(jsonish.Map{"value": nil}, ""),
	} {
		require.False(t, resolution.Found)
		require.False(t, resolution.Matched)
		require.True(t, resolution.Missing)
	}
}

func TestResolveHandlesCyclicAndDeeplyNestedArrays(t *testing.T) {
	cyclic := make([]any, 2)
	cyclic[0] = cyclic
	cyclic[1] = "found"

	var deeplyNested any = "deep"
	for range 100_000 {
		deeplyNested = []any{deeplyNested}
	}

	path, err := Parse("values")
	require.NoError(t, err)
	require.True(t, path.ResolveString(jsonish.Map{"values": cyclic}, "found").Matched)
	require.True(t, path.ResolveString(jsonish.Map{"values": deeplyNested}, "deep").Matched)
}

func TestPathDefinitionAndNotationUseCompiledSchema(t *testing.T) {
	isArray := true
	actorType := "actor"
	class := &schema.ClassDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"actors": {
				CommonAttributeDefinition: schema.CommonAttributeDefinition{
					Type:       "object_t",
					IsArray:    &isArray,
					ObjectType: &actorType,
				},
			},
		},
	}}
	objects := map[string]*schema.ObjectDefinition{
		"actor": {ItemDefinition: schema.ItemDefinition{
			Attributes: map[string]*schema.ItemAttributeDefinition{
				"name": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "string_t"}},
				"ldap": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "string_t"},
					Profiles:                  []string{"ldap"},
				},
			},
		}},
	}

	tests := []struct {
		name  string
		style pathstyle.Style
	}{
		{name: "actors.name", style: pathstyle.Simple},
		{name: "actors[].name", style: pathstyle.ArrayBrackets},
		{name: "actors[*].name", style: pathstyle.ArrayWildcard},
		{name: "actors[0].name", style: pathstyle.ArrayIndexed},
		{name: "$.actors[0].name", style: pathstyle.JSONPath},
	}
	for _, test := range tests {
		path, err := Parse(test.name)
		require.NoError(t, err)
		require.Equal(t, DefinitionDefined, path.Definition(class, objects, nil), test.name)
		require.True(t, path.UsesNotation(test.style, class, objects), test.name)
	}

	profilePath, err := Parse("actors[].ldap")
	require.NoError(t, err)
	require.Equal(t, DefinitionUndefined, profilePath.Definition(class, objects, nil))
	require.Equal(t, DefinitionDefined, profilePath.Definition(class, objects, schema.ProfileSet{"ldap": {}}))
}

func TestPathDefinitionStopsBelowRepeatedObjectAttribute(t *testing.T) {
	fileType := "file"
	class := &schema.ClassDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"file": {
				CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &fileType},
			},
		},
	}}
	objects := map[string]*schema.ObjectDefinition{
		"file": {ItemDefinition: schema.ItemDefinition{Attributes: map[string]*schema.ItemAttributeDefinition{
			"name": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "string_t"}},
			"parent": {
				CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &fileType},
			},
		}}},
	}

	for _, name := range []string{"file.parent", "file.parent.parent"} {
		path, err := Parse(name)
		require.NoError(t, err)
		require.Equal(t, DefinitionDefined, path.Definition(class, objects, nil), name)
	}
	path, err := Parse("file.parent.parent.name")
	require.NoError(t, err)
	require.Equal(t, DefinitionTraversalLimited, path.Definition(class, objects, nil))
}

func BenchmarkResolveArrayPath(b *testing.B) {
	event := jsonish.Map{
		"actors": []jsonish.Map{
			{"ports": []json.Number{"80", "443"}},
			{"ports": []json.Number{"8080"}},
		},
	}
	path, err := Parse("actors[].ports[]")
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !path.ResolveString(event, "443").Matched {
			b.Fatal("expected path to match")
		}
	}
}

// BenchmarkResolveSimplePath covers the no-brackets fast path, which resolveSingle answers without
// falling back to resolve's array-candidate walk.
func BenchmarkResolveSimplePath(b *testing.B) {
	event := jsonish.Map{
		"actor": jsonish.Map{
			"user": jsonish.Map{"name": "root"},
		},
	}
	path, err := Parse("actor.user.name")
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !path.ResolveString(event, "root").Matched {
			b.Fatal("expected path to match")
		}
	}
}
