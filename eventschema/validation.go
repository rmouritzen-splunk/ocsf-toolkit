package eventschema

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ocsf/ocsf-processor/internal/coerce"
	"github.com/ocsf/ocsf-processor/jsonish"
)

const (
	issuePhaseValidation = "validation"
	issueSeverityError   = "error"
	issueSeverityWarning = "warning"
)

type validationProcessor struct {
	config validationConfig
}

func (p *validationProcessor) onClass(context *processingContext, visit classVisit) {
	switch visit.status {
	case classVisitUIDMissing:
		context.addRequiredAttributeMissing("class_uid", "class_uid")
		return
	case classVisitUIDWrongType:
		context.addWrongType("class_uid", "class_uid", visit.event["class_uid"], "integer_t", "")
		return
	case classVisitUIDUnknown:
		context.addError(
			"class_uid_unknown",
			fmt.Sprintf("Unknown \"class_uid\" value; no class is defined for %d.", visit.classUID),
			jsonish.Map{
				"attribute_path": "class_uid",
				"attribute":      "class_uid",
				"value":          visit.event["class_uid"],
			},
		)
		return
	case classVisitResolved:
	default:
		return
	}

	if context.class.Deprecated != nil {
		context.addWarning(
			"class_deprecated",
			fmt.Sprintf(
				"Class %q uid %d is deprecated. %s",
				context.class.Name,
				context.class.Uid,
				context.class.Deprecated.Message,
			),
			jsonish.Map{
				"class_uid":  context.class.Uid,
				"class_name": context.class.Name,
				"since":      context.class.Deprecated.Since,
			},
		)
	}

	metadata, ok := visit.event["metadata"].(jsonish.Map)
	if !ok {
		return
	}
	profilesValue := metadata["profiles"]
	profiles, ok := asSlice(profilesValue)
	if profilesValue == nil || !ok {
		return
	}
	for index, profileValue := range profiles {
		profile, ok := profileValue.(string)
		if !ok {
			continue
		}
		if _, present := context.profiles[profile]; present {
			continue
		}
		attributePath := makeArrayElementPath("metadata.profiles", index)
		context.addError(
			"profile_unknown",
			fmt.Sprintf(
				"Unknown profile at %q; no profile is defined for %q.",
				attributePath,
				profile,
			),
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      "profiles",
				"value":          profile,
			},
		)
	}
}

func (p *validationProcessor) onClassDone(context *processingContext, visit itemVisit) {
	context.validateUnknownKeys(visit.item, visit.validationParentPath, visit.itemDefinition, visit.filteredAttributes)
}

func (p *validationProcessor) onObjectDone(context *processingContext, visit itemVisit) {
	context.validateUnknownKeys(visit.item, visit.validationParentPath, visit.itemDefinition, visit.filteredAttributes)
	context.validateConstraints(visit.item, visit.itemDefinition, visit.validationParentPath)
}

func (p *validationProcessor) onAttribute(
	context *processingContext,
	visit attributeVisit,
) {
	switch visit.status {
	case attributeVisitMissing:
		context.validateRequirement(visit.validationPath, visit.attributeName, visit.attrDef)
		return
	case attributeVisitArrayWrongType:
		context.addWrongType(visit.validationPath, visit.attributeName, visit.value, "array of "+visit.attrDef.Type, "")
		return
	case attributeVisitEnum:
		context.validateEnum(visit)
		return
	case attributeVisitPrimitive:
		context.validatePrimitiveValue(visit.value, visit.validationPath, visit.attributeName, visit.attrDef)
		return
	case attributeVisitPresent:
	default:
		return
	}

	if visit.attrDef.Deprecated == nil {
		return
	}
	context.addWarning(
		"attribute_deprecated",
		fmt.Sprintf("Attribute %q is deprecated. %s", visit.attributeName, visit.attrDef.Deprecated.Message),
		jsonish.Map{
			"attribute_path": visit.validationPath,
			"attribute":      visit.attributeName,
			"since":          visit.attrDef.Deprecated.Since,
		},
	)
}

