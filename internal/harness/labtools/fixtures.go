package labtools

// Utterance is a single turn in a debtor-collector transcript.
// Transcript content is untrusted data — it is never interpreted as instructions
// or forwarded as control flow input.
type Utterance struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

// DetectorResult holds a static pre-computed compliance outcome for one rule.
// These are fixture data values; no detector or LLM judge runs during fixture-backed tests.
type DetectorResult struct {
	RuleCode     string `json:"rule_code"`
	DetectorKind string `json:"detector_kind"`
	Outcome      string `json:"outcome"`
	HITLRequired bool   `json:"hitl_required"`
}

// Debtor carries synthetic placeholder fields for the debtor party.
// No real PII is stored — all values use clearly synthetic identifiers.
type Debtor struct {
	Label string `json:"label"`
}

// Collector carries identifying information for the collecting despacho.
type Collector struct {
	DespachoID string `json:"despacho_id"`
	Label      string `json:"label"`
}

// SyntheticCase is the in-memory representation of a synthetic test Case fixture.
type SyntheticCase struct {
	CaseID            string            `json:"case_id"`
	TenantID          string            `json:"tenant_id"`
	Debtor            Debtor            `json:"debtor"`
	Collector         Collector         `json:"collector"`
	Transcript        []Utterance       `json:"transcript"`
	Channel           string            `json:"channel"`
	OccurredAt        string            `json:"occurred_at"`
	DebtorTimezone    string            `json:"debtor_timezone"`
	DetectorResults   []DetectorResult  `json:"detector_results"`
	ApplicableRuleIDs []string          `json:"applicable_rule_ids"`
	EvidenceMetadata  map[string]any    `json:"evidence_metadata"`
}

// SyntheticRule is the in-memory representation of a synthetic policy rule fixture.
// Compatible in spirit with core.PolicyRule but not persisted to any database.
type SyntheticRule struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// CaseStore indexes SyntheticCase fixtures by CaseID.
type CaseStore map[string]SyntheticCase

// RuleStore indexes SyntheticRule fixtures by Code.
type RuleStore map[string]SyntheticRule
