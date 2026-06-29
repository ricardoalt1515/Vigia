package harness

import "context"

// ToolCall is a proposed tool invocation from a validated model output.
type ToolCall struct {
	Name  string
	Input map[string]any
}

// Tool executes a proposed tool call after the permission gate allows it.
type Tool interface {
	Execute(ctx context.Context, call ToolCall) (ToolResult, error)
}

// ToolRegistry maps tool names to their local executors.
type ToolRegistry map[string]Tool

// ToolStatus identifies the outcome of a proposed tool call.
type ToolStatus string

const (
	ToolStatusSuccess          ToolStatus = "success"
	ToolStatusDenied           ToolStatus = "denied"
	ToolStatusApprovalRequired ToolStatus = "approval_required"
	ToolStatusNotFound         ToolStatus = "not_found"
)

// ToolResult is the structured result of evaluating or executing a tool call.
type ToolResult struct {
	Status ToolStatus
	Output map[string]any
	Reason string
}
