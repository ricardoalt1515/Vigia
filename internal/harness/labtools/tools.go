package labtools

import (
	"context"
	"fmt"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// ReadCaseTool retrieves a synthetic case from the CaseStore by case_id.
// RiskClass is RiskClassRead — always allowed by the LabPermissionGate.
// Transcript content is passed through as typed []Utterance and is never
// interpreted as instructions or forwarded as control flow.
type ReadCaseTool struct {
	cases CaseStore
}

// RiskClass returns the static risk class for the read_case tool.
func (t *ReadCaseTool) RiskClass() harness.RiskClass {
	return harness.RiskClassRead
}

// Execute decodes call.Input as ReadCaseRequest, looks up the case in the store,
// and returns the full case view. Unknown case_id returns ToolStatusNotFound.
func (t *ReadCaseTool) Execute(ctx context.Context, call harness.ToolCall) (harness.ToolResult, error) {
	req, err := decode[ReadCaseRequest](call.Input)
	if err != nil {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if req.CaseID == "" {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: "case_id is required"}, nil
	}
	sc, ok := t.cases[req.CaseID]
	if !ok {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: fmt.Sprintf("case not found: %s", req.CaseID)}, nil
	}
	resp := ReadCaseResponse{
		CaseID: req.CaseID,
		Case: SyntheticCaseView{
			TenantID:          sc.TenantID,
			Debtor:            sc.Debtor,
			Collector:         sc.Collector,
			Transcript:        sc.Transcript,
			Channel:           sc.Channel,
			OccurredAt:        sc.OccurredAt,
			DebtorTimezone:    sc.DebtorTimezone,
			DetectorResults:   sc.DetectorResults,
			ApplicableRuleIDs: sc.ApplicableRuleIDs,
			EvidenceMetadata:  sc.EvidenceMetadata,
		},
	}
	out, err := encode(resp)
	if err != nil {
		return harness.ToolResult{}, fmt.Errorf("encode read_case response: %w", err)
	}
	return harness.ToolResult{Status: harness.ToolStatusSuccess, Output: out}, nil
}

// ReadPolicyRuleTool retrieves a synthetic policy rule from the RuleStore by rule_code.
// RiskClass is RiskClassRead — always allowed by the LabPermissionGate.
type ReadPolicyRuleTool struct {
	rules RuleStore
}

// RiskClass returns the static risk class for the read_policy_rule tool.
func (t *ReadPolicyRuleTool) RiskClass() harness.RiskClass {
	return harness.RiskClassRead
}

// Execute decodes call.Input as ReadPolicyRuleRequest, looks up the rule in the store,
// and returns the rule view. Unknown rule_code returns ToolStatusNotFound.
func (t *ReadPolicyRuleTool) Execute(ctx context.Context, call harness.ToolCall) (harness.ToolResult, error) {
	req, err := decode[ReadPolicyRuleRequest](call.Input)
	if err != nil {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if req.RuleCode == "" {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: "rule_code is required"}, nil
	}
	sr, ok := t.rules[req.RuleCode]
	if !ok {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: fmt.Sprintf("rule not found: %s", req.RuleCode)}, nil
	}
	resp := ReadPolicyRuleResponse{
		Rule: SyntheticRuleView{
			Code:        sr.Code,
			Title:       sr.Title,
			Description: sr.Description,
			Severity:    sr.Severity,
		},
	}
	out, err := encode(resp)
	if err != nil {
		return harness.ToolResult{}, fmt.Errorf("encode read_policy_rule response: %w", err)
	}
	return harness.ToolResult{Status: harness.ToolStatusSuccess, Output: out}, nil
}

// ListApplicableRulesTool returns rule summaries for all rules applicable to a given
// case, preserving the order of ApplicableRuleIDs from the case fixture.
// RiskClass is RiskClassRead — always allowed by the LabPermissionGate.
type ListApplicableRulesTool struct {
	cases CaseStore
	rules RuleStore
}

// RiskClass returns the static risk class for the list_applicable_rules tool.
func (t *ListApplicableRulesTool) RiskClass() harness.RiskClass {
	return harness.RiskClassRead
}