func (c *processingContext) validateEnum(visit attributeVisit) {
	valueString := coerce.StringLenient(visit.value)
	enumDetail, enumPresent := visit.attrDef.Enum[valueString]
	if !enumPresent || enumDetail == nil {
		code := "attribute_enum_value_unknown"
		message := fmt.Sprintf(
			"Unknown enum value at %q; value %v is not defined for enum %q.",
			visit.validationPath,
			visit.value,
			visit.attributeName,
		)
		if visit.arrayIndex >= 0 {
			code = "attribute_enum_array_value_unknown"
			message = fmt.Sprintf(
				"Unknown enum array value at %q; value %v is not defined for enum %q.",
				visit.validationPath,
				visit.value,
				visit.attributeName,
			)
		}
		c.addError(
			code,
			message,
			jsonish.Map{
				"attribute_path": visit.validationPath,
				"attribute":      visit.attributeName,
				"value":          visit.value,
			},
		)
		return
	}
	if visit.arrayIndex >= 0 {
		c.validateEnumArraySibling(visit.item, visit.value, visit.validationPath, visit.attrDef, enumDetail, visit.arrayIndex)
		c.validateEnumArrayValueDeprecated(visit.value, visit.validationPath, visit.attributeName, enumDetail)
		return
	}
	c.validateEnumSibling(visit.item, visit.value, visit.validationPath, visit.attrDef, enumDetail)
	c.validateEnumValueDeprecated(visit.value, visit.validationPath, visit.attributeName, enumDetail)
}

func (p *validationProcessor) onObject(
	context *processingContext,
	visit objectVisit,
) {
	switch visit.status {
	case objectVisitWrongType:
		expectedType := "object"
		if visit.attrDef.ObjectType != nil {
			expectedType = *visit.attrDef.ObjectType + " (object)"
		}
		context.addWrongType(visit.validationPath, visit.attributeName, visit.value, expectedType, "")
		return
	case objectVisitSchemaMissing:
		context.addError(
			"schema_bug_object_missing",
			fmt.Sprintf("SCHEMA BUG: Object %q is not defined.", visit.objectType),
			jsonish.Map{
				"attribute_path": visit.validationPath,
				"attribute":      visit.attributeName,
				"object_type":    visit.objectType,
				"value":          visit.value,
			},
		)
		return
	case objectVisitValid:
	default:
		return
	}

	if visit.objectDef.Deprecated == nil {
		return
	}
	context.addWarning(
		"object_deprecated",
		fmt.Sprintf("Object %q is deprecated. %s", visit.objectDef.Name, visit.objectDef.Deprecated.Message),
		jsonish.Map{
			"attribute_path": visit.validationPath,
			"attribute":      visit.attributeName,
			"object_name":    visit.objectDef.Name,
			"since":          visit.objectDef.Deprecated.Since,
		},
	)
}

func (p *validationProcessor) onEventDone(context *processingContext, event jsonish.Map) {
	context.validateVersion(event)
	context.validateTypeUID(event)
	context.validateConstraints(event, &context.class.commonItemDefinition, "")
	context.validateObservables(event)
}

func (c *processingContext) warnOnMissingRecommended() bool {
	return c.validation != nil && c.validation.config.warnOnMissingRecommended
}

func (c *processingContext) resolveClass(event jsonish.Map) {
	classUID, present, ok := getInt64(event["class_uid"])
	if !present {
		c.visitClass(classVisit{event: event, status: classVisitUIDMissing})
		c.stopped = true
		return
	}
	if !ok {
		c.visitClass(classVisit{event: event, status: classVisitUIDWrongType})
		c.stopped = true
		return
	}

	class, classPresent := c.classes[classUID]
	if !classPresent {
		c.visitClass(classVisit{event: event, classUID: classUID, status: classVisitUIDUnknown})
		c.stopped = true
		return
	}

	c.class = class
	c.classObservables = class.Observables
}

func (c *processingContext) validateAndReturnProfiles(event jsonish.Map) []string {
	metadata, ok := event["metadata"].(jsonish.Map)
	if !ok {
		return nil
	}
	profilesValue := metadata["profiles"]
	profiles, ok := asSlice(profilesValue)
	if profilesValue == nil || !ok {
		return nil
	}

	result := make([]string, 0, len(profiles))
	for _, profileValue := range profiles {
		profile, ok := profileValue.(string)
		if !ok {
			continue
		}
		result = append(result, profile)
	}
	return result
}

func (c *processingContext) validateRequirement(
	attributePath string,
	attributeName string,
	attrDef *itemAttributeDefinition,
) {
	if attrDef == nil {
		return
	}
	switch attrDef.Requirement {
	case "required":
		c.addRequiredAttributeMissing(attributePath, attributeName)
	case "recommended":
		if c.warnOnMissingRecommended() {
			c.addWarning(
				"attribute_recommended_missing",
				fmt.Sprintf("Recommended attribute %q is missing.", attributePath),
				jsonish.Map{
					"attribute_path": attributePath,
					"attribute":      attributeName,
				},
			)
		}
	}
}

