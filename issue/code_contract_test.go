package issue_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ocsf/ocsf-toolkit/issue"
)

type issueCodeContract struct {
	code      issue.Code
	name      string
	mandatory bool
}

var issueCodeContracts = []issueCodeContract{
	{issue.EventTraversalLimited, "issue_event_traversal_limited", true},
	{issue.EnrichmentObservableNotAddedWrongType, "issue_enrichment_observable_not_added_wrong_type", false},
	{issue.EnrichmentObservableNotAddedJSONType, "issue_enrichment_observable_not_added_json_type", false},
	{issue.EnrichmentEnumSiblingNotAdded, "issue_enrichment_enum_sibling_not_added", false},
	{issue.EnrichmentEnumSiblingOtherAdded, "issue_enrichment_enum_sibling_other_added", false},
	{issue.EnrichmentObservablesNotAddedWrongType, "issue_enrichment_observables_not_added_wrong_type", false},
	{issue.EnrichmentObservableDuplicateSkipped, "issue_enrichment_observable_duplicate_skipped", false},
	{issue.EnrichmentRemovalEnumSiblingNotRemoved, "issue_enrichment_removal_enum_sibling_not_removed", false},
	{issue.ObservableArrayWrongType, "issue_observable_array_wrong_type", false},
	{issue.ObservableElementWrongType, "issue_observable_element_wrong_type", false},
	{issue.ObservableNameMissing, "issue_observable_name_missing", false},
	{issue.ObservableNameWrongType, "issue_observable_name_wrong_type", false},
	{issue.ObservableNameInvalidSyntax, "issue_observable_name_invalid_syntax", false},
	{issue.ObservableNameInvalidReference, "issue_observable_name_invalid_reference", false},
	{issue.ObservablePathNotFound, "issue_observable_path_not_found", false},
	{issue.ObservablePathNotObject, "issue_observable_path_not_object", false},
	{issue.ObservableValueWrongType, "issue_observable_value_wrong_type", false},
	{issue.ObservableValueNotFound, "issue_observable_value_not_found", false},
	{issue.ClassUIDMissing, "issue_class_uid_missing", true},
	{issue.ClassUIDWrongType, "issue_class_uid_wrong_type", true},
	{issue.ClassUIDUnknown, "issue_class_uid_unknown", true},
	{
		issue.AtInitSchemaEnumSiblingSourceNotIntegral,
		"issue_at_init_schema_enum_sibling_source_not_integral",
		false,
	},
	{
		issue.AtInitSchemaEnumSiblingTargetNotFound,
		"issue_at_init_schema_enum_sibling_target_not_found",
		false,
	},
	{
		issue.AtInitSchemaEnumSiblingTargetIsEnum,
		"issue_at_init_schema_enum_sibling_target_is_enum",
		false,
	},
	{
		issue.AtInitSchemaEnumSiblingTargetNotString,
		"issue_at_init_schema_enum_sibling_target_not_string",
		false,
	},
}

func TestInvariantIssueCodeContract(t *testing.T) {
	// Invariant test: every v1 issue code retains its exported constant, ordinal, text and JSON identity, default
	// level, and mandatory classification; additions append to this independently reviewed manifest.
	wantCodes := make([]issue.Code, len(issueCodeContracts))
	for index, contract := range issueCodeContracts {
		t.Run(contract.name, func(t *testing.T) {
			wantCodes[index] = contract.code
			require.Equal(t, index+1, int(contract.code))
			require.True(t, contract.code.Valid())
			require.Equal(t, contract.name, contract.code.String())
			require.Equal(t, issue.LevelWarning, contract.code.DefaultLevel())
			require.Equal(t, !contract.mandatory, contract.code.Ignorable())
			require.NotEmpty(t, contract.code.Description())

			parsed, ok := issue.ParseCode(contract.name)
			require.True(t, ok)
			require.Equal(t, contract.code, parsed)

			encoded, err := json.Marshal(contract.code)
			require.NoError(t, err)
			require.Equal(t, `"`+contract.name+`"`, string(encoded))
			var decoded issue.Code
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, contract.code, decoded)
		})
	}
	require.Equal(t, wantCodes, issue.Codes())
}
