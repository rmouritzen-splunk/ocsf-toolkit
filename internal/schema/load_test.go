package schema

import (
	"bytes"
	"os"
	"testing"

	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/stretchr/testify/require"
)

type observedLenReader struct {
	*bytes.Reader
	called bool
}

func (r *observedLenReader) Len() int {
	r.called = true
	return r.Reader.Len()
}

func TestEngineeringInvariantLoadReaderDoesNotConsultOptionalLen(t *testing.T) {
	// Engineering invariant test: schema loading allocates only in response to bytes read, not an optional size hint.
	data, err := os.ReadFile("testdata/enum_sibling_redirect.json")
	require.NoError(t, err)
	reader := &observedLenReader{Reader: bytes.NewReader(data)}

	compiled, _, err := LoadReader(reader)

	require.NoError(t, err)
	require.NotNil(t, compiled)
	require.False(t, reader.called)
}

func TestLoadBytesReturnsInitializationIssues(t *testing.T) {
	compiled, issues, err := LoadBytes([]byte(`{
		"compile_version": 1,
		"version": "1.0.0",
		"classes": {
			"alpha": {
					"name": "alpha",
					"uid": 1,
					"attributes": {
						"code": {"type": "integer_t", "enum": {"1": {"caption": "Jammed"}}, "sibling": "message"},
						"message": {"type": "string_t", "is_array": true}
					}
			}
		}
	}`))

	require.NoError(t, err)
	require.NotNil(t, compiled)
	require.Len(t, issues, 1)
	require.Equal(t, "issue_at_init_schema_enum_sibling_target_not_string", issues[0].Code.String())
	require.Equal(t, jsonish.Map{
		"item_type":         "class",
		"item_name":         "alpha",
		"attribute":         "code",
		"sibling":           "message",
		"expected_type":     "string_t",
		"expected_is_array": false,
		"actual_type":       "string_t",
		"actual_is_array":   true,
	}, issues[0].Details)
}

func TestLoadBytesLoadsExtensions(t *testing.T) {
	compiled, _, err := LoadBytes([]byte(`{
		"compile_version": 1,
		"version": "1.9.0",
		"classes": {"base_event": {"name": "base_event", "uid": 0}},
		"extensions": {
			"win": {"uid": 2, "name": "win", "platform_extension?": true, "version": "1.9.0"},
			"aws": {"uid": 998, "name": "aws", "platform_extension?": false, "version": "1.1.0"}
		}
	}`))

	require.NoError(t, err)
	require.Equal(t, []Extension{
		{UID: 998, Name: "aws", Version: "1.1.0"},
		{UID: 2, Name: "win", Version: "1.9.0", PlatformExtension: true},
	}, compiled.Extensions)
}

func TestLoadBytesLoadsValidationExtensionProvenance(t *testing.T) {
	compiled, _, err := LoadBytes([]byte(`{
		"compile_version": 1,
		"version": "1.9.0",
		"classes": {
			"win/prefetch_query": {
				"name": "prefetch_query",
				"uid": 205019,
				"extension": "win",
				"extension_id": 2,
				"attributes": {
					"run_count": {"type": "integer_t", "extension": "win", "extension_id": 2}
				}
			}
		},
		"objects": {
			"win/win_service": {
				"name": "win_service",
				"extension": "win",
				"extension_id": 2
			},
			"evidences": {
				"name": "evidences",
				"attributes": {
					"reg_key": {"type": "object_t", "extension": "win", "extension_id": 2}
				}
			}
		},
		"profiles": {
			"linux/linux_users": {"extension": "linux", "extension_id": 1}
		}
	}`))

	require.NoError(t, err)
	class := compiled.Classes[205019]
	require.Equal(t, "win", class.Extension)
	require.EqualValues(t, 2, class.ExtensionID)
	require.Equal(t, "win", class.Attributes["run_count"].Extension)
	require.EqualValues(t, 2, class.Attributes["run_count"].ExtensionID)
	require.Equal(t, "win", compiled.Objects["win/win_service"].Extension)
	require.EqualValues(t, 2, compiled.Objects["win/win_service"].ExtensionID)
	require.Equal(t, "win", compiled.Objects["evidences"].Attributes["reg_key"].Extension)
	require.EqualValues(t, 2, compiled.Objects["evidences"].Attributes["reg_key"].ExtensionID)
	require.Equal(t, "linux", compiled.Profiles["linux/linux_users"].Extension)
	require.EqualValues(t, 1, compiled.Profiles["linux/linux_users"].ExtensionID)
}

func TestLoadIgnoresEnumSiblingThatTargetsAnotherEnum(t *testing.T) {
	compiled, issues, err := Load("testdata/enum_sibling_dual_role.json")

	require.NoError(t, err)
	require.NotNil(t, compiled)
	require.Len(t, issues, 2)
	require.Equal(t, "issue_at_init_schema_enum_sibling_source_not_integral", issues[0].Code.String())
	require.Equal(t, "issue_at_init_schema_enum_sibling_target_is_enum", issues[1].Code.String())
}

func TestLoadAllowsEnumSiblingRelationshipsToVaryByItem(t *testing.T) {
	_, _, err := Load("testdata/enum_sibling_redirect.json")

	require.NoError(t, err)
}
