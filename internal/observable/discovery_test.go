package observable

import (
	"maps"
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/stretchr/testify/require"
)

func TestTypeIDUsesCompiledSchemaPrecedence(t *testing.T) {
	typeObservable := int64(1)
	dictionaryObservable := int64(2)
	attributeObservable := int64(3)
	compiled := &schema.Compiled{Dictionary: &schema.DictionaryDefinition{
		Attributes: map[string]*schema.CommonAttributeDefinition{
			"value": {Observable: &dictionaryObservable},
		},
		Types: &schema.TypesDefinition{Attributes: map[string]*schema.TypeDefinition{
			"custom_t": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Observable: &typeObservable}},
		}},
	}}
	attrDef := &schema.ItemAttributeDefinition{CommonAttributeDefinition: schema.CommonAttributeDefinition{
		Type:       "custom_t",
		Observable: &attributeObservable,
	}}
	var path eventpath.Path
	path.PushAttribute("value")

	typeID, present := TypeID(compiled, "value", attrDef)
	require.True(t, present)
	require.Equal(t, int64(1), typeID)

	delete(compiled.Dictionary.Types.Attributes, "custom_t")
	typeID, present = TypeID(compiled, "value", attrDef)
	require.True(t, present)
	require.Equal(t, int64(2), typeID)

	delete(compiled.Dictionary.Attributes, "value")
	typeID, present = TypeID(compiled, "value", attrDef)
	require.True(t, present)
	require.Equal(t, int64(3), typeID)

	attrDef.Observable = nil
	_, present = TypeID(compiled, "value", attrDef)
	require.False(t, present)
}

func TestAttributeMayGenerateTraversesObjectDefinitionsAndStopsCycles(t *testing.T) {
	parentType := "parent"
	childType := "child"
	observableType := int64(10)
	compiled := &schema.Compiled{
		Dictionary: &schema.DictionaryDefinition{
			Attributes: map[string]*schema.CommonAttributeDefinition{},
			Types: &schema.TypesDefinition{Attributes: map[string]*schema.TypeDefinition{
				"observable_t": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Observable: &observableType},
				},
			}},
		},
		Objects: map[string]*schema.ObjectDefinition{},
	}
	compiled.Objects["parent"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"parent": {
				CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &parentType},
			},
			"child": {
				CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &childType},
			},
		},
	}}
	compiled.Objects["child"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"value": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "observable_t"}},
		},
	}}
	attrDef := &schema.ItemAttributeDefinition{CommonAttributeDefinition: schema.CommonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &parentType,
	}}
	require.True(t, AttributeMayGenerate(compiled, "parent", attrDef, nil, nil))
	delete(compiled.Objects, "child")
	require.False(t, AttributeMayGenerate(compiled, "parent", attrDef, nil, nil))
}

// cyclicEscapeSchema builds a compiled schema where objects "a" and "b" reference each other (a 2-cycle), and "b"
// additionally references "c", which is directly observable. Every object in the cycle can reach an observable only
// through that escape edge, not through the cycle itself; a resolver that commits a node's answer to a shared cache
// while a cycle-mate is still being resolved (rather than only once its own full attribute set has been checked)
// can wrongly cache "a" as unobservable when the object graph happens to be walked starting from "b".
func cyclicEscapeSchema() *schema.Compiled {
	observableType := int64(10)
	aType, bType, cType := "a", "b", "c"
	compiled := &schema.Compiled{
		Dictionary: &schema.DictionaryDefinition{
			Attributes: map[string]*schema.CommonAttributeDefinition{},
			Types: &schema.TypesDefinition{Attributes: map[string]*schema.TypeDefinition{
				"observable_t": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Observable: &observableType},
				},
			}},
		},
		Objects: map[string]*schema.ObjectDefinition{},
	}
	compiled.Objects["a"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"toB": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &bType}},
		},
	}}
	compiled.Objects["b"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"toA": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &aType}},
			"toC": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &cType}},
		},
	}}
	compiled.Objects["c"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"value": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "observable_t"}},
		},
	}}
	return compiled
}

