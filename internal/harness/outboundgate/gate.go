// Package outboundgate adapts outbound guardrail decisions to the Harness permission gate seam.
package outboundgate

import (
	"context"
	"fmt"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/outbound"
)

const sendOutboundUtteranceTool = "send_outbound_utterance"

type Decider interface {
	Decide(ctx context.Context, req outbound.DecisionRequest) (outbound.Decision, error)
}

type Config struct {
	TenantID  string
	ActorID   string
	Decider   Decider
	Recorder  outbound.DecisionRecorder
	Fallback  harness.PermissionGate
	RequestID string
}

type Gate struct {
	config Config
}

func NewGate(config Config) Gate {
	return Gate{config: config}
}

func (g Gate) Decide(ctx context.Context, call harness.ToolCall) harness.PermissionDecision {
	if call.Name != sendOutboundUtteranceTool {
		if g.config.Fallback == nil {
			return harness.PermissionDecision{Kind: harness.PermissionDenied, Reason: "permission gate fallback is not configured"}
		}
		return g.config.Fallback.Decide(ctx, call)
	}

	proposal, err := decodeSendProposal(call.Input)
	if err != nil {
		proposal := partialSendProposal(call.Input)
		decision := outbound.Decision{
			ID:         proposal.ProposalID,
			Mode:       outbound.DecisionModeEnforcement,
			ActionKind: outbound.ActionSendOutboundUtterance,
			ProposalID: proposal.ProposalID,
			Outcome:    outbound.DecisionDeny,
			Reason:     "send_outbound_utterance input schema is invalid",
			FailClosedReasons: []outbound.ContextGap{{
				Code:        "invalid_send_outbound_schema",
				Field:       "tool_input",
				Remediation: err.Error(),
			}},
		}
		return g.mapAndRecordSchemaDenial(ctx, proposal, decision)
	}
	if g.config.Decider == nil {
		return mapDecision(outbound.Decision{
			ID:         proposal.ProposalID,
			Mode:       outbound.DecisionModeEnforcement,
			ActionKind: proposal.Kind,
			ProposalID: proposal.ProposalID,
			Outcome:    outbound.DecisionDeny,
			Reason:     "outbound decider is not configured",
			FailClosedReasons: []outbound.ContextGap{{
				Code:        "outbound_decider_unconfigured",
				Field:       "outbound_decider",
				Remediation: "Configure the outbound decider before registering authority-bearing send tools.",
			}},
		})
	}

	decision, err := g.config.Decider.Decide(ctx, outbound.DecisionRequest{
		TenantID:  g.config.TenantID,
		ActorID:   g.config.ActorID,
		Mode:      outbound.DecisionModeEnforcement,
		Proposal:  proposal,
		RequestID: g.config.RequestID,
	})
	if err != nil {
		return mapDecision(outbound.Decision{
			ID:         proposal.ProposalID,
			Mode:       outbound.DecisionModeEnforcement,
			ActionKind: proposal.Kind,
			ProposalID: proposal.ProposalID,
			Outcome:    outbound.DecisionDeny,
			Reason:     "outbound decision failed closed",
			FailClosedReasons: []outbound.ContextGap{{
				Code:        "outbound_decider_error",
				Field:       "outbound_decider",
				Remediation: "Resolve the outbound decider failure before sending.",
			}},
		})
	}
	return mapDecision(decision)
}

