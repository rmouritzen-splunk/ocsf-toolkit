package eventschema

import (
	"github.com/ocsf/ocsf-toolkit/internal/coerce"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

type enrichmentProcessor struct {
	eventProcessVisitorBase
	config enrichmentConfig
}

func (p *enrichmentProcessor) onObject(
	context *processingContext,
	visit objectVisit,
) {
	if p.config.addObservables && visit.status == objectVisitValid && visit.objectDef.Observable != nil {
		context.addObjectObservable(visit.enrichmentPath, *visit.objectDef.Observable)
	}
}

func (p *enrichmentProcessor) onAttribute(
	context *processingContext,
	visit attributeVisit,
) {
	if visit.status == attributeVisitEnum {
		p.addEnumSibling(context, visit)
		return
	}
	if visit.status != attributeVisitPrimitive || visit.attrDef.Enum != nil || !p.config.addObservables {
		return
	}
	if typeID, present := context.getObservableTypeID(visit.enrichmentPath, visit.attributeName, visit.attrDef); present {
		context.addValueObservable(visit.enrichmentPath, typeID, visit.value)
	}
}

func (p *enrichmentProcessor) addEnumSibling(context *processingContext, visit attributeVisit) {
	if !p.config.addEnumSiblings || visit.arrayIndex >= 0 {
		return
	}
	valueString := coerce.StringLenient(visit.value)
	enumDetail := visit.attrDef.Enum[valueString]
	context.addEnumSibling(visit.item, valueString, enumDetail, visit.attrDef)
}

func (p *enrichmentProcessor) onEventDone(context *processingContext, event jsonish.Map) {
	if p.config.addObservables && len(context.observables) > 0 {
		event["observables"] = context.observables
	}
}

func (c *processingContext) addEnumSiblings() bool {
	return c.enrichment != nil && c.enrichment.config.addEnumSiblings
}

func (c *processingContext) addEnumSibling(
	item jsonish.Map,
	valueString string,
	enumDetail *enumDefinition,
	attrDef *itemAttributeDefinition,
) {
	if attrDef == nil || attrDef.Sibling == nil || enumDetail == nil || enumDetail.Caption == "" {
		return
	}
	sibling := *attrDef.Sibling
	if _, siblingPresent := item[sibling]; siblingPresent {
		return
	}
	if valueString == "" {
		return
	}
	item[sibling] = enumDetail.Caption
	c.result.Enrichment.EnumSiblingsAdded++
}

func (c *processingContext) getObservableTypeID(
	attributePath string,
	attribute string,
	attrDef *itemAttributeDefinition,
) (int64, bool) {
	if attrDef != nil {
		typeDef, present := c.dictionary.Types.Attributes[attrDef.Type]
		if present && typeDef != nil && typeDef.Observable != nil {
			return *typeDef.Observable, true
		}
	}
	dictAttrDef, dictAttrPresent := c.dictionary.Attributes[attribute]
	if dictAttrPresent && dictAttrDef != nil && dictAttrDef.Observable != nil {
		return *dictAttrDef.Observable, true
	}
	if attrDef != nil && attrDef.Observable != nil {
		return *attrDef.Observable, true
	}
	if c.classObservables != nil {
		typeID, present := c.classObservables[attributePath]
		return typeID, present
	}
	return 0, false
}

func (c *processingContext) addObjectObservable(attributePath string, observableTypeID int64) {
	observable := jsonish.Map{
		"name":    attributePath,
		"type_id": observableTypeID,
	}
	if c.addEnumSiblings() {
		if typeStr, present := c.observableTypes[observableTypeID]; present {
			observable["type"] = typeStr
		}
	}
	c.observables = append(c.observables, observable)
	c.result.Enrichment.ObservablesAdded++
}

func (c *processingContext) addValueObservable(attributePath string, observableTypeID int64, value any) {
	valueStr := coerce.StringLenient(value)
	if valueStr == "" {
		return
	}
	observable := jsonish.Map{
		"name":    attributePath,
		"type_id": observableTypeID,
		"value":   valueStr,
	}
	if c.addEnumSiblings() {
		if typeStr, present := c.observableTypes[observableTypeID]; present {
			observable["type"] = typeStr
		}
	}
	c.observables = append(c.observables, observable)
	c.result.Enrichment.ObservablesAdded++
}
