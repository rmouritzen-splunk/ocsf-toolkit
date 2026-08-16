package processing

import (
	"encoding/json"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/jsonish"
	"github.com/ocsf/ocsf-toolkit/pathstyle"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/stretchr/testify/require"
)

type enumDefinition = schema.EnumDefinition
type commonAttributeDefinition = schema.CommonAttributeDefinition
type itemAttributeDefinition = schema.ItemAttributeDefinition
type commonItemDefinition = schema.ItemDefinition
type classDefinition = schema.ClassDefinition
type objectDefinition = schema.ObjectDefinition
type typeDefinition = schema.TypeDefinition
type typesDefinition = schema.TypesDefinition
type dictionaryDefinition = schema.DictionaryDefinition
type profileDefinition = schema.ProfileDefinition
type schemaDefinition = schema.Definition

func testPtrTo[T any](value T) *T {
	return &value
}

// EventProcessor is internal-test-only sugar: each NewXxx constructor below builds a PipelineConfig that only
// touches the fields for its own concern (leaving the others at their zero/None value), and
// mustNewEventProcessorPipeline merges the concerns from multiple EventProcessor values into the single
// PipelineConfig that Schema.NewPipeline now expects, so existing tests composing validation/enrichment/removal
// did not need to be restructured when the public API moved to one resolved PipelineConfig per pipeline.
type EventProcessor = PipelineConfig
type ValidationOption func(*ValidationConfig)

type enrichmentHelperConfig struct {
	enumSiblingsAction     enrichment.Action
	observablesAction      enrichment.Action
	observableTypeIDs      []int64
	pathNotation           pathstyle.Style
	pathNotationConfigured bool
}

type EnrichmentOption func(*enrichmentHelperConfig)
type EnrichmentRemovalOption func(*enrichmentHelperConfig)

