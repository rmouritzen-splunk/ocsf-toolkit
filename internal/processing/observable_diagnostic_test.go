package processing

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/internal/observable"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

type observableProblemDisposition struct {
	issueCode      issue.IssueCode
	validationCode validation.Code
	structural     bool
	traversalLimit bool
}

func TestEveryObservableResolutionProblemHasAnExplicitDisposition(t *testing.T) {
	expected := map[observable.Problem]observableProblemDisposition{
		observable.ProblemNone: {},
		observable.ProblemArrayWrongType: {
			issueCode:  issue.ObservableArrayWrongType,
			structural: true,
		},
		observable.ProblemElementWrongType: {
			issueCode:  issue.ObservableElementWrongType,
			structural: true,
		},
		observable.ProblemNameMissing: {
			issueCode:  issue.ObservableNameMissing,
			structural: true,
		},
		observable.ProblemNameWrongType: {
			issueCode:  issue.ObservableNameWrongType,
			structural: true,
		},
		observable.ProblemNameInvalidSyntax: {
			issueCode:      issue.ObservableNameInvalidSyntax,
			validationCode: validation.ObservableNameInvalidSyntax,
		},
		observable.ProblemNameInvalidReference: {
			issueCode:      issue.ObservableNameInvalidReference,
			validationCode: validation.ObservableNameInvalidReference,
		},
		observable.ProblemTraversalLimited: {
			traversalLimit: true,
		},
		observable.ProblemPathNotFound: {
			issueCode:      issue.ObservablePathNotFound,
			validationCode: validation.ObservablePathNotFound,
		},
		observable.ProblemPathNotObject: {
			issueCode:      issue.ObservablePathNotObject,
			validationCode: validation.ObservablePathNotObject,
		},
		observable.ProblemValueWrongType: {
			issueCode:  issue.ObservableValueWrongType,
			structural: true,
		},
		observable.ProblemValueNotFound: {
			issueCode:      issue.ObservableValueNotFound,
			validationCode: validation.ObservableValueNotFound,
		},
	}

	require.Len(t, expected, int(observable.ProblemCount))
	for problem := range observable.ProblemCount {
		disposition, defined := expected[problem]
		require.True(t, defined, "observable resolution problem %d has no explicit disposition", problem)

		issueCode, hasIssueCode := observableIssueCode(problem)
		require.Equal(
			t, disposition.issueCode.Valid(), hasIssueCode, "unexpected issue disposition for problem %d", problem,
		)
		require.Equal(t, disposition.issueCode, issueCode, "incorrect issue code for problem %d", problem)

		validationCode, hasValidationCode := nonStructuralObservableValidationCode(problem)
		require.Equal(t, disposition.validationCode.Valid(), hasValidationCode,
			"unexpected validation disposition for problem %d", problem)
		require.Equal(
			t, disposition.validationCode, validationCode, "incorrect validation code for problem %d", problem,
		)

		if disposition.structural {
			require.False(t, hasValidationCode,
				"structural problem %d should be covered by ordinary attribute validation", problem)
		}
		require.Equal(t, problem == observable.ProblemTraversalLimited, disposition.traversalLimit,
			"incorrect traversal-limit disposition for problem %d", problem)
	}
}
