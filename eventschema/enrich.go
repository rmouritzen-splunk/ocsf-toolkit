package eventschema

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ocsf/ocsf-processor/internal/coerce"
	"github.com/ocsf/ocsf-processor/jsonish"
)

var errClassUIDMissing = errors.New("event \"uid\" (class uid) is missing")

type enrichmentContext struct {
	*schemaImpl
	addEnumSiblings  bool
	addObservables   bool
	classObservables map[string]int32
	observables      []jsonish.Map
}

func (si *schemaImpl) Enrich(event jsonish.Map, addEnumSiblings, addObservables bool) error {
	if !addEnumSiblings && !addObservables {
		// nothing to do
		return nil
	}
	classUID, present, err := getAsInt32(event, "class_uid")
	if err != nil {
		return fmt.Errorf("failed getting event \"class_uid\" value: %w", err)
	}
	if !present {
		return errClassUIDMissing
	}
	class, classPresent := si.classes[classUID]
	if !classPresent {
		return fmt.Errorf("event \"class_uid\" value is not defined in schema version %s: %d", si.version, classUID)
	}

	context := &enrichmentContext{
		schemaImpl:       si,
		addEnumSiblings:  addEnumSiblings,
		addObservables:   addObservables,
		classObservables: class.Observables,
	}

	context.enrich("", event, &class.commonItemDefinition)

	if addObservables && len(context.observables) > 0 {
		event["observables"] = context.observables
	}

	return nil
}

func getAsInt32(m jsonish.Map, key string) (int32, bool, error) {
	v, present := m[key]
	if !present {
		return 0, false, nil
	}

	switch v := v.(type) {
	case int32:
		return v, true, nil
	case int:
		return int32(v), true, nil
	case int64:
		return int32(v), true, nil
	case float64:
		return int32(v), true, nil
	case json.Number:
		if i, err := v.Int64(); err != nil {
			return 0, true, err
		} else {
			return int32(i), true, nil
		}
	default:
		return 0, true, fmt.Errorf("unhandled type: %T", v)
	}
}

func (c *enrichmentContext) enrich(
	parentAttributePath string,
	item jsonish.Map,
	itemDefinition *commonItemDefinition,
) {
	for key, value := range item {
		attributePath := makeAttributePath(parentAttributePath, key)

		attrDef, attrDefPresent := itemDefinition.Attributes[key]
		if attrDefPresent && attrDef != nil && attrDef.Type != "json_t" {
			if attrDef.Enum != nil {
				if c.addEnumSiblings {
					c.addEnumSibling(item, value, attrDef)
				}
			} else {
				// TODO: Confirm that class-level attribute path observables are handled for all situations
				switch value := value.(type) {
				case jsonish.Map:
					c.enrichObjectByName(attributePath, value, attrDef.ObjectType)
				case []any: // []any happens with generic json decode
					c.enrichArrayOfAny(attributePath, key, value, attrDef)
				case []jsonish.Map: // []jsonish.Map happens with unit test code
					c.enrichArrayOfMap(attributePath, value, attrDef)
				default:
					// This is a primitive type (or some random type, and those will end up being ignored)
					if c.addObservables {
						typeID, present := c.getObservableTypeID(attributePath, key, attrDef)
						if present {
							c.addValueObservable(attributePath, typeID, value)
						}
					}
				}
			}
		}
	}
}

func makeAttributePath(parentAttributePath, attribute string) string {
	if parentAttributePath == "" {
		return attribute
	} else {
		return parentAttributePath + "." + attribute
	}
}

func (c *enrichmentContext) addEnumSibling(item jsonish.Map, enumValue any, attrDef *itemAttributeDefinition) {
	if c.addEnumSiblings && attrDef != nil && attrDef.Enum != nil && attrDef.Sibling != nil {
		sibling := *attrDef.Sibling
		_, siblingPresent := item[sibling]
		if !siblingPresent {
			valueStr := coerce.StringLenient(enumValue)
			if valueStr != "" {
				enumDetail, present := (attrDef.Enum)[valueStr]
				if present && enumDetail != nil && enumDetail.Caption != "" {
					item[sibling] = enumDetail.Caption
				}
			}
		}
	}
}

func (c *enrichmentContext) enrichObjectByName(attributePath string, item jsonish.Map, objectName *string) {
	if objectName != nil {
		objectDef, present := c.objects[*objectName]
		if present && objectDef != nil {
			c.enrichObject(attributePath, item, objectDef)
		}
	}
}

