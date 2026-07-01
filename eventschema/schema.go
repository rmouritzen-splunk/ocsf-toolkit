package eventschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

const expectedCompileVersion = 1

// Schema is a loaded compiled OCSF schema.
//
// Schema values are safe for concurrent use after construction.
type Schema interface {
	// NewEventProcessorPipeline builds a reusable pipeline from the requested event processors.
	//
	// Validation processors run after other processors, regardless of the order supplied, so
	// validation observes the event after enrichment and any future mutating processors.
	// An error is returned when no processing action is enabled, a processor is configured
	// more than once, or enrichment would both add and remove the same content. The returned
	// error joins all detected configuration problems.
	NewEventProcessorPipeline(processors ...EventProcessor) (EventProcessorPipeline, error)
}

// EventProcessorPipeline processes OCSF events in-place.
//
// EventProcessorPipeline values are safe for concurrent use after construction, provided each
// concurrent ProcessEvent call receives a distinct event map.
type EventProcessorPipeline interface {
	// ProcessEvent adds or removes enrichment and/or validates event in place.
	//
	// The event map and any nested maps or slices it contains must not be accessed or mutated
	// concurrently while ProcessEvent is running.
	//
	// Processing is not transactional. When enrichment removal, enrichment, or future mutating
	// processors are enabled, the event may be partially modified if ProcessEvent returns an error.
	// Callers that need to preserve the original event should deep-copy it before processing.
	//
	// Invalid events are reported in the returned ProcessingResult. The error return is for
	// processor failures or unusable caller input, not OCSF validation failures.
	ProcessEvent(event jsonish.Map) (ProcessingResult, error)
}

// EventProcessor configures one processing phase in an EventProcessorPipeline.
//
// Callers normally create processors with NewEnrichment, NewEnrichmentRemoval, NewValidation,
// or future processor constructors from this package.
type EventProcessor interface {
	applyProcessor(*pipelineConfig)
}

type eventProcessorFunc func(*pipelineConfig)

func (f eventProcessorFunc) applyProcessor(config *pipelineConfig) {
	f(config)
}

type validationConfig struct {
	warnOnMissingRecommended bool
}

type enrichmentConfig struct {
	addEnumSiblings bool
	addObservables  bool
}

type enrichmentRemovalConfig struct {
	removeEnumSiblings      bool
	removeObservables       bool
	forceRemoveEnumSiblings bool
	forceRemoveObservables  bool
	retainEnumSiblings      bool
	retainObservables       bool
}

// ValidationOption configures the validation processor created by NewValidation.
type ValidationOption interface {
	applyValidation(*validationConfig)
}

type validationOptionFunc func(*validationConfig)

func (f validationOptionFunc) applyValidation(config *validationConfig) {
	f(config)
}

// NewValidation creates an event processor that validates OCSF events.
func NewValidation(options ...ValidationOption) EventProcessor {
	config := validationConfig{}
	for _, option := range options {
		if option != nil {
			option.applyValidation(&config)
		}
	}
	return eventProcessorFunc(func(pipeline *pipelineConfig) {
		pipeline.processors = append(pipeline.processors, configuredProcessor{
			kind: processorKindValidation,
			factory: func() eventProcessVisitor {
				return &validationProcessor{config: config}
			},
		})
	})
}

// WithWarnOnMissingRecommended reports missing recommended attributes as validation warnings.
func WithWarnOnMissingRecommended() ValidationOption {
	return validationOptionFunc(func(config *validationConfig) {
		config.warnOnMissingRecommended = true
	})
}

// EnrichmentOption configures the enrichment processor created by NewEnrichment.
type EnrichmentOption interface {
	applyEnrichment(*enrichmentConfig)
}

type enrichmentOptionFunc func(*enrichmentConfig)

func (f enrichmentOptionFunc) applyEnrichment(config *enrichmentConfig) {
	f(config)
}

// NewEnrichment creates an event processor that enriches OCSF events.
//
// Enum sibling and observable enrichment are enabled by default.
func NewEnrichment(options ...EnrichmentOption) EventProcessor {
	config := enrichmentConfig{
		addEnumSiblings: true,
		addObservables:  true,
	}
	for _, option := range options {
		if option != nil {
			option.applyEnrichment(&config)
		}
	}
	return eventProcessorFunc(func(pipeline *pipelineConfig) {
		pipeline.processors = append(pipeline.processors, configuredProcessor{
			kind:       processorKindEnrichment,
			enrichment: config,
			factory: func() eventProcessVisitor {
				return &enrichmentProcessor{config: config}
			},
		})
	})
}