func NewValidation(options ...ValidationOption) EventProcessor {
	config := ValidationConfig{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return PipelineConfig{
		EnumSiblingsAction: enrichment.None,
		ObservablesAction:  enrichment.None,
		ValidationEnabled:  true,
		Validation:         config,
	}
}

func WithWarnOnMissingRecommended() ValidationOption {
	return func(config *ValidationConfig) {
		config.WarnOnMissingRecommended = true
	}
}

func WithValidationObservablePathNotation(style pathstyle.Style) ValidationOption {
	return func(config *ValidationConfig) {
		config.PathNotation = style
		config.PathNotationConfigured = true
	}
}

func NewEnrichment(options ...EnrichmentOption) EventProcessor {
	config := enrichmentHelperConfig{
		enumSiblingsAction: enrichment.Add,
		observablesAction:  enrichment.Add,
		pathNotation:       pathstyle.Simple,
	}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return PipelineConfig{
		EnumSiblingsAction: config.enumSiblingsAction,
		ObservablesAction:  config.observablesAction,
		Observables: ObservablesConfig{
			TypeIDs:                config.observableTypeIDs,
			PathNotation:           config.pathNotation,
			PathNotationConfigured: config.pathNotationConfigured,
		},
	}
}

func WithAddEnumSiblings(add bool) EnrichmentOption {
	return func(config *enrichmentHelperConfig) {
		config.enumSiblingsAction = boolToAction(add, enrichment.Add)
	}
}

func WithAddObservables(add bool, observableTypeIDs ...int64) EnrichmentOption {
	ids := append([]int64(nil), observableTypeIDs...)
	return func(config *enrichmentHelperConfig) {
		config.observablesAction = boolToAction(add, enrichment.Add)
		config.observableTypeIDs = ids
	}
}

func WithEnrichmentObservablePathNotation(style pathstyle.Style) EnrichmentOption {
	return func(config *enrichmentHelperConfig) {
		config.pathNotation = style
		config.pathNotationConfigured = true
	}
}

func NewEnrichmentRemoval(options ...EnrichmentRemovalOption) EventProcessor {
	config := enrichmentHelperConfig{
		enumSiblingsAction: enrichment.Remove,
		observablesAction:  enrichment.Remove,
	}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return PipelineConfig{
		EnumSiblingsAction: config.enumSiblingsAction,
		ObservablesAction:  config.observablesAction,
		Observables:        ObservablesConfig{PathNotation: pathstyle.Simple},
	}
}

func WithRemoveEnumSiblings(remove bool) EnrichmentRemovalOption {
	return func(config *enrichmentHelperConfig) {
		config.enumSiblingsAction = boolToAction(remove, enrichment.Remove)
	}
}

func WithRemoveObservables(remove bool) EnrichmentRemovalOption {
	return func(config *enrichmentHelperConfig) {
		config.observablesAction = boolToAction(remove, enrichment.Remove)
	}
}

func WithForceRemoveEnumSiblings() EnrichmentRemovalOption {
	return func(config *enrichmentHelperConfig) {
		config.enumSiblingsAction = enrichment.ForceRemove
	}
}

func WithForceRemoveObservables() EnrichmentRemovalOption {
	return func(config *enrichmentHelperConfig) {
		config.observablesAction = enrichment.ForceRemove
	}
}

func boolToAction(enabled bool, whenEnabled enrichment.Action) enrichment.Action {
	if enabled {
		return whenEnabled
	}
	return enrichment.None
}

// mergeEventProcessors combines the concern-scoped EventProcessor values built by NewValidation, NewEnrichment,
// and NewEnrichmentRemoval into the single PipelineConfig that Schema.NewPipeline expects. Each concern's fields
// are only copied over when that concern's EventProcessor actually enabled it, so processors passed together
// (for example enrichment plus validation) combine instead of overwriting each other.
func mergeEventProcessors(processors []EventProcessor) PipelineConfig {
	merged := PipelineConfig{
		EnumSiblingsAction: enrichment.None,
		ObservablesAction:  enrichment.None,
		Observables:        ObservablesConfig{PathNotation: pathstyle.Simple},
	}
	for _, processor := range processors {
		if processor.EnumSiblingsAction != enrichment.None {
			merged.EnumSiblingsAction = processor.EnumSiblingsAction
		}
		if processor.ObservablesAction != enrichment.None {
			merged.ObservablesAction = processor.ObservablesAction
			merged.Observables = processor.Observables
		}
		if processor.ValidationEnabled {
			merged.ValidationEnabled = true
			merged.Validation = processor.Validation
		}
		if processor.IssueSuppression.Configured {
			merged.IssueSuppression = processor.IssueSuppression
		}
	}
	return merged
}

func mustNewEventProcessorPipeline(
	assert *require.Assertions,
	factory *PipelineFactory,
	processors ...EventProcessor,
) *Pipeline {
	pipeline, err := factory.NewPipeline(mergeEventProcessors(processors))
	assert.NoError(err)
	return pipeline
}

func makeTestSchema(assert *require.Assertions) *PipelineFactory {
	classNameAttribute := commonAttributeDefinition{
		Type: "string_t",
	}
	nameAttribute := commonAttributeDefinition{
		Type: "string_t",
	}
	typeStr := "type"
	typeIDAttribute := commonAttributeDefinition{
		Type:    "integer_t",
		Sibling: &typeStr,
	}
	typeAttribute := commonAttributeDefinition{
		Type: "string_t",
	}
	valueAttribute := commonAttributeDefinition{
		Type: "string_t",
	}

	redAttribute := commonAttributeDefinition{
		Type: "string_t",
	}
	greenAttribute := commonAttributeDefinition{
		Type: "string_t",
	}

	ballStr := "ball"
	ballAttribute := commonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &ballStr,
	}
	observableStr := "observable"
	trueValue := true
	observablesAttribute := commonAttributeDefinition{
		Type:       "object_t",
		ObjectType: &observableStr,
		IsArray:    &trueValue,
	}

	classNameStr := "class_name"
	classes := map[string]*classDefinition{
		"alpha": {
			Uid: int64(1),
			Observables: map[string]int64{
				"ball.green": 1000,
			},
			ItemDefinition: commonItemDefinition{
				Name: "Alpha",
				Attributes: map[string]*itemAttributeDefinition{
					"class_uid": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type: "integer_t",
							Enum: map[string]*enumDefinition{
								"1": {Caption: "Alpha"},
							},
							Sibling: &classNameStr,
						},
					},
					"class_name": {
						CommonAttributeDefinition: classNameAttribute,
					},
					"observables": {
						CommonAttributeDefinition: observablesAttribute,
					},
					"red": {
						CommonAttributeDefinition: redAttribute,
					},
					"ball": {
						CommonAttributeDefinition: ballAttribute,
					},
				},
			},
		},
	}
	objects := map[string]*objectDefinition{
		"ball": {
			ItemDefinition: commonItemDefinition{
				Name: "Ball",
				Attributes: map[string]*itemAttributeDefinition{
					"green": {
						CommonAttributeDefinition: greenAttribute,
					},
				},
			},
		},
		"observable": {
			ItemDefinition: commonItemDefinition{
				Name: "Observable",
				Attributes: map[string]*itemAttributeDefinition{
					"name": {
						CommonAttributeDefinition: nameAttribute,
					},
					"type_id": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type: "integer_t",
							Enum: map[string]*enumDefinition{
								"1000": {
									Caption: "Class path ball.green",
								},
							},
							Sibling: &typeStr,
						},
					},
					"type": {
						CommonAttributeDefinition: typeAttribute,
					},
					"value": {
						CommonAttributeDefinition: valueAttribute,
					},
				},
			},
		},
	}
	dictionaryTypes := &typesDefinition{
		Attributes: map[string]*typeDefinition{
			"integer_t": {},
			"string_t":  {},
		},
	}
	dictionaryAttributes := map[string]*commonAttributeDefinition{
		"class_uid": {
			Type:    "integer_t",
			Sibling: &classNameStr,
		},
		"class_name":  &classNameAttribute,
		"name":        &nameAttribute,
		"type_id":     &typeIDAttribute,
		"type":        &typeAttribute,
		"value":       &valueAttribute,
		"observables": &observablesAttribute,
		"red":         &redAttribute,
		"green":       &greenAttribute,
		"ball":        &ballAttribute,
	}
	dictionary := &dictionaryDefinition{
		Attributes: dictionaryAttributes,
		Types:      dictionaryTypes,
	}
	sd := &schemaDefinition{
		CompileVersion: 1,
		Classes:        classes,
		Objects:        objects,
		Dictionary:     dictionary,
		Version:        "0.1.0",
	}

	compiled, err := schema.New(sd)
	assert.NoError(err)
	assert.NotNil(compiled, "schema should not be nil")
	return NewPipelineFactory(compiled)
}

