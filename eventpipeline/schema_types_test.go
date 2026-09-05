package eventpipeline

import "github.com/ocsf/ocsf-toolkit/internal/schema"

type deprecatedDefinition = schema.DeprecatedDefinition
type enumDefinition = schema.EnumDefinition
type commonAttributeDefinition = schema.CommonAttributeDefinition
type itemAttributeDefinition = schema.ItemAttributeDefinition
type commonItemDefinition = schema.ItemDefinition
type classDefinition = schema.ClassDefinition
type objectDefinition = schema.ObjectDefinition
type typeDefinition = schema.TypeDefinition
type typesDefinition = schema.TypesDefinition
type dictionaryDefinition = schema.DictionaryDefinition
type profileDefinition = schema.ProfileDefinition
type schemaDefinition = schema.Definition

func (s *Schema) compiledForTest() *schema.Compiled {
	return s.compiled
}
