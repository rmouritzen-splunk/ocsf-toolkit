package processing

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/observable"
	"github.com/ocsf/ocsf-toolkit/internal/observablepath"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/internal/semver"
	"github.com/ocsf/ocsf-toolkit/internal/validationcache"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
)

type validationProcessor struct {
	config ValidationConfig
	cache  *validationcache.Cache
	policy validationPolicy
}

// validationPolicySetting is immutable pipeline-owned state for one explicitly configured code. A false suppressed
// value and invalid level represent no level override, though such an empty setting is not stored in validationPolicy.
type validationPolicySetting struct {
	suppressed bool
	level      validation.Level
}

type validationPolicy struct {
	settings         map[validation.Code]validationPolicySetting
	suppressAll      bool
	errorsAsWarnings bool
	warningsAsErrors bool
}

func (p *validationProcessor) onClassUIDMissing(context *processContext) {
	p.addFinding(
		context,
		validation.ClassUIDMissing,
		jsonish.Map{"attribute_path": "class_uid", "attribute": "class_uid"},
		`Required attribute "class_uid" is missing.`,
	)
}

func (p *validationProcessor) onClassUIDWrongType(context *processContext, value any) {
	valueType, valueTypeExtra := eventvalue.DescribeType(value)
	p.addFindingString2(
		context,
		validation.ClassUIDWrongType,
		jsonish.Map{
			"attribute_path": "class_uid",
			"attribute":      "class_uid",
			"value_type":     valueType,
			"expected_type":  "integer_t",
		},
		`Attribute "class_uid" value has wrong type; expected integer_t, got `,
		valueType,
		"",
		valueTypeExtra,
		".",
	)
}

func (p *validationProcessor) onClassUIDUnknown(context *processContext) {
	p.addFinding(
		context,
		validation.ClassUIDUnknown,
		jsonish.Map{
			"attribute_path": "class_uid",
			"attribute":      "class_uid",
		},
		"Unknown \"class_uid\" value; no class is defined for it.",
	)
}

func (p *validationProcessor) onClass(context *processContext, event jsonish.Map) {

	if context.class.Deprecated != nil {
		p.addFindingQuote1Int1String1(
			context,
			validation.ClassDeprecated,
			jsonish.Map{
				"class_uid":  context.class.Uid,
				"class_name": context.class.Name,
				"since":      context.class.Deprecated.Since,
			},
			"Class ",
			context.class.Name,
			" uid ",
			context.class.Uid,
			" is deprecated. ",
			context.class.Deprecated.Message,
			"",
		)
	}

	metadata, ok := event["metadata"].(jsonish.Map)
	if !ok {
		return
	}
	profilesValue := metadata["profiles"]
	profiles, ok := eventvalue.NewArrayView(profilesValue)
	if profilesValue == nil || !ok {
		return
	}
	var profilePath eventpath.Path
	profilePath.PushAttribute("metadata")
	profilePath.PushAttribute("profiles")
	for index := range profiles.Len() {
		profileValue := profiles.At(index)
		profile, ok := eventvalue.AsString(profileValue)
		if !ok {
			continue
		}
		if _, present := context.compiled.Profiles[profile]; present {
			continue
		}
		profilePath.PushArrayIndex(index)
		attributePath := profilePath.String(pathstyle.ArrayIndexed)
		profilePath.Pop()
		p.addFindingQuote1(
			context,
			validation.ProfileUnknown,
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      "profiles",
			},
			"Unknown profile at ",
			attributePath,
			"; no profile is defined for it.",
		)
	}
}

func (p *validationProcessor) onObjectDone(
	context *processContext,
	item jsonish.Map,
	objectDefinition *schema.ObjectDefinition,
) {
	p.validateConstraints(context, item, &objectDefinition.ItemDefinition, &context.path)
}

func (p *validationProcessor) onInactiveAttribute(
	c *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	itemDefinition *schema.ItemDefinition,
) {
	requiredProfiles := slices.Clone(attrDef.Profiles)
	slices.Sort(requiredProfiles)
	valueValidation, validationResult := p.onInactiveAttributeValue(c, item, value, attributeName, attrDef)

	attributePath := c.path.String(pathstyle.ArrayIndexed)
	details := jsonish.Map{
		"attribute_path":    attributePath,
		"attribute":         attributeName,
		"required_profiles": requiredProfiles,
		"value_validation":  valueValidation,
	}
	p.addItemDetails(c, details, itemDefinition)
	message := inactiveAttributeMessage(attributePath, requiredProfiles)
	switch valueValidation {
	case "valid":
		message += " The value is valid."
	case "shallowly_valid":
		message += " The value passes a shallow validation; full validation requires enabling a profile."
	case "invalid":
		details["invalid_value"] = validationResultMap(validationResult)
		message += " The value would be invalid if enabled by a profile."
	}
	p.addFinding(c, validation.AttributeRequiresProfile, details, message)
}

func (p *validationProcessor) onInactiveAttributeValue(
	c *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) (string, eventresult.ValidationResult) {
	valueValidator := *p
	valueValidator.policy = validationPolicy{}
	validationContext := processContext{compiled: c.compiled, class: c.class, path: c.path}
	shallow := attrDef.Enum != nil
	switch {
	case attrDef.IsArray != nil && *attrDef.IsArray:
		shallow = true
		valueValidator.onInactiveArray(&validationContext, item, value, attributeName, attrDef)
	case attrDef.Type == "object_t":
		shallow = true
		valueValidator.onInactiveObject(&validationContext, value, attributeName, attrDef)
	default:
		valueValidator.onInactivePrimitive(&validationContext, item, value, attributeName, attrDef)
	}

	result := validationContext.result.Validation
	if result.Count(validation.LevelError) != 0 {
		return "invalid", result
	}
	if shallow {
		return "shallowly_valid", result
	}
	return "valid", result
}

func (p *validationProcessor) onInactiveArray(
	c *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	if _, ok := eventvalue.NewArrayView(value); !ok {
		p.onAttribute(c, item, value, attributeName, attrDef, -1, attributeArrayWrongType)
	}
}

func (p *validationProcessor) onInactiveObject(
	c *processContext,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	if _, ok := value.(jsonish.Map); !ok {
		p.onObjectWrongType(c, value, attributeName, attrDef)
		return
	}
	if attrDef.ObjectType != nil && attrDef.ResolvedObject == nil {
		p.onObjectSchemaMissing(c, attributeName, *attrDef.ObjectType)
	}
}

func (p *validationProcessor) onInactivePrimitive(
	c *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	p.onAttribute(c, item, value, attributeName, attrDef, -1, attributePrimitive)
}

func validationResultMap(result eventresult.ValidationResult) jsonish.Map {
	details := jsonish.Map{}
	if len(result.Findings) != 0 {
		findings := make([]any, len(result.Findings))
		for index, finding := range result.Findings {
			findingMap := jsonish.Map{
				"level":   finding.Level.String(),
				"code":    finding.Code.String(),
				"message": finding.Message,
			}
			if len(finding.Details) != 0 {
				findingMap["details"] = finding.Details
			}
			findings[index] = findingMap
		}
		details["findings"] = findings
	}
	if result.SuppressedErrorCount != 0 {
		details["suppressed_error_count"] = result.SuppressedErrorCount
	}
	if result.SuppressedWarningCount != 0 {
		details["suppressed_warning_count"] = result.SuppressedWarningCount
	}
	return details
}

func (p *validationProcessor) onUnknownAttribute(
	c *processContext,
	attributeName string,
	itemDefinition *schema.ItemDefinition,
) {
	attributePath := c.path.ChildString(attributeName, pathstyle.ArrayIndexed)
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
	}
	structDesc := p.addItemDetails(c, details, itemDefinition)
	p.addFindingQuote2String1(
		c,
		validation.AttributeUnknown,
		details,
		"Unknown attribute at ",
		attributePath,
		"; attribute ",
		attributeName,
		" is not defined in ",
		structDesc,
		".",
	)
}