func makeValidationTestSchema(assert *require.Assertions) *PipelineFactory {
	classNameSibling := "class_name"
	activityNameSibling := "activity_name"
	modeSibling := "mode"
	statusesSibling := "statuses"
	ballObject := "ball"
	metadataObject := "metadata"
	observableObject := "observable"
	trueValue := true

	classes := map[string]*classDefinition{
		"alpha": {
			Uid: int64(1),
			Observables: map[string]int64{
				"ball.green": 1000,
			},
			ItemDefinition: commonItemDefinition{
				Name:        "alpha",
				Constraints: map[string][]string{"at_least_one": {"name", "ball.green"}},
				Attributes: map[string]*itemAttributeDefinition{
					"class_uid": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "integer_t",
							Requirement: "required",
							Sibling:     &classNameSibling,
							Enum: map[string]*enumDefinition{
								"1": {Caption: "Alpha"},
							},
						},
					},
					"class_name": {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"activity_id": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "integer_t",
							Requirement: "required",
							Sibling:     &activityNameSibling,
							Enum: map[string]*enumDefinition{
								"1":  {Caption: "Do"},
								"99": {Caption: "Other"},
							},
						},
					},
					"activity_name": {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"type_uid": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "long_t",
							Requirement: "required",
						},
					},
					"metadata": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "object_t",
							ObjectType:  &metadataObject,
							Requirement: "required",
						},
					},
					"name": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
					"red": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "recommended",
						},
					},
					"port": {CommonAttributeDefinition: commonAttributeDefinition{Type: "port_t"}},
					"count": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"},
					},
					"long_value": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "long_t"},
					},
					"bounded_count": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "bounded_int_t"},
					},
					"short_text": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "short_text_t"},
					},
					"code": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "upper_code_t"},
					},
					"level": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "level_t"},
					},
					"state": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type: "string_t",
							Enum: map[string]*enumDefinition{
								"open":   {Caption: "Open"},
								"closed": {Caption: "Closed"},
							},
						},
					},
					"mode_id": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:    "integer_t",
							Sibling: &modeSibling,
							Enum: map[string]*enumDefinition{
								"1":  {Caption: "Known"},
								"99": {Caption: "Other"},
							},
						},
					},
					"mode": {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"status_ids": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:    "integer_t",
							IsArray: &trueValue,
							Sibling: &statusesSibling,
							Enum: map[string]*enumDefinition{
								"1": {Caption: "Open"},
								"2": {Caption: "Closed"},
							},
						},
					},
					"statuses": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:    "string_t",
							IsArray: &trueValue,
						},
					},
					"ball": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:       "object_t",
							ObjectType: &ballObject,
						},
					},
					"profile_attr": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"},
						Profiles:                  []string{"p1"},
					},
					"observables": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:       "object_t",
							ObjectType: &observableObject,
							IsArray:    &trueValue,
						},
					},
				},
			},
		},
	}
	objects := map[string]*objectDefinition{
		"metadata": {
			ItemDefinition: commonItemDefinition{
				Name: "metadata",
				Attributes: map[string]*itemAttributeDefinition{
					"version": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
					"profiles": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:    "string_t",
							IsArray: &trueValue,
						},
					},
				},
			},
		},
		"ball": {
			ItemDefinition: commonItemDefinition{
				Name:        "ball",
				Constraints: map[string][]string{"at_least_one": {"green"}},
				Attributes: map[string]*itemAttributeDefinition{
					"green": {
						CommonAttributeDefinition: commonAttributeDefinition{
							Type:        "string_t",
							Requirement: "required",
						},
					},
				},
			},
		},
		"observable": {
			ItemDefinition: commonItemDefinition{
				Name: "observable",
				Attributes: map[string]*itemAttributeDefinition{
					"name": {
						CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t", Requirement: "required"},
					},
					"type_id": {CommonAttributeDefinition: commonAttributeDefinition{Type: "integer_t"}},
					"type":    {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
					"value":   {CommonAttributeDefinition: commonAttributeDefinition{Type: "string_t"}},
				},
			},
		},
	}
	dictionary := &dictionaryDefinition{
		Attributes: map[string]*commonAttributeDefinition{
			"green": {Type: "string_t", Observable: testPtrTo(int64(1000))},
		},
		Types: &typesDefinition{
			Attributes: map[string]*typeDefinition{
				"integer_t": {},
				"long_t":    {},
				"string_t":  {},
				"port_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "integer_t",
					},
					Range: []int64{0, 65535},
				},
				"bounded_int_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "integer_t",
					},
					Range: []int64{-10, 10},
				},
				"short_text_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "string_t",
					},
					MaxLen: testPtrTo(int64(3)),
				},
				"upper_code_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "string_t",
					},
					RegEx: testPtrTo("^[A-Z]+$"),
				},
				"level_t": {
					CommonAttributeDefinition: commonAttributeDefinition{
						Type: "integer_t",
					},
					Values: []any{json.Number("1"), json.Number("2")},
				},
			},
		},
	}

	compiled, err := schema.New(&schemaDefinition{
		CompileVersion: 1,
		Classes:        classes,
		Objects:        objects,
		Dictionary:     dictionary,
		Profiles:       map[string]profileDefinition{"p1": {}},
		Version:        "1.0.0",
	})
	assert.NoError(err)
	return NewPipelineFactory(compiled)
}

