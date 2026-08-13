package schema

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/stretchr/testify/require"
)

type observedLenReader struct {
	*bytes.Reader
	called bool
}

func (r *observedLenReader) Len() int {
	r.called = true
	return r.Reader.Len()
}

func TestEngineeringInvariantLoadReaderDoesNotConsultOptionalLen(t *testing.T) {
	// Engineering invariant test: schema loading allocates only in response to bytes read, not an optional size hint.
	data, err := os.ReadFile("testdata/enum_sibling_redirect.json")
	require.NoError(t, err)
	reader := &observedLenReader{Reader: bytes.NewReader(data)}

	compiled, _, err := LoadReader(reader)

	require.NoError(t, err)
	require.NotNil(t, compiled)
	require.False(t, reader.called)
}

func TestEnsureTraversalCacheIndexesCanonicalIntegralEnums(t *testing.T) {
	want := &EnumDefinition{Caption: "One"}
	wantTwo := &EnumDefinition{Caption: "Two"}
	wantOther := &EnumDefinition{Caption: "Other"}
	alias := &EnumDefinition{Caption: "Alias"}
	stringEnum := &EnumDefinition{Caption: "String"}
	integerAttribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "integer_t",
		Enum: map[string]*EnumDefinition{
			"1": want, "2": wantTwo, "99": wantOther, "01": alias, "invalid": alias,
		},
	}}
	sparseAttribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "integer_t",
		Enum: map[string]*EnumDefinition{"0": want, "100": wantTwo},
	}}
	mapDefinitions := make(map[string]*EnumDefinition, numericEnumScanThreshold+1)
	for value := range numericEnumScanThreshold + 1 {
		mapDefinitions[strconv.Itoa(value*2)] = want
	}
	mapAttribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "integer_t",
		Enum: mapDefinitions,
	}}
	stringAttribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "string_t",
		Enum: map[string]*EnumDefinition{"1": stringEnum},
	}}
	compiled := &Compiled{Classes: map[int64]*ClassDefinition{
		1: {ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{
			"integer": integerAttribute,
			"map":     mapAttribute,
			"sparse":  sparseAttribute,
			"string":  stringAttribute,
		}}},
	}}
	require.Nil(t, integerAttribute.NumericEnumDefinition(1))

	compiled.EnsureTraversalCache()

	require.Same(t, want, integerAttribute.NumericEnumDefinition(1))
	require.Same(t, wantTwo, integerAttribute.NumericEnumDefinition(2))
	require.Same(t, wantOther, integerAttribute.NumericEnumDefinition(99))
	require.Nil(t, integerAttribute.NumericEnumDefinition(3))
	require.NotNil(t, integerAttribute.numericEnums.dense)
	require.Nil(t, integerAttribute.numericEnums.sparse)
	require.False(t, integerAttribute.NumericEnumKeysCanonical())
	require.Same(t, want, sparseAttribute.NumericEnumDefinition(0))
	require.Same(t, wantTwo, sparseAttribute.NumericEnumDefinition(100))
	require.Nil(t, sparseAttribute.numericEnums.dense)
	require.NotNil(t, sparseAttribute.numericEnums.scan)
	require.Nil(t, sparseAttribute.numericEnums.sparse)
	require.True(t, sparseAttribute.NumericEnumKeysCanonical())
	require.Same(t, want, mapAttribute.NumericEnumDefinition(2))
	require.Nil(t, mapAttribute.numericEnums.dense)
	require.Nil(t, mapAttribute.numericEnums.scan)
	require.NotNil(t, mapAttribute.numericEnums.sparse)
	require.True(t, mapAttribute.NumericEnumKeysCanonical())
	require.Nil(t, stringAttribute.NumericEnumDefinition(1))
}