func (c *processingContext) validateEnumSibling(
	item jsonish.Map,
	value any,
	validationPath string,
	attrDef *itemAttributeDefinition,
	enumDetail *enumDefinition,
) {
	if attrDef.Sibling == nil {
		return
	}
	siblingName := *attrDef.Sibling
	siblingValue, siblingPresent := item[siblingName]
	if !siblingPresent {
		return
	}

	siblingPath := makeAttributePath(parentPath(validationPath), siblingName)
	if isOtherEnumValue(value) {
		if enumDetail.Caption == siblingValue {
			c.addWarning(
				"attribute_enum_sibling_suspicious_other",
				fmt.Sprintf(
					"Attribute %q enum sibling value %v suspiciously matches the caption of enum %q value 99 (%q).",
					siblingPath,
					siblingValue,
					validationPath,
					enumDetail.Caption,
				),
				jsonish.Map{
					"attribute_path": siblingPath,
					"attribute":      siblingName,
					"value":          siblingValue,
				},
			)
		}
		return
	}

	if enumDetail.Caption != siblingValue {
		c.addWarning(
			"attribute_enum_sibling_incorrect",
			fmt.Sprintf(
				"Attribute %q enum sibling value %v does not match the caption of enum %q value %v; expected %q, got %v.",
				siblingPath,
				siblingValue,
				validationPath,
				value,
				enumDetail.Caption,
				siblingValue,
			),
			jsonish.Map{
				"attribute_path": siblingPath,
				"attribute":      siblingName,
				"value":          siblingValue,
				"expected_value": enumDetail.Caption,
			},
		)
	}
}

func (c *processingContext) validateEnumArraySibling(
	item jsonish.Map,
	value any,
	validationPath string,
	attrDef *itemAttributeDefinition,
	enumDetail *enumDefinition,
	arrayIndex int,
) {
	if attrDef.Sibling == nil || isOtherEnumValue(value) {
		return
	}

	siblingName := *attrDef.Sibling
	siblingArrayValue, siblingPresent := item[siblingName]
	if !siblingPresent {
		return
	}

	siblingArray, ok := asSlice(siblingArrayValue)
	if !ok {
		return
	}

	siblingPath := makeArrayElementPath(makeAttributePath(parentPath(validationPath), siblingName), arrayIndex)
	if arrayIndex >= len(siblingArray) || siblingArray[arrayIndex] == nil {
		c.addError(
			"attribute_enum_array_sibling_missing",
			fmt.Sprintf(
				"Attribute %q enum array sibling value is missing for enum array %q value %v.",
				siblingPath,
				validationPath,
				value,
			),
			jsonish.Map{
				"attribute_path": siblingPath,
				"attribute":      siblingName,
				"expected_value": enumDetail.Caption,
			},
		)
		return
	}

	siblingValue := siblingArray[arrayIndex]
	if siblingValue != enumDetail.Caption {
		c.addError(
			"attribute_enum_array_sibling_incorrect",
			fmt.Sprintf(
				"Attribute %q enum array sibling value %v is incorrect for enum array %q value %v; expected %q, got %v.",
				siblingPath,
				siblingValue,
				validationPath,
				value,
				enumDetail.Caption,
				siblingValue,
			),
			jsonish.Map{
				"attribute_path": siblingPath,
				"attribute":      siblingName,
				"value":          siblingValue,
				"expected_value": enumDetail.Caption,
			},
		)
	}
}

func (c *processingContext) validateEnumValueDeprecated(
	value any,
	validationPath string,
	attributeName string,
	enumDetail *enumDefinition,
) {
	if enumDetail.Deprecated == nil {
		return
	}
	c.addWarning(
		"attribute_enum_value_deprecated",
		fmt.Sprintf(
			"Deprecated enum value at %q; value %v is deprecated. %s",
			validationPath,
			value,
			enumDetail.Deprecated.Message,
		),
		jsonish.Map{
			"attribute_path": validationPath,
			"attribute":      attributeName,
			"value":          value,
			"since":          enumDetail.Deprecated.Since,
		},
	)
}

func (c *processingContext) validateEnumArrayValueDeprecated(
	value any,
	validationPath string,
	attributeName string,
	enumDetail *enumDefinition,
) {
	if enumDetail.Deprecated == nil {
		return
	}
	c.addWarning(
		"attribute_enum_array_value_deprecated",
		fmt.Sprintf(
			"Deprecated enum array value at %q; value %v is deprecated. %s",
			validationPath,
			value,
			enumDetail.Deprecated.Message,
		),
		jsonish.Map{
			"attribute_path": validationPath,
			"attribute":      attributeName,
			"value":          value,
			"since":          enumDetail.Deprecated.Since,
		},
	)
}

