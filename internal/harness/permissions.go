package harness

import "context"

// PermissionDecisionKind describes whether a proposed tool may execute.
type PermissionDecisionKind string

const (
	PermissionAllowed          PermissionDecisionKind = "allowed"
	PermissionDenied           PermissionDecisionKind = "denied"
	PermissionApprovalRequired PermissionDecisionKind = "approval_required"
)

// PermissionDecision is the typed result of evaluating one proposed tool call.
type PermissionDecision struct {
	Kind     PermissionDecisionKind
	Reason   string
	Metadata map[string]any
}

// PermissionGate decides whether the runtime may execute a proposed tool call.
type PermissionGate interface {
	Decide(ctx context.Context, call ToolCall) PermissionDecision
}
