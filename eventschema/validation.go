package eventschema

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ocsf/ocsf-toolkit/internal/coerce"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

const (
	issuePhaseValidation = "validation"
	issueSeverityError   = "error"
	issueSeverityWarning = "warning"
)

type validationProcessor struct {
	config validationConfig
}

type schemaValidationMetadata struct {
	versionOK         bool
	version           parsedVersion
	regexConstraints  map[string]compiledRegexConstraint
	valueConstraints  map[string]compiledValueConstraint
	rangeConstraints  map[string]compiledRangeConstraint
	maxLenConstraints map[string]compiledMaxLenConstraint
}

type compiledRangeConstraint struct {
	typeName string
	low      int64
	high     int64
}

type compiledMaxLenConstraint struct {
	typeName string
	maxLen   int64
}

type compiledValueConstraint struct {
	typeName     string
	intValues    []int64
	floatValues  []float64
	stringValues []string
	boolValues   []bool
}

type compiledRegexConstraint struct {
	typeName string
	regex    string
	compiled *regexp.Regexp
	err      error
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
	profiles, ok := newArrayView(profilesValue)
	if profilesValue == nil || !ok {
		return
	}
	for index := range profiles.Len() {
		profileValue := profiles.At(index)
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
	context.validateUnknownKeys(
		visit.item,
		visit.validationParentPath,
		visit.itemDefinition,
		visit.allowUnknownAttributes,
	)
}

func (p *validationProcessor) onObjectDone(context *processingContext, visit itemVisit) {
	context.validateUnknownKeys(
		visit.item,
		visit.validationParentPath,
		visit.itemDefinition,
		visit.allowUnknownAttributes,
	)
	context.validateConstraints(visit.item, visit.itemDefinition, visit.validationParentPath)
}

func (p *validationProcessor) onAttribute(
	context *processingContext,
	visit attributeVisit,
) {
	switch visit.status {
	case attributeVisitMissing:
		context.validateRequirement(
			visit.validationPath,
			visit.attributeName,
			visit.attrDef,
			p.config.warnOnMissingRecommended,
		)
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
	profiles, ok := newArrayView(profilesValue)
	if profilesValue == nil || !ok {
		return nil
	}

	result := make([]string, 0, profiles.Len())
	for index := range profiles.Len() {
		profileValue := profiles.At(index)
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
	warnOnMissingRecommended bool,
) {
	if attrDef == nil {
		return
	}
	switch attrDef.Requirement {
	case "required":
		c.addRequiredAttributeMissing(attributePath, attributeName)
	case "recommended":
		if warnOnMissingRecommended {
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
	siblingValue, siblingPresent := attributeValue(item, siblingName)
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
	siblingArrayValue, siblingPresent := attributeValue(item, siblingName)
	if !siblingPresent {
		return
	}

	siblingArray, ok := newArrayView(siblingArrayValue)
	if !ok {
		return
	}

	siblingPath := makeArrayElementPath(makeAttributePath(parentPath(validationPath), siblingName), arrayIndex)
	if arrayIndex >= siblingArray.Len() || siblingArray.At(arrayIndex) == nil {
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

	siblingValue := siblingArray.At(arrayIndex)
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
	constraint, present := c.validationMetadata.valueConstraints[attributeTypeName]
	if !present {
		return
	}
	if constraint.contains(value) {
		return
	}
	_, typeDef := c.resolveValuesConstraint(attributeTypeName)

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
	if constraint.typeName != attributeTypeName {
		code = "attribute_value_not_in_super_type_values"
		details["super_type"] = constraint.typeName
		message = fmt.Sprintf(
			"Attribute %q, type %q, value is not in super type %q list of allowed values.",
			attributePath,
			attributeTypeName,
			constraint.typeName,
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
	constraint, present := c.validationMetadata.rangeConstraints[attributeTypeName]
	if !present {
		return
	}

	low := constraint.low
	high := constraint.high
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
	if constraint.typeName != attributeTypeName {
		code = "attribute_value_exceeds_super_type_range"
		details["super_type"] = constraint.typeName
		details["super_type_range"] = []int64{low, high}
		message = fmt.Sprintf(
			"Attribute %q, type %q, value is outside super type %q range of %d to %d.",
			attributePath,
			attributeTypeName,
			constraint.typeName,
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
	constraint, present := c.validationMetadata.maxLenConstraints[attributeTypeName]
	if !present {
		return
	}

	length := utf8.RuneCountInString(value)
	maxLen := constraint.maxLen
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
	if constraint.typeName != attributeTypeName {
		code = "attribute_value_exceeds_super_type_max_len"
		details["super_type"] = constraint.typeName
		message = fmt.Sprintf(
			"Attribute %q, type %q, value length %d exceeds super type %q max length %d.",
			attributePath,
			attributeTypeName,
			length,
			constraint.typeName,
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
	constraint, present := c.validationMetadata.regexConstraints[attributeTypeName]
	if !present {
		return
	}

	if constraint.err != nil {
		c.addError(
			"schema_bug_type_regex_invalid",
			fmt.Sprintf("SCHEMA BUG: Type %q specifies an invalid regex: %s.", constraint.typeName, constraint.err),
			jsonish.Map{
				"attribute_path":       attributePath,
				"attribute":            attributeName,
				"type":                 constraint.typeName,
				"regex":                constraint.regex,
				"regex_error_message":  constraint.err.Error(),
				"regex_error_position": nil,
			},
		)
		return
	}
	if constraint.compiled.MatchString(value) {
		return
	}

	code := "attribute_value_regex_not_matched"
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
		"regex":          constraint.regex,
		"value":          value,
	}
	message := fmt.Sprintf("Attribute %q value does not match regex of type %q.", attributePath, attributeTypeName)
	if constraint.typeName != attributeTypeName {
		code = "attribute_value_super_type_regex_not_matched"
		details["super_type"] = constraint.typeName
		message = fmt.Sprintf(
			"Attribute %q, type %q, value does not match regex of super type %q.",
			attributePath,
			attributeTypeName,
			constraint.typeName,
		)
	}
	c.addWarning(code, message, details)
}

func (c *processingContext) resolveValuesConstraint(typeName string) (string, *typeDefinition) {
	return c.resolveTypeConstraint(typeName, func(typeDef *typeDefinition) bool {
		return len(typeDef.Values) > 0
	})
}

func (c *processingContext) resolveTypeConstraint(
	typeName string,
	predicate func(*typeDefinition) bool,
) (string, *typeDefinition) {
	return resolveTypeConstraint(c.dictionary, typeName, predicate)
}

func (si *schemaImpl) ensureValidationMetadata() error {
	si.validationMetadataOnce.Do(func() {
		version, versionOK := parseVersion(si.version)
		metadata := schemaValidationMetadata{
			version:           version,
			versionOK:         versionOK,
			regexConstraints:  make(map[string]compiledRegexConstraint),
			valueConstraints:  make(map[string]compiledValueConstraint),
			rangeConstraints:  make(map[string]compiledRangeConstraint),
			maxLenConstraints: make(map[string]compiledMaxLenConstraint),
		}
		for typeName := range si.dictionary.Types.Attributes {
			resolvedName, typeDef := resolveTypeConstraint(si.dictionary, typeName, func(typeDef *typeDefinition) bool {
				return typeDef.RegEx != nil
			})
			if typeDef != nil && typeDef.RegEx != nil {
				regex := *typeDef.RegEx
				compiled, err := regexp.Compile(regex)
				metadata.regexConstraints[typeName] = compiledRegexConstraint{
					typeName: resolvedName,
					regex:    regex,
					compiled: compiled,
					err:      err,
				}
			}

			resolvedValueType, valueTypeDef := resolveTypeConstraint(si.dictionary, typeName, func(typeDef *typeDefinition) bool {
				return len(typeDef.Values) > 0
			})
			if valueTypeDef != nil {
				constraint, err := compileValueConstraint(resolvedValueType, valueTypeDef)
				if err != nil {
					si.validationMetadataErr = err
					return
				}
				metadata.valueConstraints[typeName] = constraint
			}

			resolvedRangeType, rangeTypeDef := resolveTypeConstraint(si.dictionary, typeName, func(typeDef *typeDefinition) bool {
				return len(typeDef.Range) == 2
			})
			if rangeTypeDef != nil {
				metadata.rangeConstraints[typeName] = compiledRangeConstraint{
					typeName: resolvedRangeType,
					low:      rangeTypeDef.Range[0],
					high:     rangeTypeDef.Range[1],
				}
			}

			resolvedMaxLenType, maxLenTypeDef := resolveTypeConstraint(si.dictionary, typeName, func(typeDef *typeDefinition) bool {
				return typeDef.MaxLen != nil
			})
			if maxLenTypeDef != nil {
				metadata.maxLenConstraints[typeName] = compiledMaxLenConstraint{
					typeName: resolvedMaxLenType,
					maxLen:   *maxLenTypeDef.MaxLen,
				}
			}
		}
		si.validationMetadata = metadata
	})
	return si.validationMetadataErr
}

func compileValueConstraint(typeName string, typeDef *typeDefinition) (compiledValueConstraint, error) {
	constraint := compiledValueConstraint{typeName: typeName}
	primitiveType := typeName
	if typeDef.Type != "" {
		primitiveType = typeDef.Type
	}
	for index, value := range typeDef.Values {
		switch primitiveType {
		case "integer_t", "long_t":
			normalized, ok := getInt64Value(value)
			if !ok {
				return compiledValueConstraint{}, fmt.Errorf(
					"type %q allowed value at index %d is not a signed 64-bit integer", typeName, index,
				)
			}
			constraint.intValues = append(constraint.intValues, normalized)
		case "float_t":
			normalized, ok := schemaFloat64(value)
			if !ok {
				return compiledValueConstraint{}, fmt.Errorf(
					"type %q allowed value at index %d is not a finite float64", typeName, index,
				)
			}
			constraint.floatValues = append(constraint.floatValues, normalized)
		case "string_t":
			normalized, ok := value.(string)
			if !ok {
				return compiledValueConstraint{}, fmt.Errorf(
					"type %q allowed value at index %d is not a string", typeName, index,
				)
			}
			constraint.stringValues = append(constraint.stringValues, normalized)
		case "boolean_t":
			normalized, ok := value.(bool)
			if !ok {
				return compiledValueConstraint{}, fmt.Errorf(
					"type %q allowed value at index %d is not a boolean", typeName, index,
				)
			}
			constraint.boolValues = append(constraint.boolValues, normalized)
		default:
			return compiledValueConstraint{}, fmt.Errorf(
				"type %q has allowed values but unsupported primitive type %q", typeName, primitiveType,
			)
		}
	}
	return constraint, nil
}

func schemaFloat64(value any) (float64, bool) {
	var normalized float64
	switch value := value.(type) {
	case json.Number:
		var err error
		normalized, err = value.Float64()
		if err != nil {
			return 0, false
		}
	case float32:
		normalized = float64(value)
	case float64:
		normalized = value
	default:
		if integer, ok := getInt64Value(value); ok {
			normalized = float64(integer)
		} else {
			return 0, false
		}
	}
	return normalized, !math.IsNaN(normalized) && !math.IsInf(normalized, 0)
}

func (c compiledValueConstraint) contains(value any) bool {
	if c.intValues != nil {
		candidate, ok := getInt64Value(value)
		if !ok {
			return false
		}
		return slices.Contains(c.intValues, candidate)
	}
	if c.floatValues != nil {
		candidate, ok := getFloat64(value)
		return ok && slices.Contains(c.floatValues, candidate)
	}
	if c.stringValues != nil {
		candidate, ok := value.(string)
		return ok && slices.Contains(c.stringValues, candidate)
	}
	candidate, ok := value.(bool)
	return ok && slices.Contains(c.boolValues, candidate)
}

func resolveTypeConstraint(
	dictionary *dictionaryDefinition,
	typeName string,
	predicate func(*typeDefinition) bool,
) (string, *typeDefinition) {
	if dictionary == nil || dictionary.Types == nil {
		return "", nil
	}
	typeDef, present := dictionary.Types.Attributes[typeName]
	if !present || typeDef == nil {
		return "", nil
	}
	if predicate(typeDef) {
		return typeName, typeDef
	}
	if typeDef.Type == "" {
		return "", nil
	}
	superType, present := dictionary.Types.Attributes[typeDef.Type]
	if !present || superType == nil || !predicate(superType) {
		return "", nil
	}
	return typeDef.Type, superType
}

func (c *processingContext) validateUnknownKeys(
	item jsonish.Map,
	parentAttributePath string,
	itemDefinition *commonItemDefinition,
	allowUnknownAttributes bool,
) {
	if allowUnknownAttributes {
		return
	}

	var unknownKeys []string
	for key := range item {
		if item[key] == nil {
			continue
		}
		attribute, present := itemDefinition.Attributes[key]
		if present && c.attributeActive(attribute) {
			continue
		}
		unknownKeys = append(unknownKeys, key)
	}
	sort.Strings(unknownKeys)

	for _, key := range unknownKeys {
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
	versionValue, present := attributeValue(metadata, "version")
	if !present {
		return
	}
	version, ok := versionValue.(string)
	if !ok {
		return
	}

	if !c.validationMetadata.versionOK {
		return
	}
	schemaVersion := c.validationMetadata.version
	eventVersion, eventVersionOK := schemaVersion, true
	if version != c.version {
		eventVersion, eventVersionOK = parseVersion(version)
	}
	if !eventVersionOK {
		c.addError(
			"version_invalid_format",
			fmt.Sprintf(
				"Event version %q at \"metadata.version\" has invalid format; expected semantic versioning format.",
				version,
			),
			jsonish.Map{
				"attribute_path": "metadata.version",
				"attribute":      "version",
				"value":          version,
				"expected_regex": versionPattern.String(),
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

	for _, constraintKey := range itemDefinition.processing.constraintKeys {
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
	if _, present := attributeValue(eventItem, path); present {
		return true
	}
	current := eventItem
	for {
		part, remainder, nested := strings.Cut(path, ".")
		value, present := attributeValue(current, part)
		if !present {
			return false
		}
		if !nested {
			return true
		}
		next, ok := value.(jsonish.Map)
		if !ok {
			return false
		}
		current = next
		path = remainder
	}
}

func (c *processingContext) validateObservables(event jsonish.Map) {
	resolution := c.resolveObservables(event)
	if resolution == nil {
		return
	}
	for _, entry := range resolution.entries {
		if !entry.removed && entry.diagnostic != nil && !diagnosticCoveredByStructuralValidation(entry.diagnostic) {
			c.addError(entry.diagnostic.code, entry.diagnostic.message, entry.diagnostic.details)
		}
	}
}

func diagnosticCoveredByStructuralValidation(diagnostic *processingDiagnostic) bool {
	switch diagnostic.code {
	case "observable_array_wrong_type",
		"observable_element_wrong_type",
		"observable_name_missing",
		"observable_name_wrong_type",
		"observable_value_wrong_type":
		return true
	default:
		return false
	}
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
	core := value
	prerelease := ""
	if before, after, found := strings.Cut(value, "-"); found {
		if after == "" || strings.ContainsRune(after, '\n') {
			return parsedVersion{}, false
		}
		core = before
		prerelease = after
	}
	majorText, remainder, found := strings.Cut(core, ".")
	if !found {
		return parsedVersion{}, false
	}
	minorText, patchText, found := strings.Cut(remainder, ".")
	if !found || strings.Contains(patchText, ".") {
		return parsedVersion{}, false
	}
	major, ok := parseVersionNumber(majorText)
	if !ok {
		return parsedVersion{}, false
	}
	minor, ok := parseVersionNumber(minorText)
	if !ok {
		return parsedVersion{}, false
	}
	patch, ok := parseVersionNumber(patchText)
	if !ok {
		return parsedVersion{}, false
	}
	return parsedVersion{
		major:      major,
		minor:      minor,
		patch:      patch,
		prerelease: prerelease,
	}, true
}

func parseVersionNumber(value string) (int, bool) {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return 0, false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(value)
	return number, err == nil
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