func TestEnsureTraversalCacheInitializesNumericEnumsConcurrently(t *testing.T) {
	want := &EnumDefinition{Caption: "One"}
	attribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "integer_t",
		Enum: map[string]*EnumDefinition{"1": want},
	}}
	compiled := &Compiled{Classes: map[int64]*ClassDefinition{
		1: {ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{
			"integer": attribute,
		}}},
	}}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			compiled.EnsureTraversalCache()
		})
	}
	workers.Wait()

	require.Same(t, want, attribute.NumericEnumDefinition(1))
}

func TestEnsureTraversalCacheResolvesNestedPrimitiveType(t *testing.T) {
	attribute := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "outer_t"}}
	compiled := &Compiled{
		Classes: map[int64]*ClassDefinition{
			1: {ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{"value": attribute}}},
		},
		Dictionary: &DictionaryDefinition{Types: &TypesDefinition{Attributes: map[string]*TypeDefinition{
			"outer_t": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "inner_t"}},
			"inner_t": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t"}},
		}}},
	}

	compiled.EnsureTraversalCache()

	require.Equal(t, "integer_t", attribute.PrimitiveType)
}

func TestResolvePrimitiveTypeRejectsCycle(t *testing.T) {
	compiled := &Compiled{Dictionary: &DictionaryDefinition{Types: &TypesDefinition{
		Attributes: map[string]*TypeDefinition{
			"alias_t":  {CommonAttributeDefinition: CommonAttributeDefinition{Type: "first_t"}},
			"first_t":  {CommonAttributeDefinition: CommonAttributeDefinition{Type: "second_t"}},
			"second_t": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "first_t"}},
		},
	}}}

	primitiveType, err := compiled.ResolvePrimitiveType("alias_t")

	require.Empty(t, primitiveType)
	require.EqualError(t, err, `type inheritance for "alias_t" contains a cycle at "first_t"`)
}

func BenchmarkNumericEnumDefinition(b *testing.B) {
	definition := &EnumDefinition{Caption: "Value"}
	dense := make([]*EnumDefinition, 16)
	for index := range dense {
		dense[index] = definition
	}
	scan := make([]numericEnumEntry, 16)
	sparse := make(map[int64]*EnumDefinition, 16)
	for index := range scan {
		value := int64(index * 100)
		scan[index] = numericEnumEntry{value: value, definition: definition}
		sparse[value] = definition
	}
	benchmarks := []struct {
		name      string
		attribute *ItemAttributeDefinition
		value     int64
	}{
		{
			name: "dense", value: 15,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				minimum: 0, maximum: 15, dense: dense, other: definition,
			}},
		},
		{
			name: "scan_first", value: 0,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				scan: scan, other: definition,
			}},
		},
		{
			name: "scan_middle", value: 800,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				scan: scan, other: definition,
			}},
		},
		{
			name: "scan_last", value: 1500,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				scan: scan, other: definition,
			}},
		},
		{
			name: "map_first", value: 0,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				sparse: sparse, other: definition,
			}},
		},
		{
			name: "map_middle", value: 800,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				sparse: sparse, other: definition,
			}},
		},
		{
			name: "map_last", value: 1500,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				sparse: sparse, other: definition,
			}},
		},
		{
			name: "other", value: 99,
			attribute: &ItemAttributeDefinition{numericEnums: &numericEnumIndex{
				minimum: 0, maximum: 15, dense: dense, other: definition,
			}},
		},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			var got *EnumDefinition
			b.ReportAllocs()
			for b.Loop() {
				got = benchmark.attribute.NumericEnumDefinition(benchmark.value)
			}
			if got != definition {
				b.Fatal("numeric enum definition was not found")
			}
		})
	}
}

