package mcp

import (
	"context"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// ReadOnlyPermissionGate allows only the first-slice read-only MCP tools.
type ReadOnlyPermissionGate struct{}

func (ReadOnlyPermissionGate) Decide(ctx context.Context, call harness.ToolCall) harness.PermissionDecision {
	switch call.Name {
	case "read_case_brief", "read_evidence_manifest":
		return harness.PermissionDecision{Kind: harness.PermissionAllowed}
	default:
		return harness.PermissionDecision{Kind: harness.PermissionDenied, Reason: "tool is not in the first-slice MCP catalog"}
	}
}
