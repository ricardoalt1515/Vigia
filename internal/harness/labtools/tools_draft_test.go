package labtools

import (
	"context"
	"reflect"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

func TestDraftEvidenceManifestTool_HappyPath(t *testing.T) {
	tool := &DraftEvidenceManifestTool{}
	call := harness.ToolCall{
		Name: "draft_evidence_manifest",
		Input: map[string]any{
			"case_id":    "CASE-SYN-001",
			"rule_codes": []any{"MX-REDECO-04"},
			"findings":   "Out-of-hours contact detected",
		},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != harness.ToolStatusSuccess {
		t.Fatalf("status = %q, want %q", result.Status, harness.ToolStatusSuccess)
	}

	// Echo: case_id must equal request
	if got := result.Output["case_id"]; got != "CASE-SYN-001" {
		t.Errorf("case_id = %v, want CASE-SYN-001", got)
	}
	// Echo: rule_codes must equal request
	ruleCodes, ok := result.Output["rule_codes"].([]interface{})
	if !ok || len(ruleCodes) != 1 || ruleCodes[0] != "MX-REDECO-04" {
		t.Errorf("rule_codes = %v, want [MX-REDECO-04]", result.Output["rule_codes"])
	}
	// Echo: findings must equal request (gatekeeper correction #3)
	if got := result.Output["findings"]; got != "Out-of-hours contact detected" {
		t.Errorf("findings = %v, want Out-of-hours contact detected", got)
	}
	// authoritative must be false
	if got, _ := result.Output["authoritative"].(bool); got {
		t.Error("authoritative must be false")
	}
	// persisted must be false
	if got, _ := result.Output["persisted"].(bool); got {
		t.Error("persisted must be false")
	}
}

func TestDraftEvidenceManifestTool_ProposedAt(t *testing.T) {
	// proposed_at must equal the fixed RFC 3339 constant — never time.Now() (gatekeeper correction #2)
	tool := &DraftEvidenceManifestTool{}
	call := harness.ToolCall{
		Name: "draft_evidence_manifest",
		Input: map[string]any{
			"case_id":    "CASE-SYN-001",
			"rule_codes": []any{"MX-REDECO-04"},
			"findings":   "test",
		},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	got, _ := result.Output["proposed_at"].(string)
	if got != draftProposedAt {
		t.Errorf("proposed_at = %q, want fixed constant %q", got, draftProposedAt)
	}
}

func TestDraftSupervisorNoteTool_HappyPath(t *testing.T) {
	tool := &DraftSupervisorNoteTool{}
	call := harness.ToolCall{
		Name: "draft_supervisor_note",
		Input: map[string]any{
			"case_id":    "CASE-SYN-001",
			"rule_codes": []any{"MX-REDECO-05"},
			"note_body":  "Supervisor review required",
		},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != harness.ToolStatusSuccess {
		t.Fatalf("status = %q, want %q", result.Status, harness.ToolStatusSuccess)
	}

	// Echo: case_id must equal request
	if got := result.Output["case_id"]; got != "CASE-SYN-001" {
		t.Errorf("case_id = %v, want CASE-SYN-001", got)
	}
	// Echo: rule_codes must equal request
	ruleCodes, ok := result.Output["rule_codes"].([]interface{})
	if !ok || len(ruleCodes) != 1 || ruleCodes[0] != "MX-REDECO-05" {
		t.Errorf("rule_codes = %v, want [MX-REDECO-05]", result.Output["rule_codes"])
	}
	// Echo: note_body must equal request (gatekeeper correction #3)
	if got := result.Output["note_body"]; got != "Supervisor review required" {
		t.Errorf("note_body = %v, want Supervisor review required", got)
	}
	// authoritative must be false
	if got, _ := result.Output["authoritative"].(bool); got {
		t.Error("authoritative must be false")
	}
	// persisted must be false
	if got, _ := result.Output["persisted"].(bool); got {
		t.Error("persisted must be false")
	}
}

func TestDraftSupervisorNoteTool_ProposedAt(t *testing.T) {
	// proposed_at must equal the fixed RFC 3339 constant — never time.Now() (gatekeeper correction #2)
	tool := &DraftSupervisorNoteTool{}
	call := harness.ToolCall{
		Name: "draft_supervisor_note",
		Input: map[string]any{
			"case_id":    "CASE-SYN-001",
			"rule_codes": []any{"MX-REDECO-05"},
			"note_body":  "test note",
		},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	got, _ := result.Output["proposed_at"].(string)
	if got != draftProposedAt {
		t.Errorf("proposed_at = %q, want fixed constant %q", got, draftProposedAt)
	}
}

func TestDraftTools_Determinism(t *testing.T) {
	// Two calls with identical input must return reflect.DeepEqual results.
	// Guaranteed by constant proposed_at + echo fields (no wall clock, no randomness).
	ctx := context.Background()

	manifestTool := &DraftEvidenceManifestTool{}
	manifestCall := harness.ToolCall{
		Name: "draft_evidence_manifest",
		Input: map[string]any{
			"case_id":    "CASE-SYN-001",
			"rule_codes": []any{"MX-REDECO-04"},
			"findings":   "determinism check",
		},
	}
	r1, _ := manifestTool.Execute(ctx, manifestCall)
	r2, _ := manifestTool.Execute(ctx, manifestCall)
	if !reflect.DeepEqual(r1, r2) {
		t.Error("draft_evidence_manifest is not deterministic: results differ between calls")
	}

	noteTool := &DraftSupervisorNoteTool{}
	noteCall := harness.ToolCall{
		Name: "draft_supervisor_note",
		Input: map[string]any{
			"case_id":    "CASE-SYN-001",
			"rule_codes": []any{"MX-REDECO-05"},
			"note_body":  "determinism check",
		},
	}
	n1, _ := noteTool.Execute(ctx, noteCall)
	n2, _ := noteTool.Execute(ctx, noteCall)
	if !reflect.DeepEqual(n1, n2) {
		t.Error("draft_supervisor_note is not deterministic: results differ between calls")
	}
}

func TestDraftTools_NoStoresMutation(t *testing.T) {
	// Draft tools must not mutate CaseStore or RuleStore.
	cases, rules := loadStoresForTest(t)

	// Snapshot pre-call state
	casesBefore := make(CaseStore, len(cases))
	for k, v := range cases {
		casesBefore[k] = v
	}
	rulesBefore := make(RuleStore, len(rules))
	for k, v := range rules {
		rulesBefore[k] = v
	}

	manifestTool := &DraftEvidenceManifestTool{}
	_, _ = manifestTool.Execute(context.Background(), harness.ToolCall{
		Name: "draft_evidence_manifest",
		Input: map[string]any{
			"case_id":    "CASE-SYN-001",
			"rule_codes": []any{"MX-REDECO-04"},
			"findings":   "mutation check",
		},
	})

	noteTool := &DraftSupervisorNoteTool{}
	_, _ = noteTool.Execute(context.Background(), harness.ToolCall{
		Name: "draft_supervisor_note",
		Input: map[string]any{
			"case_id":    "CASE-SYN-001",
			"rule_codes": []any{"MX-REDECO-05"},
			"note_body":  "mutation check",
		},
	})

	if !reflect.DeepEqual(cases, casesBefore) {
		t.Error("CaseStore was mutated by a draft tool call")
	}
	if !reflect.DeepEqual(rules, rulesBefore) {
		t.Error("RuleStore was mutated by a draft tool call")
	}
}

func TestDraftTools_RiskClass(t *testing.T) {
	tests := []struct {
		name      string
		riskClass func() harness.RiskClass
	}{
		{"DraftEvidenceManifestTool", (&DraftEvidenceManifestTool{}).RiskClass},
		{"DraftSupervisorNoteTool", (&DraftSupervisorNoteTool{}).RiskClass},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.riskClass(); got != harness.RiskClassDraft {
				t.Errorf("%s.RiskClass() = %q, want %q", tc.name, got, harness.RiskClassDraft)
			}
		})
	}
}