func (c *processingContext) validatePrimitiveValue(
	value any,
	attributePath string,
	attributeName string,
	attrDef *itemAttributeDefinition,
) {
	if attrDef.Type == "json_t" {
		return
	}

	typeDef, present := c.dictionary.Types.Attributes[attrDef.Type]
	if !present || typeDef == nil {
		c.addError(
			"schema_bug_type_missing",
			fmt.Sprintf("SCHEMA BUG: Type %q is not defined in dictionary.", attrDef.Type),
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      attributeName,
				"type":           attrDef.Type,
				"value":          value,
			},
		)
		return
	}

	primitiveType := attrDef.Type
	expectedType := attrDef.Type
	expectedTypeExtra := ""
	if typeDef.Type != "" {
		primitiveType = typeDef.Type
		expectedTypeExtra = " (" + primitiveType + ")"
	}

	switch primitiveType {
	case "boolean_t":
		if _, ok := value.(bool); !ok {
			c.addWrongType(attributePath, attributeName, value, expectedType, expectedTypeExtra)
			return
		}
		c.validateTypeValues(value, attributePath, attributeName, attrDef.Type)
	case "float_t":
		floatValue, ok := getFloat64(value)
		if !ok {
			c.addWrongType(attributePath, attributeName, value, expectedType, expectedTypeExtra)
			return
		}
		c.validateNumberRange(floatValue, value, attributePath, attributeName, attrDef.Type)
		c.validateTypeValues(value, attributePath, attributeName, attrDef.Type)
	case "integer_t", "long_t":
		intValue, ok := getInt64Value(value)
		if !ok {
			c.addWrongType(attributePath, attributeName, value, expectedType, expectedTypeExtra)
			return
		}
		c.validateNumberRange(intValue, value, attributePath, attributeName, attrDef.Type)
		c.validateTypeValues(value, attributePath, attributeName, attrDef.Type)
	case "string_t":
		stringValue, ok := value.(string)
		if !ok {
			c.addWrongType(attributePath, attributeName, value, expectedType, expectedTypeExtra)
			return
		}
		c.validateStringMaxLen(stringValue, attributePath, attributeName, attrDef.Type)
		c.validateStringRegex(stringValue, attributePath, attributeName, attrDef.Type)
		c.validateTypeValues(value, attributePath, attributeName, attrDef.Type)
	default:
		c.addError(
			"schema_bug_primitive_type_unknown",
			fmt.Sprintf("SCHEMA BUG: Unknown primitive type %q.", primitiveType),
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      attributeName,
				"type":           attrDef.Type,
				"value":          value,
			},
		)
	}
}

func (c *processingContext) validateTypeValues(
	value any,
	attributePath string,
	attributeName string,
	attributeTypeName string,
) {
	typeName, typeDef := c.firstTypeWithValues(attributeTypeName)
	if typeDef == nil {
		return
	}
	for _, allowedValue := range typeDef.Values {
		if valuesEqual(value, allowedValue) {
			return
		}
	}

	code := "attribute_value_not_in_type_values"
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
		"value":          value,
		"allowed_values": typeDef.Values,
	}
	message := fmt.Sprintf(
		"Attribute %q value is not in type %q list of allowed values.",
		attributePath,
		attributeTypeName,
	)
	if typeName != attributeTypeName {
		code = "attribute_value_not_in_super_type_values"
		details["super_type"] = typeName
		message = fmt.Sprintf(
			"Attribute %q, type %q, value is not in super type %q list of allowed values.",
			attributePath,
			attributeTypeName,
			typeName,
		)
	}
	c.addError(code, message, details)
}

func (c *processingContext) validateNumberRange(
	numericValue any,
	originalValue any,
	attributePath string,
	attributeName string,
	attributeTypeName string,
) {
	typeName, typeDef := c.firstTypeWithRange(attributeTypeName)
	if typeDef == nil || len(typeDef.Range) != 2 {
		return
	}

	low := typeDef.Range[0]
	high := typeDef.Range[1]
	outside := false
	switch value := numericValue.(type) {
	case int64:
		outside = value < low || value > high
	case float64:
		outside = value < float64(low) || value > float64(high)
	default:
		return
	}
	if !outside {
		return
	}

	code := "attribute_value_exceeds_range"
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
		"value":          originalValue,
		"range":          []int64{low, high},
	}
	message := fmt.Sprintf(
		"Attribute %q value is outside type %q range of %d to %d.",
		attributePath,
		attributeTypeName,
		low,
		high,
	)
	if typeName != attributeTypeName {
		code = "attribute_value_exceeds_super_type_range"
		details["super_type"] = typeName
		details["super_type_range"] = []int64{low, high}
		message = fmt.Sprintf(
			"Attribute %q, type %q, value is outside super type %q range of %d to %d.",
			attributePath,
			attributeTypeName,
			typeName,
			low,
			high,
		)
	}
	c.addError(code, message, details)
}

