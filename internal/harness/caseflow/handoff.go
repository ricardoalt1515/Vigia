package caseflow

// HandoffKind identifies the type of handoff artifact produced by a domain agent.
type HandoffKind string

const (
	KindPolicyExplanation     HandoffKind = "policy_explanation"
	KindCaseInvestigation     HandoffKind = "case_investigation"
	KindEvidenceManifestDraft HandoffKind = "evidence_manifest_draft"
	KindSupervisorNoteDraft   HandoffKind = "supervisor_note_draft"
)

// HandoffArtifact is implemented by every typed struct a domain agent produces.
type HandoffArtifact interface {
	Kind() HandoffKind
	CaseRef() string
}

// CaseStatus is the terminal state of a case orchestration run.
type CaseStatus string

const (
	CaseStatusComplete   CaseStatus = "complete"
	CaseStatusIncomplete CaseStatus = "incomplete"
)

// PolicyRule describes one applicable regulation with compliance-relevant metadata.
type PolicyRule struct {
	Code          string `json:"code"`
	Title         string `json:"title"`
	Severity      string `json:"severity"`
	PlainLanguage string `json:"plain_language"`
}

// PolicyExplanation is the handoff artifact produced by the PolicyExplainer agent.
type PolicyExplanation struct {
	CaseID string       `json:"case_id"`
	Rules  []PolicyRule `json:"rules"`
}

func (p *PolicyExplanation) Kind() HandoffKind { return KindPolicyExplanation }
func (p *PolicyExplanation) CaseRef() string   { return p.CaseID }

// InvestigationFinding records one compliance finding against a specific rule.
type InvestigationFinding struct {
	RuleCode string `json:"rule_code"`
	Evidence string `json:"evidence"`
	Analysis string `json:"analysis"`
}

// CaseInvestigation is the handoff artifact produced by the CaseInvestigator agent.
type CaseInvestigation struct {
	CaseID   string                 `json:"case_id"`
	Findings []InvestigationFinding `json:"findings"`
}

func (c *CaseInvestigation) Kind() HandoffKind { return KindCaseInvestigation }
func (c *CaseInvestigation) CaseRef() string   { return c.CaseID }

// EvidenceManifestDraft is the handoff artifact produced by the EvidencePackager agent.
// Authoritative and Persisted must never be true in agent output; the validator rejects them.
type EvidenceManifestDraft struct {
	CaseID        string   `json:"case_id"`
	RuleCodes     []string `json:"rule_codes"`
	Findings      string   `json:"findings"`
	ProposedAt    string   `json:"proposed_at"`
	Authoritative bool     `json:"authoritative"`
	Persisted     bool     `json:"persisted"`
}

func (e *EvidenceManifestDraft) Kind() HandoffKind { return KindEvidenceManifestDraft }
func (e *EvidenceManifestDraft) CaseRef() string   { return e.CaseID }

// SupervisorNoteDraft is the handoff artifact produced by the SupervisorNoteDrafter agent.
// Authoritative and Persisted must never be true in agent output; the validator rejects them.
type SupervisorNoteDraft struct {
	CaseID        string   `json:"case_id"`
	RuleCodes     []string `json:"rule_codes"`
	NoteBody      string   `json:"note_body"`
	ProposedAt    string   `json:"proposed_at"`
	Authoritative bool     `json:"authoritative"`
	Persisted     bool     `json:"persisted"`
}

func (s *SupervisorNoteDraft) Kind() HandoffKind { return KindSupervisorNoteDraft }
func (s *SupervisorNoteDraft) CaseRef() string   { return s.CaseID }

// StageEntry records one completed agent stage in an orchestration run.
type StageEntry struct {
	AgentName string
	Handoff   HandoffArtifact
}

// CaseBrief is the terminal summary of one case orchestration run.
type CaseBrief struct {
	CaseID        string
	Status        CaseStatus
	Stages        []StageEntry
	FailedAgent   string
	FailureReason string
}