func (*validationProcessor) addItemDetails(
	c *processContext,
	details jsonish.Map,
	itemDefinition *schema.ItemDefinition,
) string {
	if c.class != nil && itemDefinition == &c.class.ItemDefinition {
		details["class_uid"] = c.class.Uid
		details["class_name"] = c.class.Name
		return "class " + strconv.Quote(c.class.Name) + " uid " + strconv.FormatInt(c.class.Uid, 10)
	}
	details["object_name"] = itemDefinition.Name
	return "object " + strconv.Quote(itemDefinition.Name)
}

func inactiveAttributeMessage(attributePath string, requiredProfiles []string) string {
	if len(requiredProfiles) == 1 {
		return "Attribute at " + strconv.Quote(attributePath) + " requires profile " +
			strconv.Quote(requiredProfiles[0]) + ", which is not listed in metadata.profiles."
	}
	quotedProfiles := make([]string, len(requiredProfiles))
	for index, profile := range requiredProfiles {
		quotedProfiles[index] = strconv.Quote(profile)
	}
	return "Attribute at " + strconv.Quote(attributePath) + " requires one of the profiles " +
		strings.Join(quotedProfiles, " or ") + "; none is listed in metadata.profiles."
}

func (p *validationProcessor) onAttribute(
	context *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
	status attributeState,
) {
	switch status {
	case attributeMissing:
		p.validateRequirement(context, &context.path, attributeName, attrDef)
		return
	case attributeArrayWrongType:
		p.addWrongType(
			context,
			context.path.String(pathstyle.ArrayIndexed),
			attributeName,
			value,
			"array of "+attrDef.Type,
			"",
		)
		return
	case attributeEnum:
		p.validateEnum(context, item, value, attributeName, attrDef, arrayIndex)
		return
	case attributePrimitive:
		p.validatePrimitiveValue(context, value, &context.path, attributeName, attrDef)
		return
	case attributePresent:
	default:
		return
	}

	p.validateAttributeDeprecation(context, &context.path, attributeName, attrDef)
}

func (p *validationProcessor) validateEnum(
	c *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
) {
	enumDetail, lookupStatus := lookupEnumDefinition(c, attrDef, value)
	if lookupStatus == enumLookupValueUnusable {
		return
	}
	if lookupStatus == enumLookupDefinitionMissing {
		validationPath := c.path.String(pathstyle.ArrayIndexed)
		code := validation.AttributeEnumValueUnknown
		message := "Unknown enum value at " + strconv.Quote(validationPath) +
			"; it is not defined for enum " + strconv.Quote(attributeName) + "."
		if isArrayElement(arrayIndex) {
			code = validation.AttributeEnumArrayValueUnknown
			message = "Unknown enum array value at " + strconv.Quote(validationPath) +
				"; it is not defined for enum " + strconv.Quote(attributeName) + "."
		}
		p.addFinding(
			c,
			code,
			jsonish.Map{
				"attribute_path": validationPath,
				"attribute":      attributeName,
			},
			message,
		)
		return
	}
	if isArrayElement(arrayIndex) {
		if schema.AttributeActive(attrDef.ResolvedEnumSibling, c.activeProfiles) {
			p.validateEnumArraySibling(
				c, item, attrDef, arrayIndex, &c.path, enumDetail, attributeIsOtherEnumValue(value),
			)
		}
		p.validateEnumArrayValueDeprecated(c, attributeName, &c.path, enumDetail)
		return
	}
	if schema.AttributeActive(attrDef.ResolvedEnumSibling, c.activeProfiles) {
		p.validateEnumSibling(c, item, value, attrDef, &c.path, enumDetail)
	}
	p.validateEnumValueDeprecated(c, attributeName, &c.path, enumDetail)
}

func (p *validationProcessor) validateArrayEnum(
	c *processContext,
	item jsonish.Map,
	values eventvalue.ArrayView,
	index int,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	var enumDetail *schema.EnumDefinition
	var lookupStatus enumLookupStatus
	other := false
	switch attrDef.PrimitiveType {
	case "string_t":
		value, ok := values.AsStringAt(index)
		if !ok {
			return
		}
		enumDetail = attrDef.Enum[value]
	case "integer_t":
		value, ok := values.AsIntegerAt(index)
		if !ok {
			return
		}
		other = value == 99
		enumDetail = attrDef.NumericEnumDefinition(value)
	case "long_t":
		value, ok := values.AsLongAt(index)
		if !ok {
			return
		}
		other = value == 99
		enumDetail = attrDef.NumericEnumDefinition(value)
	default:
		enumDetail, lookupStatus = lookupEnumDefinition(c, attrDef, values.At(index))
		if lookupStatus == enumLookupValueUnusable {
			return
		}
	}
	if enumDetail == nil {
		validationPath := c.path.String(pathstyle.ArrayIndexed)
		p.addFindingQuote2(
			c,
			validation.AttributeEnumArrayValueUnknown,
			jsonish.Map{"attribute_path": validationPath, "attribute": attributeName},
			"Unknown enum array value at ",
			validationPath,
			"; it is not defined for enum ",
			attributeName,
			".",
		)
		return
	}
	if schema.AttributeActive(attrDef.ResolvedEnumSibling, c.activeProfiles) {
		p.validateEnumArraySibling(c, item, attrDef, index, &c.path, enumDetail, other)
	}
	p.validateEnumArrayValueDeprecated(c, attributeName, &c.path, enumDetail)
}

// onEnumSiblingPairAttributes validates an enum and its resolved string sibling instead of ordinary
// attribute dispatch. Array pairs delegate their element walks to onAttribute so they retain the ordinary array
// validation behavior while being dispatched as one logical pair.
func (p *validationProcessor) onEnumSiblingPairAttributes(
	c *processContext,
	item jsonish.Map,
	enumAttributeName string,
	enumAttrDef *schema.ItemAttributeDefinition,
	siblingAttributeName string,
	siblingAttrDef *schema.ItemAttributeDefinition,
) error {
	if enumAttrDef.IsArray != nil && *enumAttrDef.IsArray {
		p.validateEnumArraySiblingLengths(c, item, enumAttributeName, siblingAttributeName)
		if err := p.validateEnumPairArrayAttribute(c, item, enumAttributeName, enumAttrDef); err != nil {
			return err
		}
		return p.validateEnumPairArrayAttribute(c, item, siblingAttributeName, siblingAttrDef)
	}

	enumValue, enumPresent := eventvalue.Attribute(item, enumAttributeName)
	c.path.PushAttribute(enumAttributeName)
	if !enumPresent {
		p.validateRequirement(c, &c.path, enumAttributeName, enumAttrDef)
	} else {
		p.validateAttributeDeprecation(c, &c.path, enumAttributeName, enumAttrDef)
		p.validateEnum(c, item, enumValue, enumAttributeName, enumAttrDef, -1)
		p.validatePrimitiveValue(c, enumValue, &c.path, enumAttributeName, enumAttrDef)
	}
	c.path.Pop()

	siblingValue, siblingPresent := eventvalue.Attribute(item, siblingAttributeName)
	siblingPath := c.path.ChildString(siblingAttributeName, pathstyle.ArrayIndexed)
	if siblingPresent && !enumPresent {
		enumPath := c.path.ChildString(enumAttributeName, pathstyle.ArrayIndexed)
		p.addFindingQuote2(
			c,
			validation.AttributeEnumSiblingWithoutEnum,
			jsonish.Map{
				"attribute_path":      siblingPath,
				"attribute":           siblingAttributeName,
				"enum_attribute_path": enumPath,
				"enum_attribute":      enumAttributeName,
			},
			"Enum sibling ",
			siblingPath,
			" exists without its enum attribute ",
			enumPath,
			".",
		)
	}

	c.path.PushAttribute(siblingAttributeName)
	defer c.path.Pop()
	if !siblingPresent {
		if enumPresent && attributeIsOtherEnumValue(enumValue) {
			p.addRequiredAttributeMissing(c, siblingPath, siblingAttributeName)
		} else if enumPresent {
			p.validateRequirement(c, &c.path, siblingAttributeName, siblingAttrDef)
		}
		return nil
	}
	p.validateAttributeDeprecation(c, &c.path, siblingAttributeName, siblingAttrDef)
	p.validatePrimitiveValue(c, siblingValue, &c.path, siblingAttributeName, siblingAttrDef)
	return nil
}

