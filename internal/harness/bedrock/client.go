// Package bedrock implements harness.ModelProvider against Claude via Amazon Bedrock Runtime.
// It is an opt-in infrastructure adapter: no test in this package requires live AWS credentials
// or network access, and no AWS SDK type crosses the package's exported surface.
package bedrock

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// invoker is the structural seam matched verbatim against bedrockruntime.Client.InvokeModel.
// The real *bedrockruntime.Client satisfies it with zero adapter code; tests inject a fakeInvoker
// instead, so no test in this package performs a live AWS network call.
type invoker interface {
	InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}