func TestCompileObjectObservabilityResolvesCycleWithIndependentEscape(t *testing.T) {
	compiled := cyclicEscapeSchema()

	for range 20 {
		result := CompileObjectObservability(compiled, nil)
		require.True(t, result[compiled.Objects["a"]])
		require.True(t, result[compiled.Objects["b"]])
		require.True(t, result[compiled.Objects["c"]])
	}
}

func TestAttributeMayGenerateResolvesCycleWithIndependentEscapeWithoutCache(t *testing.T) {
	compiled := cyclicEscapeSchema()
	aType := "a"
	attrDef := &schema.ItemAttributeDefinition{CommonAttributeDefinition: schema.CommonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &aType,
	}}

	for range 20 {
		require.True(t, AttributeMayGenerate(compiled, "a", attrDef, nil, nil))
	}
}

func TestAttributeMayGenerateResolvesCycleWithIndependentEscapeUsingCompiledCache(t *testing.T) {
	compiled := cyclicEscapeSchema()
	aType := "a"
	attrDef := &schema.ItemAttributeDefinition{CommonAttributeDefinition: schema.CommonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &aType,
	}}

	for range 20 {
		cache := CompileObjectObservability(compiled, nil)
		require.True(t, AttributeMayGenerate(compiled, "a", attrDef, nil, cache))
	}
}

// twoObjectCycleSchema builds a compiled schema where objects "x" and "y" reference only each other, with no
// observable anywhere in the graph: a bare 2-cycle with no escape at all.
func twoObjectCycleSchema() *schema.Compiled {
	xType, yType := "x", "y"
	compiled := &schema.Compiled{
		Dictionary: &schema.DictionaryDefinition{Attributes: map[string]*schema.CommonAttributeDefinition{}},
		Objects:    map[string]*schema.ObjectDefinition{},
	}
	compiled.Objects["x"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"toY": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &yType}},
		},
	}}
	compiled.Objects["y"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"toX": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &xType}},
		},
	}}
	return compiled
}

func TestCompileObjectObservabilityResolvesCycleWithNoEscapeAsFalse(t *testing.T) {
	compiled := twoObjectCycleSchema()

	for range 20 {
		result := CompileObjectObservability(compiled, nil)
		require.False(t, result[compiled.Objects["x"]])
		require.False(t, result[compiled.Objects["y"]])
	}
}

func TestAttributeMayGenerateResolvesCycleWithNoEscapeAsFalseWithoutCache(t *testing.T) {
	compiled := twoObjectCycleSchema()
	xType := "x"
	attrDef := &schema.ItemAttributeDefinition{CommonAttributeDefinition: schema.CommonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &xType,
	}}

	for range 20 {
		require.False(t, AttributeMayGenerate(compiled, "x", attrDef, nil, nil))
	}
}

// selfReferencingSchema builds a compiled schema with a single object "self" whose only attribute references
// itself, and (when observable is true) a second, directly observable attribute on the same object.
func selfReferencingSchema(observableType int64, observable bool) *schema.Compiled {
	selfType := "self"
	compiled := &schema.Compiled{
		Dictionary: &schema.DictionaryDefinition{
			Attributes: map[string]*schema.CommonAttributeDefinition{},
			Types: &schema.TypesDefinition{Attributes: map[string]*schema.TypeDefinition{
				"observable_t": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Observable: &observableType},
				},
			}},
		},
		Objects: map[string]*schema.ObjectDefinition{},
	}
	attributes := map[string]*schema.ItemAttributeDefinition{
		"self": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &selfType}},
	}
	if observable {
		attributes["value"] = &schema.ItemAttributeDefinition{
			CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "observable_t"},
		}
	}
	compiled.Objects["self"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{Attributes: attributes}}
	return compiled
}

