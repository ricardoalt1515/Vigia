package labtools

import (
	"context"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// LabPermissionGate implements harness.PermissionGate for the synthetic lab environment.
// It allows read and draft tools; denies authority tools and any unknown tool (fail-closed).
// The gate is pure in-memory catalog lookup — no external calls, deterministic.
type LabPermissionGate struct{}

// NewLabPermissionGate constructs a LabPermissionGate. No configuration is required.
func NewLabPermissionGate() *LabPermissionGate {
	return &LabPermissionGate{}
}

// Decide looks up call.Name in the risk catalog and returns the appropriate decision.
//
//   - RiskClassRead or RiskClassDraft  → PermissionAllowed
//   - RiskClassAuthority               → PermissionDenied
//   - Unknown name (not in catalog)    → PermissionDenied (fail-closed)
func (g *LabPermissionGate) Decide(ctx context.Context, call harness.ToolCall) harness.PermissionDecision {
	rc, ok := riskClassFor(call.Name)
	if !ok {
		// Fail-closed: unregistered tools are never allowed.
		return harness.PermissionDecision{
			Kind:   harness.PermissionDenied,
			Reason: "authority-bearing or unregistered tool",
		}
	}
	switch rc {
	case harness.RiskClassRead, harness.RiskClassDraft:
		return harness.PermissionDecision{Kind: harness.PermissionAllowed}
	default:
		// RiskClassAuthority — and any future unknown class — are denied.
		return harness.PermissionDecision{
			Kind:   harness.PermissionDenied,
			Reason: "authority-bearing or unregistered tool",
		}
	}
}
