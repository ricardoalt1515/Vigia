package caseflow_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

// --- helpers -----------------------------------------------------------------

func toolOutput(name string, input map[string]any) harness.ModelOutput {
	return harness.ModelOutput{ToolCall: &harness.ToolCall{Name: name, Input: input}}
}

func finalOutput(s string) harness.ModelOutput {
	return harness.ModelOutput{FinalOutput: s}
}

// validPolicyExplanationJSON returns a valid PolicyExplanation JSON for a given caseID.
func validPolicyExplanationJSON(caseID string) string {
	b, _ := json.Marshal(caseflow.PolicyExplanation{
		CaseID: caseID,
		Rules: []caseflow.PolicyRule{
			{Code: "MX-01", Title: "Rule 1", Severity: "high", PlainLanguage: "Plain text here"},
		},
	})
	return string(b)
}

func validCaseInvestigationJSON(caseID string) string {
	b, _ := json.Marshal(caseflow.CaseInvestigation{
		CaseID: caseID,
		Findings: []caseflow.InvestigationFinding{
			{RuleCode: "MX-01", Evidence: "transcript excerpt", Analysis: "non-compliant"},
		},
	})
	return string(b)
}

func validEvidenceManifestJSON(caseID string) string {
	b, _ := json.Marshal(caseflow.EvidenceManifestDraft{
		CaseID:     caseID,
		RuleCodes:  []string{"MX-01"},
		Findings:   "evidence summary",
		ProposedAt: "2026-01-01T00:00:00Z",
	})
	return string(b)
}

func validSupervisorNoteJSON(caseID string) string {
	b, _ := json.Marshal(caseflow.SupervisorNoteDraft{
		CaseID:     caseID,
		RuleCodes:  []string{"MX-01"},
		NoteBody:   "supervisor note body",
		ProposedAt: "2026-01-01T00:00:00Z",
	})
	return string(b)
}

// fourAgentDefs returns four minimal AgentDefinitions for orchestrator tests.
// Each has MaxModelAttempts=1 and a trivial pass-through validator.
func fourAgentDefs(providers map[string]*caseflowQueuedProvider) ([]caseflow.AgentDefinition, caseflow.ProviderFactory) {
	defs := []caseflow.AgentDefinition{
		{
			Name:          "PolicyExplainer",
			Instructions:  "explain policy",
			ToolAllowlist: []string{"list_applicable_rules"},
			Budget:        harness.Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
			MaxSteps:      4,
			Validator:     acceptAll,
			DecodeHandoff: func(s string) (caseflow.HandoffArtifact, error) {
				var p caseflow.PolicyExplanation
				if err := json.Unmarshal([]byte(s), &p); err != nil {
					return nil, err
				}
				return &p, nil
			},
		},
		{
			Name:          "CaseInvestigator",
			Instructions:  "investigate case",
			ToolAllowlist: []string{"read_case"},
			Budget:        harness.Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
			MaxSteps:      3,
			Validator:     acceptAll,
			DecodeHandoff: func(s string) (caseflow.HandoffArtifact, error) {
				var ci caseflow.CaseInvestigation
				if err := json.Unmarshal([]byte(s), &ci); err != nil {
					return nil, err
				}
				return &ci, nil
			},
		},
		{
			Name:          "EvidencePackager",
			Instructions:  "package evidence",
			ToolAllowlist: []string{"draft_evidence_manifest"},
			Budget:        harness.Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
			MaxSteps:      3,
			Validator:     acceptAll,
			DecodeHandoff: func(s string) (caseflow.HandoffArtifact, error) {
				var e caseflow.EvidenceManifestDraft
				if err := json.Unmarshal([]byte(s), &e); err != nil {
					return nil, err
				}
				return &e, nil
			},
		},
		{
			Name:          "SupervisorNoteDrafter",
			Instructions:  "draft note",
			ToolAllowlist: []string{"draft_supervisor_note"},
			Budget:        harness.Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
			MaxSteps:      3,
			Validator:     acceptAll,
			DecodeHandoff: func(s string) (caseflow.HandoffArtifact, error) {
				var n caseflow.SupervisorNoteDraft
				if err := json.Unmarshal([]byte(s), &n); err != nil {
					return nil, err
				}
				return &n, nil
			},
		},
	}

	factory := func(name string) harness.ModelProvider {
		q, ok := providers[name]
		if !ok {
			panic("factory: no provider for agent " + name)
		}
		return q
	}
	return defs, factory
}