func (p *validationProcessor) validateEnumArraySiblingLengths(
	c *processContext,
	item jsonish.Map,
	enumAttributeName string,
	siblingAttributeName string,
) {
	enumValue, enumPresent := eventvalue.Attribute(item, enumAttributeName)
	siblingValue, siblingPresent := eventvalue.Attribute(item, siblingAttributeName)
	if !enumPresent || !siblingPresent {
		return
	}
	enumLength, enumArray := eventvalue.ArrayLen(enumValue)
	siblingLength, siblingArray := eventvalue.ArrayLen(siblingValue)
	if !enumArray || !siblingArray || enumLength == siblingLength {
		return
	}
	enumPath := c.path.ChildString(enumAttributeName, pathstyle.ArrayIndexed)
	siblingPath := c.path.ChildString(siblingAttributeName, pathstyle.ArrayIndexed)
	p.addEnumArraySiblingLengthMismatch(
		c, enumPath, siblingPath, enumAttributeName, siblingAttributeName, enumLength, siblingLength,
	)
}

func (p *validationProcessor) validateEnumPairArrayAttribute(
	c *processContext,
	item jsonish.Map,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) error {
	value, present := eventvalue.Attribute(item, attributeName)
	c.path.PushAttribute(attributeName)
	defer c.path.Pop()
	if !present {
		p.onAttribute(c, item, nil, attributeName, attrDef, -1, attributeMissing)
		return nil
	}
	p.onAttribute(c, item, value, attributeName, attrDef, -1, attributePresent)
	values, ok := eventvalue.NewArrayView(value)
	if !ok {
		p.onAttribute(c, item, value, attributeName, attrDef, -1, attributeArrayWrongType)
		return nil
	}
	if isScalarArrayAttribute(attrDef) {
		for index := range values.Len() {
			c.path.PushArrayIndex(index)
			if attrDef.Enum != nil {
				p.validateArrayEnum(c, item, values, index, attributeName, attrDef)
			}
			p.validateArrayPrimitiveValue(c, values, index, attributeName, attrDef, &c.path)
			c.path.Pop()
		}
		return nil
	}
	for index := range values.Len() {
		c.path.PushArrayIndex(index)
		element := values.At(index)
		if attrDef.Enum != nil {
			p.onAttribute(c, item, element, attributeName, attrDef, index, attributeEnum)
		}
		p.onAttribute(c, item, element, attributeName, attrDef, index, attributePrimitive)
		c.path.Pop()
	}
	return nil
}

func (p *validationProcessor) validateAttributeDeprecation(
	c *processContext,
	path *eventpath.Path,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	if attrDef == nil {
		return
	}
	if attrDef.Deprecated != nil {
		attributePath := path.String(pathstyle.ArrayIndexed)
		p.addFindingQuote1String1(
			c,
			validation.AttributeDeprecated,
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      attributeName,
				"since":          attrDef.Deprecated.Since,
			},
			"Attribute ",
			attributeName,
			" is deprecated. ",
			attrDef.Deprecated.Message,
			"",
		)
		return
	}
	typeDef := c.compiled.DictionaryType(attrDef.Type)
	if typeDef == nil || typeDef.Deprecated == nil {
		return
	}
	attributePath := path.String(pathstyle.ArrayIndexed)
	p.addFindingQuote2String1(
		c,
		validation.AttributeTypeDeprecated,
		jsonish.Map{
			"attribute_path": attributePath,
			"attribute":      attributeName,
			"type":           attrDef.Type,
			"since":          typeDef.Deprecated.Since,
		},
		"Attribute ",
		attributeName,
		" uses deprecated type ",
		attrDef.Type,
		". ",
		typeDef.Deprecated.Message,
		"",
	)
}

func (p *validationProcessor) onObject(
	context *processContext,
	attributeName string,
	objectDef *schema.ObjectDefinition,
) {
	if objectDef.Deprecated == nil {
		return
	}
	attributePath := context.path.String(pathstyle.ArrayIndexed)
	p.addFindingQuote1String1(
		context,
		validation.ObjectDeprecated,
		jsonish.Map{
			"attribute_path": attributePath,
			"attribute":      attributeName,
			"object_name":    objectDef.Name,
			"since":          objectDef.Deprecated.Since,
		},
		"Object ",
		objectDef.Name,
		" is deprecated. ",
		objectDef.Deprecated.Message,
		"",
	)
}

func (p *validationProcessor) onObjectWrongType(
	context *processContext,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	expectedType := "object"
	if attrDef.ObjectType != nil {
		expectedType = *attrDef.ObjectType + " (object)"
	}
	p.addWrongType(
		context,
		context.path.String(pathstyle.ArrayIndexed),
		attributeName,
		value,
		expectedType,
		"",
	)
}

func (p *validationProcessor) onObjectSchemaMissing(
	context *processContext,
	attributeName string,
	objectType string,
) {
	attributePath := context.path.String(pathstyle.ArrayIndexed)
	p.addFindingQuote1(
		context,
		validation.SchemaBugObjectMissing,
		jsonish.Map{
			"attribute_path": attributePath,
			"attribute":      attributeName,
			"object_type":    objectType,
		},
		"SCHEMA BUG: Object ",
		objectType,
		" is not defined.",
	)
}

func (p *validationProcessor) onEventDone(context *processContext, event jsonish.Map) error {
	p.validateVersion(context, event)
	p.validateTypeUID(context, event)
	p.validateConstraints(context, event, &context.class.ItemDefinition, nil)
	return p.validateObservables(context, event)
}

func (p *validationProcessor) validateRequirement(
	c *processContext,
	path *eventpath.Path,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	if attrDef == nil {
		return
	}
	switch attrDef.Requirement {
	case "required":
		attributePath := path.String(pathstyle.ArrayIndexed)
		p.addRequiredAttributeMissing(c, attributePath, attributeName)
	case "recommended":
		if p.config.WarnOnMissingRecommended {
			attributePath := path.String(pathstyle.ArrayIndexed)
			p.addFindingQuote1(
				c,
				validation.AttributeRecommendedMissing,
				jsonish.Map{
					"attribute_path": attributePath,
					"attribute":      attributeName,
				},
				"Recommended attribute ",
				attributePath,
				" is missing.",
			)
		}
	}
}

func (p *validationProcessor) validateEnumSibling(
	c *processContext,
	item jsonish.Map,
	value any,
	attrDef *schema.ItemAttributeDefinition,
	path *eventpath.Path,
	enumDetail *schema.EnumDefinition,
) {
	if attrDef.Sibling == nil {
		return
	}
	siblingName := *attrDef.Sibling
	siblingValue, siblingPresent := eventvalue.Attribute(item, siblingName)
	if !siblingPresent {
		return
	}

	if attributeIsOtherEnumValue(value) {
		if enumDetail.Caption == siblingValue {
			validationPath := path.String(pathstyle.ArrayIndexed)
			siblingPath := path.SiblingString(siblingName, pathstyle.ArrayIndexed)
			p.addFindingQuote3(
				c,
				validation.AttributeEnumSiblingSuspicious,
				jsonish.Map{
					"attribute_path": siblingPath,
					"attribute":      siblingName,
				},
				siblingPath,
				" enum sibling suspiciously matches the schema caption of enum ",
				validationPath,
				" value 99 (",
				enumDetail.Caption,
				").",
			)
		}
		return
	}

	if enumDetail.Caption != siblingValue {
		validationPath := path.String(pathstyle.ArrayIndexed)
		siblingPath := path.SiblingString(siblingName, pathstyle.ArrayIndexed)
		p.addFindingQuote3(
			c,
			validation.AttributeEnumSiblingIncorrect,
			jsonish.Map{
				"attribute_path": siblingPath,
				"attribute":      siblingName,
				"expected_value": enumDetail.Caption,
			},
			siblingPath,
			" enum sibling does not match the schema caption of enum ",
			validationPath,
			"; expected ",
			enumDetail.Caption,
			".",
		)
	}
}

