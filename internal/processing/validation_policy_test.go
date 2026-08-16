package processing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/validation"
)

func TestValidationPolicySuppressesOnlyCodesAtSelectedDefaultLevel(t *testing.T) {
	tests := []struct {
		name           string
		defaultLevel   validation.Level
		suppressedCode validation.Code
		unaffectedCode validation.Code
	}{
		{
			name:           "errors",
			defaultLevel:   validation.LevelError,
			suppressedCode: validation.AttributeRequiredMissing,
			unaffectedCode: validation.AttributeDeprecated,
		},
		{
			name:           "warnings",
			defaultLevel:   validation.LevelWarning,
			suppressedCode: validation.AttributeDeprecated,
			unaffectedCode: validation.AttributeRequiredMissing,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, problems := compileValidationPolicy([]ValidationPolicyRule{{
				Action:       ValidationPolicySuppress,
				DefaultLevel: test.defaultLevel,
			}})
			require.Empty(t, problems)
			processor := validationProcessor{policy: policy}
			context := processContext{}

			_, reported := processor.findingLevel(&context, test.suppressedCode)
			require.False(t, reported)
			level, reported := processor.findingLevel(&context, test.unaffectedCode)
			require.True(t, reported)
			require.Equal(t, test.unaffectedCode.DefaultLevel(), level)
		})
	}
}