// Execute decodes call.Input as ListApplicableRulesRequest, looks up the case,
// and returns summaries for its applicable rules in fixture order.
// Unknown case_id returns ToolStatusNotFound.
func (t *ListApplicableRulesTool) Execute(ctx context.Context, call harness.ToolCall) (harness.ToolResult, error) {
	req, err := decode[ListApplicableRulesRequest](call.Input)
	if err != nil {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if req.CaseID == "" {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: "case_id is required"}, nil
	}
	sc, ok := t.cases[req.CaseID]
	if !ok {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: fmt.Sprintf("case not found: %s", req.CaseID)}, nil
	}

	// Iterate ApplicableRuleIDs in fixture order; intersection with RuleStore preserves
	// original order. Dangling references skipped gracefully (loader already validates).
	summaries := make([]RuleSummary, 0, len(sc.ApplicableRuleIDs))
	for _, code := range sc.ApplicableRuleIDs {
		sr, ok := t.rules[code]
		if !ok {
			continue
		}
		summaries = append(summaries, RuleSummary{
			Code:     sr.Code,
			Title:    sr.Title,
			Severity: sr.Severity,
		})
	}

	resp := ListApplicableRulesResponse{Rules: summaries}
	out, err := encode(resp)
	if err != nil {
		return harness.ToolResult{}, fmt.Errorf("encode list_applicable_rules response: %w", err)
	}
	return harness.ToolResult{Status: harness.ToolStatusSuccess, Output: out}, nil
}

// DraftEvidenceManifestTool drafts a proposed evidence manifest.
// It echoes case_id, rule_codes, and findings from the request unchanged.
// It never reads from CaseStore, never writes anywhere.
// authoritative = false; persisted = false; proposed_at = draftProposedAt (fixed constant).
// RiskClass is RiskClassDraft — allowed by the LabPermissionGate.
type DraftEvidenceManifestTool struct{}

// RiskClass returns the static risk class for the draft_evidence_manifest tool.
func (t *DraftEvidenceManifestTool) RiskClass() harness.RiskClass {
	return harness.RiskClassDraft
}

// Execute decodes call.Input as DraftEvidenceManifestRequest, echoes all fields,
// and returns a deterministic response with authoritative=false and persisted=false.
func (t *DraftEvidenceManifestTool) Execute(ctx context.Context, call harness.ToolCall) (harness.ToolResult, error) {
	req, err := decode[DraftEvidenceManifestRequest](call.Input)
	if err != nil {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	resp := DraftEvidenceManifestResponse{
		CaseID:        req.CaseID,
		RuleCodes:     req.RuleCodes,
		Findings:      req.Findings,
		ProposedAt:    draftProposedAt,
		Authoritative: false,
		Persisted:     false,
	}
	out, err := encode(resp)
	if err != nil {
		return harness.ToolResult{}, fmt.Errorf("encode draft_evidence_manifest response: %w", err)
	}
	return harness.ToolResult{Status: harness.ToolStatusSuccess, Output: out}, nil
}

// DraftSupervisorNoteTool drafts a proposed supervisor note.
// It echoes case_id, rule_codes, and note_body from the request unchanged.
// It never reads from CaseStore, never writes anywhere.
// authoritative = false; persisted = false; proposed_at = draftProposedAt (fixed constant).
// RiskClass is RiskClassDraft — allowed by the LabPermissionGate.
type DraftSupervisorNoteTool struct{}

// RiskClass returns the static risk class for the draft_supervisor_note tool.
func (t *DraftSupervisorNoteTool) RiskClass() harness.RiskClass {
	return harness.RiskClassDraft
}

// Execute decodes call.Input as DraftSupervisorNoteRequest, echoes all fields,
// and returns a deterministic response with authoritative=false and persisted=false.
func (t *DraftSupervisorNoteTool) Execute(ctx context.Context, call harness.ToolCall) (harness.ToolResult, error) {
	req, err := decode[DraftSupervisorNoteRequest](call.Input)
	if err != nil {
		return harness.ToolResult{Status: harness.ToolStatusNotFound, Reason: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	resp := DraftSupervisorNoteResponse{
		CaseID:        req.CaseID,
		RuleCodes:     req.RuleCodes,
		NoteBody:      req.NoteBody,
		ProposedAt:    draftProposedAt,
		Authoritative: false,
		Persisted:     false,
	}
	out, err := encode(resp)
	if err != nil {
		return harness.ToolResult{}, fmt.Errorf("encode draft_supervisor_note response: %w", err)
	}
	return harness.ToolResult{Status: harness.ToolStatusSuccess, Output: out}, nil
}

// Registry returns a harness.ToolRegistry populated with all five lab tools:
// three read tools (read_case, read_policy_rule, list_applicable_rules) and
// two draft tools (draft_evidence_manifest, draft_supervisor_note).
// Authority tools are intentionally absent and never registered.
func Registry(cases CaseStore, rules RuleStore) harness.ToolRegistry {
	return harness.ToolRegistry{
		"read_case":               &ReadCaseTool{cases: cases},
		"read_policy_rule":        &ReadPolicyRuleTool{rules: rules},
		"list_applicable_rules":   &ListApplicableRulesTool{cases: cases, rules: rules},
		"draft_evidence_manifest": &DraftEvidenceManifestTool{},
		"draft_supervisor_note":   &DraftSupervisorNoteTool{},
	}
}
