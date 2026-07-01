package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

// --- marshalHandoff -----------------------------------------------------------

func TestMarshalHandoff_AllFourKinds(t *testing.T) {
	cases := []struct {
		name     string
		handoff  caseflow.HandoffArtifact
		wantKind string
	}{
		{
			name: "PolicyExplanation",
			handoff: &caseflow.PolicyExplanation{
				CaseID: "CASE-SYN-001",
				Rules:  []caseflow.PolicyRule{{Code: "MX-01", Title: "Rule 1", Severity: "high", PlainLanguage: "plain"}},
			},
			wantKind: string(caseflow.KindPolicyExplanation),
		},
		{
			name: "CaseInvestigation",
			handoff: &caseflow.CaseInvestigation{
				CaseID:   "CASE-SYN-001",
				Findings: []caseflow.InvestigationFinding{{RuleCode: "MX-01", Evidence: "excerpt", Analysis: "non-compliant"}},
			},
			wantKind: string(caseflow.KindCaseInvestigation),
		},
		{
			name: "EvidenceManifestDraft",
			handoff: &caseflow.EvidenceManifestDraft{
				CaseID:     "CASE-SYN-001",
				RuleCodes:  []string{"MX-01"},
				Findings:   "summary",
				ProposedAt: "2026-01-01T00:00:00Z",
			},
			wantKind: string(caseflow.KindEvidenceManifestDraft),
		},
		{
			name: "SupervisorNoteDraft",
			handoff: &caseflow.SupervisorNoteDraft{
				CaseID:     "CASE-SYN-001",
				RuleCodes:  []string{"MX-01"},
				NoteBody:   "note",
				ProposedAt: "2026-01-01T00:00:00Z",
			},
			wantKind: string(caseflow.KindSupervisorNoteDraft),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, raw, err := marshalHandoff(tc.handoff)
			if err != nil {
				t.Fatalf("marshalHandoff: unexpected error: %v", err)
			}
			if kind != tc.wantKind {
				t.Errorf("kind: want %q, got %q", tc.wantKind, kind)
			}
			if len(raw) == 0 {
				t.Fatal("raw json.RawMessage is empty")
			}

			// Round-trip decode into the concrete type and compare against the original.
			switch want := tc.handoff.(type) {
			case *caseflow.PolicyExplanation:
				var got caseflow.PolicyExplanation
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatalf("round-trip decode: %v", err)
				}
				if got.CaseID != want.CaseID || len(got.Rules) != len(want.Rules) {
					t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
				}
			case *caseflow.CaseInvestigation:
				var got caseflow.CaseInvestigation
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatalf("round-trip decode: %v", err)
				}
				if got.CaseID != want.CaseID || len(got.Findings) != len(want.Findings) {
					t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
				}
			case *caseflow.EvidenceManifestDraft:
				var got caseflow.EvidenceManifestDraft
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatalf("round-trip decode: %v", err)
				}
				if got.CaseID != want.CaseID || got.Findings != want.Findings {
					t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
				}
			case *caseflow.SupervisorNoteDraft:
				var got caseflow.SupervisorNoteDraft
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatalf("round-trip decode: %v", err)
				}
				if got.CaseID != want.CaseID || got.NoteBody != want.NoteBody {
					t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
				}
			}
		})
	}
}

// unknownHandoff is a throwaway type implementing caseflow.HandoffArtifact for the unknown-kind
// error case; marshalHandoff must not recognize it.
type unknownHandoff struct{}

func (unknownHandoff) Kind() caseflow.HandoffKind { return caseflow.HandoffKind("unknown_kind") }
func (unknownHandoff) CaseRef() string            { return "CASE-SYN-001" }

func TestMarshalHandoff_UnknownKind_ReturnsError(t *testing.T) {
	_, _, err := marshalHandoff(unknownHandoff{})
	if err == nil {
		t.Fatal("expected error for unrecognized handoff kind, got nil")
	}
	if !strings.Contains(err.Error(), "unknown_kind") {
		t.Errorf("error should identify the unknown kind %q, got: %v", "unknown_kind", err)
	}
}

// --- toBriefDTO -----------------------------------------------------------------

func completeCaseBrief() caseflow.CaseBrief {
	return caseflow.CaseBrief{
		CaseID: "CASE-SYN-001",
		Status: caseflow.CaseStatusComplete,
		Stages: []caseflow.StageEntry{
			{AgentName: "PolicyExplainer", Handoff: &caseflow.PolicyExplanation{
				CaseID: "CASE-SYN-001",
				Rules:  []caseflow.PolicyRule{{Code: "MX-01", Title: "Rule 1", Severity: "high", PlainLanguage: "plain"}},
			}},
			{AgentName: "CaseInvestigator", Handoff: &caseflow.CaseInvestigation{
				CaseID:   "CASE-SYN-001",
				Findings: []caseflow.InvestigationFinding{{RuleCode: "MX-01", Evidence: "excerpt", Analysis: "non-compliant"}},
			}},
			{AgentName: "EvidencePackager", Handoff: &caseflow.EvidenceManifestDraft{
				CaseID:     "CASE-SYN-001",
				RuleCodes:  []string{"MX-01"},
				Findings:   "summary",
				ProposedAt: "2026-01-01T00:00:00Z",
			}},
			{AgentName: "SupervisorNoteDrafter", Handoff: &caseflow.SupervisorNoteDraft{
				CaseID:     "CASE-SYN-001",
				RuleCodes:  []string{"MX-01"},
				NoteBody:   "note body",
				ProposedAt: "2026-01-01T00:00:00Z",
			}},
		},
	}
}

