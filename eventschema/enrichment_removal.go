package eventschema

import (
	"strconv"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

const issuePhaseEnrichmentRemoval = "enrichment_removal"

type enrichmentRemovalProcessor struct {
	eventProcessVisitorBase
	config enrichmentRemovalConfig
}

func (p *enrichmentRemovalProcessor) onClass(context *processingContext, visit classVisit) {
	if visit.status != classVisitResolved {
		if p.config.removeObservables && p.config.forceRemoveObservables {
			p.forceRemoveObservablesWithoutAnalysis(context, visit.event)
		}
		return
	}
	// Analyze and remove observables before other event mutation so references to removable enum
	// siblings are evaluated against the original event. Validation then walks only retained entries.
	p.removeObservables(context, visit.event)
	p.removeEnumSiblingsFromItem(context, visit.event, &context.class.commonItemDefinition)
}

func (p *enrichmentRemovalProcessor) onObject(context *processingContext, visit objectVisit) {
	if visit.status == objectVisitValid {
		p.removeEnumSiblingsFromItem(context, visit.objectValue, &visit.objectDef.commonItemDefinition)
	}
}

func (p *enrichmentRemovalProcessor) removeEnumSiblingsFromItem(
	context *processingContext,
	item jsonish.Map,
	itemDefinition *commonItemDefinition,
) {
	if !p.config.removeEnumSiblings || itemDefinition == nil {
		return
	}
	attributes := context.filterAttributes(itemDefinition.Attributes)
	for attributeName, attrDef := range attributes {
		if attrDef == nil || attrDef.Enum == nil || attrDef.Sibling == nil {
			continue
		}
		siblingName := *attrDef.Sibling
		siblingValue, siblingPresent := item[siblingName]
		if !siblingPresent {
			continue
		}
		if !context.supportedEnumSibling(attrDef, attributes[siblingName]) {
			context.result.EnrichmentRemoval.EnumSiblingsRetained++
			continue
		}
		siblingString, ok := siblingValue.(string)
		if !ok {
			context.result.EnrichmentRemoval.EnumSiblingsRetained++
			continue
		}
		enumID, ok := getInt64Value(item[attributeName])
		if !ok || enumID == 99 {
			context.result.EnrichmentRemoval.EnumSiblingsRetained++
			continue
		}
		if !p.config.forceRemoveEnumSiblings {
			enumDetail := attrDef.Enum[strconv.FormatInt(enumID, 10)]
			if enumDetail == nil || siblingString != enumDetail.Caption {
				context.result.EnrichmentRemoval.EnumSiblingsRetained++
				continue
			}
		}

		delete(item, siblingName)
		context.result.EnrichmentRemoval.EnumSiblingsRemoved++
	}
}

func (p *enrichmentRemovalProcessor) removeObservables(context *processingContext, event jsonish.Map) {
	if !p.config.removeObservables {
		return
	}
	value, present := event["observables"]
	if !present {
		return
	}

	resolution := context.resolveObservables(event)
	for _, entry := range resolution.entries {
		if entry.diagnostic != nil {
			context.addProcessorIssue(issuePhaseEnrichmentRemoval, entry.diagnostic)
		}
	}

	observables, isArray := asSlice(value)
	if p.config.forceRemoveObservables {
		if isArray {
			context.result.EnrichmentRemoval.ObservablesRemoved += len(observables)
		}
		for index := range resolution.entries {
			resolution.entries[index].removed = true
		}
		delete(event, "observables")
		return
	}
	if !isArray {
		context.result.EnrichmentRemoval.ObservablesRetained++
		return
	}

	remove := make(map[int]struct{})
	for index := range resolution.entries {
		entry := &resolution.entries[index]
		if entry.removable {
			remove[entry.index] = struct{}{}
			entry.removed = true
		}
	}
	context.result.EnrichmentRemoval.ObservablesRemoved += len(remove)
	context.result.EnrichmentRemoval.ObservablesRetained += len(observables) - len(remove)
	if len(remove) == len(observables) {
		delete(event, "observables")
		return
	}
	event["observables"] = filterObservableSlice(value, remove)
}

func (p *enrichmentRemovalProcessor) forceRemoveObservablesWithoutAnalysis(
	context *processingContext,
	event jsonish.Map,
) {
	value, present := event["observables"]
	if !present {
		return
	}
	if observables, ok := asSlice(value); ok {
		context.result.EnrichmentRemoval.ObservablesRemoved += len(observables)
	}
	delete(event, "observables")
}

func (c *processingContext) supportedEnumSibling(
	attrDef *itemAttributeDefinition,
	siblingDef *itemAttributeDefinition,
) bool {
	if attrDef.IsArray != nil && *attrDef.IsArray {
		return false
	}
	if siblingDef == nil || siblingDef.IsArray != nil && *siblingDef.IsArray ||
		!c.typeDerivedFrom(siblingDef.Type, "string_t") {
		return false
	}
	return c.typeDerivedFrom(attrDef.Type, "integer_t") || c.typeDerivedFrom(attrDef.Type, "long_t")
}

func (c *processingContext) typeDerivedFrom(typeName, baseType string) bool {
	seen := make(map[string]struct{})
	for typeName != "" {
		if typeName == baseType {
			return true
		}
		if _, duplicate := seen[typeName]; duplicate {
			return false
		}
		seen[typeName] = struct{}{}
		typeDef := c.dictionary.Types.Attributes[typeName]
		if typeDef == nil {
			return false
		}
		typeName = typeDef.Type
	}
	return false
}

func filterObservableSlice(value any, remove map[int]struct{}) any {
	switch values := value.(type) {
	case []jsonish.Map:
		filtered := make([]jsonish.Map, 0, len(values)-len(remove))
		for index, element := range values {
			if _, removed := remove[index]; !removed {
				filtered = append(filtered, element)
			}
		}
		return filtered
	case []any:
		filtered := make([]any, 0, len(values)-len(remove))
		for index, element := range values {
			if _, removed := remove[index]; !removed {
				filtered = append(filtered, element)
			}
		}
		return filtered
	default:
		return value
	}
}
