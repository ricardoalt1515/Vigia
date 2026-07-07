package outbound

import (
	"context"
	"errors"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/judge"
)

type AuthorityContextResolver interface {
	Resolve(ctx context.Context, tenantID string, p OutboundActionProposal) (AuthorityContext, error)
}

type ActiveBundleResolver interface {
	ResolveActiveBundle(ctx context.Context, tenantID string) (ResolvedPolicyBundle, error)
}

type DecisionRecorder interface {
	Record(ctx context.Context, req DecisionRequest, decision Decision) (RecordedDecision, error)
}

type Decider struct {
	ContextResolver AuthorityContextResolver
	BundleResolver  ActiveBundleResolver
	ContactWindow   detection.Window
	ToneJudge       judge.Judge
	ToneRubric      judge.Rubric
	Recorder        DecisionRecorder
}

func (d Decider) Decide(ctx context.Context, req DecisionRequest) (Decision, error) {
	decisionID := decisionID(req)
	withContext := func(decision Decision) Decision {
		decision.ActionKind = req.Proposal.Kind
		decision.ProposalID = req.Proposal.ProposalID
		return decision
	}
	finalize := func(decision Decision) Decision {
		decision = withContext(decision)
		if req.Mode != DecisionModeEnforcement || (decision.Outcome != DecisionDeny && decision.Outcome != DecisionApprovalRequired) || d.Recorder == nil {
			return decision
		}
		recorded, err := d.Recorder.Record(ctx, req, decision)
		if err != nil {
			decision.FailClosedReasons = append(decision.FailClosedReasons, ContextGap{
				Code:        "decision_recording_failed",
				Field:       "evidence_ledger",
				Remediation: "Retry after the enforcement evidence recorder is available; do not send until the blocked decision is auditable.",
			})
			return decision
		}
		decision.EventRefs = append(decision.EventRefs, recorded.EventRefs...)
		decision.EvidenceRefs = append(decision.EvidenceRefs, recorded.EvidenceRefs...)
		return decision
	}
	checkedAt := req.Proposal.ProposedAt
	if checkedAt.IsZero() {
		return finalize(denyContextGap(decisionID, req.Mode, time.Time{}, ContextGap{
			Code:        "missing_proposed_at",
			Field:       "proposed_at",
			Remediation: "Provide the proposed send time before evaluating authority-bearing outbound sends.",
		})), nil
	}

	if d.ContextResolver == nil {
		return finalize(denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "authority_context_unresolved",
			Field:       "authority_context",
			Remediation: "Configure an authority context resolver before evaluating outbound sends.",
		})), nil
	}
	authorityContext, err := d.ContextResolver.Resolve(ctx, req.TenantID, req.Proposal)
	if err != nil {
		return finalize(denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "authority_context_unresolved",
			Field:       "authority_context",
			Remediation: "Resolve tenant-scoped debtor, recipient, channel, and audit context before sending.",
		})), nil
	}

	if d.BundleResolver == nil {
		return finalize(denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "policy_bundle_unresolved",
			Field:       "policy_bundle",
			Remediation: "Configure an active policy bundle resolver before evaluating outbound sends.",
		})), nil
	}
	bundle, err := d.BundleResolver.ResolveActiveBundle(ctx, req.TenantID)
	if err != nil || bundle.Version == "" {
		code := "policy_bundle_unresolved"
		if errors.Is(err, ErrPolicyBundleNotFound) || bundle.Version == "" {
			code = "unknown_policy_bundle"
		}
		return finalize(denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        code,
			Field:       "policy_bundle",
			Remediation: "Activate a tenant policy bundle before evaluating authority-bearing outbound sends.",
		})), nil
	}

	if authorityContext.RecipientAmbiguous {
		decision := denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "ambiguous_recipient",
			Field:       "recipient_ref",
			Remediation: "Resolve the outbound proposal to exactly one tenant-scoped recipient before sending.",
		})
		decision.PolicyBundleID = bundle.ID
		decision.PolicyBundleVersion = bundle.Version
		return finalize(decision), nil
	}
	if authorityContext.ChannelAmbiguous {
		decision := denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "ambiguous_channel",
			Field:       "channel",
			Remediation: "Resolve the outbound proposal to exactly one authorized channel before sending.",
		})
		decision.PolicyBundleID = bundle.ID
		decision.PolicyBundleVersion = bundle.Version
		return finalize(decision), nil
	}

	if authorityContext.DebtorTimezone == "" {
		decision := denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "missing_debtor_timezone",
			Field:       "debtor_timezone",
			Remediation: "Add a debtor-local IANA timezone before evaluating contact hours.",
		})
		decision.PolicyBundleID = bundle.ID
		decision.PolicyBundleVersion = bundle.Version
		return finalize(decision), nil
	}
	if _, err := time.LoadLocation(authorityContext.DebtorTimezone); err != nil {
		decision := denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "invalid_debtor_timezone",
			Field:       "debtor_timezone",
			Remediation: "Use a resolvable IANA debtor timezone before evaluating contact hours.",
		})
		decision.PolicyBundleID = bundle.ID
		decision.PolicyBundleVersion = bundle.Version
		return finalize(decision), nil
	}

	occurredAt := authorityContext.ProposedAt
	if occurredAt.IsZero() {
		occurredAt = req.Proposal.ProposedAt
	}
	interaction := detection.Interaction{
		OccurredAt:               occurredAt,
		DebtorTimezone:           authorityContext.DebtorTimezone,
		Channel:                  authorityContext.Channel,
		ContactPartyRelationship: authorityContext.ContactPartyRelationship,
		AuthorizedChannels:       authorityContext.AuthorizedChannels,
		PaymentRecipient:         authorityContext.PaymentRecipient,
	}
	if decision, blocked := d.blockingDetectorDecision(decisionID, req.Mode, checkedAt, bundle, detection.ContactHoursDetector{Window: d.contactWindow()}.Evaluate(interaction), "MX-REDECO-02", "contact hours rule blocked the outbound proposal", "Schedule the outbound contact inside the debtor-local permitted contact window."); blocked {
		return finalize(decision), nil
	}
	if decision, blocked := d.blockingDetectorDecision(decisionID, req.Mode, checkedAt, bundle, detection.ThirdPartyContactDetector{}.Evaluate(interaction), "MX-REDECO-06", "third-party contact rule blocked the outbound proposal", "Contact only the debtor or a verified authorized third party."); blocked {
		return finalize(decision), nil
	}
	if decision, blocked := d.blockingDetectorDecision(decisionID, req.Mode, checkedAt, bundle, detection.AuthorizedChannelDetector{}.Evaluate(interaction), "MX-REDECO-11", "authorized channel rule blocked the outbound proposal", "Use a debtor-authorized outbound channel or update the verified channel authorization."); blocked {
		return finalize(decision), nil
	}
	if decision, blocked := d.blockingDetectorDecision(decisionID, req.Mode, checkedAt, bundle, detection.PaymentRoutingDetector{}.Evaluate(interaction), "MX-REDECO-10", "payment routing rule blocked the outbound proposal", "Route payment instructions only to the creditor-approved payment recipient."); blocked {
		return finalize(decision), nil
	}

	if d.ToneJudge == nil {
		decision := denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "judge_unavailable",
			Field:       "tone_judge",
			Remediation: "Configure the semantic tone/threat judge before allowing authority-bearing outbound sends.",
		})
		decision.PolicyBundleID = bundle.ID
		decision.PolicyBundleVersion = bundle.Version
		return finalize(decision), nil
	}
	judgeResult, err := d.ToneJudge.Evaluate(ctx, judge.JudgeInput{
		Utterances: []judge.Utterance{{Speaker: "agent", Text: req.Proposal.Text}},
		Rubric:     d.ToneRubric,
	})
	if err != nil {
		decision := denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "judge_unavailable",
			Field:       "tone_judge",
			Remediation: "Retry after the semantic tone/threat judge is available; do not send until compliance can be proven.",
		})
		decision.PolicyBundleID = bundle.ID
		decision.PolicyBundleVersion = bundle.Version
		return finalize(decision), nil
	}
	if judgeResult.Confidence < 0.5 {
		decision := denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "judge_inconclusive",
			Field:       "tone_judge",
			Remediation: "Obtain a conclusive semantic tone/threat result before sending.",
		})
		decision.PolicyBundleID = bundle.ID
		decision.PolicyBundleVersion = bundle.Version
		return finalize(decision), nil
	}
	switch judgeResult.Outcome {
	case judge.OutcomeBlock:
		return finalize(Decision{
			ID:                  decisionID,
			Mode:                req.Mode,
			Outcome:             DecisionDeny,
			Reason:              "semantic tone/threat rule blocked the outbound proposal",
			PolicyBundleID:      bundle.ID,
			PolicyBundleVersion: bundle.Version,
			CheckedAt:           checkedAt,
			Violations: []RuleViolation{{
				RuleCode:    "MX-REDECO-05",
				Severity:    core.SeverityHigh,
				Rationale:   judgeResult.Rationale,
				Remediation: "Rewrite the message to remove threatening, abusive, coercive, or misleading language.",
			}},
			DraftSuggestion: &DraftSuggestion{
				Text:      "Please revise this message into a compliant, non-threatening draft before any new send decision.",
				DraftOnly: true,
				BasedOn:   req.Proposal.ProposalID,
			},
		}), nil
	case judge.OutcomePass:
		// Continue to allow below.
	default:
		decision := denyContextGap(decisionID, req.Mode, checkedAt, ContextGap{
			Code:        "judge_invalid_result",
			Field:       "tone_judge",
			Remediation: "Obtain a valid semantic tone/threat judge result before sending.",
		})
		decision.PolicyBundleID = bundle.ID
		decision.PolicyBundleVersion = bundle.Version
		return finalize(decision), nil
	}

	return finalize(Decision{
		ID:                  decisionID,
		Mode:                req.Mode,
		Outcome:             DecisionAllow,
		Reason:              "outbound proposal passed deterministic guardrails",
		PolicyBundleID:      bundle.ID,
		PolicyBundleVersion: bundle.Version,
		CheckedAt:           checkedAt,
	}), nil
}