func (p *validationProcessor) validateEnumArraySibling(
	c *processContext,
	item jsonish.Map,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
	path *eventpath.Path,
	enumDetail *schema.EnumDefinition,
	other bool,
) {
	if attrDef.Sibling == nil {
		return
	}

	siblingName := *attrDef.Sibling
	siblingArrayValue, siblingPresent := eventvalue.Attribute(item, siblingName)
	if !siblingPresent {
		if other {
			p.addEnumArraySiblingMissing(c, path, siblingName, enumDetail.Caption)
		}
		return
	}

	siblingArray, ok := eventvalue.NewArrayView(siblingArrayValue)
	if !ok {
		return
	}

	elementIndex := arrayIndex
	if elementIndex >= siblingArray.Len() {
		return
	}
	if siblingString, siblingIsString := siblingArray.AsStringAt(elementIndex); siblingIsString {
		if other {
			if siblingString == enumDetail.Caption {
				p.addEnumArraySiblingSuspicious(c, path, siblingName, enumDetail.Caption)
			}
			return
		}
		if siblingString == enumDetail.Caption {
			return
		}
		p.addEnumArraySiblingIncorrect(c, path, siblingName, enumDetail.Caption)
		return
	}
	siblingValue := siblingArray.At(elementIndex)
	if siblingValue == nil {
		p.addEnumArraySiblingMissing(c, path, siblingName, enumDetail.Caption)
		return
	}
	if other {
		if siblingString, siblingIsString := eventvalue.AsString(siblingValue); siblingIsString &&
			siblingString == enumDetail.Caption {
			p.addEnumArraySiblingSuspicious(c, path, siblingName, enumDetail.Caption)
		}
		return
	}
	if siblingValue != enumDetail.Caption {
		p.addEnumArraySiblingIncorrect(c, path, siblingName, enumDetail.Caption)
	}
}

func (p *validationProcessor) addEnumArraySiblingLengthMismatch(
	c *processContext,
	enumPath string,
	siblingPath string,
	attributeName string,
	siblingName string,
	enumLength int,
	siblingLength int,
) {
	p.addFindingQuote2Int2(
		c,
		validation.AttributeEnumArraySiblingLengthMismatch,
		jsonish.Map{
			"attribute_path":      siblingPath,
			"attribute":           siblingName,
			"enum_attribute_path": enumPath,
			"enum_attribute":      attributeName,
			"enum_length":         enumLength,
			"sibling_length":      siblingLength,
		},
		"Enum array ",
		enumPath,
		" and its sibling array ",
		siblingPath,
		" have different lengths (",
		int64(enumLength),
		" and ",
		int64(siblingLength),
		").",
	)
}

func (p *validationProcessor) addEnumArraySiblingSuspicious(
	c *processContext,
	path *eventpath.Path,
	siblingName string,
	schemaCaption string,
) {
	validationPath := path.String(pathstyle.ArrayIndexed)
	siblingPath := path.SiblingString(siblingName, pathstyle.ArrayIndexed)
	p.addFindingQuote3(
		c,
		validation.AttributeEnumSiblingSuspicious,
		jsonish.Map{
			"attribute_path": siblingPath,
			"attribute":      siblingName,
		},
		siblingPath,
		" enum sibling suspiciously matches the schema caption of enum ",
		validationPath,
		" value 99 (",
		schemaCaption,
		").",
	)
}

func (p *validationProcessor) addEnumArraySiblingMissing(
	c *processContext,
	path *eventpath.Path,
	siblingName string,
	expectedCaption any,
) {
	validationPath := path.String(pathstyle.ArrayIndexed)
	siblingPath := path.SiblingString(siblingName, pathstyle.ArrayIndexed)
	p.addFindingQuote2(
		c,
		validation.AttributeEnumArraySiblingMissing,
		jsonish.Map{
			"attribute_path": siblingPath,
			"attribute":      siblingName,
			"expected_value": expectedCaption,
		},
		"Attribute ",
		siblingPath,
		" enum array sibling value is missing for enum array ",
		validationPath,
		".",
	)
}

func (p *validationProcessor) addEnumArraySiblingIncorrect(
	c *processContext,
	path *eventpath.Path,
	siblingName string,
	expectedCaption string,
) {
	validationPath := path.String(pathstyle.ArrayIndexed)
	siblingPath := path.SiblingString(siblingName, pathstyle.ArrayIndexed)
	p.addFindingQuote3(
		c,
		validation.AttributeEnumArraySiblingIncorrect,
		jsonish.Map{
			"attribute_path": siblingPath,
			"attribute":      siblingName,
			"expected_value": expectedCaption,
		},
		siblingPath,
		" enum array sibling is incorrect for enum array ",
		validationPath,
		"; expected ",
		expectedCaption,
		".",
	)
}

func (p *validationProcessor) validateEnumValueDeprecated(
	c *processContext,
	attributeName string,
	path *eventpath.Path,
	enumDetail *schema.EnumDefinition,
) {
	if enumDetail.Deprecated == nil {
		return
	}
	validationPath := path.String(pathstyle.ArrayIndexed)
	p.addFindingQuote1String1(
		c,
		validation.AttributeEnumValueDeprecated,
		jsonish.Map{
			"attribute_path": validationPath,
			"attribute":      attributeName,
			"since":          enumDetail.Deprecated.Since,
		},
		"Enum value at ",
		validationPath,
		" is deprecated. ",
		enumDetail.Deprecated.Message,
		"",
	)
}

func (p *validationProcessor) validateEnumArrayValueDeprecated(
	c *processContext,
	attributeName string,
	path *eventpath.Path,
	enumDetail *schema.EnumDefinition,
) {
	if enumDetail.Deprecated == nil {
		return
	}
	validationPath := path.String(pathstyle.ArrayIndexed)
	p.addFindingQuote1String1(
		c,
		validation.AttributeEnumArrayValueDeprecated,
		jsonish.Map{
			"attribute_path": validationPath,
			"attribute":      attributeName,
			"since":          enumDetail.Deprecated.Since,
		},
		"Enum array value at ",
		validationPath,
		" is deprecated. ",
		enumDetail.Deprecated.Message,
		"",
	)
}