func TestLoadBytesReturnsInitializationIssues(t *testing.T) {
	compiled, issues, err := LoadBytes([]byte(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {
			"alpha": {
					"name": "alpha",
					"uid": 1,
					"attributes": {
						"code": {"type": "integer_t", "enum": {"1": {"caption": "Jammed"}}, "sibling": "message"},
						"message": {"type": "string_t", "is_array": true}
					}
			}
		}
	}`))

	require.NoError(t, err)
	require.NotNil(t, compiled)
	require.Len(t, issues, 1)
	require.Equal(t, "issue_at_init_schema_enum_sibling_target_not_string", issues[0].Code.String())
	require.Equal(t, jsonish.Map{
		"item_type":         "class",
		"item_name":         "alpha",
		"attribute":         "code",
		"sibling":           "message",
		"expected_type":     "string_t",
		"expected_is_array": false,
		"actual_type":       "string_t",
		"actual_is_array":   true,
	}, issues[0].Details)
}

func TestTypeDerivedFromFollowsTypeChainAndStopsCycles(t *testing.T) {
	attributes := map[string]*TypeDefinition{
		"child_t":  {CommonAttributeDefinition: CommonAttributeDefinition{Type: "parent_t"}},
		"parent_t": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"}},
		"cycle_a":  {CommonAttributeDefinition: CommonAttributeDefinition{Type: "cycle_b"}},
		"cycle_b":  {CommonAttributeDefinition: CommonAttributeDefinition{Type: "cycle_a"}},
	}
	compiled := &Compiled{Dictionary: &DictionaryDefinition{Types: &TypesDefinition{Attributes: attributes}}}

	require.True(t, compiled.TypeDerivedFrom("child_t", "string_t"))
	require.True(t, compiled.TypeDerivedFrom("string_t", "string_t"))
	require.False(t, compiled.TypeDerivedFrom("child_t", "integer_t"))
	require.False(t, compiled.TypeDerivedFrom("cycle_a", "string_t"))
}

func TestNewRejectsNilDefinition(t *testing.T) {
	compiled, err := New(nil)
	require.Nil(t, compiled)
	require.EqualError(t, err, "compiled schema definition is nil")
}

func TestNewRejectsNullDefinitions(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(*Definition)
		errorContains string
	}{
		{
			name: "object",
			modify: func(definition *Definition) {
				definition.Objects["null_object"] = nil
			},
			errorContains: `object "null_object" is null`,
		},
		{
			name: "class attribute",
			modify: func(definition *Definition) {
				definition.Classes["alpha"].Attributes["null_attribute"] = nil
			},
			errorContains: `class "alpha" attribute "null_attribute" is null`,
		},
		{
			name: "object attribute",
			modify: func(definition *Definition) {
				definition.Objects["object"].Attributes["null_attribute"] = nil
			},
			errorContains: `object "object" attribute "null_attribute" is null`,
		},
		{
			name: "dictionary attribute",
			modify: func(definition *Definition) {
				definition.Dictionary.Attributes["null_attribute"] = nil
			},
			errorContains: `dictionary attribute "null_attribute" is null`,
		},
		{
			name: "dictionary type",
			modify: func(definition *Definition) {
				definition.Dictionary.Types.Attributes["null_t"] = nil
			},
			errorContains: `dictionary type "null_t" is null`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := minimalDefinition()
			test.modify(definition)

			compiled, err := New(definition)

			require.Nil(t, compiled)
			require.ErrorContains(t, err, test.errorContains)
		})
	}
}

func TestNewRejectsCyclicDictionaryTypeInheritance(t *testing.T) {
	definition := minimalDefinition()
	definition.Dictionary.Types.Attributes["alias_t"] = &TypeDefinition{
		CommonAttributeDefinition: CommonAttributeDefinition{Type: "first_t"},
	}
	definition.Dictionary.Types.Attributes["first_t"] = &TypeDefinition{
		CommonAttributeDefinition: CommonAttributeDefinition{Type: "second_t"},
	}
	definition.Dictionary.Types.Attributes["second_t"] = &TypeDefinition{
		CommonAttributeDefinition: CommonAttributeDefinition{Type: "first_t"},
	}

	compiled, err := New(definition)

	require.Nil(t, compiled)
	require.EqualError(
		t,
		err,
		`compiled schema dictionary type inheritance for "alias_t" contains a cycle at "first_t"`,
	)
}

func TestNewRejectsInvalidObservableTypeID(t *testing.T) {
	definition := minimalDefinition()
	definition.Objects["observable"] = &ObjectDefinition{ItemDefinition: ItemDefinition{
		Attributes: map[string]*ItemAttributeDefinition{
			"type_id": {CommonAttributeDefinition: CommonAttributeDefinition{
				Enum: map[string]*EnumDefinition{"not-an-integer": {Caption: "Invalid"}},
			}},
		},
	}}

	compiled, err := New(definition)

	require.Nil(t, compiled)
	require.ErrorContains(t, err, `observable type enum ID "not-an-integer" is invalid`)
}

func TestNewRejectsAliasedObservableTypeIDs(t *testing.T) {
	definition := minimalDefinition()
	definition.Objects["observable"] = &ObjectDefinition{ItemDefinition: ItemDefinition{
		Attributes: map[string]*ItemAttributeDefinition{
			"type_id": {CommonAttributeDefinition: CommonAttributeDefinition{
				Enum: map[string]*EnumDefinition{
					"01": {Caption: "Leading zero"},
					"1":  {Caption: "Canonical"},
				},
			}},
		},
	}}

	compiled, err := New(definition)

	require.Nil(t, compiled)
	require.EqualError(t, err, `observable type enum IDs "01" and "1" both normalize to 1`)
}

func minimalDefinition() *Definition {
	return &Definition{
		CompileVersion: 1,
		Version:        "1.0.0",
		Classes: map[string]*ClassDefinition{
			"alpha": {ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{}}, Uid: 1},
		},
		Objects: map[string]*ObjectDefinition{
			"object": {ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{}}},
		},
		Dictionary: &DictionaryDefinition{
			Attributes: map[string]*CommonAttributeDefinition{},
			Types:      &TypesDefinition{Attributes: map[string]*TypeDefinition{}},
		},
	}
}

func TestResolveEventClassClassifiesClassUID(t *testing.T) {
	class := &ClassDefinition{ItemDefinition: ItemDefinition{Name: "test"}, Uid: 1}
	compiled := &Compiled{Classes: map[int64]*ClassDefinition{1: class}}

	tests := []struct {
		name   string
		event  jsonish.Map
		status ClassResolution
		uid    int64
		class  *ClassDefinition
	}{
		{name: "missing", event: jsonish.Map{}, status: ClassUIDMissing},
		{name: "null", event: jsonish.Map{"class_uid": nil}, status: ClassUIDMissing},
		{name: "wrong type", event: jsonish.Map{"class_uid": "1"}, status: ClassUIDWrongType},
		{name: "unknown", event: jsonish.Map{"class_uid": json.Number("2")}, status: ClassUIDUnknown, uid: 2},
		{
			name:   "resolved",
			event:  jsonish.Map{"class_uid": json.Number("1")},
			status: ClassResolved,
			uid:    1,
			class:  class,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, uid, status := compiled.ResolveEventClass(test.event)
			require.Equal(t, test.status, status)
			require.Equal(t, test.uid, uid)
			require.Same(t, test.class, resolved)
		})
	}
}

func TestEventProfileSetAndAttributeActive(t *testing.T) {
	event := jsonish.Map{"metadata": jsonish.Map{
		"profiles": []any{"cloud", json.Number("1"), "ldap", "cloud"},
	}}
	profileSet := EventProfileSet(event)
	require.Equal(t, ProfileSet{"cloud": {}, "ldap": {}}, profileSet)
	require.True(t, AttributeActive(&ItemAttributeDefinition{}, profileSet))
	require.True(t, AttributeActive(&ItemAttributeDefinition{Profiles: []string{"ldap"}}, profileSet))
	require.False(t, AttributeActive(&ItemAttributeDefinition{Profiles: []string{"security_control"}}, profileSet))
	require.False(t, AttributeActive(nil, profileSet))
}

func TestEventProfileSetReturnsNilWithoutStringProfiles(t *testing.T) {
	tests := []jsonish.Map{
		nil,
		{},
		{"metadata": nil},
		{"metadata": jsonish.Map{}},
		{"metadata": jsonish.Map{"profiles": "cloud"}},
		{"metadata": jsonish.Map{"profiles": []any{json.Number("1"), nil}}},
	}
	for _, event := range tests {
		require.Nil(t, EventProfileSet(event))
	}
}

func TestEventProfileSetAllocationCeiling(t *testing.T) {
	event := jsonish.Map{"metadata": jsonish.Map{"profiles": []any{"cloud", "ldap"}}}
	allocations := testing.AllocsPerRun(1000, func() {
		if len(EventProfileSet(event)) != 2 {
			panic("unexpected profile set")
		}
	})
	require.LessOrEqual(t, allocations, float64(2))
}

func TestExpectedTypeUIDDetectsOverflow(t *testing.T) {
	value, ok := ExpectedTypeUID(1, 2)
	require.True(t, ok)
	require.Equal(t, int64(102), value)

	_, ok = ExpectedTypeUID(math.MaxInt64/100+1, 0)
	require.False(t, ok)
	_, ok = ExpectedTypeUID(math.MinInt64/100-1, 0)
	require.False(t, ok)
}

func TestEngineeringInvariantEnumSiblingEligibilityUsesDirectTypesAndMatchingShape(t *testing.T) {
	// Engineering invariant test: enum-sibling cache links use direct OCSF types and preserve scalar/array shape.
	attributes := map[string]*TypeDefinition{
		"enum_t":    {CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t"}},
		"caption_t": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"}},
	}
	compiled := &Compiled{Dictionary: &DictionaryDefinition{Types: &TypesDefinition{Attributes: attributes}}}
	require.False(t, compiled.EnumSiblingSupported(
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "enum_t"}},
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"}},
	))
	require.False(t, compiled.EnumSiblingSupported(
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t"}},
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "caption_t"}},
	))
	require.True(t, compiled.EnumSiblingSupported(
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t"}},
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"}},
	))
	require.True(t, compiled.EnumSiblingSupported(
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "long_t"}},
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"}},
	))

	array := true
	enumArray := &ItemAttributeDefinition{
		CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t", IsArray: &array},
	}
	captionArray := &ItemAttributeDefinition{
		CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t", IsArray: &array},
	}

	require.True(t, compiled.EnumSiblingSupported(enumArray, captionArray))
	require.True(t, compiled.EnumSiblingSupported(
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "long_t", IsArray: &array}},
		captionArray,
	))
	require.False(t, compiled.EnumSiblingSupported(
		enumArray,
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "caption_t"}},
	))
	require.False(t, compiled.EnumSiblingSupported(
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t"}},
		captionArray,
	))
	targetEnum := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: "string_t", Enum: map[string]*EnumDefinition{"one": {Caption: "One"}},
	}}
	require.False(t, compiled.EnumSiblingSupported(
		&ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t"}},
		targetEnum,
	))
}

func TestEngineeringInvariantEnumSiblingInitializationIssuesAreSpecificAndNonfatal(t *testing.T) {
	// Engineering invariant test: malformed enum-sibling declarations are ignored with stable, specific diagnostics.
	array := true
	tests := []struct {
		name       string
		attributes map[string]*ItemAttributeDefinition
		wantCodes  []string
	}{
		{
			name: "source subtype",
			attributes: enumSiblingTestAttributes(
				"enum_t", nil, "caption", "string_t", nil, false,
			),
			wantCodes: []string{"issue_at_init_schema_enum_sibling_source_not_integral"},
		},
		{
			name: "source string enum",
			attributes: enumSiblingTestAttributes(
				"string_t", nil, "caption", "string_t", nil, false,
			),
			wantCodes: []string{"issue_at_init_schema_enum_sibling_source_not_integral"},
		},
		{
			name: "target missing",
			attributes: enumSiblingTestAttributes(
				"integer_t", nil, "missing", "", nil, false,
			),
			wantCodes: []string{"issue_at_init_schema_enum_sibling_target_not_found"},
		},
		{
			name: "target subtype",
			attributes: enumSiblingTestAttributes(
				"integer_t", nil, "caption", "caption_t", nil, false,
			),
			wantCodes: []string{"issue_at_init_schema_enum_sibling_target_not_string"},
		},
		{
			name: "target shape mismatch",
			attributes: enumSiblingTestAttributes(
				"integer_t", &array, "caption", "string_t", nil, false,
			),
			wantCodes: []string{"issue_at_init_schema_enum_sibling_target_not_string"},
		},
		{
			name: "target enum takes precedence",
			attributes: enumSiblingTestAttributes(
				"integer_t", nil, "caption", "integer_t", &array, true,
			),
			wantCodes: []string{"issue_at_init_schema_enum_sibling_target_is_enum"},
		},
	}

	for _, itemKind := range []string{"class", "object"} {
		for _, test := range tests {
			t.Run(itemKind+" "+test.name, func(t *testing.T) {
				compiled, err := New(enumSiblingTestDefinition(itemKind, test.attributes))
				require.NoError(t, err)

				issues := compiled.InitializationIssues()
				codes := make([]string, len(issues))
				for index, found := range issues {
					codes[index] = found.Code.String()
				}
				require.Equal(t, test.wantCodes, codes)
				require.Nil(t, test.attributes["enum"].ResolvedEnumSibling)
			})
		}
	}
}

func TestEnumSiblingIrrelevantDeclarationsDoNotProduceInitializationIssues(t *testing.T) {
	siblingName := "caption"
	for name, attribute := range map[string]*ItemAttributeDefinition{
		"non-enum with sibling": {
			CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t", Sibling: &siblingName},
		},
		"ineligible enum without sibling": {
			CommonAttributeDefinition: CommonAttributeDefinition{
				Type: "string_t", Enum: map[string]*EnumDefinition{"one": {Caption: "One"}},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			compiled, err := New(enumSiblingTestDefinition("class", map[string]*ItemAttributeDefinition{
				"enum":    attribute,
				"caption": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"}},
			}))
			require.NoError(t, err)
			require.Empty(t, compiled.InitializationIssues())
		})
	}
}

func enumSiblingTestAttributes(
	sourceType string,
	sourceArray *bool,
	targetName string,
	targetType string,
	targetArray *bool,
	targetHasEnum bool,
) map[string]*ItemAttributeDefinition {
	attributes := map[string]*ItemAttributeDefinition{}
	attributes["enum"] = &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
		Type: sourceType, IsArray: sourceArray, Enum: map[string]*EnumDefinition{"1": {Caption: "One"}},
		Sibling: &targetName,
	}}
	if targetType != "" {
		target := &ItemAttributeDefinition{CommonAttributeDefinition: CommonAttributeDefinition{
			Type: targetType, IsArray: targetArray,
		}}
		if targetHasEnum {
			target.Enum = map[string]*EnumDefinition{"1": {Caption: "One"}}
		}
		attributes[targetName] = target
	}
	return attributes
}

func enumSiblingTestDefinition(itemKind string, attributes map[string]*ItemAttributeDefinition) *Definition {
	definition := &Definition{
		CompileVersion: expectedCompileVersion,
		Version:        "1.0.0",
		Classes: map[string]*ClassDefinition{
			"base": {Uid: 1, ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{}}},
		},
		Dictionary: &DictionaryDefinition{Types: &TypesDefinition{Attributes: map[string]*TypeDefinition{
			"enum_t":    {CommonAttributeDefinition: CommonAttributeDefinition{Type: "integer_t"}},
			"caption_t": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"}},
		}}},
	}
	if itemKind == "class" {
		definition.Classes["base"].Attributes = attributes
	} else {
		definition.Objects = map[string]*ObjectDefinition{
			"base": {ItemDefinition: ItemDefinition{Attributes: attributes}},
		}
	}
	return definition
}

func TestNewAllowsEnumSiblingPairWithDifferentProfiles(t *testing.T) {
	siblingName := "action"
	definition := &Definition{
		CompileVersion: expectedCompileVersion,
		Version:        "1.0.0",
		Classes: map[string]*ClassDefinition{
			"test": {
				Uid: 1,
				ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{
					"action_id": {
						CommonAttributeDefinition: CommonAttributeDefinition{
							Type:    "integer_t",
							Enum:    map[string]*EnumDefinition{"1": {Caption: "Start"}},
							Sibling: &siblingName,
						},
						Profiles: []string{"cloud"},
					},
					"action": {
						CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"},
					},
				}},
			},
		},
	}

	_, err := New(definition)

	require.NoError(t, err)
}

func TestLoadIgnoresEnumSiblingThatTargetsAnotherEnum(t *testing.T) {
	compiled, issues, err := Load("testdata/enum_sibling_dual_role.json")

	require.NoError(t, err)
	require.NotNil(t, compiled)
	require.Len(t, issues, 2)
	require.Equal(t, "issue_at_init_schema_enum_sibling_source_not_integral", issues[0].Code.String())
	require.Equal(t, "issue_at_init_schema_enum_sibling_target_is_enum", issues[1].Code.String())
}

func TestLoadAllowsEnumSiblingRelationshipsToVaryByItem(t *testing.T) {
	_, _, err := Load("testdata/enum_sibling_redirect.json")

	require.NoError(t, err)
}

func TestNewRejectsAmbiguousEnumSiblingRelationships(t *testing.T) {
	sharedSibling := "caption"
	tests := []struct {
		name       string
		attributes map[string]*ItemAttributeDefinition
		wantErr    string
	}{
		{
			name: "shared sibling",
			attributes: map[string]*ItemAttributeDefinition{
				"first_id": {
					CommonAttributeDefinition: CommonAttributeDefinition{
						Type:    "integer_t",
						Enum:    map[string]*EnumDefinition{"1": {Caption: "First"}},
						Sibling: &sharedSibling,
					},
				},
				"second_id": {
					CommonAttributeDefinition: CommonAttributeDefinition{
						Type:    "integer_t",
						Enum:    map[string]*EnumDefinition{"2": {Caption: "Second"}},
						Sibling: &sharedSibling,
					},
				},
				"caption": {CommonAttributeDefinition: CommonAttributeDefinition{Type: "string_t"}},
			},
			wantErr: `enum attributes "first_id" and "second_id" cannot share sibling "caption"`,
		},
	}

	for _, itemKind := range []string{"class", "object"} {
		for _, test := range tests {
			t.Run(itemKind+" "+test.name, func(t *testing.T) {
				definition := &Definition{
					CompileVersion: expectedCompileVersion,
					Version:        "1.0.0",
					Classes: map[string]*ClassDefinition{
						"base": {
							Uid:            1,
							ItemDefinition: ItemDefinition{Attributes: map[string]*ItemAttributeDefinition{}},
						},
					},
				}
				itemName := "base"
				if itemKind == "class" {
					definition.Classes[itemName].Attributes = test.attributes
				} else {
					itemName = "test"
					definition.Objects = map[string]*ObjectDefinition{
						itemName: {ItemDefinition: ItemDefinition{Attributes: test.attributes}},
					}
				}

				_, err := New(definition)

				require.EqualError(t, err, "compiled schema "+itemKind+` "`+itemName+`" `+test.wantErr)
			})
		}
	}
}