func (c *processingContext) validateStringMaxLen(
	value string,
	attributePath string,
	attributeName string,
	attributeTypeName string,
) {
	typeName, typeDef := c.firstTypeWithMaxLen(attributeTypeName)
	if typeDef == nil || typeDef.MaxLen == nil {
		return
	}

	length := utf8.RuneCountInString(value)
	maxLen := *typeDef.MaxLen
	if int64(length) <= maxLen {
		return
	}

	code := "attribute_value_exceeds_max_len"
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
		"length":         length,
		"max_len":        maxLen,
		"value":          value,
	}
	message := fmt.Sprintf(
		"Attribute %q value length of %d exceeds type %q max length %d.",
		attributePath,
		length,
		attributeTypeName,
		maxLen,
	)
	if typeName != attributeTypeName {
		code = "attribute_value_exceeds_super_type_max_len"
		details["super_type"] = typeName
		message = fmt.Sprintf(
			"Attribute %q, type %q, value length %d exceeds super type %q max length %d.",
			attributePath,
			attributeTypeName,
			length,
			typeName,
			maxLen,
		)
	}
	c.addError(code, message, details)
}

func (c *processingContext) validateStringRegex(
	value string,
	attributePath string,
	attributeName string,
	attributeTypeName string,
) {
	typeName, typeDef := c.firstTypeWithRegex(attributeTypeName)
	if typeDef == nil || typeDef.RegEx == nil {
		return
	}

	regex := *typeDef.RegEx
	compiledRegex, err := regexp.Compile(regex)
	if err != nil {
		c.addError(
			"schema_bug_type_regex_invalid",
			fmt.Sprintf("SCHEMA BUG: Type %q specifies an invalid regex: %s.", typeName, err),
			jsonish.Map{
				"attribute_path":       attributePath,
				"attribute":            attributeName,
				"type":                 typeName,
				"regex":                regex,
				"regex_error_message":  err.Error(),
				"regex_error_position": nil,
			},
		)
		return
	}
	if compiledRegex.MatchString(value) {
		return
	}

	code := "attribute_value_regex_not_matched"
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
		"regex":          regex,
		"value":          value,
	}
	message := fmt.Sprintf("Attribute %q value does not match regex of type %q.", attributePath, attributeTypeName)
	if typeName != attributeTypeName {
		code = "attribute_value_super_type_regex_not_matched"
		details["super_type"] = typeName
		message = fmt.Sprintf(
			"Attribute %q, type %q, value does not match regex of super type %q.",
			attributePath,
			attributeTypeName,
			typeName,
		)
	}
	c.addWarning(code, message, details)
}

func (c *processingContext) firstTypeWithValues(typeName string) (string, *typeDefinition) {
	return c.firstTypeWith(typeName, func(typeDef *typeDefinition) bool {
		return len(typeDef.Values) > 0
	})
}

func (c *processingContext) firstTypeWithRange(typeName string) (string, *typeDefinition) {
	return c.firstTypeWith(typeName, func(typeDef *typeDefinition) bool {
		return len(typeDef.Range) > 0
	})
}

func (c *processingContext) firstTypeWithMaxLen(typeName string) (string, *typeDefinition) {
	return c.firstTypeWith(typeName, func(typeDef *typeDefinition) bool {
		return typeDef.MaxLen != nil
	})
}

func (c *processingContext) firstTypeWithRegex(typeName string) (string, *typeDefinition) {
	return c.firstTypeWith(typeName, func(typeDef *typeDefinition) bool {
		return typeDef.RegEx != nil
	})
}

func (c *processingContext) firstTypeWith(
	typeName string,
	predicate func(*typeDefinition) bool,
) (string, *typeDefinition) {
	typeDef, present := c.dictionary.Types.Attributes[typeName]
	if !present || typeDef == nil {
		return "", nil
	}
	if predicate(typeDef) {
		return typeName, typeDef
	}
	if typeDef.Type == "" {
		return "", nil
	}
	superType, present := c.dictionary.Types.Attributes[typeDef.Type]
	if !present || superType == nil || !predicate(superType) {
		return "", nil
	}
	return typeDef.Type, superType
}

func (c *processingContext) validateUnknownKeys(
	item jsonish.Map,
	parentAttributePath string,
	itemDefinition *commonItemDefinition,
	filteredAttributes map[string]*itemAttributeDefinition,
) {
	if len(filteredAttributes) == 0 {
		return
	}

	eventKeys := make([]string, 0, len(item))
	for key := range item {
		eventKeys = append(eventKeys, key)
	}
	sort.Strings(eventKeys)

	for _, key := range eventKeys {
		if _, present := filteredAttributes[key]; present {
			continue
		}
		attributePath := makeAttributePath(parentAttributePath, key)
		details := jsonish.Map{
			"attribute_path": attributePath,
			"attribute":      key,
		}
		var structDesc string
		if c.class != nil && itemDefinition == &c.class.commonItemDefinition {
			structDesc = fmt.Sprintf("class %q uid %d", c.class.Name, c.class.Uid)
			details["class_uid"] = c.class.Uid
			details["class_name"] = c.class.Name
		} else {
			structDesc = fmt.Sprintf("object %q", itemDefinition.Name)
			details["object_name"] = itemDefinition.Name
		}
		c.addError(
			"attribute_unknown",
			fmt.Sprintf(
				"Unknown attribute at %q; attribute %q is not defined in %s.",
				attributePath,
				key,
				structDesc,
			),
			details,
		)
	}
}