func (p *validationProcessor) validatePrimitiveValue(
	c *processContext,
	value any,
	path *eventpath.Path,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	typeValidation := p.cache.Types[attrDef.Type]
	if typeValidation == nil || typeValidation.Definition == nil {
		attributePath := path.String(pathstyle.ArrayIndexed)
		p.addFindingQuote1(
			c,
			validation.SchemaBugTypeMissing,
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      attributeName,
				"type":           attrDef.Type,
			},
			"SCHEMA BUG: Type ",
			attrDef.Type,
			" is not defined in dictionary.",
		)
		return
	}

	primitiveType := typeValidation.PrimitiveType
	expectedType := attrDef.Type

	switch primitiveType {
	case "json_t":
		// json_t's structured-or-scalar value has no fixed shape to validate.
	case "boolean_t":
		if _, ok := eventvalue.AsBoolean(value); !ok {
			p.addWrongType(c, path.String(pathstyle.ArrayIndexed), attributeName, value, expectedType,
				expectedPrimitiveType(expectedType, primitiveType))
			return
		}
		p.validateTypeValues(c, value, path, attributeName, attrDef.Type, typeValidation)
	case "float_t":
		floatValue, ok := eventvalue.AsFloat(value)
		if !ok {
			p.addWrongType(c, path.String(pathstyle.ArrayIndexed), attributeName, value, expectedType,
				expectedPrimitiveType(expectedType, primitiveType))
			return
		}
		p.validateFloatRange(c, value, floatValue, path, attributeName, attrDef.Type, typeValidation)
		p.validateTypeValues(c, value, path, attributeName, attrDef.Type, typeValidation)
	case "integer_t", "long_t":
		var intValue int64
		var ok bool
		if primitiveType == "long_t" {
			intValue, ok = eventvalue.AsLong(value)
		} else {
			intValue, ok = eventvalue.AsInteger(value)
		}
		if !ok {
			p.addWrongType(c, path.String(pathstyle.ArrayIndexed), attributeName, value, expectedType,
				expectedPrimitiveType(expectedType, primitiveType))
			return
		}
		p.validateNumberRange(c, intValue, path, attributeName, attrDef.Type, typeValidation)
		p.validateTypeValues(c, value, path, attributeName, attrDef.Type, typeValidation)
	case "string_t":
		stringValue, ok := eventvalue.AsString(value)
		if !ok {
			p.addWrongType(c, path.String(pathstyle.ArrayIndexed), attributeName, value, expectedType,
				expectedPrimitiveType(expectedType, primitiveType))
			return
		}
		p.validateStringMaxLen(c, stringValue, path, attributeName, attrDef.Type, typeValidation)
		p.validateStringRegex(c, stringValue, path, attributeName, attrDef.Type, typeValidation)
		p.validateTypeValues(c, value, path, attributeName, attrDef.Type, typeValidation)
	default:
		attributePath := path.String(pathstyle.ArrayIndexed)
		p.addFindingQuote1(
			c,
			validation.SchemaBugPrimitiveTypeUnknown,
			jsonish.Map{
				"attribute_path": attributePath,
				"attribute":      attributeName,
				"type":           attrDef.Type,
			},
			"SCHEMA BUG: Unknown primitive type ",
			primitiveType,
			".",
		)
	}
}

// validateArrayPrimitiveValue keeps primitive dispatch together because it is an event-processing hot path and each
// switch branch directly expresses one OCSF primitive's validation contract.
func (p *validationProcessor) validateArrayPrimitiveValue(
	c *processContext,
	values eventvalue.ArrayView,
	arrayIndex int,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	path *eventpath.Path,
) {
	valueAny := func() any { return values.At(arrayIndex) }
	typeValidation := p.cache.Types[attrDef.Type]
	if typeValidation == nil || typeValidation.Definition == nil {
		p.validatePrimitiveValue(c, valueAny(), path, attributeName, attrDef)
		return
	}
	primitiveType := typeValidation.PrimitiveType
	wrongType := func() {
		p.addWrongType(
			c,
			path.String(pathstyle.ArrayIndexed),
			attributeName,
			valueAny(),
			attrDef.Type,
			expectedPrimitiveType(attrDef.Type, primitiveType),
		)
	}

	switch primitiveType {
	case "json_t":
		// json_t's structured-or-scalar value has no fixed shape to validate.
	case "boolean_t":
		value, ok := values.AsBooleanAt(arrayIndex)
		if !ok {
			wrongType()
			return
		}
		if typeValidation.HasValue && !typeValidation.Value.ContainsBool(value) {
			p.validateTypeValues(
				c, valueAny(), path, attributeName, attrDef.Type, typeValidation,
			)
		}
	case "float_t":
		value, ok := values.AsFloatAt(arrayIndex)
		if !ok {
			wrongType()
			return
		}
		if typeValidation.HasRange {
			if integer, integral := values.AsIntegerAt(arrayIndex); integral {
				if integer < typeValidation.Range.Low || integer > typeValidation.Range.High {
					p.validateNumberRange(c, integer, path, attributeName, attrDef.Type, typeValidation)
				}
			} else if floatOutsideRange(value, typeValidation.Range.Low, typeValidation.Range.High) {
				p.validateNumberRange(c, value, path, attributeName, attrDef.Type, typeValidation)
			}
		}
		if typeValidation.HasValue && !typeValidation.Value.ContainsFloat64(value) {
			p.validateTypeValues(
				c, valueAny(), path, attributeName, attrDef.Type, typeValidation,
			)
		}
	case "integer_t", "long_t":
		var value int64
		var ok bool
		if primitiveType == "long_t" {
			value, ok = values.AsLongAt(arrayIndex)
		} else {
			value, ok = values.AsIntegerAt(arrayIndex)
		}
		if !ok {
			wrongType()
			return
		}
		if typeValidation.HasRange &&
			(value < typeValidation.Range.Low || value > typeValidation.Range.High) {
			p.validateNumberRange(c, value, path, attributeName, attrDef.Type, typeValidation)
		}
		if typeValidation.HasValue && !typeValidation.Value.ContainsInt64(value) {
			p.validateTypeValues(
				c, valueAny(), path, attributeName, attrDef.Type, typeValidation,
			)
		}
	case "string_t":
		value, ok := values.AsStringAt(arrayIndex)
		if !ok {
			wrongType()
			return
		}
		p.validateStringMaxLen(c, value, path, attributeName, attrDef.Type, typeValidation)
		p.validateStringRegex(c, value, path, attributeName, attrDef.Type, typeValidation)
		if typeValidation.HasValue && !typeValidation.Value.ContainsString(value) {
			p.validateTypeValues(
				c, valueAny(), path, attributeName, attrDef.Type, typeValidation,
			)
		}
	default:
		p.validatePrimitiveValue(c, valueAny(), path, attributeName, attrDef)
	}
}

func expectedPrimitiveType(expectedType, primitiveType string) string {
	if expectedType == primitiveType {
		return ""
	}
	return " (" + primitiveType + ")"
}

func (p *validationProcessor) validateTypeValues(
	c *processContext,
	value any,
	path *eventpath.Path,
	attributeName string,
	attributeTypeName string,
	typeValidation *validationcache.TypeValidation,
) {
	if !typeValidation.HasValue {
		return
	}
	constraint := typeValidation.Value
	if constraint.Contains(value) {
		return
	}
	attributePath := path.String(pathstyle.ArrayIndexed)
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
	}
	var code validation.Code
	var message string
	if constraint.TypeName != attributeTypeName {
		code = validation.AttributeValueNotInSuperTypeValues
		details["super_type"] = constraint.TypeName
		message = "Attribute " + strconv.Quote(attributePath) + ", type " + strconv.Quote(attributeTypeName) +
			", value is not in super type " + strconv.Quote(constraint.TypeName) + " list of allowed values."
	} else {
		code = validation.AttributeValueNotInTypeValues
		message = "Attribute " + strconv.Quote(attributePath) + " value is not in type " +
			strconv.Quote(attributeTypeName) + " list of allowed values."
	}
	p.addFinding(c, code, details, message)
}

