package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/bedrock"
	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

// newBedrockFactory is a test seam: production code always resolves to bedrock.NewFactory.
// Tests override this package variable to inject a fake/mock Bedrock construction path so a
// full CLI run can be exercised deterministically with no live AWS network call or credentials.
var newBedrockFactory = bedrock.NewFactory

// selectProviderFactory resolves the caseflow.ProviderFactory to use for a run, based on the
// --provider flag value. "fake" (or an empty string, the flag's default) returns
// demoProviderFactory unchanged, with no Bedrock construction attempted. "bedrock" reads
// AWS_REGION and BEDROCK_MODEL_ID directly via os.LookupEnv — intentionally not
// internal/config.Load/LoadFromEnv, which unconditionally requires unrelated DATABASE_URL and
// OBJECT_STORE_* keys that would regress this DB-free demo CLI — and delegates to
// bedrock.NewFactory (via the newBedrockFactory seam). Any other value is a usage error.
func selectProviderFactory(ctx context.Context, provider string) (caseflow.ProviderFactory, error) {
	switch provider {
	case "", "fake":
		return demoProviderFactory, nil
	case "bedrock":
		region, _ := os.LookupEnv("AWS_REGION")
		modelID, _ := os.LookupEnv("BEDROCK_MODEL_ID")

		factory, err := newBedrockFactory(ctx, bedrock.Options{Region: region, ModelID: modelID})
		if err != nil {
			return nil, err
		}
		return factory, nil
	default:
		return nil, fmt.Errorf("unknown --provider value %q, want \"fake\" or \"bedrock\"", provider)
	}
}

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
