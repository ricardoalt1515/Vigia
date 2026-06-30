package caseflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

// marshalOutput marshals v to JSON and returns a ModelOutput with FinalOutput set.
func marshalOutput(t *testing.T, v any) harness.ModelOutput {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalOutput: json.Marshal: %v", err)
	}
	return harness.ModelOutput{FinalOutput: string(data)}
}

// validPolicyExplanation returns a fully populated PolicyExplanation with no denylist terms.
func validPolicyExplanation() caseflow.PolicyExplanation {
	return caseflow.PolicyExplanation{
		CaseID: "CASE-SYN-001",
		Rules: []caseflow.PolicyRule{
			{Code: "MX-01", Title: "Rule 1", Severity: "high", PlainLanguage: "Debt collector must identify themselves."},
		},
	}
}

// validCaseInvestigation returns a fully populated CaseInvestigation with no denylist terms.
func validCaseInvestigation() caseflow.CaseInvestigation {
	return caseflow.CaseInvestigation{
		CaseID: "CASE-SYN-001",
		Findings: []caseflow.InvestigationFinding{
			{RuleCode: "MX-01", Evidence: "Call recording shows no identification.", Analysis: "Violation confirmed."},
		},
	}
}

// validEvidenceManifestDraft returns a fully populated EvidenceManifestDraft with no denylist terms.
func validEvidenceManifestDraft() caseflow.EvidenceManifestDraft {
	return caseflow.EvidenceManifestDraft{
		CaseID:        "CASE-SYN-001",
		RuleCodes:     []string{"MX-01"},
		Findings:      "Evidence of failure to identify collected.",
		ProposedAt:    "2024-01-01T00:00:00Z",
		Authoritative: false,
		Persisted:     false,
	}
}

// validSupervisorNoteDraft returns a fully populated SupervisorNoteDraft with no denylist terms.
func validSupervisorNoteDraft() caseflow.SupervisorNoteDraft {
	return caseflow.SupervisorNoteDraft{
		CaseID:        "CASE-SYN-001",
		RuleCodes:     []string{"MX-01"},
		NoteBody:      "Collector failed to identify themselves during the call.",
		ProposedAt:    "2024-01-01T00:00:00Z",
		Authoritative: false,
		Persisted:     false,
	}
}

// --- Schema completeness ---

