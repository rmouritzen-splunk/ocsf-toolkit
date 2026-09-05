package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