func (c *processingContext) validateVersion(event jsonish.Map) {
	metadata, ok := event["metadata"].(jsonish.Map)
	if !ok {
		return
	}
	versionValue, present := metadata["version"]
	if !present {
		return
	}
	version, ok := versionValue.(string)
	if !ok {
		return
	}

	eventVersion, eventVersionOK := parseVersion(version)
	schemaVersion, schemaVersionOK := parseVersion(c.version)
	if !schemaVersionOK {
		return
	}
	if !eventVersionOK {
		c.addWarning(
			"version_earlier",
			fmt.Sprintf(
				"Event version %q at \"metadata.version\" is earlier than schema version %q.",
				version,
				c.version,
			),
			jsonish.Map{
				"attribute_path": "metadata.version",
				"attribute":      "version",
				"value":          version,
			},
		)
		return
	}
	if eventVersion.equal(schemaVersion) {
		return
	}
	if eventVersion.beforeOrEqual(schemaVersion) {
		switch {
		case eventVersion.major == 0:
			c.addError(
				"version_incompatible_initial_development",
				fmt.Sprintf(
					"Event version %q at \"metadata.version\" is an initial development version and is incompatible with schema version %q.",
					version,
					c.version,
				),
				jsonish.Map{
					"attribute_path": "metadata.version",
					"attribute":      "version",
					"value":          version,
				},
			)
		case eventVersion.prerelease != "":
			c.addError(
				"version_incompatible_prerelease",
				fmt.Sprintf(
					"Event version %q at \"metadata.version\" is a prerelease version and is incompatible with schema version %q.",
					version,
					c.version,
				),
				jsonish.Map{
					"attribute_path": "metadata.version",
					"attribute":      "version",
					"value":          version,
				},
			)
		default:
			c.addWarning(
				"version_earlier",
				fmt.Sprintf(
					"Event version %q at \"metadata.version\" is earlier than schema version %q.",
					version,
					c.version,
				),
				jsonish.Map{
					"attribute_path": "metadata.version",
					"attribute":      "version",
					"value":          version,
				},
			)
		}
		return
	}

	c.addError(
		"version_incompatible_later",
		fmt.Sprintf(
			"Event version %q at \"metadata.version\" is incompatible with schema version %q because it is a later version.",
			version,
			c.version,
		),
		jsonish.Map{
			"attribute_path": "metadata.version",
			"attribute":      "version",
			"value":          version,
		},
	)
}

func (c *processingContext) validateTypeUID(event jsonish.Map) {
	classUID, classOK := getInt64Value(event["class_uid"])
	activityID, activityOK := getInt64Value(event["activity_id"])
	typeUID, typeOK := getInt64Value(event["type_uid"])
	if !classOK || !activityOK || !typeOK {
		return
	}

	expectedTypeUID, ok := calculateExpectedTypeUID(classUID, activityID)
	if !ok {
		c.addError(
			"type_uid_expected_value_overflow",
			fmt.Sprintf(
				"Event's expected \"type_uid\" value cannot be represented as an int64 (class_uid %d * 100 + activity_id %d).",
				classUID,
				activityID,
			),
			jsonish.Map{
				"attribute_path": "type_uid",
				"attribute":      "type_uid",
				"value":          event["type_uid"],
				"class_uid":      classUID,
				"activity_id":    activityID,
			},
		)
		return
	}
	if typeUID == expectedTypeUID {
		return
	}
	c.addError(
		"type_uid_incorrect",
		fmt.Sprintf(
			"Event's \"type_uid\" value of %d does not match expected value of %d (class_uid %d * 100 + activity_id %d = %d).",
			typeUID,
			expectedTypeUID,
			classUID,
			activityID,
			expectedTypeUID,
		),
		jsonish.Map{
			"attribute_path": "type_uid",
			"attribute":      "type_uid",
			"value":          event["type_uid"],
			"expected_value": expectedTypeUID,
		},
	)
}

func calculateExpectedTypeUID(classUID int64, activityID int64) (int64, bool) {
	if classUID > math.MaxInt64/100 || classUID < math.MinInt64/100 {
		return 0, false
	}
	base := classUID * 100
	if activityID > 0 && base > math.MaxInt64-activityID {
		return 0, false
	}
	if activityID < 0 && base < math.MinInt64-activityID {
		return 0, false
	}
	return base + activityID, true
}

