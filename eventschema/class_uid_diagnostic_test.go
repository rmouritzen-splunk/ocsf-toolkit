package eventschema

import (
	"strings"
	"testing"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

func TestClassUIDIssueAndValidationCodesRemainConsistent(t *testing.T) {
	pairs := []struct {
		issueCode      issue.IssueCode
		validationCode validation.Code
	}{
		{issueCode: issue.ClassUIDMissing, validationCode: validation.ClassUIDMissing},
		{issueCode: issue.ClassUIDWrongType, validationCode: validation.ClassUIDWrongType},
		{issueCode: issue.ClassUIDUnknown, validationCode: validation.ClassUIDUnknown},
	}

	for _, pair := range pairs {
		issueSuffix := strings.TrimPrefix(pair.issueCode.String(), "issue_")
		validationSuffix := strings.TrimPrefix(pair.validationCode.String(), "validation_")
		require.Equal(t, validationSuffix, issueSuffix)
		require.Equal(t, pair.validationCode.Description(), pair.issueCode.Description())
		require.False(t, pair.issueCode.Suppressible())
		require.False(t, pair.validationCode.Suppressible())
	}
}
