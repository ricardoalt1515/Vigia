package labtools

import "encoding/json"

// draftProposedAt is the fixed deterministic RFC 3339 timestamp used by all draft tools.
// It MUST NOT be time.Now() and MUST NOT be omitted. This constant guarantees determinism.
const draftProposedAt = "2025-01-01T00:00:00Z"

// --- Read tool DTOs ---

// ReadCaseRequest is the typed input for the read_case tool.
type ReadCaseRequest struct {
	CaseID string `json:"case_id"`
}

// SyntheticCaseView is the typed response payload for read_case.
type SyntheticCaseView struct {
	TenantID          string         `json:"tenant_id"`
	Debtor            Debtor         `json:"debtor"`
	Collector         Collector      `json:"collector"`
	Transcript        []Utterance    `json:"transcript"`
	Channel           string         `json:"channel"`
	OccurredAt        string         `json:"occurred_at"`
	DebtorTimezone    string         `json:"debtor_timezone"`
	DetectorResults   []DetectorResult `json:"detector_results"`
	ApplicableRuleIDs []string       `json:"applicable_rule_ids"`
	EvidenceMetadata  map[string]any `json:"evidence_metadata"`
}

// ReadCaseResponse is the typed output for read_case.
// CaseID echoes the request's case_id per the typed response schema requirement.
type ReadCaseResponse struct {
	CaseID string            `json:"case_id"`
	Case   SyntheticCaseView `json:"case"`
}

// ReadPolicyRuleRequest is the typed input for read_policy_rule.
type ReadPolicyRuleRequest struct {
	RuleCode string `json:"rule_code"`
}

// SyntheticRuleView is the typed read view for a policy rule.
type SyntheticRuleView struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// ReadPolicyRuleResponse is the typed output for read_policy_rule.
type ReadPolicyRuleResponse struct {
	Rule SyntheticRuleView `json:"rule"`
}

// ListApplicableRulesRequest is the typed input for list_applicable_rules.
type ListApplicableRulesRequest struct {
	CaseID string `json:"case_id"`
}

// RuleSummary is a lightweight rule descriptor used in list responses.
type RuleSummary struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
}

// ListApplicableRulesResponse is the typed output for list_applicable_rules.
type ListApplicableRulesResponse struct {
	Rules []RuleSummary `json:"rules"`
}

// --- Draft tool DTOs ---

// DraftEvidenceManifestRequest is the typed input for draft_evidence_manifest.
type DraftEvidenceManifestRequest struct {
	CaseID    string   `json:"case_id"`
	RuleCodes []string `json:"rule_codes"`
	Findings  string   `json:"findings"`
}

// DraftEvidenceManifestResponse is the typed output for draft_evidence_manifest.
// authoritative is always false; persisted is always false; proposed_at is the fixed constant.
type DraftEvidenceManifestResponse struct {
	CaseID       string   `json:"case_id"`
	RuleCodes    []string `json:"rule_codes"`
	Findings     string   `json:"findings"`
	ProposedAt   string   `json:"proposed_at"`
	Authoritative bool    `json:"authoritative"`
	Persisted    bool     `json:"persisted"`
}

// DraftSupervisorNoteRequest is the typed input for draft_supervisor_note.
type DraftSupervisorNoteRequest struct {
	CaseID    string   `json:"case_id"`
	RuleCodes []string `json:"rule_codes"`
	NoteBody  string   `json:"note_body"`
}

// DraftSupervisorNoteResponse is the typed output for draft_supervisor_note.
// authoritative is always false; persisted is always false; proposed_at is the fixed constant.
type DraftSupervisorNoteResponse struct {
	CaseID       string   `json:"case_id"`
	RuleCodes    []string `json:"rule_codes"`
	NoteBody     string   `json:"note_body"`
	ProposedAt   string   `json:"proposed_at"`
	Authoritative bool    `json:"authoritative"`
	Persisted    bool     `json:"persisted"`
}

// --- JSON round-trip codec ---

// decode decodes a map[string]any into a typed struct via JSON round-trip.
// No new dependencies — stdlib encoding/json only.
func decode[T any](m map[string]any) (T, error) {
	var zero T
	b, err := json.Marshal(m)
	if err != nil {
		return zero, err
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return zero, err
	}
	return v, nil
}

// encode encodes any value to map[string]any via JSON round-trip.
// No new dependencies — stdlib encoding/json only.
func encode(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