// WithAddEnumSiblings controls whether enum siblings are added during enrichment.
func WithAddEnumSiblings(add bool) EnrichmentOption {
	return enrichmentOptionFunc(func(config *enrichmentConfig) {
		config.addEnumSiblings = add
	})
}

// WithAddObservables controls whether observables are added during enrichment.
func WithAddObservables(add bool) EnrichmentOption {
	return enrichmentOptionFunc(func(config *enrichmentConfig) {
		config.addObservables = add
	})
}

// EnrichmentRemovalOption configures the enrichment-removal processor created by NewEnrichmentRemoval.
type EnrichmentRemovalOption interface {
	applyEnrichmentRemoval(*enrichmentRemovalConfig)
}

type enrichmentRemovalOptionFunc func(*enrichmentRemovalConfig)

func (f enrichmentRemovalOptionFunc) applyEnrichmentRemoval(config *enrichmentRemovalConfig) {
	f(config)
}

// NewEnrichmentRemoval creates an event processor that removes redundant enrichment from OCSF events.
//
// Safe enum sibling and observable removal are enabled by default. Safe removal preserves values that
// cannot be proven redundant. Force options may discard malformed or non-standard event content.
func NewEnrichmentRemoval(options ...EnrichmentRemovalOption) EventProcessor {
	config := enrichmentRemovalConfig{
		removeEnumSiblings: true,
		removeObservables:  true,
	}
	for _, option := range options {
		if option != nil {
			option.applyEnrichmentRemoval(&config)
		}
	}
	return eventProcessorFunc(func(pipeline *pipelineConfig) {
		pipeline.processors = append(pipeline.processors, configuredProcessor{
			kind:    processorKindEnrichmentRemoval,
			removal: config,
			factory: func() eventProcessVisitor {
				return &enrichmentRemovalProcessor{config: config}
			},
		})
	})
}

// WithRemoveEnumSiblings controls whether scalar integral enum siblings are removed.
func WithRemoveEnumSiblings(remove bool) EnrichmentRemovalOption {
	return enrichmentRemovalOptionFunc(func(config *enrichmentRemovalConfig) {
		config.removeEnumSiblings = remove
		config.retainEnumSiblings = config.retainEnumSiblings || !remove
	})
}

// WithRemoveObservables controls whether redundant observable entries are removed.
func WithRemoveObservables(remove bool) EnrichmentRemovalOption {
	return enrichmentRemovalOptionFunc(func(config *enrichmentRemovalConfig) {
		config.removeObservables = remove
		config.retainObservables = config.retainObservables || !remove
	})
}

// WithForceRemoveEnumSiblings removes supported enum siblings without requiring them to match the schema caption.
// Enum ID 99 siblings and unsupported enum forms are always retained.
func WithForceRemoveEnumSiblings() EnrichmentRemovalOption {
	return enrichmentRemovalOptionFunc(func(config *enrichmentRemovalConfig) {
		config.removeEnumSiblings = true
		config.forceRemoveEnumSiblings = true
	})
}

// WithForceRemoveObservables removes the event's observables attribute regardless of its contents.
func WithForceRemoveObservables() EnrichmentRemovalOption {
	return enrichmentRemovalOptionFunc(func(config *enrichmentRemovalConfig) {
		config.removeObservables = true
		config.forceRemoveObservables = true
	})
}

// ProcessingResult is the result returned by EventProcessorPipeline.ProcessEvent.
//
// Validation errors and warnings are reported here instead of through the Go error return.
type ProcessingResult struct {
	// Validation contains validation errors and warnings found while processing the event.
	Validation ValidationResult `json:"validation"`

	// Enrichment contains counts for values added to the event during enrichment.
	Enrichment EnrichmentResult `json:"enrichment"`

	// EnrichmentRemoval contains counts for values removed or retained during enrichment removal.
	EnrichmentRemoval EnrichmentRemovalResult `json:"enrichment_removal"`

	// Issues contains validation issues and non-fatal issues from enrichment or other processors.
	Issues []ProcessingIssue `json:"issues,omitempty"`
}

// ValidationResult contains OCSF validation issues split by severity.
type ValidationResult struct {
	// Errors contains validation issues that make the event invalid.
	Errors []ProcessingIssue `json:"errors,omitempty"`

	// Warnings contains validation issues that should be reviewed but do not make the event invalid.
	Warnings []ProcessingIssue `json:"warnings,omitempty"`
}

// EnrichmentResult reports what enrichment added to the processed event.
type EnrichmentResult struct {
	// EnumSiblingsAdded is the number of enum sibling fields added to the event.
	EnumSiblingsAdded int `json:"enum_siblings_added"`

	// ObservablesAdded is the number of observable entries added to the event.
	ObservablesAdded int `json:"observables_added"`
}

