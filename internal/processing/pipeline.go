package processing

import (
	"fmt"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/internal/eventvalue"
	"github.com/ocsf/ocsf-toolkit/internal/observable"
	"github.com/ocsf/ocsf-toolkit/internal/schema"
	"github.com/ocsf/ocsf-toolkit/internal/validationcache"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

// PipelineFactory owns immutable compiled schema data and lazily constructed processing caches used by the pipelines
// it creates.
type PipelineFactory struct {
	compiled   *schema.Compiled
	validation validationcache.Lazy
}

// NewPipelineFactory creates a pipeline factory for a compiled schema.
func NewPipelineFactory(compiled *schema.Compiled) *PipelineFactory {
	return &PipelineFactory{compiled: compiled}
}

// Compiled returns the compiled schema owned by this pipeline factory.
func (f *PipelineFactory) Compiled() *schema.Compiled {
	if f == nil {
		return nil
	}
	return f.compiled
}

// Pipeline is the concrete internal event-processing engine.
type Pipeline struct {
	compiled   *schema.Compiled
	mutations  []*mutationDispatcher
	validation *validationProcessor
	// requiresEventWalk lets observable-removal-only pipelines skip the schema-guided event walk.
	requiresEventWalk bool
}

// mutationDispatcher forwards to whichever family member (add, safe-remove, or force-remove) handles this
// dispatcher's configured action. Exactly one field is non-nil, so which pointer is non-nil is the tag of this
// type's union, in place of a separate tag field. A concrete union here, rather than an interface field, measurably
// avoids one heap allocation per ProcessEvent call: an interface-based version of this type (same methods, one
// mutationHooks interface field instead of three concrete pointer fields) adds exactly 1 alloc/640 B per op to
// every mutation-touching benchmark (confirmed via `go test ./eventschema/... -bench
// BenchmarkProcessEventEnrichment -benchmem`: 0 B/op here vs 640 B/op through an interface). This is not because
// these methods inline into their caller — escape analysis (`go build -gcflags="-m -m"`) shows most of them do not,
// exceeding the inlining cost budget — the concrete union still avoids the allocation an interface forces
// regardless. See "Hot-Loop Performance" in docs/architecture.md.
//
// An explicit kind field switched by value, instead of nil-pointer-as-tag, was also tried and rejected: disassembly
// (`go build -gcflags="-S"`) showed no jump table for either form (Go's switch lowering only tables dense integer
// switches with more cases than these three), and the explicit-tag form needs one more memory load per call in the
// fast path, since it loads the tag and the processor pointer separately instead of getting both from the single
// load that already serves as the nil check here. Benchmarked runs bore this out: the explicit-tag form was never
// faster and consistently at or slightly above these numbers, never below.
type mutationDispatcher struct {
	add         *enrichmentProcessor
	safeRemove  *enrichmentSafeRemovalProcessor
	forceRemove *enrichmentForceRemovalProcessor
}

func (d *mutationDispatcher) validate() error {
	count := 0
	if d != nil && d.add != nil {
		count++
	}
	if d != nil && d.safeRemove != nil {
		count++
	}
	if d != nil && d.forceRemove != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("mutation dispatcher must contain exactly one processor; found %d", count)
	}
	return nil
}

func (d *mutationDispatcher) onClass(c *processContext, event jsonish.Map) error {
	switch {
	case d.add != nil:
		d.add.onClass(c)
	case d.safeRemove != nil:
		return d.safeRemove.onClass(c, event)
	case d.forceRemove != nil:
		d.forceRemove.onClass(c, event)
	}
	return nil
}

func (d *mutationDispatcher) onClassDone(
	c *processContext,
	item jsonish.Map,
	_ *schema.ItemDefinition,
) error {
	switch {
	case d.safeRemove != nil:
		return d.safeRemove.onClassDone(c, item)
	case d.forceRemove != nil:
		d.forceRemove.onClassDone(c, item)
	}
	return nil
}

func (d *mutationDispatcher) onObject(
	c *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	objectDef *schema.ObjectDefinition,
) {
	if d.add != nil {
		d.add.onObject(c, attributeName, attrDef, objectDef)
	}
}

func (d *mutationDispatcher) onObjectWrongType(
	c *processContext,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
) {
	if d.add != nil {
		d.add.onObjectWrongType(c, attributeName, attrDef)
	}
}

