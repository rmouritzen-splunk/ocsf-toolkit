package eventschema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/coerce"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

type observableResolution struct {
	entries []observableEntryResolution
}

type observableEntryResolution struct {
	index      int
	removable  bool
	removed    bool
	diagnostic *processingDiagnostic
}

type observablePath struct {
	segments []observablePathSegment
}

type observablePathSegment struct {
	attribute string
	selectors []observableArraySelector
}

type observableArraySelector struct {
	all   bool
	index int
}

func (c *processingContext) resolveObservables(event jsonish.Map) *observableResolution {
	if c.observableResolution != nil {
		return c.observableResolution
	}
	resolution := &observableResolution{}
	c.observableResolution = resolution

	value, present := event["observables"]
	if !present {
		return resolution
	}
	observables, ok := asSlice(value)
	if !ok {
		resolution.entries = append(resolution.entries, observableEntryResolution{
			index: -1,
			diagnostic: newProcessingDiagnostic(
				"observable_array_wrong_type",
				"The observables attribute is not an array.",
				jsonish.Map{"attribute_path": "observables", "attribute": "observables", "value": value},
			),
		})
		return resolution
	}

	resolution.entries = make([]observableEntryResolution, 0, len(observables))
	for index, value := range observables {
		resolution.entries = append(resolution.entries, c.resolveObservableEntry(event, index, value))
	}
	return resolution
}

func (c *processingContext) resolveObservableEntry(
	event jsonish.Map,
	index int,
	value any,
) observableEntryResolution {
	result := observableEntryResolution{index: index}
	attributePath := makeArrayElementPath("observables", index)
	observable, ok := value.(jsonish.Map)
	if !ok {
		result.diagnostic = newProcessingDiagnostic(
			"observable_element_wrong_type",
			fmt.Sprintf("Observable index %d is not an object.", index),
			jsonish.Map{"attribute_path": attributePath, "attribute": "observables", "value": value},
		)
		return result
	}

	nameValue, namePresent := observable["name"]
	if !namePresent {
		result.diagnostic = newProcessingDiagnostic(
			"observable_name_missing",
			fmt.Sprintf("Observable index %d is missing its name attribute.", index),
			jsonish.Map{"attribute_path": makeAttributePath(attributePath, "name"), "attribute": "name"},
		)
		return result
	}
	name, ok := nameValue.(string)
	if !ok {
		result.diagnostic = newProcessingDiagnostic(
			"observable_name_wrong_type",
			fmt.Sprintf("Observable index %d name is not a string.", index),
			jsonish.Map{
				"attribute_path": makeAttributePath(attributePath, "name"),
				"attribute":      "name",
				"value":          nameValue,
			},
		)
		return result
	}

	path, err := parseObservablePath(name)
	if err != nil {
		result.diagnostic = newProcessingDiagnostic(
			"observable_name_invalid_syntax",
			fmt.Sprintf("Observable index %d name %q has invalid path syntax: %s.", index, name, err),
			observableDetails(attributePath, name, observable["value"]),
		)
		return result
	}
	if path.segments[0].attribute == "observables" || !c.observablePathDefined(path) {
		result.diagnostic = newProcessingDiagnostic(
			"observable_name_invalid_reference",
			fmt.Sprintf("Observable index %d name %q does not refer to an attribute defined for the event class.", index, name),
			observableDetails(attributePath, name, observable["value"]),
		)
		return result
	}

	candidates := resolveObservablePath(event, path)
	if len(candidates) == 0 {
		result.diagnostic = newProcessingDiagnostic(
			"observable_path_not_found",
			fmt.Sprintf("Observable index %d name %q does not resolve to a value in the event.", index, name),
			observableDetails(attributePath, name, observable["value"]),
		)
		return result
	}

	observableValue, valuePresent := observable["value"]
	if !valuePresent {
		if anyObservableObject(candidates) {
			result.removable = true
			return result
		}
		result.diagnostic = newProcessingDiagnostic(
			"observable_path_not_object",
			fmt.Sprintf("Observable index %d without a value does not refer to an object at name %q.", index, name),
			observableDetails(attributePath, name, nil),
		)
		return result
	}
	if observableValue == nil {
		if anyNilObservableValue(candidates) {
			result.removable = true
			return result
		}
		result.diagnostic = newProcessingDiagnostic(
			"observable_value_not_found",
			fmt.Sprintf("Observable index %d null value is not present at name %q.", index, name),
			observableDetails(attributePath, name, nil),
		)
		return result
	}
	valueString, ok := observableValue.(string)
	if !ok {
		result.diagnostic = newProcessingDiagnostic(
			"observable_value_wrong_type",
			fmt.Sprintf("Observable index %d value is not a string or null.", index),
			observableDetails(attributePath, name, observableValue),
		)
		return result
	}
	if observableStringValueFound(candidates, valueString) {
		result.removable = true
		return result
	}
	result.diagnostic = newProcessingDiagnostic(
		"observable_value_not_found",
		fmt.Sprintf("Observable index %d value %q is not present at name %q.", index, valueString, name),
		observableDetails(attributePath, name, valueString),
	)
	return result
}

func observableDetails(attributePath, name string, value any) jsonish.Map {
	details := jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      "observables",
		"name":           name,
	}
	if value != nil {
		details["value"] = value
	}
	return details
}