// EnrichmentRemovalResult reports what enrichment removal changed or retained in the processed event.
type EnrichmentRemovalResult struct {
	// EnumSiblingsRemoved is the number of enum sibling fields removed from the event.
	EnumSiblingsRemoved int `json:"enum_siblings_removed"`

	// EnumSiblingsRetained is the number of enum sibling fields retained because removal was unsafe or unsupported.
	EnumSiblingsRetained int `json:"enum_siblings_retained"`

	// ObservablesRemoved is the number of observable entries removed from the event.
	ObservablesRemoved int `json:"observables_removed"`

	// ObservablesRetained is the number of observable entries retained because removal was unsafe.
	ObservablesRetained int `json:"observables_retained"`
}

// ProcessingIssue describes a validation, enrichment, or future processing issue.
type ProcessingIssue struct {
	// Phase identifies the processor that produced the issue.
	Phase string `json:"phase"`

	// Severity is the issue severity, such as error or warning.
	Severity string `json:"severity"`

	// Code is a short machine-readable issue identifier suitable for searching, grouping,
	// metrics, and structured logs.
	Code string `json:"code"`

	// Message is a human-readable issue description.
	Message string `json:"message"`

	// AttributePath is the dotted path to the affected event attribute, when available.
	AttributePath string `json:"attribute_path,omitempty"`

	// Attribute is the affected event attribute name, when available.
	Attribute string `json:"attribute,omitempty"`

	// Value is the offending or relevant event value, when available.
	Value any `json:"value,omitempty"`

	// Details contains issue-specific structured context.
	Details jsonish.Map `json:"details,omitempty"`
}

// New loads a compiled OCSF schema from name.
//
// The file must be in the compiled schema format produced by the OCSF Schema Compiler.
func New(name string) (Schema, error) {
	var f *os.File
	var err error
	if f, err = os.Open(name); err != nil {
		return nil, fmt.Errorf("failed to open schema file %q: %w", name, err)
	}
	defer func(f *os.File) { _ = f.Close() }(f)
	var sd schemaDefinition
	decoder := json.NewDecoder(f)
	if err = decoder.Decode(&sd); err != nil {
		return nil, fmt.Errorf("failed to decode schema file %q: %w", name, err)
	}
	if err := ensureSchemaEOF(decoder); err != nil {
		return nil, fmt.Errorf("failed to decode schema file %q: %w", name, err)
	}
	schema, err := newSchemaImpl(&sd)
	if err != nil {
		return nil, fmt.Errorf("failed to load schema file %q: %w", name, err)
	}
	return schema, nil
}

func newSchemaImpl(sd *schemaDefinition) (*schemaImpl, error) {
	if sd.CompileVersion != expectedCompileVersion {
		return nil, fmt.Errorf("unsupported compile_version: %d", sd.CompileVersion)
	}
	normalizeSchemaDefinition(sd)
	if len(sd.Classes) == 0 {
		return nil, errors.New("compiled schema is missing classes")
	}

	// transform classes map of class names (like "base_event") to class definitions, to class uid to class definition
	classes := make(map[int64]*classDefinition, len(sd.Classes))
	for className, definition := range sd.Classes {
		if definition == nil {
			return nil, fmt.Errorf("compiled schema class %q is nil", className)
		}
		if existing, present := classes[definition.Uid]; present {
			return nil, fmt.Errorf(
				"compiled schema has duplicate class uid %d for classes %q and %q",
				definition.Uid,
				existing.Name,
				definition.Name,
			)
		}
		classes[definition.Uid] = definition
	}
	return &schemaImpl{
		classes:         classes,
		objects:         sd.Objects,
		dictionary:      sd.Dictionary,
		profiles:        sd.Profiles,
		version:         sd.Version,
		observableTypes: makeObservableTypes(sd.Objects),
	}, nil
}

func normalizeSchemaDefinition(sd *schemaDefinition) {
	if sd.Dictionary == nil {
		sd.Dictionary = &dictionaryDefinition{}
	}
	if sd.Dictionary.Attributes == nil {
		sd.Dictionary.Attributes = make(map[string]*commonAttributeDefinition)
	}
	if sd.Dictionary.Types == nil {
		sd.Dictionary.Types = &typesDefinition{}
	}
	if sd.Dictionary.Types.Attributes == nil {
		sd.Dictionary.Types.Attributes = make(map[string]*typeDefinition)
	}
	if sd.Classes == nil {
		sd.Classes = make(map[string]*classDefinition)
	}
	if sd.Objects == nil {
		sd.Objects = make(map[string]*objectDefinition)
	}
	if sd.Profiles == nil {
		sd.Profiles = make(map[string]*profileDefinition)
	}
}

func ensureSchemaEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func makeObservableTypes(objects map[string]*objectDefinition) map[int64]string {
	observableObjectDef, objectDefPresent := objects["observable"]
	if objectDefPresent && observableObjectDef != nil {
		typeIDDef, typeIDDefPresent := observableObjectDef.Attributes["type_id"]
		if typeIDDefPresent && typeIDDef != nil && typeIDDef.Enum != nil {
			observableTypes := make(map[int64]string, len(typeIDDef.Enum))
			for typeIDStr, enumDef := range typeIDDef.Enum {
				i, err := strconv.ParseInt(typeIDStr, 10, 64)
				if err == nil { // add if successfully parsed int, otherwise ignore err
					observableTypes[i] = enumDef.Caption
				}
			}
			return observableTypes
		}
	}
	return make(map[int64]string)
}

type deprecatedDefinition struct {
	Since   string `json:"since"`
	Message string `json:"message"`
}

type enumDefinition struct {
	Deprecated  *deprecatedDefinition `json:"@deprecated,omitempty"`
	Caption     string                `json:"caption,omitempty"`
	Description string                `json:"description,omitempty"`
}

type commonAttributeDefinition struct {
	Deprecated  *deprecatedDefinition      `json:"@deprecated,omitempty"`
	Caption     string                     `json:"caption,omitempty"`
	Description string                     `json:"description,omitempty"`
	Type        string                     `json:"type,omitempty"`
	Requirement string                     `json:"requirement,omitempty"`
	IsArray     *bool                      `json:"is_array,omitempty"`
	Group       *string                    `json:"group,omitempty"`
	Enum        map[string]*enumDefinition `json:"enum,omitempty"`
	Sibling     *string                    `json:"sibling,omitempty"`
	ObjectType  *string                    `json:"object_type,omitempty"`
	Observable  *int64                     `json:"observable,omitempty"`
}

type itemAttributeDefinition struct {
	commonAttributeDefinition
	Profiles []string `json:"profiles,omitempty"`
}

// commonItemDefinition is the common structure shared by classes and objects.
// (The term "item" is used as the generic name an object or class).
type commonItemDefinition struct {
	Deprecated  *deprecatedDefinition               `json:"@deprecated,omitempty"`
	Name        string                              `json:"name"`
	Caption     string                              `json:"caption,omitempty"`
	Description string                              `json:"description,omitempty"`
	Profiles    []string                            `json:"profiles,omitempty"`
	Constraints map[string][]string                 `json:"constraints,omitempty"`
	Attributes  map[string]*itemAttributeDefinition `json:"attributes,omitempty"`
}

type classDefinition struct {
	commonItemDefinition
	Uid         int64            `json:"uid"`
	Category    string           `json:"category"`
	Observables map[string]int64 `json:"observables,omitempty"`
}

type objectDefinition struct {
	commonItemDefinition
	Observable *int64 `json:"observable,omitempty"`
}

type typeDefinition struct {
	commonAttributeDefinition
	TypeName *string `json:"type_name,omitempty"`
	MaxLen   *int64  `json:"max_len,omitempty"`
	Range    []int64 `json:"range,omitempty"`
	RegEx    *string `json:"regex,omitempty"`
	Values   []any   `json:"values,omitempty"`
}

type typesDefinition struct {
	Attributes map[string]*typeDefinition `json:"attributes"`
}
type dictionaryDefinition struct {
	Attributes map[string]*commonAttributeDefinition `json:"attributes"`
	Types      *typesDefinition                      `json:"types,omitempty"`
}

type profileDefinition struct {
	Deprecated  *deprecatedDefinition `json:"@deprecated,omitempty"`
	Name        string                `json:"name"`
	Caption     string                `json:"caption,omitempty"`
	Description string                `json:"description,omitempty"`
}

// schemaDefinition is the union of supported compiled schema formats.
type schemaDefinition struct {
	CompileVersion int                           `json:"compile_version"`
	Classes        map[string]*classDefinition   `json:"classes"`
	Objects        map[string]*objectDefinition  `json:"objects"`
	Dictionary     *dictionaryDefinition         `json:"dictionary"`
	Profiles       map[string]*profileDefinition `json:"profiles"`
	Version        string                        `json:"version"`
}

// schemaImpl is a lightly transformed variation of schemaDefinition that is more useful for enrichment and validation.
type schemaImpl struct {
	schemaDefinition
	classes         map[int64]*classDefinition
	objects         map[string]*objectDefinition
	dictionary      *dictionaryDefinition
	profiles        map[string]*profileDefinition
	version         string
	observableTypes map[int64]string
}