func (p *validationProcessor) validateNumberRange(
	c *processContext,
	numericValue any,
	path *eventpath.Path,
	attributeName string,
	attributeTypeName string,
	typeValidation *validationcache.TypeValidation,
) {
	if !typeValidation.HasRange {
		return
	}
	constraint := typeValidation.Range

	low := constraint.Low
	high := constraint.High
	outside := false
	switch value := numericValue.(type) {
	case int64:
		outside = value < low || value > high
	case float64:
		outside = floatOutsideRange(value, low, high)
	default:
		return
	}
	if !outside {
		return
	}
	attributePath := path.String(pathstyle.ArrayIndexed)
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
		"range":          []int64{low, high},
	}
	var code validation.Code
	var message string
	if constraint.TypeName != attributeTypeName {
		code = validation.AttributeValueExceedsSuperTypeRange
		details["super_type"] = constraint.TypeName
		details["super_type_range"] = []int64{low, high}
		message = "Attribute " + strconv.Quote(attributePath) + ", type " + strconv.Quote(attributeTypeName) +
			", value is outside super type " + strconv.Quote(constraint.TypeName) + " range of " +
			strconv.FormatInt(low, 10) + " to " + strconv.FormatInt(high, 10) + "."
	} else {
		code = validation.AttributeValueExceedsRange
		message = "Attribute " + strconv.Quote(attributePath) + " value is outside type " +
			strconv.Quote(attributeTypeName) + " range of " + strconv.FormatInt(low, 10) + " to " +
			strconv.FormatInt(high, 10) + "."
	}
	p.addFinding(c, code, details, message)
}

func (p *validationProcessor) validateFloatRange(
	c *processContext,
	originalValue any,
	floatValue float64,
	path *eventpath.Path,
	attributeName string,
	attributeTypeName string,
	typeValidation *validationcache.TypeValidation,
) {
	if !typeValidation.HasRange {
		return
	}
	if integer, integral := eventvalue.AsInteger(originalValue); integral {
		p.validateNumberRange(c, integer, path, attributeName, attributeTypeName, typeValidation)
		return
	}
	p.validateNumberRange(c, floatValue, path, attributeName, attributeTypeName, typeValidation)
}

func floatOutsideRange(value float64, low, high int64) bool {
	return math.IsNaN(value) ||
		math.IsInf(value, 0) ||
		floatLessThanInt64(value, low) ||
		floatGreaterThanInt64(value, high)
}

// Compare in the integer domain when possible. Converting a bound such as math.MaxInt64 to
// float64 rounds it to 1<<63 and would incorrectly admit an out-of-range floating-point value.
func floatLessThanInt64(value float64, bound int64) bool {
	if value < math.MinInt64 {
		return true
	}
	if value >= float64(uint64(1)<<63) {
		return false
	}
	integer := int64(value)
	if integer != bound {
		return integer < bound
	}
	return value < float64(bound)
}

func floatGreaterThanInt64(value float64, bound int64) bool {
	if value >= float64(uint64(1)<<63) {
		return true
	}
	if value < math.MinInt64 {
		return false
	}
	integer := int64(value)
	if integer != bound {
		return integer > bound
	}
	return value > float64(bound)
}

func (p *validationProcessor) validateStringMaxLen(
	c *processContext,
	value string,
	path *eventpath.Path,
	attributeName string,
	attributeTypeName string,
	typeValidation *validationcache.TypeValidation,
) {
	if !typeValidation.HasMaxLen {
		return
	}
	constraint := typeValidation.MaxLen

	length := utf8.RuneCountInString(value)
	maxLen := constraint.MaxLen
	if int64(length) <= maxLen {
		return
	}
	attributePath := path.String(pathstyle.ArrayIndexed)
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
		"length":         length,
		"max_len":        maxLen,
	}
	var code validation.Code
	var message string
	if constraint.TypeName != attributeTypeName {
		code = validation.AttributeValueExceedsSuperTypeMaxLen
		details["super_type"] = constraint.TypeName
		message = "Attribute " + strconv.Quote(attributePath) + ", type " + strconv.Quote(attributeTypeName) +
			", value length " + strconv.Itoa(length) + " exceeds super type " + strconv.Quote(constraint.TypeName) +
			" max length " + strconv.FormatInt(maxLen, 10) + "."
	} else {
		code = validation.AttributeValueExceedsMaxLen
		message = "Attribute " + strconv.Quote(attributePath) + " value length of " + strconv.Itoa(length) +
			" exceeds type " + strconv.Quote(attributeTypeName) + " max length " + strconv.FormatInt(maxLen, 10) + "."
	}
	p.addFinding(c, code, details, message)
}

func (p *validationProcessor) validateStringRegex(
	c *processContext,
	value string,
	path *eventpath.Path,
	attributeName string,
	attributeTypeName string,
	typeValidation *validationcache.TypeValidation,
) {
	if !typeValidation.HasRegex {
		return
	}
	constraint := typeValidation.Regex

	if constraint.Err != nil {
		attributePath := path.String(pathstyle.ArrayIndexed)
		p.addFindingQuote1String1(
			c,
			validation.SchemaBugTypeRegexInvalid,
			jsonish.Map{
				"attribute_path":      attributePath,
				"attribute":           attributeName,
				"type":                constraint.TypeName,
				"regex":               constraint.Regex,
				"regex_error_message": constraint.Err.Error(),
			},
			"SCHEMA BUG: Type ",
			constraint.TypeName,
			" specifies an invalid regex: ",
			constraint.Err.Error(),
			".",
		)
		return
	}
	if constraint.Compiled.MatchString(value) {
		return
	}
	attributePath := path.String(pathstyle.ArrayIndexed)
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attributeName,
		"type":           attributeTypeName,
		"regex":          constraint.Regex,
	}
	var code validation.Code
	var message string
	if constraint.TypeName != attributeTypeName {
		code = validation.AttributeValueSuperTypeRegexNotMatched
		details["super_type"] = constraint.TypeName
		message = "Attribute " + strconv.Quote(attributePath) + ", type " + strconv.Quote(attributeTypeName) +
			", value does not match regex of super type " + strconv.Quote(constraint.TypeName) + "."
	} else {
		code = validation.AttributeValueRegexNotMatched
		message = "Attribute " + strconv.Quote(attributePath) + " value does not match regex of type " +
			strconv.Quote(attributeTypeName) + "."
	}
	p.addFinding(c, code, details, message)
}

func (p *validationProcessor) validateVersion(c *processContext, event jsonish.Map) {
	metadata, ok := event["metadata"].(jsonish.Map)
	if !ok {
		return
	}
	versionValue, present := eventvalue.Attribute(metadata, "version")
	if !present {
		return
	}
	version, ok := eventvalue.AsString(versionValue)
	if !ok {
		return
	}

	if !p.cache.VersionOK {
		return
	}
	schemaVersion := p.cache.Version
	eventVersion, eventVersionOK := schemaVersion, true
	if version != c.compiled.Version {
		eventVersion, eventVersionOK = semver.Parse(version)
	}
	if !eventVersionOK {
		p.addFinding(
			c,
			validation.VersionInvalidFormat,
			jsonish.Map{
				"attribute_path": "metadata.version",
				"attribute":      "version",
				"expected_regex": semver.Pattern,
			},
			"Event version at \"metadata.version\" has invalid format; expected semantic versioning format.",
		)
		return
	}
	comparison := semver.Compare(eventVersion, schemaVersion)
	if comparison == 0 {
		return
	}
	if comparison < 0 {
		switch {
		case eventVersion.IsInitialDevelopment():
			p.addFindingQuote1(
				c,
				validation.VersionIncompatibleInitialDevelopment,
				jsonish.Map{
					"attribute_path": "metadata.version",
					"attribute":      "version",
				},
				"Event version at \"metadata.version\" is an initial development version"+
					" and is incompatible with schema version ",
				c.compiled.Version,
				".",
			)
		case eventVersion.IsPrerelease():
			p.addFindingQuote1(
				c,
				validation.VersionIncompatiblePrerelease,
				jsonish.Map{
					"attribute_path": "metadata.version",
					"attribute":      "version",
				},
				"Event version at \"metadata.version\" is a prerelease version"+
					" and is incompatible with schema version ",
				c.compiled.Version,
				".",
			)
		default:
			p.addFindingQuote1(
				c,
				validation.VersionEarlier,
				jsonish.Map{
					"attribute_path": "metadata.version",
					"attribute":      "version",
				},
				"Event version at \"metadata.version\" is earlier than schema version ",
				c.compiled.Version,
				".",
			)
		}
		return
	}

	p.addFindingQuote1(
		c,
		validation.VersionIncompatibleLater,
		jsonish.Map{
			"attribute_path": "metadata.version",
			"attribute":      "version",
		},
		"Event version at \"metadata.version\" is incompatible with schema version ",
		c.compiled.Version,
		" because it is a later version.",
	)
}