func parseObservablePath(name string) (observablePath, error) {
	if strings.HasPrefix(name, "$") {
		if name == "$" {
			return observablePath{}, fmt.Errorf("the root marker must be followed by an attribute")
		}
		if !strings.HasPrefix(name, "$.") {
			return observablePath{}, fmt.Errorf("the root marker must be followed by a dot")
		}
		name = name[2:]
	}
	if name == "" {
		return observablePath{}, fmt.Errorf("the path is empty")
	}

	parts := strings.Split(name, ".")
	path := observablePath{segments: make([]observablePathSegment, 0, len(parts))}
	for _, part := range parts {
		segment, err := parseObservablePathSegment(part)
		if err != nil {
			return observablePath{}, err
		}
		path.segments = append(path.segments, segment)
	}
	return path, nil
}

func parseObservablePathSegment(part string) (observablePathSegment, error) {
	if part == "" {
		return observablePathSegment{}, fmt.Errorf("the path contains an empty attribute")
	}
	open := strings.IndexByte(part, '[')
	if open < 0 {
		return observablePathSegment{attribute: part}, nil
	}
	if open == 0 {
		return observablePathSegment{}, fmt.Errorf("an array selector has no attribute")
	}
	segment := observablePathSegment{attribute: part[:open]}
	remainder := part[open:]
	for remainder != "" {
		if remainder[0] != '[' {
			return observablePathSegment{}, fmt.Errorf("unexpected text after an array selector")
		}
		close := strings.IndexByte(remainder, ']')
		if close < 0 {
			return observablePathSegment{}, fmt.Errorf("an array selector is not closed")
		}
		selectorText := remainder[1:close]
		selector := observableArraySelector{}
		switch selectorText {
		case "", "*":
			selector.all = true
		default:
			for _, r := range selectorText {
				if r < '0' || r > '9' {
					return observablePathSegment{}, fmt.Errorf("array selector %q is not a non-negative index", selectorText)
				}
			}
			index, err := strconv.Atoi(selectorText)
			if err != nil {
				return observablePathSegment{}, fmt.Errorf("array index %q is too large", selectorText)
			}
			selector.index = index
		}
		segment.selectors = append(segment.selectors, selector)
		remainder = remainder[close+1:]
	}
	return segment, nil
}

func (c *processingContext) observablePathDefined(path observablePath) bool {
	itemDefinition := &c.class.commonItemDefinition
	for index, segment := range path.segments {
		attributes := c.filterAttributes(itemDefinition.Attributes)
		attrDef, present := attributes[segment.attribute]
		if !present || attrDef == nil {
			return false
		}
		if len(segment.selectors) > 0 && (attrDef.IsArray == nil || !*attrDef.IsArray) {
			return false
		}
		if len(segment.selectors) > 1 {
			return false
		}
		if index == len(path.segments)-1 {
			return true
		}
		if attrDef.Type != "object_t" || attrDef.ObjectType == nil {
			return false
		}
		objectDef, present := c.objects[*attrDef.ObjectType]
		if !present || objectDef == nil {
			return false
		}
		itemDefinition = &objectDef.commonItemDefinition
	}
	return false
}

func resolveObservablePath(event jsonish.Map, path observablePath) []any {
	current := []any{event}
	for _, segment := range path.segments {
		next := make([]any, 0)
		for _, value := range current {
			resolveObservableSegment(value, segment, &next)
		}
		if len(next) == 0 {
			return nil
		}
		current = next
	}
	return flattenObservableCandidates(current)
}

func resolveObservableSegment(value any, segment observablePathSegment, result *[]any) {
	if values, ok := asSlice(value); ok {
		for _, element := range values {
			resolveObservableSegment(element, segment, result)
		}
		return
	}
	item, ok := value.(jsonish.Map)
	if !ok {
		return
	}
	value, present := item[segment.attribute]
	if !present {
		return
	}
	selected := []any{value}
	for _, selector := range segment.selectors {
		selected = applyObservableSelector(selected, selector)
		if len(selected) == 0 {
			return
		}
	}
	*result = append(*result, selected...)
}

func applyObservableSelector(values []any, selector observableArraySelector) []any {
	result := make([]any, 0)
	for _, value := range values {
		array, ok := asSlice(value)
		if !ok {
			continue
		}
		if selector.all {
			result = append(result, array...)
			continue
		}
		if selector.index < len(array) {
			result = append(result, array[selector.index])
		}
	}
	return result
}

func flattenObservableCandidates(values []any) []any {
	result := make([]any, 0, len(values))
	var appendValue func(any)
	appendValue = func(value any) {
		if array, ok := asSlice(value); ok {
			for _, element := range array {
				appendValue(element)
			}
			return
		}
		result = append(result, value)
	}
	for _, value := range values {
		appendValue(value)
	}
	return result
}

func anyObservableObject(values []any) bool {
	for _, value := range values {
		if _, ok := value.(jsonish.Map); ok {
			return true
		}
	}
	return false
}

func anyNilObservableValue(values []any) bool {
	for _, value := range values {
		if value == nil {
			return true
		}
	}
	return false
}

func observableStringValueFound(values []any, expected string) bool {
	for _, value := range values {
		switch value.(type) {
		case jsonish.Map, []any, []jsonish.Map, []string, nil:
			continue
		}
		actual, err := coerce.String(value)
		if err == nil && actual == expected {
			return true
		}
	}
	return false
}
