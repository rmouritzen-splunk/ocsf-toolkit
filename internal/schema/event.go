package schema

import (
	"math"

	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// ExpectedTypeUID calculates class_uid * 100 + activity_id and reports integer overflow.
func ExpectedTypeUID(classUID, activityID int64) (int64, bool) {
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

// ClassResolution identifies the result of resolving an event's class_uid against a compiled schema.
type ClassResolution uint8

const (
	// ClassResolved indicates that class_uid identifies a compiled event class.
	ClassResolved ClassResolution = iota
	// ClassUIDMissing indicates that class_uid is absent or null.
	ClassUIDMissing
	// ClassUIDWrongType indicates that class_uid is not a signed integral value.
	ClassUIDWrongType
	// ClassUIDUnknown indicates that no compiled class has the supplied class_uid.
	ClassUIDUnknown
)

// ResolveEventClass resolves an event's class_uid against the compiled schema.
func (s *Compiled) ResolveEventClass(event jsonish.Map) (*ClassDefinition, int64, ClassResolution) {
	value, present := eventvalue.Attribute(event, "class_uid")
	if !present {
		return nil, 0, ClassUIDMissing
	}
	classUID, valid := eventvalue.AsInteger(value)
	if !valid {
		return nil, 0, ClassUIDWrongType
	}
	class := s.Classes[classUID]
	if class == nil {
		return nil, classUID, ClassUIDUnknown
	}
	return class, classUID, ClassResolved
}

// EventProfileSet returns the string entries in metadata.profiles as an active profile set. Structural validation
// reports malformed values separately.
func EventProfileSet(event jsonish.Map) ProfileSet {
	metadata, ok := event["metadata"].(jsonish.Map)
	if !ok {
		return nil
	}
	profilesValue := metadata["profiles"]
	profiles, ok := eventvalue.NewArrayView(profilesValue)
	if profilesValue == nil || !ok {
		return nil
	}

	result := make(ProfileSet, profiles.Len())
	for index := range profiles.Len() {
		if profile, ok := profiles.AsStringAt(index); ok {
			result[profile] = struct{}{}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// ProfileSet is the active event profile set used for schema attribute filtering.
type ProfileSet map[string]struct{}

// AttributeActive reports whether an attribute is unscoped or belongs to an active event profile.
func AttributeActive(attribute *ItemAttributeDefinition, profiles ProfileSet) bool {
	if attribute == nil {
		return false
	}
	if len(attribute.Profiles) == 0 {
		return true
	}
	for _, profile := range attribute.Profiles {
		if _, present := profiles[profile]; present {
			return true
		}
	}
	return false
}

// EnumSiblingSupported reports whether processing handles this direct integral enum and direct string sibling pair.
// Scalar and array pairs are supported when both attributes have the same shape. Named subtypes do not qualify.
func (*Compiled) EnumSiblingSupported(attribute, sibling *ItemAttributeDefinition) bool {
	if attribute == nil || sibling == nil || sibling.Enum != nil || isArray(attribute) != isArray(sibling) {
		return false
	}
	if sibling.Type != "string_t" {
		return false
	}
	return attribute.Type == "integer_t" || attribute.Type == "long_t"
}

func isArray(attribute *ItemAttributeDefinition) bool {
	return attribute.IsArray != nil && *attribute.IsArray
}