func (p *validationProcessor) validateTypeUID(c *processContext, event jsonish.Map) {
	classUID, classOK := eventvalue.AsInteger(event["class_uid"])
	activityID, activityOK := eventvalue.AsInteger(event["activity_id"])
	typeUID, typeOK := eventvalue.AsInteger(event["type_uid"])
	if !classOK || !activityOK || !typeOK {
		return
	}

	expectedTypeUID, ok := schema.ExpectedTypeUID(classUID, activityID)
	if !ok {
		p.addFinding(
			c,
			validation.TypeUIDExpectedValueOverflow,
			jsonish.Map{
				"attribute_path": "type_uid",
				"attribute":      "type_uid",
			},
			"The expected \"type_uid\" derived from \"class_uid\" and \"activity_id\""+
				" cannot be represented as an int64.",
		)
		return
	}
	if typeUID == expectedTypeUID {
		return
	}
	p.addFinding(
		c,
		validation.TypeUIDIncorrect,
		jsonish.Map{
			"attribute_path": "type_uid",
			"attribute":      "type_uid",
		},
		"Event \"type_uid\" does not match the value derived from \"class_uid\" and \"activity_id\".",
	)
}

func (p *validationProcessor) validateConstraints(
	c *processContext,
	eventItem jsonish.Map,
	itemDefinition *schema.ItemDefinition,
	path *eventpath.Path,
) {
	if itemDefinition == nil || len(itemDefinition.Constraints) == 0 {
		return
	}

	for _, constraintKey := range itemDefinition.OrderedConstraintKeys {
		constraintDetails := itemDefinition.Constraints[constraintKey]
		switch constraintKey {
		case "at_least_one":
			if anyConstraintPathPresent(eventItem, constraintDetails) {
				continue
			}
			description, details := p.constraintInfo(c, itemDefinition, path, constraintKey, constraintDetails)
			p.addFindingString1(
				c,
				validation.ConstraintFailed,
				details,
				"Constraint failed: ",
				description,
				"; expected at least one constraint attribute, but got none.",
			)
		case "just_one":
			count := countConstraintPathsPresent(eventItem, constraintDetails)
			if count == 1 {
				continue
			}
			description, details := p.constraintInfo(c, itemDefinition, path, constraintKey, constraintDetails)
			details["value_count"] = count
			p.addFindingString1Int1(
				c,
				validation.ConstraintFailed,
				details,
				"Constraint failed: ",
				description,
				"; expected exactly 1 constraint attribute, got ",
				int64(count),
				".",
			)
		default:
			description, details := p.constraintInfo(c, itemDefinition, path, constraintKey, constraintDetails)
			p.addFindingString1(
				c,
				validation.ConstraintUnknown,
				details,
				"SCHEMA BUG: Unknown constraint ",
				description,
				".",
			)
		}
	}
}

func (p *validationProcessor) constraintInfo(
	c *processContext,
	itemDefinition *schema.ItemDefinition,
	path *eventpath.Path,
	constraintKey string,
	constraintDetails []string,
) (string, jsonish.Map) {
	constraint := jsonish.Map{constraintKey: slices.Clone(constraintDetails)}
	if path != nil {
		attributePath := path.String(pathstyle.ArrayIndexed)
		description := strconv.Quote(constraintKey) + " from object " + strconv.Quote(itemDefinition.Name) + " at " +
			strconv.Quote(attributePath)
		details := jsonish.Map{
			"attribute_path": attributePath,
			"constraint":     constraint,
			"object_name":    itemDefinition.Name,
		}
		return description, details
	}
	description := strconv.Quote(constraintKey) + " from class " + strconv.Quote(c.class.Name) + " uid " +
		strconv.FormatInt(c.class.Uid, 10)
	details := jsonish.Map{
		"constraint": constraint,
		"class_uid":  c.class.Uid,
		"class_name": c.class.Name,
	}
	return description, details
}

func anyConstraintPathPresent(eventItem jsonish.Map, paths []string) bool {
	for _, path := range paths {
		if eventvalue.HasPathOrKey(eventItem, path) {
			return true
		}
	}
	return false
}

func countConstraintPathsPresent(eventItem jsonish.Map, paths []string) int {
	count := 0
	for _, path := range paths {
		if eventvalue.HasPathOrKey(eventItem, path) {
			count++
		}
	}
	return count
}

func (p *validationProcessor) validateObservables(c *processContext, event jsonish.Map) error {
	analyzer, present := observable.NewAnalyzer(event, c.class, c.compiled.Objects, c.activeProfiles)
	if !present {
		return nil
	}
	if c.generatedObservablesStart >= 0 {
		analyzer.LimitEntries(c.generatedObservablesStart)
	}
	var observablePath eventpath.Path
	observablePath.PushAttribute("observables")
	for {
		index, entry, ok, err := analyzer.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		observablePath.PushArrayIndex(index)
		if entry.Problem == observable.ProblemTraversalLimited {
			c.addProcessingTraversalLimitIssue(observablePath.String(pathstyle.ArrayIndexed), "observables", "")
			observablePath.Pop()
			continue
		}
		if p.config.PathNotationConfigured && entry.PathDefined {
			p.validateObservablePathNotation(c, index, entry.Path, &observablePath, p.config.PathNotation)
		}
		if code, known := nonStructuralObservableValidationCode(entry.Problem); known {
			if diagnostic, present := observable.EntryToDiagnostic(entry, index, &observablePath); present {
				p.addFinding(c, code, diagnostic.Details, diagnostic.Message)
			}
		}
		observablePath.Pop()
	}
	if c.generatedObservablesStart >= 0 && p.config.PathNotationConfigured &&
		p.config.PathNotation != c.generatedObservablesPathNotation {
		p.validateGeneratedObservablePathNotation(c, event, &observablePath, p.config.PathNotation)
	}
	return nil
}

func nonStructuralObservableValidationCode(problem observable.Problem) (validation.Code, bool) {
	switch problem {
	case observable.ProblemNameInvalidSyntax:
		return validation.ObservableNameInvalidSyntax, true
	case observable.ProblemNameInvalidReference:
		return validation.ObservableNameInvalidReference, true
	case observable.ProblemPathNotFound:
		return validation.ObservablePathNotFound, true
	case observable.ProblemPathNotObject:
		return validation.ObservablePathNotObject, true
	case observable.ProblemValueNotFound:
		return validation.ObservableValueNotFound, true
	default:
		return validation.None, false
	}
}

func (p *validationProcessor) validateGeneratedObservablePathNotation(
	c *processContext,
	event jsonish.Map,
	observablePath *eventpath.Path,
	preferred pathstyle.Style,
) {
	observables, ok := eventvalue.NewArrayView(event["observables"])
	if !ok {
		return
	}
	for index := c.generatedObservablesStart; index < observables.Len(); index++ {
		entry, ok := observables.At(index).(jsonish.Map)
		if !ok {
			continue
		}
		name, ok := eventvalue.AsString(entry["name"])
		if !ok {
			continue
		}
		path, err := observablepath.Parse(name)
		if err != nil {
			continue
		}
		observablePath.PushArrayIndex(index)
		p.validateObservablePathNotation(c, index, path, observablePath, preferred)
		observablePath.Pop()
	}
}