func decodeSendProposal(input map[string]any) (outbound.OutboundActionProposal, error) {
	proposalID := stringValue(input, "proposal_id")
	caseID := stringValue(input, "case_id")
	debtorID := stringValue(input, "debtor_id")
	channel := stringValue(input, "channel")
	recipientRef := stringValue(input, "recipient_ref")
	text := stringValue(input, "text")
	proposedAtRaw := stringValue(input, "proposed_at")
	if proposalID == "" || caseID == "" || debtorID == "" || channel == "" || recipientRef == "" || text == "" || proposedAtRaw == "" {
		return outbound.OutboundActionProposal{}, fmt.Errorf("provide proposal_id, case_id, debtor_id, channel, recipient_ref, text, and proposed_at")
	}
	if !validChannel(channel) {
		return outbound.OutboundActionProposal{}, fmt.Errorf("provide channel as one of call, message, or email")
	}
	proposedAt, err := time.Parse(time.RFC3339, proposedAtRaw)
	if err != nil {
		return outbound.OutboundActionProposal{}, fmt.Errorf("provide proposed_at as RFC3339: %w", err)
	}
	return outbound.OutboundActionProposal{
		Kind:          outbound.ActionSendOutboundUtterance,
		ProposalID:    proposalID,
		CaseID:        caseID,
		DebtorID:      debtorID,
		Channel:       core.InteractionChannel(channel),
		RecipientRef:  recipientRef,
		Text:          text,
		ProposedAt:    proposedAt,
		PaymentTarget: stringValue(input, "payment_target"),
	}, nil
}

func partialSendProposal(input map[string]any) outbound.OutboundActionProposal {
	proposedAt, _ := time.Parse(time.RFC3339, stringValue(input, "proposed_at"))
	return outbound.OutboundActionProposal{
		Kind:          outbound.ActionSendOutboundUtterance,
		ProposalID:    stringValue(input, "proposal_id"),
		CaseID:        stringValue(input, "case_id"),
		DebtorID:      stringValue(input, "debtor_id"),
		Channel:       core.InteractionChannel(stringValue(input, "channel")),
		RecipientRef:  stringValue(input, "recipient_ref"),
		Text:          stringValue(input, "text"),
		ProposedAt:    proposedAt,
		PaymentTarget: stringValue(input, "payment_target"),
	}
}

func stringValue(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

func validChannel(channel string) bool {
	switch core.InteractionChannel(channel) {
	case core.InteractionChannelCall, core.InteractionChannelMessage, core.InteractionChannelEmail:
		return true
	default:
		return false
	}
}

func (g Gate) mapAndRecordSchemaDenial(ctx context.Context, proposal outbound.OutboundActionProposal, decision outbound.Decision) harness.PermissionDecision {
	if g.config.Recorder == nil {
		decision.FailClosedReasons = append(decision.FailClosedReasons, outbound.ContextGap{Code: "decision_recorder_unconfigured", Field: "evidence_ledger", Remediation: "Configure the enforcement evidence recorder before accepting realtime outbound send proposals."})
		return mapDecision(decision)
	}
	if g.config.TenantID == "" || proposal.DebtorID == "" || proposal.ProposedAt.IsZero() {
		decision.FailClosedReasons = append(decision.FailClosedReasons, outbound.ContextGap{Code: "decision_recording_unavailable", Field: "tool_input", Remediation: "Include tenant scope, debtor_id, and proposed_at so invalid realtime outbound denials can be written to the evidence ledger."})
		return mapDecision(decision)
	}
	recorded, err := g.config.Recorder.Record(ctx, outbound.DecisionRequest{TenantID: g.config.TenantID, ActorID: g.config.ActorID, Mode: outbound.DecisionModeEnforcement, Proposal: proposal, RequestID: decision.ID}, decision)
	if err != nil || len(recorded.EventRefs) == 0 || len(recorded.EvidenceRefs) == 0 {
		decision.FailClosedReasons = append(decision.FailClosedReasons, outbound.ContextGap{Code: "decision_recording_failed", Field: "evidence_ledger", Remediation: "Retry after the enforcement evidence recorder is available; do not send until the blocked decision is auditable."})
		return mapDecision(decision)
	}
	decision.EventRefs = recorded.EventRefs
	decision.EvidenceRefs = recorded.EvidenceRefs
	return mapDecision(decision)
}

func mapDecision(decision outbound.Decision) harness.PermissionDecision {
	kind := harness.PermissionDenied
	switch decision.Outcome {
	case outbound.DecisionAllow:
		kind = harness.PermissionAllowed
	case outbound.DecisionApprovalRequired:
		kind = harness.PermissionApprovalRequired
	}
	return harness.PermissionDecision{Kind: kind, Reason: decision.Reason, Metadata: decision.Metadata()}
}