func validValidationEvent() jsonish.Map {
	return jsonish.Map{
		"class_uid":   json.Number("1"),
		"activity_id": json.Number("1"),
		"type_uid":    json.Number("101"),
		"metadata":    jsonish.Map{"version": "1.0.0"},
		"name":        "event name",
		"red":         "recommended present",
	}
}

type resultFinding interface {
	eventresult.ProcessingIssue | eventresult.ValidationFinding
}

func findingsAtLevel(findings []eventresult.ValidationFinding, level validation.Level) []eventresult.ValidationFinding {
	selected := make([]eventresult.ValidationFinding, 0)
	for _, finding := range findings {
		if finding.Level == level {
			selected = append(selected, finding)
		}
	}
	return selected
}

func issueCodes[T resultFinding](issues []T) []string {
	codes := make([]string, len(issues))
	for index, issue := range issues {
		codes[index] = resultFindingCode(issue)
	}
	return codes
}

func resultFindingCode[T resultFinding](finding T) string {
	switch finding := any(finding).(type) {
	case eventresult.ProcessingIssue:
		return finding.Code.String()
	case eventresult.ValidationFinding:
		return finding.Code.String()
	default:
		return ""
	}
}

func issuesWithCode[T resultFinding](issues []T, code string) []T {
	result := make([]T, 0)
	for _, issue := range issues {
		if resultFindingCode(issue) == code {
			result = append(result, issue)
		}
	}
	return result
}