func (p *validationProcessor) validateObservablePathNotation(
	c *processContext,
	index int,
	path observablepath.Path,
	observablePath *eventpath.Path,
	preferred pathstyle.Style,
) {
	if path.UsesNotation(preferred, c.class, c.compiled.Objects) {
		return
	}
	attributePath := observablePath.ChildString("name", pathstyle.ArrayIndexed)
	p.addFindingInt1Quote1(
		c,
		validation.ObservableNamePathNotation,
		jsonish.Map{
			"attribute_path":          attributePath,
			"attribute":               "name",
			"preferred_path_notation": preferred,
		},
		"Observable index ",
		int64(index),
		" name does not use the preferred ",
		string(preferred),
		" path notation.",
	)
}

func (p *validationProcessor) addRequiredAttributeMissing(
	c *processContext,
	attributePath string,
	attributeName string,
) {
	p.addFindingQuote1(
		c,
		validation.AttributeRequiredMissing,
		jsonish.Map{
			"attribute_path": attributePath,
			"attribute":      attributeName,
		},
		"Required attribute ",
		attributePath,
		" is missing.",
	)
}

func (p *validationProcessor) addWrongType(
	c *processContext,
	attributePath string,
	attributeName string,
	value any,
	expectedType string,
	expectedTypeExtra string,
) {
	valueType, valueTypeExtra := eventvalue.DescribeType(value)
	p.addFindingQuote1String4(
		c,
		validation.AttributeWrongType,
		jsonish.Map{
			"attribute_path": attributePath,
			"attribute":      attributeName,
			"value_type":     valueType,
			"expected_type":  expectedType,
		},
		"Attribute ",
		attributePath,
		" value has wrong type; expected ",
		expectedType,
		"",
		expectedTypeExtra,
		", got ",
		valueType,
		"",
		valueTypeExtra,
		".",
	)
}

func (p *validationProcessor) addFinding(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	message string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(c, level, code, details, message)
}

// The addFinding formatting-helper suffixes encode dynamic values in message order: Quote uses strconv.Quote, String
// copies a string verbatim, and Int renders a base-10 integer. The count is the number of adjacent values of that kind.
// These suppression-aware, fixed-argument helpers defer conversion and message allocation until a finding is known to
// be reportable, avoiding the reflection, boxing, and escaping behavior of fmt.Sprintf with variadic any arguments.
func (p *validationProcessor) addFindingQuote1(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	value string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(c, level, code, details, prefix+strconv.Quote(value)+suffix)
}

// addFindingQuote2 is the two-value form of addFindingQuote1.
func (p *validationProcessor) addFindingQuote2(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	firstValue string,
	middle string,
	secondValue string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(
		c, level, code, details,
		prefix+strconv.Quote(firstValue)+middle+strconv.Quote(secondValue)+suffix,
	)
}

// addFindingQuote3 is the three-value form of addFindingQuote1.
func (p *validationProcessor) addFindingQuote3(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	firstValue string,
	firstMiddle string,
	secondValue string,
	secondMiddle string,
	thirdValue string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(
		c, level, code, details,
		"Attribute "+strconv.Quote(firstValue)+firstMiddle+strconv.Quote(secondValue)+secondMiddle+
			strconv.Quote(thirdValue)+suffix,
	)
}

func (p *validationProcessor) addFindingString1(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	value string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(c, level, code, details, prefix+value+suffix)
}

func (p *validationProcessor) addFindingString2(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	firstValue string,
	middle string,
	secondValue string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(c, level, code, details, prefix+firstValue+middle+secondValue+suffix)
}

func (p *validationProcessor) addFindingQuote1String1(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	quotedValue string,
	middle string,
	stringValue string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(c, level, code, details, prefix+strconv.Quote(quotedValue)+middle+stringValue+suffix)
}

func (p *validationProcessor) addFindingQuote2String1(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	firstQuotedValue string,
	firstMiddle string,
	secondQuotedValue string,
	secondMiddle string,
	stringValue string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(
		c, level, code, details,
		prefix+strconv.Quote(firstQuotedValue)+firstMiddle+strconv.Quote(secondQuotedValue)+secondMiddle+
			stringValue+suffix,
	)
}

func (p *validationProcessor) addFindingQuote1Int1String1(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	quotedValue string,
	firstMiddle string,
	intValue int64,
	secondMiddle string,
	stringValue string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(
		c, level, code, details,
		prefix+strconv.Quote(quotedValue)+firstMiddle+strconv.FormatInt(intValue, 10)+secondMiddle+stringValue+suffix,
	)
}

func (p *validationProcessor) addFindingQuote2Int2(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	firstQuotedValue string,
	firstMiddle string,
	secondQuotedValue string,
	secondMiddle string,
	firstIntValue int64,
	thirdMiddle string,
	secondIntValue int64,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(
		c, level, code, details,
		prefix+strconv.Quote(firstQuotedValue)+firstMiddle+strconv.Quote(secondQuotedValue)+secondMiddle+
			strconv.FormatInt(firstIntValue, 10)+thirdMiddle+strconv.FormatInt(secondIntValue, 10)+suffix,
	)
}

func (p *validationProcessor) addFindingString1Int1(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	stringValue string,
	middle string,
	intValue int64,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(
		c, level, code, details,
		prefix+stringValue+middle+strconv.FormatInt(intValue, 10)+suffix,
	)
}

func (p *validationProcessor) addFindingInt1Quote1(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	intValue int64,
	middle string,
	quotedValue string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(
		c, level, code, details,
		prefix+strconv.FormatInt(intValue, 10)+middle+strconv.Quote(quotedValue)+suffix,
	)
}

func (p *validationProcessor) addFindingQuote1String4(
	c *processContext,
	code validation.Code,
	details jsonish.Map,
	prefix string,
	quotedValue string,
	firstMiddle string,
	firstStringValue string,
	secondMiddle string,
	secondStringValue string,
	thirdMiddle string,
	thirdStringValue string,
	fourthMiddle string,
	fourthStringValue string,
	suffix string,
) {
	level, report := p.findingLevel(c, code)
	if !report {
		return
	}
	p.appendFinding(
		c, level, code, details,
		prefix+strconv.Quote(quotedValue)+firstMiddle+firstStringValue+secondMiddle+secondStringValue+
			thirdMiddle+thirdStringValue+fourthMiddle+fourthStringValue+suffix,
	)
}

func (*validationProcessor) appendFinding(
	c *processContext,
	level validation.Level,
	code validation.Code,
	details jsonish.Map,
	message string,
) {
	c.result.Validation.Findings = append(
		c.result.Validation.Findings,
		eventresult.ValidationFinding{
			Level: level, Code: code, Message: sanitizeDiagnosticMessage(message), Details: details,
		},
	)
}

func (p *validationProcessor) findingLevel(c *processContext, code validation.Code) (validation.Level, bool) {
	defaultLevel := code.DefaultLevel()
	if p.policy.settings != nil {
		if setting, configured := p.policy.settings[code]; configured {
			return p.applyValidationPolicySetting(c, defaultLevel, setting)
		}
	}
	if p.policy.suppressAll && code.Suppressible() {
		return p.suppressFinding(c, defaultLevel)
	}
	if defaultLevel == validation.LevelError {
		if p.policy.errorsAsWarnings {
			return validation.LevelWarning, true
		}
	} else {
		if p.policy.warningsAsErrors {
			return validation.LevelError, true
		}
	}
	return defaultLevel, true
}

func (p *validationProcessor) applyValidationPolicySetting(
	c *processContext,
	defaultLevel validation.Level,
	setting validationPolicySetting,
) (validation.Level, bool) {
	if setting.suppressed {
		return p.suppressFinding(c, defaultLevel)
	}
	return setting.level, true
}

func (*validationProcessor) suppressFinding(
	c *processContext,
	defaultLevel validation.Level,
) (validation.Level, bool) {
	if defaultLevel == validation.LevelError {
		c.result.Validation.SuppressedErrorCount++
	} else {
		c.result.Validation.SuppressedWarningCount++
	}
	return 0, false
}
