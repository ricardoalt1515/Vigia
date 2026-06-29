package harness

import "context"

// ModelRequest carries the inputs for one model generation call.
type ModelRequest struct {
	// Input is the prompt or context passed to the model.
	Input string
}

// ModelOutput is the typed result of one model generation call.
type ModelOutput struct {
	// Plan is the optional visible plan produced by the model.
	Plan string
	// ToolCall is the optional tool the model proposes to call.
	ToolCall *ToolCall
	// FinalOutput is the optional finished answer when no further tool calls are needed.
	FinalOutput string
}

// ModelProvider is the narrow provider boundary for the Harness runtime.
// It intentionally has no prompts, model IDs, streaming, or provider SDK types.
type ModelProvider interface {
	Generate(ctx context.Context, request ModelRequest) (ModelOutput, error)
}