func (d *mutationDispatcher) onAttribute(
	c *processContext,
	item jsonish.Map,
	value any,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	arrayIndex int,
	status attributeState,
) {
	switch {
	case d.add != nil:
		d.add.onAttribute(c, item, value, attributeName, attrDef, arrayIndex, status)
	case d.safeRemove != nil:
		d.safeRemove.onAttribute(c, item, attributeName, attrDef, status)
	case d.forceRemove != nil:
		d.forceRemove.onAttribute(c, item, attributeName, attrDef, status)
	}
}

func (d *mutationDispatcher) onArrayElement(
	c *processContext,
	values eventvalue.ArrayView,
	index int,
	attributeName string,
	attrDef *schema.ItemAttributeDefinition,
	status attributeState,
) {
	if d.add != nil {
		d.add.onArrayElement(c, values, index, attributeName, attrDef, status)
	}
}

func (d *mutationDispatcher) onEnumSiblingPairAttributes(
	c *processContext,
	item jsonish.Map,
	enumAttributeName string,
	enumAttrDef *schema.ItemAttributeDefinition,
	siblingAttributeName string,
	siblingAttrDef *schema.ItemAttributeDefinition,
) error {
	switch {
	case d.add != nil:
		d.add.onEnumSiblingPairAttributes(
			c, item, enumAttributeName, enumAttrDef, siblingAttributeName, siblingAttrDef,
		)
	case d.safeRemove != nil:
		return d.safeRemove.onEnumSiblingPairAttributes(c, item, enumAttributeName, enumAttrDef, siblingAttributeName)
	case d.forceRemove != nil:
		return d.forceRemove.onEnumSiblingPairAttributes(c, item, enumAttributeName, enumAttrDef, siblingAttributeName)
	}
	return nil
}

func (d *mutationDispatcher) onEventDone(c *processContext, event jsonish.Map) error {
	if d.add != nil {
		return d.add.onEventDone(c, event)
	}
	return nil
}

// NewPipeline builds a reusable pipeline from a resolved configuration. Validation runs after mutating processors.
// Enum-sibling work always runs ahead of observable work, with no exception, so newly added, retained, or deleted
// enum siblings are visible (or already absent) before observables are generated or analyzed; building the
// enum-siblings dispatcher before the observables dispatcher below is what guarantees that order for per-attribute
// work (onAttribute/onEnumSiblingPairAttributes).
//
// Observable safe-removal analyzes the whole event once. Enum-sibling work occurs during the attribute walk, so an
// observables-only safe-removal dispatcher must defer analysis until class completion whenever another dispatcher
// handles enum siblings. Force-removing observables never analyzes sibling data and does not need this deferral.
func (f *PipelineFactory) NewPipeline(config PipelineConfig) (*Pipeline, error) {
	compiledValidationPolicy, err := config.validate()
	if err != nil {
		return nil, err
	}
	if f == nil || f.compiled == nil {
		return nil, errUninitializedSchema
	}
	f.compiled.EnsureTraversalCache()

	var validationCache *validationcache.Cache
	if config.ValidationEnabled {
		cache, err := f.validation.Get(f.compiled)
		if err != nil {
			return nil, err
		}
		validationCache = cache
	}

	// requiresEventWalk decides whether ProcessEvent traverses the event at all. Every processor family that
	// inspects per-attribute or per-object state (for example a future lint or update processor) must be added to
	// this condition, independently of whether it also participates in PipelineConfig.Validate's "at least one
	// action" gate in config.go; the two conditions are not the same predicate (observable removal, for instance,
	// counts as an action there but does not require the event walk here).
	pipeline := &Pipeline{
		compiled: f.compiled,
		requiresEventWalk: config.EnumSiblingsAction != enrichment.None ||
			config.ObservablesAction == enrichment.Add ||
			config.ValidationEnabled,
	}

	suppression := newIssueSuppression(config.IssueSuppression)

	// Only an observables-only safe-removal dispatcher uses this pipeline-wide deferral; see the note above.
	deferObservablesRemoval := config.EnumSiblingsAction != enrichment.None

	if config.EnumSiblingsAction == config.ObservablesAction {
		if config.EnumSiblingsAction != enrichment.None {
			dispatcher, err := newMutationDispatcher(
				f.compiled,
				config.EnumSiblingsAction,
				true,
				true,
				false,
				config.Observables,
				suppression,
			)
			if err != nil {
				return nil, err
			}
			pipeline.mutations = append(pipeline.mutations, dispatcher)
		}
	} else {
		var enumDispatcher, observablesDispatcher *mutationDispatcher
		if config.EnumSiblingsAction != enrichment.None {
			dispatcher, err := newMutationDispatcher(
				f.compiled,
				config.EnumSiblingsAction,
				true,
				false,
				false,
				config.Observables,
				suppression,
			)
			if err != nil {
				return nil, err
			}
			enumDispatcher = dispatcher
		}
		if config.ObservablesAction != enrichment.None {
			dispatcher, err := newMutationDispatcher(
				f.compiled,
				config.ObservablesAction,
				false,
				true,
				deferObservablesRemoval,
				config.Observables,
				suppression,
			)
			if err != nil {
				return nil, err
			}
			observablesDispatcher = dispatcher
		}

		pipeline.mutations = append(
			pipeline.mutations,
			orderedMutationDispatchers(enumDispatcher, observablesDispatcher)...,
		)
	}

	if config.ValidationEnabled {
		pipeline.validation = &validationProcessor{
			config: config.Validation,
			cache:  validationCache,
			policy: compiledValidationPolicy,
		}
	}

	return pipeline, nil
}

