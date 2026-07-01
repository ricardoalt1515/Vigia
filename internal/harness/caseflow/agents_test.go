package caseflow_test

import (
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

func TestAllAgentDefinitions_Count(t *testing.T) {
	defs := caseflow.AllAgentDefinitions()
	if len(defs) != 4 {
		t.Fatalf("AllAgentDefinitions: want 4 definitions, got %d", len(defs))
	}
}

func TestAllAgentDefinitions_Order(t *testing.T) {
	want := []string{"PolicyExplainer", "CaseInvestigator", "EvidencePackager", "SupervisorNoteDrafter"}
	defs := caseflow.AllAgentDefinitions()
	if len(defs) != len(want) {
		t.Fatalf("AllAgentDefinitions: want %d, got %d", len(want), len(defs))
	}
	for i, name := range want {
		if defs[i].Name != name {
			t.Errorf("AllAgentDefinitions()[%d].Name: want %q, got %q", i, name, defs[i].Name)
		}
	}
}

func TestAllAgentDefinitions_NoPersistentMutableState(t *testing.T) {
	first := caseflow.AllAgentDefinitions()
	second := caseflow.AllAgentDefinitions()
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Errorf("AllAgentDefinitions()[%d].Name inconsistent across calls: %q vs %q",
				i, first[i].Name, second[i].Name)
		}
		if first[i].MaxSteps != second[i].MaxSteps {
			t.Errorf("AllAgentDefinitions()[%d].MaxSteps inconsistent: %d vs %d",
				i, first[i].MaxSteps, second[i].MaxSteps)
		}
	}
}

// TestAgentDefinitions_StructuralProperties verifies the static shape of each AgentDefinition
// against the spec table: ToolAllowlist, MaxSteps, Budget, non-nil Validator and DecodeHandoff.
func TestAgentDefinitions_StructuralProperties(t *testing.T) {
	type row struct {
		name          string
		toolAllowlist []string
		maxSteps      int
	}
	table := []row{
		// PolicyExplainer has 2 tools (list_applicable_rules, read_policy_rule) + 2 margin = 4.
		{"PolicyExplainer", []string{"list_applicable_rules", "read_policy_rule"}, 4},
		// The remaining three agents each have 1 tool + 2 margin = 3.
		{"CaseInvestigator", []string{"read_case"}, 3},
		{"EvidencePackager", []string{"draft_evidence_manifest"}, 3},
		{"SupervisorNoteDrafter", []string{"draft_supervisor_note"}, 3},
	}

	defs := caseflow.AllAgentDefinitions()
	if len(defs) != len(table) {
		t.Fatalf("AllAgentDefinitions: want %d, got %d", len(table), len(defs))
	}

	for i, tt := range table {
		def := defs[i]
		t.Run(tt.name, func(t *testing.T) {
			if def.Name != tt.name {
				t.Errorf("Name: want %q, got %q", tt.name, def.Name)
			}
			if def.Instructions == "" {
				t.Error("Instructions: must be non-empty")
			}
			if def.Validator == nil {
				t.Error("Validator: must be non-nil")
			}
			if def.DecodeHandoff == nil {
				t.Error("DecodeHandoff: must be non-nil")
			}
			if def.Budget.MaxModelAttempts != 1 {
				t.Errorf("Budget.MaxModelAttempts: want 1, got %d", def.Budget.MaxModelAttempts)
			}
			if def.Budget.MaxToolCalls != 1 {
				t.Errorf("Budget.MaxToolCalls: want 1, got %d", def.Budget.MaxToolCalls)
			}
			if def.MaxSteps != tt.maxSteps {
				t.Errorf("MaxSteps: want %d, got %d", tt.maxSteps, def.MaxSteps)
			}
			if len(def.ToolAllowlist) != len(tt.toolAllowlist) {
				t.Fatalf("ToolAllowlist length: want %d, got %d",
					len(tt.toolAllowlist), len(def.ToolAllowlist))
			}
			for j, toolName := range tt.toolAllowlist {
				if def.ToolAllowlist[j] != toolName {
					t.Errorf("ToolAllowlist[%d]: want %q, got %q", j, toolName, def.ToolAllowlist[j])
				}
			}
		})
	}
}

