// Package outbound evaluates authority-bearing outbound action proposals before send.
package outbound

import (
	"errors"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
)

type ActionKind string

const (
	ActionDraftOutboundUtterance ActionKind = "draft_outbound_utterance"
	ActionSendOutboundUtterance  ActionKind = "send_outbound_utterance"
)

type DecisionMode string

const (
	DecisionModeEnforcement DecisionMode = "enforcement"
	DecisionModeDryRun      DecisionMode = "dry_run"
)

type DecisionOutcome string

const (
	DecisionAllow            DecisionOutcome = "allow"
	DecisionDeny             DecisionOutcome = "deny"
	DecisionApprovalRequired DecisionOutcome = "approval_required"
)

var ErrPolicyBundleNotFound = errors.New("active policy bundle not found")

type OutboundActionProposal struct {
	Kind                     ActionKind
	ProposalID               string
	CaseID                   string
	CampaignID               string
	StepID                   string
	TemplateID               string
	DebtorID                 string
	Channel                  core.InteractionChannel
	RecipientRef             string
	Text                     string
	ProposedAt               time.Time
	DebtorTimezone           string
	ContactPartyRelationship string
	AuthorizedChannels       []string
	PaymentTarget            string
}

type DecisionRequest struct {
	TenantID  string
	ActorID   string
	Mode      DecisionMode
	Proposal  OutboundActionProposal
	RequestID string
}

type Decision struct {
	ID                  string
	Mode                DecisionMode
	ActionKind          ActionKind
	ProposalID          string
	Outcome             DecisionOutcome
	Reason              string
	Violations          []RuleViolation
	FailClosedReasons   []ContextGap
	PolicyBundleID      string
	PolicyBundleVersion string
	CheckedAt           time.Time
	EventRefs           []DecisionRef
	EvidenceRefs        []DecisionRef
	DraftSuggestion     *DraftSuggestion
}

type DecisionRef struct {
	Type string       `json:"type"`
	ID   string       `json:"id"`
	Mode DecisionMode `json:"mode"`
}

type RecordedDecision struct {
	EventRefs    []DecisionRef
	EvidenceRefs []DecisionRef
}

func (d Decision) Metadata() map[string]any {
	metadata := map[string]any{
		"decision_id":      d.ID,
		"decision_mode":    string(d.Mode),
		"decision_outcome": string(d.Outcome),
	}
	if d.ActionKind != "" {
		metadata["action_kind"] = string(d.ActionKind)
	}
	if d.ProposalID != "" {
		metadata["proposal_id"] = d.ProposalID
	}
	if d.PolicyBundleID != "" {
		metadata["policy_bundle_id"] = d.PolicyBundleID
	}
	if d.PolicyBundleVersion != "" {
		metadata["policy_bundle_version"] = d.PolicyBundleVersion
	}
	if len(d.FailClosedReasons) > 0 {
		codes := make([]string, 0, len(d.FailClosedReasons))
		for _, gap := range d.FailClosedReasons {
			codes = append(codes, gap.Code)
		}
		metadata["fail_closed_codes"] = codes
	}
	if len(d.Violations) > 0 {
		codes := make([]string, 0, len(d.Violations))
		for _, violation := range d.Violations {
			codes = append(codes, violation.RuleCode)
		}
		metadata["rule_codes"] = codes
		metadata["violated_rule_codes"] = codes
	}
	if len(d.EventRefs) > 0 {
		metadata["event_refs"] = decisionRefsMetadata(d.EventRefs)
	}
	if len(d.EvidenceRefs) > 0 {
		metadata["evidence_refs"] = decisionRefsMetadata(d.EvidenceRefs)
	}
	return metadata
}

func decisionRefsMetadata(refs []DecisionRef) []map[string]string {
	metadata := make([]map[string]string, 0, len(refs))
	for _, ref := range refs {
		metadata = append(metadata, map[string]string{
			"type": ref.Type,
			"id":   ref.ID,
			"mode": string(ref.Mode),
		})
	}
	return metadata
}

type RuleViolation struct {
	RuleCode    string
	Severity    core.Severity
	Rationale   string
	Remediation string
	ComponentID string
}

type ContextGap struct {
	Code        string `json:"code"`
	Field       string `json:"field"`
	Remediation string `json:"remediation"`
}

type DraftSuggestion struct {
	Text      string `json:"text"`
	DraftOnly bool   `json:"draft_only"`
	BasedOn   string `json:"based_on"`
}

type ResolvedPolicyBundle struct {
	ID      string
	Version string
}

type AuthorityContext struct {
	TenantID                 string
	DebtorID                 string
	DebtorTimezone           string
	Channel                  core.InteractionChannel
	ProposedAt               time.Time
	RecipientAmbiguous       bool
	ChannelAmbiguous         bool
	ContactPartyRelationship string
	AuthorizedChannels       []string
	PaymentRecipient         string
}
