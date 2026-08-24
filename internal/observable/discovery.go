package observable

import "github.com/ocsf/ocsf-toolkit/internal/schema"

// TypeID resolves the observable type for an event attribute using OCSF override precedence.
func TypeID(
	compiled *schema.Compiled,
	attribute string,
	attrDef *schema.ItemAttributeDefinition,
) (int64, bool) {
	if compiled == nil {
		return 0, false
	}
	if attrDef != nil && compiled.Dictionary != nil && compiled.Dictionary.Types != nil {
		typeDef := compiled.Dictionary.Types.Attributes[attrDef.Type]
		if typeDef != nil && typeDef.Observable != nil {
			return *typeDef.Observable, true
		}
	}
	if compiled.Dictionary != nil {
		dictAttrDef := compiled.Dictionary.Attributes[attribute]
		if dictAttrDef != nil && dictAttrDef.Observable != nil {
			return *dictAttrDef.Observable, true
		}
	}
	if attrDef != nil && attrDef.Observable != nil {
		return *attrDef.Observable, true
	}
	return 0, false
}

// ObjectObservability caches, per object definition, whether any of its descendant attributes may generate an
// observable under a fixed typeIDAllowed. Build it once via CompileObjectObservability and pass the same instance
// into every AttributeMayGenerate call sharing that (compiled, typeIDAllowed) pair; passing nil is also valid and
// falls back to computing the answer for that one call without caching it.
type ObjectObservability map[*schema.ObjectDefinition]bool

// CompileObjectObservability precomputes, for every object definition in compiled, whether it or a schema-defined
// descendant may generate an observable under typeIDAllowed. This is a reachability problem: an object definition
// qualifies if it can reach, via object_t attributes, some definition that is directly observable. It is solved by
// seeding every directly observable definition as true and propagating that backward along reversed object_t edges
// to their referencing definitions, which is correct for any object graph, including cyclic ones, without ever
// needing to memoize a value discovered only partway through a traversal (the bug this replaced).
// The complete graph algorithm intentionally remains in one function so its construction-time invariants stay
// visible; it runs while building a pipeline and adds no per-event cost.
func CompileObjectObservability(compiled *schema.Compiled, typeIDAllowed func(int64) bool) ObjectObservability {
	if compiled == nil {
		return nil
	}
	result := make(ObjectObservability, len(compiled.Objects))
	referrers := make(map[*schema.ObjectDefinition][]*schema.ObjectDefinition)
	pending := make([]*schema.ObjectDefinition, 0, len(compiled.Objects))

	for _, objectDef := range compiled.Objects {
		if objectDef == nil {
			continue
		}
		direct := false
		for attributeName, attrDef := range objectDef.Attributes {
			if attrDef == nil {
				continue
			}
			if attrDef.Type != "object_t" || attrDef.ObjectType == nil {
				typeID, present := TypeID(compiled, attributeName, attrDef)
				if present && observableTypeAllowed(typeID, typeIDAllowed) {
					direct = true
				}
				continue
			}
			nested := compiled.Objects[*attrDef.ObjectType]
			if nested == nil {
				continue
			}
			if directlyObservable(compiled, attributeName, attrDef, nested, typeIDAllowed) {
				direct = true
				continue
			}
			referrers[nested] = append(referrers[nested], objectDef)
		}
		if direct {
			result[objectDef] = true
			pending = append(pending, objectDef)
		}
	}

	for len(pending) > 0 {
		last := len(pending) - 1
		objectDef := pending[last]
		pending = pending[:last]
		for _, referrer := range referrers[objectDef] {
			if result[referrer] {
				continue
			}
			result[referrer] = true
			pending = append(pending, referrer)
		}
	}

	return result
}

// AttributeMayGenerate reports whether an object attribute or any schema-defined descendant can generate an
// observable. objectObservability may be nil; pass a cache built by CompileObjectObservability to avoid re-walking
// the object graph on every call.
func AttributeMayGenerate(
	compiled *schema.Compiled,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	typeIDAllowed func(int64) bool,
	objectObservability ObjectObservability,
) bool {
	if compiled == nil || attrDef == nil || attrDef.ObjectType == nil {
		return false
	}
	objectDef := compiled.Objects[*attrDef.ObjectType]
	if objectDef == nil {
		return false
	}
	if directlyObservable(compiled, attributeName, attrDef, objectDef, typeIDAllowed) {
		return true
	}
	if objectObservability != nil {
		return objectObservability[objectDef]
	}
	return objectMayGenerate(compiled, objectDef, typeIDAllowed)
}

// directlyObservable reports whether the object_t attribute attrDef (named attributeName, referencing objectDef via
// its object_type) is itself observable, without considering objectDef's own attributes: either objectDef is
// directly tagged observable, or the attribute's own dictionary/type override is.
func directlyObservable(
	compiled *schema.Compiled,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	objectDef *schema.ObjectDefinition,
	typeIDAllowed func(int64) bool,
) bool {
	if objectDef.Observable != nil {
		return observableTypeAllowed(*objectDef.Observable, typeIDAllowed)
	}
	typeID, present := TypeID(compiled, attributeName, attrDef)
	return present && observableTypeAllowed(typeID, typeIDAllowed)
}

// objectMayGenerate performs a one-shot forward search for whether objectDef or a descendant reachable through it is
// observable. Used only when the caller has no ObjectObservability cache: unlike CompileObjectObservability, it
// answers a single query, so a plain DFS marking each definition visited on entry (and never unmarking it) is
// sufficient to break cycles without a whole-graph precomputation.
func objectMayGenerate(
	compiled *schema.Compiled,
	objectDef *schema.ObjectDefinition,
	typeIDAllowed func(int64) bool,
) bool {
	visited := make(map[*schema.ObjectDefinition]struct{})
	var visit func(*schema.ObjectDefinition) bool
	visit = func(objectDef *schema.ObjectDefinition) bool {
		if _, done := visited[objectDef]; done {
			return false
		}
		visited[objectDef] = struct{}{}
		for attributeName, attrDef := range objectDef.Attributes {
			if attrDef == nil {
				continue
			}
			if attrDef.Type != "object_t" || attrDef.ObjectType == nil {
				typeID, present := TypeID(compiled, attributeName, attrDef)
				if present && observableTypeAllowed(typeID, typeIDAllowed) {
					return true
				}
				continue
			}
			nested := compiled.Objects[*attrDef.ObjectType]
			if nested == nil {
				continue
			}
			if directlyObservable(compiled, attributeName, attrDef, nested, typeIDAllowed) || visit(nested) {
				return true
			}
		}
		return false
	}
	return visit(objectDef)
}

func observableTypeAllowed(typeID int64, allowed func(int64) bool) bool {
	return allowed == nil || allowed(typeID)
}