// TestAgentDefinitions_DecodeHandoff_RoundTrip verifies that each agent's DecodeHandoff
// function accepts valid JSON and returns the expected HandoffArtifact with the correct CaseRef.
func TestAgentDefinitions_DecodeHandoff_RoundTrip(t *testing.T) {
	type row struct {
		agentName string
		jsonStr   string
		wantKind  string
	}
	table := []row{
		{
			agentName: "PolicyExplainer",
			jsonStr:   `{"case_id":"CASE-SYN-001","rules":[{"code":"MX-REDECO-04","title":"Calling Hours","severity":"high","plain_language":"Collectors must not contact debtors outside permitted hours."}]}`,
			wantKind:  "policy_explanation",
		},
		{
			agentName: "CaseInvestigator",
			jsonStr:   `{"case_id":"CASE-SYN-001","findings":[{"rule_code":"MX-REDECO-04","evidence":"Call at 21:00","analysis":"Outside permitted hours"}]}`,
			wantKind:  "case_investigation",
		},
		{
			agentName: "EvidencePackager",
			jsonStr:   `{"case_id":"CASE-SYN-001","rule_codes":["MX-REDECO-04"],"findings":"Evidence of calling outside hours","proposed_at":"2025-01-01T00:00:00Z","authoritative":false,"persisted":false}`,
			wantKind:  "evidence_manifest_draft",
		},
		{
			agentName: "SupervisorNoteDrafter",
			jsonStr:   `{"case_id":"CASE-SYN-001","rule_codes":["MX-REDECO-04"],"note_body":"Supervisor: calling hours violation detected","proposed_at":"2025-01-01T00:00:00Z","authoritative":false,"persisted":false}`,
			wantKind:  "supervisor_note_draft",
		},
	}

	defs := caseflow.AllAgentDefinitions()
	defByName := make(map[string]caseflow.AgentDefinition, len(defs))
	for _, d := range defs {
		defByName[d.Name] = d
	}

	for _, tt := range table {
		t.Run(tt.agentName, func(t *testing.T) {
			def, ok := defByName[tt.agentName]
			if !ok {
				t.Fatalf("agent %q not found in AllAgentDefinitions", tt.agentName)
			}
			artifact, err := def.DecodeHandoff(tt.jsonStr)
			if err != nil {
				t.Fatalf("DecodeHandoff(%q): unexpected error: %v", tt.agentName, err)
			}
			if artifact == nil {
				t.Fatal("DecodeHandoff: returned nil artifact")
			}
			if artifact.CaseRef() != "CASE-SYN-001" {
				t.Errorf("CaseRef: want %q, got %q", "CASE-SYN-001", artifact.CaseRef())
			}
			if string(artifact.Kind()) != tt.wantKind {
				t.Errorf("Kind: want %q, got %q", tt.wantKind, string(artifact.Kind()))
			}
		})
	}
}

// TestAgentDefinitions_DecodeHandoff_MalformedJSON verifies that malformed JSON returns a non-nil error.
func TestAgentDefinitions_DecodeHandoff_MalformedJSON(t *testing.T) {
	defs := caseflow.AllAgentDefinitions()
	for _, def := range defs {
		t.Run(def.Name, func(t *testing.T) {
			_, err := def.DecodeHandoff("{broken")
			if err == nil {
				t.Errorf("DecodeHandoff({broken}): want non-nil error for malformed JSON, got nil")
			}
		})
	}
}

// TestNewOrchestrator_WithAllAgentDefinitions verifies the construction guard passes
// when all definitions have MaxModelAttempts == 1 (triangulate WU4).
func TestNewOrchestrator_WithAllAgentDefinitions(t *testing.T) {
	reg := allToolsRegistry(
		"list_applicable_rules", "read_policy_rule",
		"read_case", "draft_evidence_manifest", "draft_supervisor_note",
	)
	gate := gateAll(harness.PermissionAllowed)
	factory := func(_ string) harness.ModelProvider {
		return &caseflowQueuedProvider{}
	}
	_, err := caseflow.NewOrchestrator(factory, reg, gate, caseflow.AllAgentDefinitions())
	if err != nil {
		t.Errorf("NewOrchestrator(AllAgentDefinitions()): unexpected error: %v", err)
	}
}
