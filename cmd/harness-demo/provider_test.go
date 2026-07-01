package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

// agentToolAllowlist mirrors caseflow.AllAgentDefinitions()'s per-agent tool allowlist so the
// test can assert the scripted tool call matches the real allowlist without importing test-only
// caseflow internals.
func agentToolAllowlist(t *testing.T, agentName string) string {
	t.Helper()
	for _, def := range caseflow.AllAgentDefinitions() {
		if def.Name == agentName {
			if len(def.ToolAllowlist) == 0 {
				t.Fatalf("agent %q has empty tool allowlist", agentName)
			}
			return def.ToolAllowlist[0]
		}
	}
	t.Fatalf("no AgentDefinition found for %q", agentName)
	return ""
}

func decodeHandoffFor(t *testing.T, agentName string) func(string) (caseflow.HandoffArtifact, error) {
	t.Helper()
	for _, def := range caseflow.AllAgentDefinitions() {
		if def.Name == agentName {
			return def.DecodeHandoff
		}
	}
	t.Fatalf("no AgentDefinition found for %q", agentName)
	return nil
}

func TestDemoProviderFactory_ScriptsToolCallThenSynthesisPerAgent(t *testing.T) {
	agentNames := []string{"PolicyExplainer", "CaseInvestigator", "EvidencePackager", "SupervisorNoteDrafter"}

	for _, name := range agentNames {
		t.Run(name, func(t *testing.T) {
			provider := demoProviderFactory(name)
			if provider == nil {
				t.Fatal("demoProviderFactory returned nil")
			}

			wantTool := agentToolAllowlist(t, name)
			decode := decodeHandoffFor(t, name)

			// Step 1: tool-call step.
			out1, err := provider.Generate(context.Background(), harness.ModelRequest{})
			if err != nil {
				t.Fatalf("Generate (tool-call step): unexpected error: %v", err)
			}
			if out1.ToolCall == nil {
				t.Fatal("expected a ToolCall in the first scripted step, got nil")
			}
			if out1.ToolCall.Name != wantTool {
				t.Errorf("ToolCall.Name: want %q, got %q", wantTool, out1.ToolCall.Name)
			}

			// Step 2: synthesis step.
			out2, err := provider.Generate(context.Background(), harness.ModelRequest{})
			if err != nil {
				t.Fatalf("Generate (synthesis step): unexpected error: %v", err)
			}
			if out2.FinalOutput == "" {
				t.Fatal("expected non-empty FinalOutput in the second scripted step")
			}

			// The synthesis output must be syntactically valid JSON.
			var generic map[string]any
			if err := json.Unmarshal([]byte(out2.FinalOutput), &generic); err != nil {
				t.Fatalf("FinalOutput is not valid JSON: %v", err)
			}

			// The synthesis output must decode into this agent's handoff shape and carry
			// CASE-SYN-001 as case_id.
			handoff, err := decode(out2.FinalOutput)
			if err != nil {
				t.Fatalf("DecodeHandoff: unexpected error: %v", err)
			}
			if handoff.CaseRef() != "CASE-SYN-001" {
				t.Errorf("CaseRef(): want %q, got %q", "CASE-SYN-001", handoff.CaseRef())
			}
		})
	}
}

func TestDemoProviderFactory_UnknownAgent_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected demoProviderFactory to panic on an unscripted agent name")
		}
	}()
	demoProviderFactory("UnknownAgent")
}
