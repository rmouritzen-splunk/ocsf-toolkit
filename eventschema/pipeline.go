package eventschema

import (
	"errors"
	"fmt"

	"github.com/ocsf/ocsf-toolkit/jsonish"
)

type processorKind string

const (
	processorKindEnrichment        processorKind = "enrichment"
	processorKindEnrichmentRemoval processorKind = "enrichment-removal"
	processorKindValidation        processorKind = "validation"
)

type pipelineConfig struct {
	processors []configuredProcessor
}

type configuredProcessor struct {
	kind       processorKind
	factory    processorFactory
	enrichment enrichmentConfig
	removal    enrichmentRemovalConfig
}

type processorFactory func() eventProcessVisitor

type eventProcessorPipelineImpl struct {
	schema     *schemaImpl
	processors []eventProcessVisitor
}

func (si *schemaImpl) NewEventProcessorPipeline(processors ...EventProcessor) (EventProcessorPipeline, error) {
	var config pipelineConfig
	for _, processor := range processors {
		if processor != nil {
			processor.applyProcessor(&config)
		}
	}
	if err := validatePipelineConfig(config); err != nil {
		return nil, err
	}
	visitors := make([]eventProcessVisitor, 0, len(config.processors))
	for _, processor := range config.processors {
		if processor.kind != processorKindValidation {
			visitors = append(visitors, processor.factory())
		}
	}
	for _, processor := range config.processors {
		if processor.kind == processorKindValidation {
			visitors = append(visitors, processor.factory())
		}
	}
	return &eventProcessorPipelineImpl{
		schema:     si,
		processors: visitors,
	}, nil
}

func validatePipelineConfig(config pipelineConfig) error {
	if len(config.processors) == 0 {
		return errors.New("at least one event processing action is required")
	}

	counts := make(map[processorKind]int, 3)
	for _, processor := range config.processors {
		counts[processor.kind]++
	}

	var problems []error
	for _, kind := range []processorKind{
		processorKindValidation,
		processorKindEnrichment,
		processorKindEnrichmentRemoval,
	} {
		if counts[kind] > 1 {
			problems = append(problems, fmt.Errorf("%s processor may only be configured once", kind))
		}
	}

	var addEnumSiblings, addObservables bool
	var removeEnumSiblings, removeObservables bool
	for index, processor := range config.processors {
		label := processorLabel(processor.kind, index, counts[processor.kind])
		if processor.factory == nil {
			problems = append(problems, fmt.Errorf("%s has no factory", label))
		}
		switch processor.kind {
		case processorKindEnrichment:
			if !processor.enrichment.addEnumSiblings && !processor.enrichment.addObservables {
				problems = append(problems, fmt.Errorf("%s must enable at least one action", label))
			}
			addEnumSiblings = addEnumSiblings || processor.enrichment.addEnumSiblings
			addObservables = addObservables || processor.enrichment.addObservables
		case processorKindEnrichmentRemoval:
			if !processor.removal.removeEnumSiblings && !processor.removal.removeObservables {
				problems = append(problems, fmt.Errorf("%s must enable at least one action", label))
			}
			if processor.removal.forceRemoveEnumSiblings && processor.removal.retainEnumSiblings {
				problems = append(problems, fmt.Errorf("%s forces and retains enum siblings", label))
			}
			if processor.removal.forceRemoveObservables && processor.removal.retainObservables {
				problems = append(problems, fmt.Errorf("%s forces and retains observables", label))
			}
			removeEnumSiblings = removeEnumSiblings || processor.removal.removeEnumSiblings
			removeObservables = removeObservables || processor.removal.removeObservables
		case processorKindValidation:
		default:
			problems = append(problems, fmt.Errorf("%s has an unknown processor kind", label))
		}
	}

	if addEnumSiblings && removeEnumSiblings {
		problems = append(problems, errors.New("adding and removing enum siblings are mutually exclusive"))
	}
	if addObservables && removeObservables {
		problems = append(problems, errors.New("adding and removing observables are mutually exclusive"))
	}
	return errors.Join(problems...)
}

func processorLabel(kind processorKind, index int, count int) string {
	if count > 1 {
		return fmt.Sprintf("%s processor at position %d", kind, index+1)
	}
	return string(kind) + " processor"
}

func (p *eventProcessorPipelineImpl) ProcessEvent(event jsonish.Map) (ProcessingResult, error) {
	return p.schema.processEvent(event, p.processors)
}