func (c *enrichmentContext) enrichObject(attributePath string, item jsonish.Map, objectDef *objectDefinition) {
	if objectDef != nil {
		if c.addObservables && objectDef.Observable != nil {
			c.addObjectObservable(attributePath, *objectDef.Observable)
		}
		c.enrich(attributePath, item, &objectDef.commonItemDefinition)
	}
}

func (c *enrichmentContext) enrichArrayOfAny(
	attributePath string,
	attribute string,
	array []any,
	attrDef *itemAttributeDefinition,
) {
	if attrDef.IsArray != nil && *attrDef.IsArray {
		arrayOfObject, converted := optionalCastToArrayOfObject(array)
		if converted {
			c.enrichArrayOfMap(attributePath, arrayOfObject, attrDef)
		} else {
			// At this point we know we have an array of primitives
			// (or an element is itself an array like [1, 2, [3, 4]], but this does not normally occur in OCSF events)
			if c.addObservables {
				typeID, present := c.getObservableTypeID(attributePath, attribute, attrDef)
				if present {
					for _, value := range array {
						c.addValueObservable(attributePath, typeID, value)
					}
				}
			}
		}
	}
}

func optionalCastToArrayOfObject(array []any) ([]jsonish.Map, bool) {
	var arrayOfObject []jsonish.Map
	for _, element := range array {
		switch element := element.(type) {
		case jsonish.Map:
			if arrayOfObject == nil {
				// pre-allocate arrayOfObject for efficiency
				arrayOfObject = make([]jsonish.Map, 0, len(array))
			}
			arrayOfObject = append(arrayOfObject, element)
		default:
			return nil, false // all elements need to a map
		}
	}
	return arrayOfObject, true
}

func (c *enrichmentContext) enrichArrayOfMap(
	attributePath string,
	array []jsonish.Map,
	attrDef *itemAttributeDefinition,
) {
	if attrDef.IsArray != nil && *attrDef.IsArray && attrDef.ObjectType != nil {
		objectDef, present := c.objects[*attrDef.ObjectType]
		if present && objectDef != nil {
			for _, element := range array {
				c.enrichObject(attributePath, element, objectDef)
			}
		}
	}
}

func (c *enrichmentContext) getObservableTypeID(
	attributePath string,
	attribute string,
	attrDef *itemAttributeDefinition,
) (int32, bool) {
	// First, look for observable type_id by attribute type
	if attrDef != nil {
		typeDef, present := c.dictionary.Types.Attributes[attrDef.Type]
		if present && typeDef != nil && typeDef.Observable != nil {
			return *typeDef.Observable, true
		}
	}
	// Second, look for observable type_id by dictionary attribute
	dictAttrDef, dictAttrPresent := c.dictionary.Attributes[attribute]
	if dictAttrPresent && dictAttrDef != nil && dictAttrDef.Observable != nil {
		return *dictAttrDef.Observable, true
	}
	// Third, look for observable type_id by type-specific attribute (class / object specific)
	if attrDef != nil && attrDef.Observable != nil {
		return *attrDef.Observable, true
	}
	// Fourth, look for observable by class-path attribute path
	if c.classObservables != nil {
		typeID, present := c.classObservables[attributePath]
		return typeID, present
	}
	return 0, false
}

func (c *enrichmentContext) addObjectObservable(attributePath string, observableTypeID int32) {
	if c.addEnumSiblings {
		typeStr, present := c.observableTypes[observableTypeID]
		if present {
			observable := jsonish.Map{
				"name":    attributePath,
				"type":    typeStr,
				"type_id": observableTypeID,
			}
			c.observables = append(c.observables, observable)
			return
		}
	}
	observable := jsonish.Map{
		"name":    attributePath,
		"type_id": observableTypeID,
	}
	c.observables = append(c.observables, observable)
}

func (c *enrichmentContext) addValueObservable(attributePath string, observableTypeID int32, value any) {
	valueStr := coerce.StringLenient(value)
	// don't add failed conversions and actual blank strings
	if valueStr != "" {
		if c.addEnumSiblings {
			typeStr, present := c.observableTypes[observableTypeID]
			if present {
				observable := jsonish.Map{
					"name":    attributePath,
					"type":    typeStr,
					"type_id": observableTypeID,
					"value":   valueStr,
				}
				c.observables = append(c.observables, observable)
				return
			}
		}
		observable := jsonish.Map{
			"name":    attributePath,
			"type_id": observableTypeID,
			"value":   valueStr,
		}
		c.observables = append(c.observables, observable)
	}
}
