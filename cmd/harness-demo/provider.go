package main

import (
	"context"
	"errors"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// scriptedProvider is a queued Fake harness.ModelProvider for the demo CLI. It mirrors the #20
// e2e test's caseflowQueuedProvider pattern but lives in package main so demo data never enters
// caseflow.
type scriptedProvider struct {
	outputs []harness.ModelOutput
	calls   int
}

// Generate returns the next queued output, in order. It errors if the queue is exhausted; the
// demo CLI never calls a scripted agent more than its script length permits under a nominal run.
func (p *scriptedProvider) Generate(_ context.Context, _ harness.ModelRequest) (harness.ModelOutput, error) {
	p.calls++
	if len(p.outputs) == 0 {
		return harness.ModelOutput{}, errors.New("scriptedProvider: unexpected model call, queue empty")
	}
	out := p.outputs[0]
	p.outputs = p.outputs[1:]
	return out, nil
}

// Scripted valid handoff JSON for each agent — CASE-SYN-001, no denylist terms,
// authoritative/persisted false. Mirrors the shape and content of the #20 e2e test fixtures.
const (
	demoPolicyExplanationJSON = `{"case_id":"CASE-SYN-001","rules":[{"code":"MX-REDECO-04","title":"Calling Hours","severity":"high","plain_language":"Collectors must not contact debtors outside permitted hours."}]}`

	demoCaseInvestigationJSON = `{"case_id":"CASE-SYN-001","findings":[{"rule_code":"MX-REDECO-04","evidence":"Call placed at 23:30 local time","analysis":"Exceeds permitted calling hours under MX-REDECO-04"}]}`

	demoEvidenceManifestJSON = `{"case_id":"CASE-SYN-001","rule_codes":["MX-REDECO-04"],"findings":"Collector contacted debtor outside permitted hours on 2024-03-15","proposed_at":"2026-01-01T00:00:00Z","authoritative":false,"persisted":false}`

	demoSupervisorNoteJSON = `{"case_id":"CASE-SYN-001","rule_codes":["MX-REDECO-04"],"note_body":"Supervisor: calling hours violation detected. Collector contacted debtor at 23:30 on 2024-03-15. Refer for remediation.","proposed_at":"2026-01-01T00:00:00Z","authoritative":false,"persisted":false}`
)

// demoProviderFactory returns a scripted harness.ModelProvider for one of the four case-flow
// agent names, producing deterministic output for CASE-SYN-001: one tool-call step matching that
// agent's allowlisted tool, followed by one synthesis step producing a valid handoff payload. It
// panics on an unrecognized name — an unscripted agent name is a programmer error, never reached
// at runtime because main.go's case-id guard runs before the orchestrator is constructed.
func demoProviderFactory(name string) harness.ModelProvider {
	switch name {
	case "PolicyExplainer":
		return &scriptedProvider{outputs: []harness.ModelOutput{
			{ToolCall: &harness.ToolCall{Name: "list_applicable_rules", Input: map[string]any{"case_id": "CASE-SYN-001"}}},
			{FinalOutput: demoPolicyExplanationJSON},
		}}
	case "CaseInvestigator":
		return &scriptedProvider{outputs: []harness.ModelOutput{
			{ToolCall: &harness.ToolCall{Name: "read_case", Input: map[string]any{"case_id": "CASE-SYN-001"}}},
			{FinalOutput: demoCaseInvestigationJSON},
		}}
	case "EvidencePackager":
		return &scriptedProvider{outputs: []harness.ModelOutput{
			{ToolCall: &harness.ToolCall{Name: "draft_evidence_manifest", Input: map[string]any{
				"case_id":    "CASE-SYN-001",
				"rule_codes": []any{"MX-REDECO-04"},
				"findings":   "Collector contacted debtor outside permitted hours on 2024-03-15",
			}}},
			{FinalOutput: demoEvidenceManifestJSON},
		}}
	case "SupervisorNoteDrafter":
		return &scriptedProvider{outputs: []harness.ModelOutput{
			{ToolCall: &harness.ToolCall{Name: "draft_supervisor_note", Input: map[string]any{
				"case_id":    "CASE-SYN-001",
				"rule_codes": []any{"MX-REDECO-04"},
				"note_body":  "Supervisor: calling hours violation detected. Collector contacted debtor at 23:30 on 2024-03-15. Refer for remediation.",
			}}},
			{FinalOutput: demoSupervisorNoteJSON},
		}}
	default:
		panic("demoProviderFactory: no script for agent " + name)
	}
}