func (c *processingContext) validateConstraints(
	eventItem jsonish.Map,
	itemDefinition *commonItemDefinition,
	attributePath string,
) {
	if itemDefinition == nil || len(itemDefinition.Constraints) == 0 {
		return
	}

	constraintKeys := make([]string, 0, len(itemDefinition.Constraints))
	for constraintKey := range itemDefinition.Constraints {
		constraintKeys = append(constraintKeys, constraintKey)
	}
	sort.Strings(constraintKeys)

	for _, constraintKey := range constraintKeys {
		constraintDetails := itemDefinition.Constraints[constraintKey]
		switch constraintKey {
		case "at_least_one":
			if anyConstraintPathPresent(eventItem, constraintDetails) {
				continue
			}
			description, details := c.constraintInfo(itemDefinition, attributePath, constraintKey, constraintDetails)
			c.addError(
				"constraint_failed",
				fmt.Sprintf(
					"Constraint failed: %s; expected at least one constraint attribute, but got none.",
					description,
				),
				details,
			)
		case "just_one":
			count := countConstraintPathsPresent(eventItem, constraintDetails)
			if count == 1 {
				continue
			}
			description, details := c.constraintInfo(itemDefinition, attributePath, constraintKey, constraintDetails)
			details["value_count"] = count
			c.addError(
				"constraint_failed",
				fmt.Sprintf(
					"Constraint failed: %s; expected exactly 1 constraint attribute, got %d.",
					description,
					count,
				),
				details,
			)
		default:
			description, details := c.constraintInfo(itemDefinition, attributePath, constraintKey, constraintDetails)
			c.addError(
				"constraint_unknown",
				fmt.Sprintf("SCHEMA BUG: Unknown constraint %s.", description),
				details,
			)
		}
	}
}

func (c *processingContext) constraintInfo(
	itemDefinition *commonItemDefinition,
	attributePath string,
	constraintKey string,
	constraintDetails []string,
) (string, jsonish.Map) {
	constraint := jsonish.Map{constraintKey: constraintDetails}
	if attributePath != "" {
		return fmt.Sprintf("%q from object %q at %q", constraintKey, itemDefinition.Name, attributePath), jsonish.Map{
			"attribute_path": attributePath,
			"constraint":     constraint,
			"object_name":    itemDefinition.Name,
		}
	}
	return fmt.Sprintf("%q from class %q uid %d", constraintKey, c.class.Name, c.class.Uid), jsonish.Map{
		"constraint": constraint,
		"class_uid":  c.class.Uid,
		"class_name": c.class.Name,
	}
}

func anyConstraintPathPresent(eventItem jsonish.Map, paths []string) bool {
	for _, path := range paths {
		if hasPathOrKey(eventItem, path) {
			return true
		}
	}
	return false
}

func countConstraintPathsPresent(eventItem jsonish.Map, paths []string) int {
	count := 0
	for _, path := range paths {
		if hasPathOrKey(eventItem, path) {
			count++
		}
	}
	return count
}

func hasPathOrKey(eventItem jsonish.Map, path string) bool {
	if _, present := eventItem[path]; present {
		return true
	}
	parts := strings.Split(path, ".")
	current := eventItem
	for index, part := range parts {
		value, present := current[part]
		if !present {
			return false
		}
		if index == len(parts)-1 {
			return true
		}
		next, ok := value.(jsonish.Map)
		if !ok {
			return false
		}
		current = next
	}
	return false
}

func (c *processingContext) validateObservables(event jsonish.Map) {
	observablesValue := event["observables"]
	observables, ok := asSlice(observablesValue)
	if !ok {
		return
	}

	for index, observableValue := range observables {
		observable, ok := observableValue.(jsonish.Map)
		if !ok {
			continue
		}
		nameValue := observable["name"]
		name, ok := nameValue.(string)
		if !ok {
			continue
		}
		nameParts := splitObservableName(name)
		if c.getReferencedDefinition(nameParts, &c.class.commonItemDefinition) != nil {
			continue
		}

		attributePath := makeAttributePath(makeArrayElementPath("observables", index), "name")
		c.addError(
			"observable_name_invalid_reference",
			fmt.Sprintf(
				"Observable index %d \"name\" value %q does not refer to an attribute defined in class %q uid %d.",
				index,
				name,
				c.class.Name,
				c.class.Uid,
			),
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      "name",
				"name":           name,
				"class_uid":      c.class.Uid,
				"class_name":     c.class.Name,
			},
		)
	}
}