func TestCompileObjectObservabilityResolvesSelfReferencingObjectWithoutObservableAsFalse(t *testing.T) {
	compiled := selfReferencingSchema(10, false)

	for range 20 {
		result := CompileObjectObservability(compiled, nil)
		require.False(t, result[compiled.Objects["self"]])
	}
}

func TestCompileObjectObservabilityResolvesSelfReferencingObjectWithObservableAsTrue(t *testing.T) {
	compiled := selfReferencingSchema(10, true)

	for range 20 {
		result := CompileObjectObservability(compiled, nil)
		require.True(t, result[compiled.Objects["self"]])
	}
}

func TestAttributeMayGenerateResolvesSelfReferencingObjectWithoutObservableAsFalseWithoutCache(t *testing.T) {
	compiled := selfReferencingSchema(10, false)
	selfType := "self"
	attrDef := &schema.ItemAttributeDefinition{CommonAttributeDefinition: schema.CommonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &selfType,
	}}

	for range 20 {
		require.False(t, AttributeMayGenerate(compiled, "self", attrDef, nil, nil))
	}
}

// threeNodeCycleEscapeSchema builds a compiled schema with a 3-cycle (a -> b -> c -> a) where only "c" additionally
// escapes to "d", which is directly observable. Reaching the observable from "a" or "b" requires walking all the way
// around the cycle to "c" first, exercising propagation across more than one hop rather than a single cycle-mate.
func threeNodeCycleEscapeSchema(observableType int64) *schema.Compiled {
	aType, bType, cType, dType := "a", "b", "c", "d"
	compiled := &schema.Compiled{
		Dictionary: &schema.DictionaryDefinition{
			Attributes: map[string]*schema.CommonAttributeDefinition{},
			Types: &schema.TypesDefinition{Attributes: map[string]*schema.TypeDefinition{
				"observable_t": {
					CommonAttributeDefinition: schema.CommonAttributeDefinition{Observable: &observableType},
				},
			}},
		},
		Objects: map[string]*schema.ObjectDefinition{},
	}
	compiled.Objects["a"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"toB": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &bType}},
		},
	}}
	compiled.Objects["b"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"toC": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &cType}},
		},
	}}
	compiled.Objects["c"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"toA": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &aType}},
			"toD": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "object_t", ObjectType: &dType}},
		},
	}}
	compiled.Objects["d"] = &schema.ObjectDefinition{ItemDefinition: schema.ItemDefinition{
		Attributes: map[string]*schema.ItemAttributeDefinition{
			"value": {CommonAttributeDefinition: schema.CommonAttributeDefinition{Type: "observable_t"}},
		},
	}}
	return compiled
}

func TestCompileObjectObservabilityResolvesThreeNodeCycleWithEscape(t *testing.T) {
	compiled := threeNodeCycleEscapeSchema(10)

	for range 20 {
		result := CompileObjectObservability(compiled, nil)
		require.True(t, result[compiled.Objects["a"]])
		require.True(t, result[compiled.Objects["b"]])
		require.True(t, result[compiled.Objects["c"]])
		require.True(t, result[compiled.Objects["d"]])
	}
}

// TestCompileObjectObservabilityIsolatesDisjointCycleComponents combines an escape-free 2-cycle ("x"/"y") with an
// independently escaping 2-cycle ("a"/"b"/"c") in a single schema, so a resolver that leaks state across unrelated
// components (rather than keying strictly off the reversed-edge graph) would wrongly mark "x"/"y" as observable too.
func TestCompileObjectObservabilityIsolatesDisjointCycleComponents(t *testing.T) {
	compiled := cyclicEscapeSchema()
	noEscape := twoObjectCycleSchema()
	maps.Copy(compiled.Objects, noEscape.Objects)

	for range 20 {
		result := CompileObjectObservability(compiled, nil)
		require.True(t, result[compiled.Objects["a"]])
		require.True(t, result[compiled.Objects["b"]])
		require.True(t, result[compiled.Objects["c"]])
		require.False(t, result[compiled.Objects["x"]])
		require.False(t, result[compiled.Objects["y"]])
	}
}
