package caseflow_test

import (
	"encoding/json"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

func TestHandoffKindConstants(t *testing.T) {
	kinds := []caseflow.HandoffKind{
		caseflow.KindPolicyExplanation,
		caseflow.KindCaseInvestigation,
		caseflow.KindEvidenceManifestDraft,
		caseflow.KindSupervisorNoteDraft,
	}
	seen := map[caseflow.HandoffKind]bool{}
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("duplicate HandoffKind value: %q", k)
		}
		seen[k] = true
	}
}

func TestCaseStatusConstants(t *testing.T) {
	if caseflow.CaseStatusComplete == caseflow.CaseStatusIncomplete {
		t.Error("CaseStatusComplete and CaseStatusIncomplete must be distinct")
	}
	if caseflow.CaseStatusComplete != "complete" {
		t.Errorf("CaseStatusComplete = %q, want %q", caseflow.CaseStatusComplete, "complete")
	}
	if caseflow.CaseStatusIncomplete != "incomplete" {
		t.Errorf("CaseStatusIncomplete = %q, want %q", caseflow.CaseStatusIncomplete, "incomplete")
	}
}

// TestHandoffArtifactInterface is a compiler-asserted check that each struct satisfies HandoffArtifact.
func TestHandoffArtifactInterface(t *testing.T) {
	var _ caseflow.HandoffArtifact = &caseflow.PolicyExplanation{}
	var _ caseflow.HandoffArtifact = &caseflow.CaseInvestigation{}
	var _ caseflow.HandoffArtifact = &caseflow.EvidenceManifestDraft{}
	var _ caseflow.HandoffArtifact = &caseflow.SupervisorNoteDraft{}
}

func TestHandoffCaseRef(t *testing.T) {
	const caseID = "CASE-SYN-001"
	tests := []struct {
		name     string
		artifact caseflow.HandoffArtifact
	}{
		{"PolicyExplanation", &caseflow.PolicyExplanation{CaseID: caseID}},
		{"CaseInvestigation", &caseflow.CaseInvestigation{CaseID: caseID}},
		{"EvidenceManifestDraft", &caseflow.EvidenceManifestDraft{CaseID: caseID}},
		{"SupervisorNoteDraft", &caseflow.SupervisorNoteDraft{CaseID: caseID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.artifact.CaseRef(); got != caseID {
				t.Errorf("CaseRef() = %q, want %q", got, caseID)
			}
		})
	}
}

func TestHandoffKind(t *testing.T) {
	tests := []struct {
		name     string
		artifact caseflow.HandoffArtifact
		want     caseflow.HandoffKind
	}{
		{"PolicyExplanation", &caseflow.PolicyExplanation{}, caseflow.KindPolicyExplanation},
		{"CaseInvestigation", &caseflow.CaseInvestigation{}, caseflow.KindCaseInvestigation},
		{"EvidenceManifestDraft", &caseflow.EvidenceManifestDraft{}, caseflow.KindEvidenceManifestDraft},
		{"SupervisorNoteDraft", &caseflow.SupervisorNoteDraft{}, caseflow.KindSupervisorNoteDraft},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.artifact.Kind(); got != tt.want {
				t.Errorf("Kind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandoffJSONRoundTrip(t *testing.T) {
	const caseID = "CASE-SYN-001"
	tests := []struct {
		name     string
		artifact any
	}{
		{"PolicyExplanation", &caseflow.PolicyExplanation{
			CaseID: caseID,
			Rules: []caseflow.PolicyRule{
				{Code: "MX-01", Title: "Rule 1", Severity: "high", PlainLanguage: "plain text"},
			},
		}},
		{"CaseInvestigation", &caseflow.CaseInvestigation{
			CaseID: caseID,
			Findings: []caseflow.InvestigationFinding{
				{RuleCode: "MX-01", Evidence: "evidence", Analysis: "analysis"},
			},
		}},
		{"EvidenceManifestDraft", &caseflow.EvidenceManifestDraft{
			CaseID:        caseID,
			RuleCodes:     []string{"MX-01"},
			Findings:      "some findings",
			ProposedAt:    "2024-01-01T00:00:00Z",
			Authoritative: false,
			Persisted:     false,
		}},
		{"SupervisorNoteDraft", &caseflow.SupervisorNoteDraft{
			CaseID:        caseID,
			RuleCodes:     []string{"MX-01"},
			NoteBody:      "note body",
			ProposedAt:    "2024-01-01T00:00:00Z",
			Authoritative: false,
			Persisted:     false,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.artifact)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			gotCaseID, _ := m["case_id"].(string)
			if gotCaseID != caseID {
				t.Errorf("round-trip case_id = %q, want %q", gotCaseID, caseID)
			}
		})
	}
}

func TestCaseBriefConstruction(t *testing.T) {
	brief := caseflow.CaseBrief{
		CaseID: "CASE-SYN-001",
		Status: caseflow.CaseStatusComplete,
		Stages: []caseflow.StageEntry{
			{AgentName: "PolicyExplainer", Handoff: &caseflow.PolicyExplanation{CaseID: "CASE-SYN-001"}},
			{AgentName: "CaseInvestigator", Handoff: &caseflow.CaseInvestigation{CaseID: "CASE-SYN-001"}},
		},
		FailedAgent:   "",
		FailureReason: "",
	}
	if brief.Status != caseflow.CaseStatusComplete {
		t.Errorf("Status = %q, want %q", brief.Status, caseflow.CaseStatusComplete)
	}
	if len(brief.Stages) != 2 {
		t.Errorf("len(Stages) = %d, want 2", len(brief.Stages))
	}
	if brief.FailedAgent != "" {
		t.Errorf("FailedAgent = %q, want empty", brief.FailedAgent)
	}
	if brief.FailureReason != "" {
		t.Errorf("FailureReason = %q, want empty", brief.FailureReason)
	}
}

func TestPolicyRuleFields(t *testing.T) {
	rule := caseflow.PolicyRule{
		Code:          "MX-01",
		Title:         "title",
		Severity:      "high",
		PlainLanguage: "plain",
	}
	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, key := range []string{"code", "title", "severity", "plain_language"} {
		if _, ok := m[key]; !ok {
			t.Errorf("PolicyRule JSON missing key %q", key)
		}
	}
}

func TestInvestigationFindingFields(t *testing.T) {
	f := caseflow.InvestigationFinding{
		RuleCode: "MX-01",
		Evidence: "evidence",
		Analysis: "analysis",
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, key := range []string{"rule_code", "evidence", "analysis"} {
		if _, ok := m[key]; !ok {
			t.Errorf("InvestigationFinding JSON missing key %q", key)
		}
	}
}
