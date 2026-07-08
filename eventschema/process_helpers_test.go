package eventschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValuesEqualComparesNumbersExactly(t *testing.T) {
	testCases := []struct {
		name  string
		left  any
		right any
		want  bool
	}{
		{name: "equivalent decimal encodings", left: json.Number("1.0"), right: json.Number("1.00"), want: true},
		{name: "equivalent exponent encoding", left: json.Number("1e3"), right: json.Number("1000.0"), want: true},
		{name: "JSON and native integer", left: json.Number("9007199254740993"), right: int64(9007199254740993), want: true},
		{name: "adjacent large integers", left: json.Number("9007199254740993"), right: json.Number("9007199254740992")},
		{name: "JSON and native float", left: json.Number("0.1"), right: float64(0.1), want: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, valuesEqual(testCase.left, testCase.right))
		})
	}
}
