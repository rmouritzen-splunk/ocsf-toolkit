package schema

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/stretchr/testify/require"
)

func TestResolveEventClassClassifiesClassUID(t *testing.T) {
	class := &ClassDefinition{ItemDefinition: ItemDefinition{Name: "test"}, Uid: 1}
	compiled := &Compiled{Classes: map[int64]*ClassDefinition{1: class}}

	tests := []struct {
		name   string
		event  jsonish.Map
		status ClassResolution
		uid    int64
		class  *ClassDefinition
	}{
		{name: "missing", event: jsonish.Map{}, status: ClassUIDMissing},
		{name: "null", event: jsonish.Map{"class_uid": nil}, status: ClassUIDMissing},
		{name: "wrong type", event: jsonish.Map{"class_uid": "1"}, status: ClassUIDWrongType},
		{name: "unknown", event: jsonish.Map{"class_uid": json.Number("2")}, status: ClassUIDUnknown, uid: 2},
		{
			name:   "resolved",
			event:  jsonish.Map{"class_uid": json.Number("1")},
			status: ClassResolved,
			uid:    1,
			class:  class,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, uid, status := compiled.ResolveEventClass(test.event)
			require.Equal(t, test.status, status)
			require.Equal(t, test.uid, uid)
			require.Same(t, test.class, resolved)
		})
	}
}

func TestEventProfileSetAndAttributeActive(t *testing.T) {
	event := jsonish.Map{"metadata": jsonish.Map{
		"profiles": []any{"cloud", json.Number("1"), "ldap", "cloud"},
	}}
	profileSet := EventProfileSet(event)
	require.Equal(t, ProfileSet{"cloud": {}, "ldap": {}}, profileSet)
	require.True(t, AttributeActive(&ItemAttributeDefinition{}, profileSet))
	require.True(t, AttributeActive(&ItemAttributeDefinition{Profiles: []string{"ldap"}}, profileSet))
	require.False(t, AttributeActive(&ItemAttributeDefinition{Profiles: []string{"security_control"}}, profileSet))
	require.False(t, AttributeActive(nil, profileSet))
}

func TestEventProfileSetReturnsNilWithoutStringProfiles(t *testing.T) {
	tests := []jsonish.Map{
		nil,
		{},
		{"metadata": nil},
		{"metadata": jsonish.Map{}},
		{"metadata": jsonish.Map{"profiles": "cloud"}},
		{"metadata": jsonish.Map{"profiles": []any{json.Number("1"), nil}}},
	}
	for _, event := range tests {
		require.Nil(t, EventProfileSet(event))
	}
}

func TestEventProfileSetAllocationCeiling(t *testing.T) {
	event := jsonish.Map{"metadata": jsonish.Map{"profiles": []any{"cloud", "ldap"}}}
	allocations := testing.AllocsPerRun(1000, func() {
		if len(EventProfileSet(event)) != 2 {
			panic("unexpected profile set")
		}
	})
	require.LessOrEqual(t, allocations, float64(2))
}

func TestExpectedTypeUIDDetectsOverflow(t *testing.T) {
	value, ok := ExpectedTypeUID(1, 2)
	require.True(t, ok)
	require.Equal(t, int64(102), value)

	_, ok = ExpectedTypeUID(math.MaxInt64/100+1, 0)
	require.False(t, ok)
	_, ok = ExpectedTypeUID(math.MinInt64/100-1, 0)
	require.False(t, ok)
}
