package bedrock

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// fakeInvoker is the shared test double for the invoker seam across this package's tests.
type fakeInvoker struct {
	fn func(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

func (f *fakeInvoker) InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	return f.fn(ctx, params, optFns...)
}

// Compile-time assertions: the real Bedrock Runtime client and the test fake both satisfy the
// package's unexported invoker seam.
var (
	_ invoker = (*bedrockruntime.Client)(nil)
	_ invoker = (*fakeInvoker)(nil)
)
