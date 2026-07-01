package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/bedrock"
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

// --- selectProviderFactory (Slice 2 / #22) ------------------------------------------------------

func TestSelectProviderFactory_FakeOrEmpty_ReturnsDemoProviderFactoryUnchanged(t *testing.T) {
	for _, provider := range []string{"", "fake"} {
		t.Run(provider, func(t *testing.T) {
			factory, err := selectProviderFactory(context.Background(), provider)
			if err != nil {
				t.Fatalf("selectProviderFactory(%q) error = %v, want nil", provider, err)
			}
			gotPtr := reflect.ValueOf(factory).Pointer()
			wantPtr := reflect.ValueOf(caseflow.ProviderFactory(demoProviderFactory)).Pointer()
			if gotPtr != wantPtr {
				t.Errorf("selectProviderFactory(%q) did not return demoProviderFactory unchanged", provider)
			}
		})
	}
}

func TestSelectProviderFactory_Bedrock_DelegatesToBedrockNewFactory(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("BEDROCK_MODEL_ID", "anthropic.claude-3-sonnet")

	restore := newBedrockFactory
	defer func() { newBedrockFactory = restore }()

	called := false
	var gotOpts bedrock.Options
	newBedrockFactory = func(_ context.Context, opts bedrock.Options, _ ...bedrock.Option) (caseflow.ProviderFactory, error) {
		called = true
		gotOpts = opts
		return demoProviderFactory, nil
	}

	factory, err := selectProviderFactory(context.Background(), "bedrock")
	if err != nil {
		t.Fatalf("selectProviderFactory(bedrock) error = %v, want nil", err)
	}
	if !called {
		t.Fatal("expected newBedrockFactory to be invoked, was not")
	}
	if gotOpts.Region != "us-east-1" || gotOpts.ModelID != "anthropic.claude-3-sonnet" {
		t.Errorf("gotOpts = %+v, want Region=us-east-1 ModelID=anthropic.claude-3-sonnet", gotOpts)
	}
	if factory == nil {
		t.Fatal("factory is nil, want the fake factory returned by newBedrockFactory")
	}
}

func TestSelectProviderFactory_Bedrock_MissingEnv_ReturnsErrMissingConfig(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		modelID string
	}{
		{name: "missing AWS_REGION", region: "", modelID: "anthropic.claude-3-sonnet"},
		{name: "missing BEDROCK_MODEL_ID", region: "us-east-1", modelID: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWS_REGION", tc.region)
			t.Setenv("BEDROCK_MODEL_ID", tc.modelID)

			_, err := selectProviderFactory(context.Background(), "bedrock")
			if !errors.Is(err, bedrock.ErrMissingConfig) {
				t.Fatalf("err = %v, want errors.Is match for bedrock.ErrMissingConfig", err)
			}
		})
	}
}

func TestSelectProviderFactory_UnknownValue_ReturnsUsageError(t *testing.T) {
	_, err := selectProviderFactory(context.Background(), "something-else")
	if err == nil {
		t.Fatal("expected an error for an unknown --provider value")
	}
}
