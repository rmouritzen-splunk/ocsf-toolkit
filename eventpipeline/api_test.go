package eventpipeline_test

import (
	"testing"

	"github.com/ocsf/ocsf-toolkit/eventpipeline"
)

type withSchemaSignature func(
	*eventpipeline.Schema,
	...eventpipeline.SchemaPipelineOption,
) eventpipeline.PipelineOption

// Invariant test: WithSchema reserves variadic schema-pipeline options so future schema registration behavior can be
// added without changing the function signature.
func TestInvariantWithSchemaAcceptsSchemaPipelineOptions(t *testing.T) {
	t.Parallel()
	requireWithSchemaSignature(eventpipeline.WithSchema)
}

func requireWithSchemaSignature(_ withSchemaSignature) {
}