func TestValidatorSchemaCompleteness(t *testing.T) {
	tests := []struct {
		name            string
		out             harness.ModelOutput
		validator       func(harness.ModelOutput) error
		wantErrContains string
	}{
		{
			name:            "PolicyExplanation empty CaseID",
			out:             marshalOutput(t, caseflow.PolicyExplanation{Rules: validPolicyExplanation().Rules}),
			validator:       caseflow.ValidatePolicyExplanation,
			wantErrContains: "case",
		},
		{
			name:            "PolicyExplanation empty Rules",
			out:             marshalOutput(t, caseflow.PolicyExplanation{CaseID: "CASE-SYN-001"}),
			validator:       caseflow.ValidatePolicyExplanation,
			wantErrContains: "rules",
		},
		{
			name:            "CaseInvestigation empty CaseID",
			out:             marshalOutput(t, caseflow.CaseInvestigation{Findings: validCaseInvestigation().Findings}),
			validator:       caseflow.ValidateCaseInvestigation,
			wantErrContains: "case",
		},
		{
			name:            "CaseInvestigation empty Findings",
			out:             marshalOutput(t, caseflow.CaseInvestigation{CaseID: "CASE-SYN-001"}),
			validator:       caseflow.ValidateCaseInvestigation,
			wantErrContains: "findings",
		},
		{
			name:            "EvidenceManifestDraft empty CaseID",
			out:             marshalOutput(t, caseflow.EvidenceManifestDraft{Findings: "some findings"}),
			validator:       caseflow.ValidateEvidenceManifestDraft,
			wantErrContains: "case",
		},
		{
			name:            "EvidenceManifestDraft empty Findings",
			out:             marshalOutput(t, caseflow.EvidenceManifestDraft{CaseID: "CASE-SYN-001"}),
			validator:       caseflow.ValidateEvidenceManifestDraft,
			wantErrContains: "findings",
		},
		{
			name:            "SupervisorNoteDraft empty CaseID",
			out:             marshalOutput(t, caseflow.SupervisorNoteDraft{NoteBody: "some note"}),
			validator:       caseflow.ValidateSupervisorNoteDraft,
			wantErrContains: "case",
		},
		{
			name:            "SupervisorNoteDraft empty NoteBody",
			out:             marshalOutput(t, caseflow.SupervisorNoteDraft{CaseID: "CASE-SYN-001"}),
			validator:       caseflow.ValidateSupervisorNoteDraft,
			wantErrContains: "note",
		},
		{
			name:            "undecodable JSON for PolicyExplanation",
			out:             harness.ModelOutput{FinalOutput: "not-json"},
			validator:       caseflow.ValidatePolicyExplanation,
			wantErrContains: "parse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validator(tt.out)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantErrContains) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}

// --- Typed-field checks ---

func TestValidatorTypedFieldChecks(t *testing.T) {
	tests := []struct {
		name            string
		out             harness.ModelOutput
		validator       func(harness.ModelOutput) error
		wantErrContains string
	}{
		{
			name: "EvidenceManifestDraft Authoritative true",
			out: marshalOutput(t, caseflow.EvidenceManifestDraft{
				CaseID: "CASE-SYN-001", Findings: "some findings", Authoritative: true,
			}),
			validator:       caseflow.ValidateEvidenceManifestDraft,
			wantErrContains: "authoritative",
		},
		{
			name: "EvidenceManifestDraft Persisted true",
			out: marshalOutput(t, caseflow.EvidenceManifestDraft{
				CaseID: "CASE-SYN-001", Findings: "some findings", Persisted: true,
			}),
			validator:       caseflow.ValidateEvidenceManifestDraft,
			wantErrContains: "persisted",
		},
		{
			name: "SupervisorNoteDraft Authoritative true",
			out: marshalOutput(t, caseflow.SupervisorNoteDraft{
				CaseID: "CASE-SYN-001", NoteBody: "some note", Authoritative: true,
			}),
			validator:       caseflow.ValidateSupervisorNoteDraft,
			wantErrContains: "authoritative",
		},
		{
			name: "SupervisorNoteDraft Persisted true",
			out: marshalOutput(t, caseflow.SupervisorNoteDraft{
				CaseID: "CASE-SYN-001", NoteBody: "some note", Persisted: true,
			}),
			validator:       caseflow.ValidateSupervisorNoteDraft,
			wantErrContains: "persisted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validator(tt.out)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantErrContains) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}

// --- Denylist checks ---

func TestValidatorDenylist(t *testing.T) {
	tests := []struct {
		name            string
		out             harness.ModelOutput
		validator       func(harness.ModelOutput) error
		wantErrContains string
	}{
		{
			name: "approved in PolicyExplanation.Rules[0].PlainLanguage",
			out: marshalOutput(t, caseflow.PolicyExplanation{
				CaseID: "CASE-SYN-001",
				Rules:  []caseflow.PolicyRule{{Code: "MX-01", Title: "T", Severity: "low", PlainLanguage: "The debt is approved."}},
			}),
			validator:       caseflow.ValidatePolicyExplanation,
			wantErrContains: "approved",
		},
		{
			name: "approval_granted in PolicyExplanation.Rules[0].PlainLanguage",
			out: marshalOutput(t, caseflow.PolicyExplanation{
				CaseID: "CASE-SYN-001",
				Rules:  []caseflow.PolicyRule{{Code: "MX-01", Title: "T", Severity: "low", PlainLanguage: "approval_granted for this case."}},
			}),
			validator:       caseflow.ValidatePolicyExplanation,
			wantErrContains: "approval_granted",
		},
		{
			name: "block_campaign in CaseInvestigation.Findings[0].Evidence",
			out: marshalOutput(t, caseflow.CaseInvestigation{
				CaseID:   "CASE-SYN-001",
				Findings: []caseflow.InvestigationFinding{{RuleCode: "MX-01", Evidence: "block_campaign was observed.", Analysis: "analysis"}},
			}),
			validator:       caseflow.ValidateCaseInvestigation,
			wantErrContains: "block_campaign",
		},
		{
			name: "campaign_blocked in CaseInvestigation.Findings[0].Analysis",
			out: marshalOutput(t, caseflow.CaseInvestigation{
				CaseID:   "CASE-SYN-001",
				Findings: []caseflow.InvestigationFinding{{RuleCode: "MX-01", Evidence: "evidence", Analysis: "campaign_blocked outcome."}},
			}),
			validator:       caseflow.ValidateCaseInvestigation,
			wantErrContains: "campaign_blocked",
		},
		{
			name: "override_to_compliant in EvidenceManifestDraft.Findings",
			out: marshalOutput(t, caseflow.EvidenceManifestDraft{
				CaseID:   "CASE-SYN-001",
				Findings: "override_to_compliant based on review.",
			}),
			validator:       caseflow.ValidateEvidenceManifestDraft,
			wantErrContains: "override_to_compliant",
		},
		{
			name: "ledger_committed in SupervisorNoteDraft.NoteBody",
			out: marshalOutput(t, caseflow.SupervisorNoteDraft{
				CaseID:   "CASE-SYN-001",
				NoteBody: "ledger_committed for this account.",
			}),
			validator:       caseflow.ValidateSupervisorNoteDraft,
			wantErrContains: "ledger_committed",
		},
		{
			name: "case-insensitive BLOCK_CAMPAIGN in CaseInvestigation Evidence",
			out: marshalOutput(t, caseflow.CaseInvestigation{
				CaseID:   "CASE-SYN-001",
				Findings: []caseflow.InvestigationFinding{{RuleCode: "MX-01", Evidence: "BLOCK_CAMPAIGN noted.", Analysis: "analysis"}},
			}),
			validator:       caseflow.ValidateCaseInvestigation,
			wantErrContains: "block_campaign",
		},
		{
			name: "substring match block_campaign in CaseInvestigation Evidence",
			out: marshalOutput(t, caseflow.CaseInvestigation{
				CaseID:   "CASE-SYN-001",
				Findings: []caseflow.InvestigationFinding{{RuleCode: "MX-01", Evidence: "The block_campaign action was not taken.", Analysis: "analysis"}},
			}),
			validator:       caseflow.ValidateCaseInvestigation,
			wantErrContains: "block_campaign",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validator(tt.out)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}

// --- Valid artifacts ---

func TestValidatorValidArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		out       harness.ModelOutput
		validator func(harness.ModelOutput) error
	}{
		{
			name:      "valid PolicyExplanation",
			out:       marshalOutput(t, validPolicyExplanation()),
			validator: caseflow.ValidatePolicyExplanation,
		},
		{
			name:      "valid CaseInvestigation",
			out:       marshalOutput(t, validCaseInvestigation()),
			validator: caseflow.ValidateCaseInvestigation,
		},
		{
			name:      "valid EvidenceManifestDraft",
			out:       marshalOutput(t, validEvidenceManifestDraft()),
			validator: caseflow.ValidateEvidenceManifestDraft,
		},
		{
			name:      "valid SupervisorNoteDraft",
			out:       marshalOutput(t, validSupervisorNoteDraft()),
			validator: caseflow.ValidateSupervisorNoteDraft,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.validator(tt.out); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// --- Tool-call pass-through ---

func TestValidatorToolCallPassThrough(t *testing.T) {
	out := harness.ModelOutput{ToolCall: &harness.ToolCall{Name: "read_case", Input: map[string]any{}}}
	validators := []struct {
		name      string
		validator func(harness.ModelOutput) error
	}{
		{"PolicyExplanation", caseflow.ValidatePolicyExplanation},
		{"CaseInvestigation", caseflow.ValidateCaseInvestigation},
		{"EvidenceManifestDraft", caseflow.ValidateEvidenceManifestDraft},
		{"SupervisorNoteDraft", caseflow.ValidateSupervisorNoteDraft},
	}
	for _, tt := range validators {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.validator(out); err != nil {
				t.Errorf("tool-call pass-through: unexpected error: %v", err)
			}
		})
	}
}

// --- Structurally invalid output ---

func TestValidatorStructurallyInvalidOutput(t *testing.T) {
	out := harness.ModelOutput{} // no ToolCall, no FinalOutput
	validators := []struct {
		name      string
		validator func(harness.ModelOutput) error
	}{
		{"PolicyExplanation", caseflow.ValidatePolicyExplanation},
		{"CaseInvestigation", caseflow.ValidateCaseInvestigation},
		{"EvidenceManifestDraft", caseflow.ValidateEvidenceManifestDraft},
		{"SupervisorNoteDraft", caseflow.ValidateSupervisorNoteDraft},
	}
	for _, tt := range validators {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.validator(out); err == nil {
				t.Error("expected error for empty ModelOutput, got nil")
			}
		})
	}
}