// --- MaxModelAttempts guard --------------------------------------------------

func TestNewOrchestratorGuard_MaxModelAttempts(t *testing.T) {
	cases := []struct {
		name      string
		attempts  int
		wantError bool
	}{
		{"zero attempts rejected", 0, true},
		{"two attempts rejected", 2, true},
		{"one attempt accepted", 1, false},
	}

	gate := gateAll(harness.PermissionAllowed)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defs := []caseflow.AgentDefinition{
				{
					Name:         "TestAgent",
					Instructions: "test",
					Budget:       harness.Budget{MaxModelAttempts: tc.attempts, MaxToolCalls: 1},
					MaxSteps:     3,
					Validator:    acceptAll,
					DecodeHandoff: func(s string) (caseflow.HandoffArtifact, error) {
						return nil, nil
					},
				},
			}
			p := &caseflowQueuedProvider{}
			factory := func(string) harness.ModelProvider { return p }
			_, err := caseflow.NewOrchestrator(factory, harness.ToolRegistry{}, gate, defs)
			if tc.wantError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.wantError && err != nil && !strings.Contains(err.Error(), "TestAgent") {
				t.Errorf("error should mention agent name 'TestAgent', got: %v", err)
			}
		})
	}
}

// --- Fixed invocation order --------------------------------------------------

func TestOrchestrator_FixedInvocationOrder(t *testing.T) {
	const caseID = "CASE-SYN-001"
	// Synthesis-only: this test verifies agent order, not tool-calling behavior.
	providers := map[string]*caseflowQueuedProvider{
		"PolicyExplainer":       {outputs: []harness.ModelOutput{finalOutput(validPolicyExplanationJSON(caseID))}},
		"CaseInvestigator":      {outputs: []harness.ModelOutput{finalOutput(validCaseInvestigationJSON(caseID))}},
		"EvidencePackager":      {outputs: []harness.ModelOutput{finalOutput(validEvidenceManifestJSON(caseID))}},
		"SupervisorNoteDrafter": {outputs: []harness.ModelOutput{finalOutput(validSupervisorNoteJSON(caseID))}},
	}

	defs, factory := fourAgentDefs(providers)
	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, harness.ToolRegistry{}, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	brief, err := o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	wantOrder := []string{"PolicyExplainer", "CaseInvestigator", "EvidencePackager", "SupervisorNoteDrafter"}
	if len(brief.Stages) != 4 {
		t.Fatalf("expected 4 stages, got %d", len(brief.Stages))
	}
	for i, want := range wantOrder {
		if brief.Stages[i].AgentName != want {
			t.Errorf("stage[%d]: want %q, got %q", i, want, brief.Stages[i].AgentName)
		}
	}
	if brief.Status != caseflow.CaseStatusComplete {
		t.Errorf("expected CaseStatusComplete, got %q", brief.Status)
	}
}

// --- Agent isolation ---------------------------------------------------------

func TestOrchestrator_AgentIsolation(t *testing.T) {
	const caseID = "CASE-SYN-001"
	const markerToken = "policy-obs"

	// PolicyExplainer: tool call that returns a marker in ToolResult.Output, then synthesis.
	// The noopTool returns the marker so it ends up in the orchestrator's observation buffer.
	// CaseInvestigator must NOT see "policy-obs" in any of its inputs.
	markerRegistry := fakeRegistry([]string{"list_applicable_rules"}, map[string]any{"marker": markerToken})

	policyProvider := &caseflowQueuedProvider{outputs: []harness.ModelOutput{
		toolOutput("list_applicable_rules", map[string]any{}),
		finalOutput(validPolicyExplanationJSON(caseID)),
	}}

	investigatorBase := &caseflowQueuedProvider{outputs: []harness.ModelOutput{
		finalOutput(validCaseInvestigationJSON(caseID)),
	}}
	recorder := &recordingProvider{delegate: investigatorBase}

	providers := map[string]*caseflowQueuedProvider{
		"PolicyExplainer":       policyProvider,
		"EvidencePackager":      {outputs: []harness.ModelOutput{finalOutput(validEvidenceManifestJSON(caseID))}},
		"SupervisorNoteDrafter": {outputs: []harness.ModelOutput{finalOutput(validSupervisorNoteJSON(caseID))}},
	}

	defs, _ := fourAgentDefs(providers)
	factory := func(name string) harness.ModelProvider {
		if name == "CaseInvestigator" {
			return recorder
		}
		return providers[name]
	}

	// Each agent gets a filtered registry; markerRegistry has list_applicable_rules only.
	// We use markerRegistry as the full registry so PolicyExplainer's tool succeeds.
	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, markerRegistry, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	_, err = o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for i, input := range recorder.inputs {
		if strings.Contains(input, markerToken) {
			t.Errorf("CaseInvestigator input[%d] leaks PolicyExplainer tool observation %q", i, markerToken)
		}
	}
}