// orderedMutationDispatchers returns enumDispatcher and observablesDispatcher in the order NewPipeline's mutations
// slice must run them: enum-sibling work always ahead of observable work, with no exception; see the ordering note
// on NewPipeline. If enum-sibling work force-removes siblings before observable safe-removal analyzes them, that
// analysis correctly cannot verify (and so retains) any observable derived from a now-deleted sibling.
func orderedMutationDispatchers(enumDispatcher, observablesDispatcher *mutationDispatcher) []*mutationDispatcher {
	dispatchers := make([]*mutationDispatcher, 0, 2)
	appendDispatcher := func(dispatcher *mutationDispatcher) {
		if dispatcher != nil {
			dispatchers = append(dispatchers, dispatcher)
		}
	}
	appendDispatcher(enumDispatcher)
	appendDispatcher(observablesDispatcher)
	return dispatchers
}

// newMutationDispatcher builds the mutationDispatcher family member (add, safe-remove, or force-remove) that
// implements action for the selected components.
func newMutationDispatcher(
	compiled *schema.Compiled,
	action enrichment.Action,
	enumSiblingsEnabled, observablesEnabled, deferObservablesRemoval bool,
	observables ObservablesConfig,
	suppression issueSuppression,
) (dispatcher *mutationDispatcher, err error) {
	defer func() {
		if err == nil {
			err = dispatcher.validate()
		}
	}()
	switch action {
	case enrichment.Add:
		var observableTypes observableTypeSelector
		var classObservableTries map[int64]*classObservableTrie
		var objectObservability observable.ObjectObservability
		if observablesEnabled {
			selector, err := newObservableTypeSelector(compiled, observables.TypeIDs)
			if err != nil {
				return nil, err
			}
			observableTypes = selector
			classObservableTries = compileClassObservableTries(compiled.Classes, observableTypes)
			objectObservability = observable.CompileObjectObservability(compiled, observableTypes.allows)
		}
		return &mutationDispatcher{add: &enrichmentProcessor{
			enumSiblingsEnabled:  enumSiblingsEnabled,
			observablesEnabled:   observablesEnabled,
			pathNotation:         observables.PathNotation,
			observableTypes:      observableTypes,
			classObservableTries: classObservableTries,
			objectObservability:  objectObservability,
			issueSuppression:     suppression,
		}}, nil
	case enrichment.Remove:
		return &mutationDispatcher{safeRemove: &enrichmentSafeRemovalProcessor{
			enumSiblingsEnabled:     enumSiblingsEnabled,
			observablesEnabled:      observablesEnabled,
			deferObservablesRemoval: deferObservablesRemoval || enumSiblingsEnabled && observablesEnabled,
			issueSuppression:        suppression,
		}}, nil
	case enrichment.ForceRemove:
		return &mutationDispatcher{forceRemove: &enrichmentForceRemovalProcessor{
			enumSiblingsEnabled:     enumSiblingsEnabled,
			observablesEnabled:      observablesEnabled,
			deferObservablesRemoval: deferObservablesRemoval || enumSiblingsEnabled && observablesEnabled,
			issueSuppression:        suppression,
		}}, nil
	default:
		// enrichment.None, and defensively other unknown actions, do not identify a mutation processor.
		return nil, fmt.Errorf("unsupported enrichment action %q", action)
	}
}