func (d Decider) blockingDetectorDecision(id string, mode DecisionMode, checkedAt time.Time, bundle ResolvedPolicyBundle, result detection.Result, ruleCode, reason, remediation string) (Decision, bool) {
	if result.Outcome != detection.OutcomeBlock {
		return Decision{}, false
	}
	return Decision{
		ID:                  id,
		Mode:                mode,
		Outcome:             DecisionDeny,
		Reason:              reason,
		PolicyBundleID:      bundle.ID,
		PolicyBundleVersion: bundle.Version,
		CheckedAt:           checkedAt,
		Violations: []RuleViolation{{
			RuleCode:    ruleCode,
			Severity:    core.SeverityHigh,
			Rationale:   result.Rationale,
			Remediation: remediation,
		}},
	}, true
}

func decisionID(req DecisionRequest) string {
	if req.RequestID != "" {
		return req.RequestID
	}
	return req.Proposal.ProposalID
}

func denyContextGap(id string, mode DecisionMode, checkedAt time.Time, gap ContextGap) Decision {
	return Decision{
		ID:                id,
		Mode:              mode,
		Outcome:           DecisionDeny,
		Reason:            "required authority context could not prove compliance",
		CheckedAt:         checkedAt,
		FailClosedReasons: []ContextGap{gap},
	}
}

func (d Decider) contactWindow() detection.Window {
	if d.ContactWindow.StartHour == 0 && d.ContactWindow.EndHour == 0 {
		return detection.Window{StartHour: 8, EndHour: 21}
	}
	return d.ContactWindow
}