// --- ToolResult json.Marshal in observations (constraint #4) -----------------

func TestOrchestrator_ToolObservationJSONMarshaled(t *testing.T) {
	const caseID = "CASE-SYN-001"
	// noopTool returns rule_code in ToolResult.Output; the orchestrator must json.Marshal it
	// into the <tool_observations> section of the next synthesis step's input.
	ruleRegistry := fakeRegistry([]string{"list_applicable_rules"}, map[string]any{"rule_code": "MX-REDECO-04"})

	policyBase := &caseflowQueuedProvider{outputs: []harness.ModelOutput{
		toolOutput("list_applicable_rules", map[string]any{}),
		finalOutput(validPolicyExplanationJSON(caseID)),
	}}
	recorder := &recordingProvider{delegate: policyBase}

	providers := map[string]*caseflowQueuedProvider{
		"CaseInvestigator":      {outputs: []harness.ModelOutput{finalOutput(validCaseInvestigationJSON(caseID))}},
		"EvidencePackager":      {outputs: []harness.ModelOutput{finalOutput(validEvidenceManifestJSON(caseID))}},
		"SupervisorNoteDrafter": {outputs: []harness.ModelOutput{finalOutput(validSupervisorNoteJSON(caseID))}},
	}

	defs, _ := fourAgentDefs(providers)
	factory := func(name string) harness.ModelProvider {
		if name == "PolicyExplainer" {
			return recorder
		}
		return providers[name]
	}

	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, ruleRegistry, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	_, err = o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The second call to recorder is the synthesis step; its input must contain
	// the JSON-marshaled tool observation with the rule_code key.
	found := false
	for _, input := range recorder.inputs {
		if strings.Contains(input, `"rule_code"`) && strings.Contains(input, "MX-REDECO-04") {
			if strings.Contains(input, "<tool_observations>") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("synthesis input should contain JSON-marshaled tool observation with rule_code; inputs: %v", recorder.inputs)
	}
}

// --- Retry with feedback — succeeds on second attempt -----------------------

func TestOrchestrator_RetrySucceeds(t *testing.T) {
	const caseID = "CASE-SYN-001"

	// First synthesis: invalid JSON → ValidationFailed.
	// Second synthesis: valid JSON → success.
	policyProvider := &caseflowQueuedProvider{outputs: []harness.ModelOutput{
		finalOutput("not-valid-json"),
		finalOutput(validPolicyExplanationJSON(caseID)),
	}}
	recorder := &recordingProvider{delegate: policyProvider}

	providers := map[string]*caseflowQueuedProvider{
		"CaseInvestigator":      {outputs: []harness.ModelOutput{finalOutput(validCaseInvestigationJSON(caseID))}},
		"EvidencePackager":      {outputs: []harness.ModelOutput{finalOutput(validEvidenceManifestJSON(caseID))}},
		"SupervisorNoteDrafter": {outputs: []harness.ModelOutput{finalOutput(validSupervisorNoteJSON(caseID))}},
	}

	defs, _ := fourAgentDefs(providers)
	// PolicyExplainer validator: reject "not-valid-json", pass valid JSON.
	defs[0].Validator = vFunc(func(out harness.ModelOutput) error {
		if out.ToolCall != nil {
			return nil
		}
		var p caseflow.PolicyExplanation
		if err := json.Unmarshal([]byte(out.FinalOutput), &p); err != nil {
			return err
		}
		return nil
	})

	factory := func(name string) harness.ModelProvider {
		if name == "PolicyExplainer" {
			return recorder
		}
		return providers[name]
	}

	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, harness.ToolRegistry{}, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	brief, err := o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Two synthesis calls.
	if policyProvider.calls != 2 {
		t.Errorf("expected 2 model calls (fail+retry), got %d", policyProvider.calls)
	}
	// Second input must contain feedback from the first failure.
	if len(recorder.inputs) < 2 {
		t.Fatalf("expected at least 2 recorded inputs, got %d", len(recorder.inputs))
	}
	secondInput := recorder.inputs[1]
	if !strings.Contains(secondInput, "<validation_feedback>") {
		t.Errorf("second synthesis input should contain <validation_feedback>, got: %s", secondInput)
	}
	if brief.Status != caseflow.CaseStatusComplete {
		t.Errorf("expected CaseStatusComplete after retry, got %q", brief.Status)
	}
}

// --- Retry fails both — no third attempt, agent stopped ---------------------

func TestOrchestrator_RetryFailsBoth(t *testing.T) {
	const caseID = "CASE-SYN-001"

	policyProvider := &caseflowQueuedProvider{outputs: []harness.ModelOutput{
		finalOutput("bad-json-first"),
		finalOutput("bad-json-second"),
	}}

	providers := map[string]*caseflowQueuedProvider{
		"CaseInvestigator":      {outputs: []harness.ModelOutput{}},
		"EvidencePackager":      {outputs: []harness.ModelOutput{}},
		"SupervisorNoteDrafter": {outputs: []harness.ModelOutput{}},
	}

	defs, _ := fourAgentDefs(providers)
	defs[0].Validator = vFunc(func(out harness.ModelOutput) error {
		if out.ToolCall != nil {
			return nil
		}
		var p caseflow.PolicyExplanation
		if err := json.Unmarshal([]byte(out.FinalOutput), &p); err != nil {
			return err
		}
		return nil
	})

	factory := func(name string) harness.ModelProvider {
		if name == "PolicyExplainer" {
			return policyProvider
		}
		return providers[name]
	}

	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, harness.ToolRegistry{}, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	brief, err := o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if policyProvider.calls != 2 {
		t.Errorf("expected exactly 2 model calls, got %d", policyProvider.calls)
	}
	if brief.Status != caseflow.CaseStatusIncomplete {
		t.Errorf("expected CaseStatusIncomplete, got %q", brief.Status)
	}
	if brief.FailedAgent != "PolicyExplainer" {
		t.Errorf("expected FailedAgent=PolicyExplainer, got %q", brief.FailedAgent)
	}
	if brief.FailureReason == "" {
		t.Error("FailureReason should not be empty")
	}
}

// --- Downstream stop ---------------------------------------------------------

func TestOrchestrator_DownstreamStop(t *testing.T) {
	const caseID = "CASE-SYN-001"

	policyProvider := &caseflowQueuedProvider{outputs: []harness.ModelOutput{
		finalOutput("bad-json-1"),
		finalOutput("bad-json-2"),
	}}

	downstream := map[string]*caseflowQueuedProvider{
		"CaseInvestigator":      {},
		"EvidencePackager":      {},
		"SupervisorNoteDrafter": {},
	}

	defs, _ := fourAgentDefs(downstream)
	defs[0].Validator = vFunc(func(out harness.ModelOutput) error {
		if out.ToolCall != nil {
			return nil
		}
		var p caseflow.PolicyExplanation
		if err := json.Unmarshal([]byte(out.FinalOutput), &p); err != nil {
			return err
		}
		return nil
	})

	factory := func(name string) harness.ModelProvider {
		if name == "PolicyExplainer" {
			return policyProvider
		}
		return downstream[name]
	}

	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, harness.ToolRegistry{}, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	brief, err := o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for name, p := range downstream {
		if p.calls != 0 {
			t.Errorf("downstream agent %q should not have been called, got %d calls", name, p.calls)
		}
	}
	if brief.FailedAgent != "PolicyExplainer" {
		t.Errorf("expected FailedAgent=PolicyExplainer, got %q", brief.FailedAgent)
	}
	if brief.Status != caseflow.CaseStatusIncomplete {
		t.Errorf("expected CaseStatusIncomplete, got %q", brief.Status)
	}
}

// --- Incomplete brief — partial stages --------------------------------------

func TestOrchestrator_IncompleteBriefPartialStages(t *testing.T) {
	const caseID = "CASE-SYN-001"

	investigatorProvider := &caseflowQueuedProvider{outputs: []harness.ModelOutput{
		finalOutput("bad-json-1"),
		finalOutput("bad-json-2"),
	}}

	providers := map[string]*caseflowQueuedProvider{
		"PolicyExplainer":       {outputs: []harness.ModelOutput{finalOutput(validPolicyExplanationJSON(caseID))}},
		"EvidencePackager":      {},
		"SupervisorNoteDrafter": {},
	}

	defs, _ := fourAgentDefs(providers)
	defs[1].Validator = vFunc(func(out harness.ModelOutput) error {
		if out.ToolCall != nil {
			return nil
		}
		var ci caseflow.CaseInvestigation
		if err := json.Unmarshal([]byte(out.FinalOutput), &ci); err != nil {
			return err
		}
		return nil
	})

	factory := func(name string) harness.ModelProvider {
		if name == "CaseInvestigator" {
			return investigatorProvider
		}
		return providers[name]
	}

	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, harness.ToolRegistry{}, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	brief, err := o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(brief.Stages) != 1 {
		t.Errorf("expected 1 stage (PolicyExplainer only), got %d", len(brief.Stages))
	}
	if brief.FailedAgent != "CaseInvestigator" {
		t.Errorf("expected FailedAgent=CaseInvestigator, got %q", brief.FailedAgent)
	}
}

// --- MaxSteps outer-loop cap -------------------------------------------------

func TestOrchestrator_MaxStepsCap(t *testing.T) {
	const caseID = "CASE-SYN-001"

	// Script 10 consecutive tool calls; MaxSteps=4 so only 4 should be consumed.
	toolCalls := make([]harness.ModelOutput, 10)
	for i := range toolCalls {
		toolCalls[i] = toolOutput("list_applicable_rules", map[string]any{})
	}
	policyProvider := &caseflowQueuedProvider{outputs: toolCalls}

	providers := map[string]*caseflowQueuedProvider{
		"CaseInvestigator":      {},
		"EvidencePackager":      {},
		"SupervisorNoteDrafter": {},
	}

	defs, _ := fourAgentDefs(providers)
	factory := func(name string) harness.ModelProvider {
		if name == "PolicyExplainer" {
			return policyProvider
		}
		return providers[name]
	}

	// Registry must contain the tool so RunStep returns StepStatusCompleted (tool found)
	// rather than StepStatusToolNotFound (which would fail the agent on step 1).
	reg := allToolsRegistry("list_applicable_rules")

	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, reg, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	brief, err := o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if policyProvider.calls > defs[0].MaxSteps {
		t.Errorf("provider was called %d times, exceeding MaxSteps=%d", policyProvider.calls, defs[0].MaxSteps)
	}
	if brief.Status != caseflow.CaseStatusIncomplete {
		t.Errorf("expected CaseStatusIncomplete after MaxSteps exceeded, got %q", brief.Status)
	}
}

// --- TRIANGULATE: FailureReason from SECOND failure, not first ---------------

func TestOrchestrator_FailureReasonFromSecondFailure(t *testing.T) {
	const caseID = "CASE-SYN-001"

	callCount2 := 0
	_ = callCount2
	policyProvider := &caseflowQueuedProvider{outputs: []harness.ModelOutput{
		finalOutput("bad-json-first"),
		finalOutput("bad-json-second"),
	}}

	// We need the validator to produce distinct error messages.
	// Use a custom validator that tracks call count.
	type countingValidator struct {
		count int
	}
	cv := &countingValidator{}
	customValidator := vFuncAdapter{fn: func(out harness.ModelOutput) error {
		if out.ToolCall != nil {
			return nil
		}
		cv.count++
		if cv.count == 1 {
			return &stringError{"first-fail"}
		}
		return &stringError{"second-fail"}
	}}

	providers := map[string]*caseflowQueuedProvider{
		"CaseInvestigator":      {},
		"EvidencePackager":      {},
		"SupervisorNoteDrafter": {},
	}

	defs, _ := fourAgentDefs(providers)
	defs[0].Validator = customValidator

	factory := func(name string) harness.ModelProvider {
		if name == "PolicyExplainer" {
			return policyProvider
		}
		return providers[name]
	}

	gate := gateAll(harness.PermissionAllowed)
	o, err := caseflow.NewOrchestrator(factory, harness.ToolRegistry{}, gate, defs)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	brief, err := o.Run(context.Background(), caseID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if strings.Contains(brief.FailureReason, "first-fail") {
		t.Errorf("FailureReason should NOT contain 'first-fail', got: %q", brief.FailureReason)
	}
	if !strings.Contains(brief.FailureReason, "second-fail") {
		t.Errorf("FailureReason should contain 'second-fail', got: %q", brief.FailureReason)
	}
}

// stringError is a simple error type with a custom message, used in triangulate test.
type stringError struct{ msg string }

func (e *stringError) Error() string { return e.msg }
