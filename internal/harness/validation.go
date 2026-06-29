package harness

// Validator checks model output before the runtime accepts plans, tool calls, or final output.
type Validator interface {
	Validate(ModelOutput) error
}
