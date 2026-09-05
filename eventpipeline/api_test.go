package eventpipeline_test

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/eventpipeline"
	"github.com/ocsf/ocsf-toolkit/eventresult"
	"github.com/ocsf/ocsf-toolkit/jsonish"
)

type withSchemaSignature func(
	*eventpipeline.Schema,
	...eventpipeline.SchemaPipelineOption,
) eventpipeline.PipelineOption

type pipelineContract interface {
	ProcessEvent(jsonish.Map) (eventpipeline.ProcessingResult, error)
}

type processingResultContract interface {
	Validation() eventresult.ValidationResult
	Enrichment() eventresult.EnrichmentResult
	EnrichmentRemoval() eventresult.EnrichmentRemovalResult
	Issues() []eventresult.ProcessingIssue
}

// Invariant test: WithSchema reserves variadic schema-pipeline options so future schema registration behavior can be
// added without changing the function signature.
func TestInvariantWithSchemaAcceptsSchemaPipelineOptions(t *testing.T) {
	t.Parallel()
	requireWithSchemaSignature(eventpipeline.WithSchema)
}

func requireWithSchemaSignature(_ withSchemaSignature) {
}

// Invariant test: Pipeline and ProcessingResult retain their existing public methods and exact signatures while
// permitting backward-compatible methods to be added for future processor families.
func TestInvariantPipelineAndProcessingResultContractsRemainAdditivelyExtensible(t *testing.T) {
	t.Parallel()
	requirePipelineContract((*eventpipeline.Pipeline)(nil))
	requireProcessingResultContract(eventpipeline.ProcessingResult{})
}

func requirePipelineContract(_ pipelineContract) {
}

func requireProcessingResultContract(_ processingResultContract) {
}