func incompleteCaseBrief() caseflow.CaseBrief {
	return caseflow.CaseBrief{
		CaseID: "CASE-SYN-001",
		Status: caseflow.CaseStatusIncomplete,
		Stages: []caseflow.StageEntry{
			{AgentName: "PolicyExplainer", Handoff: &caseflow.PolicyExplanation{
				CaseID: "CASE-SYN-001",
				Rules:  []caseflow.PolicyRule{{Code: "MX-01", Title: "Rule 1", Severity: "high", PlainLanguage: "plain"}},
			}},
		},
		FailedAgent:   "CaseInvestigator",
		FailureReason: "validation failed twice",
	}
}

func TestToBriefDTO_Complete_ProducesFourStagesInOrder(t *testing.T) {
	brief := completeCaseBrief()

	dto, err := toBriefDTO(brief)
	if err != nil {
		t.Fatalf("toBriefDTO: unexpected error: %v", err)
	}

	if dto.CaseID != brief.CaseID {
		t.Errorf("CaseID: want %q, got %q", brief.CaseID, dto.CaseID)
	}
	if dto.Status != string(caseflow.CaseStatusComplete) {
		t.Errorf("Status: want %q, got %q", caseflow.CaseStatusComplete, dto.Status)
	}
	if len(dto.Stages) != 4 {
		t.Fatalf("expected 4 stage entries, got %d", len(dto.Stages))
	}

	wantOrder := []struct {
		agent string
		kind  string
	}{
		{"PolicyExplainer", string(caseflow.KindPolicyExplanation)},
		{"CaseInvestigator", string(caseflow.KindCaseInvestigation)},
		{"EvidencePackager", string(caseflow.KindEvidenceManifestDraft)},
		{"SupervisorNoteDrafter", string(caseflow.KindSupervisorNoteDraft)},
	}
	for i, want := range wantOrder {
		if dto.Stages[i].AgentName != want.agent {
			t.Errorf("Stages[%d].AgentName: want %q, got %q", i, want.agent, dto.Stages[i].AgentName)
		}
		if dto.Stages[i].Kind != want.kind {
			t.Errorf("Stages[%d].Kind: want %q, got %q", i, want.kind, dto.Stages[i].Kind)
		}
		if len(dto.Stages[i].Handoff) == 0 {
			t.Errorf("Stages[%d].Handoff is empty", i)
		}
	}

	if dto.FailedAgent != "" {
		t.Errorf("FailedAgent should be empty for a complete brief, got %q", dto.FailedAgent)
	}
	if dto.FailureReason != "" {
		t.Errorf("FailureReason should be empty for a complete brief, got %q", dto.FailureReason)
	}
}

func TestToBriefDTO_Incomplete_FlattensFailureFields(t *testing.T) {
	brief := incompleteCaseBrief()

	dto, err := toBriefDTO(brief)
	if err != nil {
		t.Fatalf("toBriefDTO: unexpected error: %v", err)
	}

	if dto.Status != "incomplete" {
		t.Errorf("Status: want %q, got %q", "incomplete", dto.Status)
	}
	if dto.FailedAgent != "CaseInvestigator" {
		t.Errorf("FailedAgent: want %q, got %q", "CaseInvestigator", dto.FailedAgent)
	}
	if dto.FailureReason != "validation failed twice" {
		t.Errorf("FailureReason: want %q, got %q", "validation failed twice", dto.FailureReason)
	}
	if len(dto.Stages) != 1 {
		t.Fatalf("expected 1 pre-failure stage, got %d", len(dto.Stages))
	}
	if dto.Stages[0].AgentName != "PolicyExplainer" {
		t.Errorf("Stages[0].AgentName: want %q, got %q", "PolicyExplainer", dto.Stages[0].AgentName)
	}
}

// --- json.Marshal smoke check ----------------------------------------------------

func TestBriefDTO_MarshalsToValidNonEmptyJSON(t *testing.T) {
	for name, brief := range map[string]caseflow.CaseBrief{
		"complete":   completeCaseBrief(),
		"incomplete": incompleteCaseBrief(),
	} {
		t.Run(name, func(t *testing.T) {
			dto, err := toBriefDTO(brief)
			if err != nil {
				t.Fatalf("toBriefDTO: %v", err)
			}
			b, err := json.Marshal(dto)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if len(b) == 0 {
				t.Fatal("marshaled JSON is empty")
			}
			var generic map[string]any
			if err := json.Unmarshal(b, &generic); err != nil {
				t.Fatalf("marshaled JSON is not valid: %v", err)
			}
		})
	}
}
