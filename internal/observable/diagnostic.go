package observable

import (
	"strconv"

	"github.com/ocsf/ocsf-toolkit/internal/eventpath"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
)

// Diagnostic describes why an observable entry could not be resolved safely.
type Diagnostic struct {
	Message string
	Details jsonish.Map
}

// EntryToDiagnostic converts an analyzed observable entry to a diagnostic, if it has one.
func EntryToDiagnostic(entry Entry, index int, diagnosticPath *eventpath.Path) (Diagnostic, bool) {
	switch entry.Problem {
	case ProblemArrayWrongType:
		return newDiagnostic(
			jsonish.Map{"attribute_path": "observables", "attribute": "observables"},
			"The observables attribute is not an array.",
		)
	case ProblemElementWrongType:
		attributePath := diagnosticPath.String(pathstyle.ArrayIndexed)
		return newIndexedDiagnostic(
			jsonish.Map{"attribute_path": attributePath, "attribute": "observables"},
			index,
			" is not an object.",
		)
	case ProblemNameMissing:
		return newIndexedDiagnostic(
			jsonish.Map{
				"attribute_path": diagnosticPath.ChildString("name", pathstyle.ArrayIndexed),
				"attribute":      "name",
			},
			index,
			" is missing its name attribute.",
		)
	case ProblemNameWrongType:
		return newIndexedDiagnostic(
			jsonish.Map{
				"attribute_path": diagnosticPath.ChildString("name", pathstyle.ArrayIndexed),
				"attribute":      "name",
			},
			index,
			" name is not a string.",
		)
	case ProblemNameInvalidSyntax:
		attributePath := diagnosticPath.ChildString("name", pathstyle.ArrayIndexed)
		return newIndexedDiagnostic(
			details(attributePath, "name"),
			index,
			" name has invalid path syntax.",
		)
	case ProblemNameInvalidReference:
		attributePath := diagnosticPath.ChildString("name", pathstyle.ArrayIndexed)
		return newIndexedDiagnostic(
			details(attributePath, "name"),
			index,
			" name does not refer to an attribute defined for the event class.",
		)
	case ProblemPathNotFound:
		attributePath := diagnosticPath.ChildString("name", pathstyle.ArrayIndexed)
		return newIndexedDiagnostic(
			details(attributePath, "name"),
			index,
			" name does not resolve to a value in the event.",
		)
	case ProblemPathNotObject:
		attributePath := diagnosticPath.ChildString("name", pathstyle.ArrayIndexed)
		return newIndexedDiagnostic(
			details(attributePath, "name"),
			index,
			" without a value does not refer to an object.",
		)
	case ProblemValueWrongType:
		attributePath := diagnosticPath.ChildString("value", pathstyle.ArrayIndexed)
		return newIndexedDiagnostic(
			details(attributePath, "value"),
			index,
			" value is not a string or null.",
		)
	case ProblemValueNotFound:
		attributePath := diagnosticPath.ChildString("value", pathstyle.ArrayIndexed)
		return newIndexedDiagnostic(
			details(attributePath, "value"),
			index,
			" value is not present at its named event path.",
		)
	default:
		return Diagnostic{}, false
	}
}

func newDiagnostic(details jsonish.Map, message string) (Diagnostic, bool) {
	return Diagnostic{Message: message, Details: details}, true
}

func newIndexedDiagnostic(details jsonish.Map, index int, messageSuffix string) (Diagnostic, bool) {
	return newDiagnostic(details, "Observable index "+strconv.Itoa(index)+messageSuffix)
}

func details(attributePath, attribute string) jsonish.Map {
	return jsonish.Map{
		"attribute_path": attributePath,
		"attribute":      attribute,
	}
}