func (c *processingContext) getReferencedDefinition(
	nameParts []string,
	itemDefinition *commonItemDefinition,
) *commonItemDefinition {
	if len(nameParts) == 0 || itemDefinition == nil {
		return nil
	}

	attributeKey, isArrayAccess := parseArrayNotation(nameParts[0])
	attributes := c.filterAttributes(itemDefinition.Attributes)
	attrDef, present := attributes[attributeKey]
	if !present || attrDef == nil {
		return nil
	}
	if len(nameParts) == 1 {
		return itemDefinition
	}
	if isArrayAccess && (attrDef.IsArray == nil || !*attrDef.IsArray) {
		return nil
	}
	if attrDef.Type != "object_t" || attrDef.ObjectType == nil {
		return nil
	}
	objectDef, present := c.objects[*attrDef.ObjectType]
	if !present || objectDef == nil {
		return nil
	}
	return c.getReferencedDefinition(nameParts[1:], &objectDef.commonItemDefinition)
}

func parseArrayNotation(key string) (string, bool) {
	openBracket := strings.IndexByte(key, '[')
	if openBracket < 0 || !strings.HasSuffix(key, "]") {
		return key, false
	}
	return key[:openBracket], true
}

func splitObservableName(name string) []string {
	var parts []string
	var current strings.Builder
	bracketDepth := 0
	for _, r := range name {
		switch r {
		case '.':
			if bracketDepth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		case '[':
			bracketDepth++
			current.WriteRune(r)
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	parts = append(parts, current.String())
	return parts
}

func (c *processingContext) addRequiredAttributeMissing(attributePath string, attributeName string) {
	c.addError(
		"attribute_required_missing",
		fmt.Sprintf("Required attribute %q is missing.", attributePath),
		jsonish.Map{
			"attribute_path": attributePath,
			"attribute":      attributeName,
		},
	)
}

func (c *processingContext) addWrongType(
	attributePath string,
	attributeName string,
	value any,
	expectedType string,
	expectedTypeExtra string,
) {
	valueType, valueTypeExtra := typeOf(value)
	c.addError(
		"attribute_wrong_type",
		fmt.Sprintf(
			"Attribute %q value has wrong type; expected %s%s, got %s%s.",
			attributePath,
			expectedType,
			expectedTypeExtra,
			valueType,
			valueTypeExtra,
		),
		jsonish.Map{
			"attribute_path": attributePath,
			"attribute":      attributeName,
			"value":          value,
			"value_type":     valueType,
			"expected_type":  expectedType,
		},
	)
}

func (c *processingContext) addError(code string, message string, details jsonish.Map) {
	c.addIssue(issueSeverityError, code, message, details)
}

func (c *processingContext) addWarning(code string, message string, details jsonish.Map) {
	c.addIssue(issueSeverityWarning, code, message, details)
}

func (c *processingContext) addIssue(severity string, code string, message string, details jsonish.Map) {
	issue := ProcessingIssue{
		Phase:    issuePhaseValidation,
		Severity: severity,
		Code:     code,
		Message:  message,
		Details:  details,
	}
	if details != nil {
		if attributePath, ok := details["attribute_path"].(string); ok {
			issue.AttributePath = attributePath
		}
		if attribute, ok := details["attribute"].(string); ok {
			issue.Attribute = attribute
		}
		if value, present := details["value"]; present {
			issue.Value = value
		}
	}
	c.result.Issues = append(c.result.Issues, issue)
	switch severity {
	case issueSeverityError:
		c.result.Validation.Errors = append(c.result.Validation.Errors, issue)
	case issueSeverityWarning:
		c.result.Validation.Warnings = append(c.result.Validation.Warnings, issue)
	}
}

type parsedVersion struct {
	major      int
	minor      int
	patch      int
	prerelease string
}

var versionPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(.+))?$`)

func parseVersion(value string) (parsedVersion, bool) {
	matches := versionPattern.FindStringSubmatch(value)
	if matches == nil {
		return parsedVersion{}, false
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return parsedVersion{}, false
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return parsedVersion{}, false
	}
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return parsedVersion{}, false
	}
	return parsedVersion{
		major:      major,
		minor:      minor,
		patch:      patch,
		prerelease: matches[4],
	}, true
}

func (v parsedVersion) equal(other parsedVersion) bool {
	return v.major == other.major &&
		v.minor == other.minor &&
		v.patch == other.patch &&
		v.prerelease == other.prerelease
}

func (v parsedVersion) beforeOrEqual(other parsedVersion) bool {
	if v.major != other.major {
		return v.major < other.major
	}
	if v.minor != other.minor {
		return v.minor < other.minor
	}
	if v.patch != other.patch {
		return v.patch < other.patch
	}
	if v.prerelease == other.prerelease {
		return true
	}
	if v.prerelease != "" && other.prerelease == "" {
		return true
	}
	if v.prerelease == "" && other.prerelease != "" {
		return false
	}
	return v.prerelease <= other.prerelease
}
