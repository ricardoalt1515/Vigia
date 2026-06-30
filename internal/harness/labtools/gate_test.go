package labtools

import (
	"context"
	"reflect"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

func TestLabPermissionGate_AllowReadTools(t *testing.T) {
	gate := NewLabPermissionGate()
	ctx := context.Background()
	readTools := []string{"read_case", "read_policy_rule", "list_applicable_rules"}
	for _, name := range readTools {
		t.Run(name, func(t *testing.T) {
			decision := gate.Decide(ctx, harness.ToolCall{Name: name})
			if decision.Kind != harness.PermissionAllowed {
				t.Errorf("%s: decision.Kind = %q, want %q", name, decision.Kind, harness.PermissionAllowed)
			}
		})
	}
}

func TestLabPermissionGate_AllowDraftTools(t *testing.T) {
	gate := NewLabPermissionGate()
	ctx := context.Background()
	draftTools := []string{"draft_evidence_manifest", "draft_supervisor_note"}
	for _, name := range draftTools {
		t.Run(name, func(t *testing.T) {
			decision := gate.Decide(ctx, harness.ToolCall{Name: name})
			if decision.Kind != harness.PermissionAllowed {
				t.Errorf("%s: decision.Kind = %q, want %q", name, decision.Kind, harness.PermissionAllowed)
			}
		})
	}
}

func TestLabPermissionGate_DenyAuthorityTools(t *testing.T) {
	gate := NewLabPermissionGate()
	ctx := context.Background()
	authorityTools := []string{"append_evidence", "update_case_state", "submit_report", "block_campaign"}
	for _, name := range authorityTools {
		t.Run(name, func(t *testing.T) {
			decision := gate.Decide(ctx, harness.ToolCall{Name: name})
			// Must never be allowed
			if decision.Kind == harness.PermissionAllowed {
				t.Errorf("%s: authority tool must never be allowed", name)
			}
			// Must be explicitly denied
			if decision.Kind != harness.PermissionDenied {
				t.Errorf("%s: decision.Kind = %q, want %q", name, decision.Kind, harness.PermissionDenied)
			}
		})
	}
}

func TestLabPermissionGate_DenyUnknownTool(t *testing.T) {
	// Unknown names must be denied (fail-closed).
	gate := NewLabPermissionGate()
	ctx := context.Background()
	unknownTools := []string{"mystery_tool", "", "inject_anything", "UNKNOWN"}
	for _, name := range unknownTools {
		t.Run("unknown:"+name, func(t *testing.T) {
			decision := gate.Decide(ctx, harness.ToolCall{Name: name})
			if decision.Kind != harness.PermissionDenied {
				t.Errorf("%q: decision.Kind = %q, want %q (fail-closed)", name, decision.Kind, harness.PermissionDenied)
			}
		})
	}
}

func TestLabPermissionGate_Determinism(t *testing.T) {
	// Decide called twice with the same ToolCall must return equal PermissionDecision values.
	gate := NewLabPermissionGate()
	ctx := context.Background()

	names := []string{
		"read_case", "read_policy_rule", "list_applicable_rules",
		"draft_evidence_manifest", "draft_supervisor_note",
		"append_evidence", "update_case_state", "submit_report", "block_campaign",
		"mystery_tool",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			call := harness.ToolCall{Name: name}
			d1 := gate.Decide(ctx, call)
			d2 := gate.Decide(ctx, call)
			if !reflect.DeepEqual(d1, d2) {
				t.Errorf("%s: Decide is not deterministic: %v != %v", name, d1, d2)
			}
		})
	}
}
