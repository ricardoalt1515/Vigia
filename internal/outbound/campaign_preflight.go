package outbound

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
)

type DecisionEngine interface {
	Decide(ctx context.Context, req DecisionRequest) (Decision, error)
}

type PreflightStatus string

const (
	PreflightStatusPassed PreflightStatus = "passed"
	PreflightStatusFailed PreflightStatus = "failed"
)

type CampaignArtifact struct {
	CampaignID string
	Name       string
	TenantID   string
	ActorID    string
	Audience   []CampaignRecipient
	Steps      []CampaignStep
	Schedule   CampaignSchedule
}

type CampaignRecipient struct {
	RecipientRef string
	DebtorID     string
	Relationship string
	ChannelRefs  []string
	Timezone     string
}

type CampaignStep struct {
	StepID        string
	TemplateID    string
	Channel       core.InteractionChannel
	TextTemplate  string
	SendOffset    time.Duration
	PaymentTarget string
}

type CampaignSchedule struct {
	StartsAt time.Time
	Timezone string
}

type PreflightBrief struct {
	CampaignID          string             `json:"campaign_id"`
	Status              PreflightStatus    `json:"status"`
	PolicyBundleVersion string             `json:"policy_bundle_version"`
	Summary             string             `json:"summary"`
	Findings            []PreflightFinding `json:"findings"`
	ContextGaps         []ContextGap       `json:"context_gaps"`
	EventRefs           []DecisionRef      `json:"event_refs"`
	EvidenceRefs        []DecisionRef      `json:"evidence_refs"`
}

type PreflightFinding struct {
	CampaignID          string           `json:"campaign_id"`
	StepID              string           `json:"step_id"`
	TemplateID          string           `json:"template_id"`
	RecipientRef        string           `json:"recipient_ref"`
	RuleCode            string           `json:"rule_code"`
	Rationale           string           `json:"rationale"`
	Remediation         string           `json:"remediation"`
	PolicyBundleVersion string           `json:"policy_bundle_version"`
	ContextGaps         []ContextGap     `json:"context_gaps"`
	DryRunRefs          []DecisionRef    `json:"dry_run_refs"`
	EvidenceRefs        []DecisionRef    `json:"evidence_refs"`
	DraftSuggestion     *DraftSuggestion `json:"draft_suggestion,omitempty"`
}

type PreflightService struct {
	Decider DecisionEngine
}

func (s PreflightService) Run(ctx context.Context, campaign CampaignArtifact) (PreflightBrief, error) {
	brief := PreflightBrief{CampaignID: campaign.CampaignID, Status: PreflightStatusPassed}
	if gaps := validateCampaign(campaign); len(gaps) > 0 {
		brief.Status = PreflightStatusFailed
		brief.ContextGaps = gaps
		brief.Summary = "campaign preflight failed because required campaign context is missing"
		return brief, nil
	}
	if s.Decider == nil {
		brief.Status = PreflightStatusFailed
		brief.ContextGaps = []ContextGap{{Code: "preflight_decider_unavailable", Field: "decider", Remediation: "Configure the outbound decider before running campaign preflight."}}
		brief.Summary = "campaign preflight failed because the outbound decider is unavailable"
		return brief, nil
	}

	for _, recipient := range campaign.Audience {
		for _, step := range campaign.Steps {
			proposal := expandCampaignProposal(campaign, recipient, step)
			req := DecisionRequest{
				TenantID:  campaign.TenantID,
				ActorID:   campaign.ActorID,
				Mode:      DecisionModeDryRun,
				Proposal:  proposal,
				RequestID: dryRunDecisionID(campaign.CampaignID, recipient.RecipientRef, step.StepID),
			}
			decision, err := s.Decider.Decide(ctx, req)
			if err != nil {
				brief.Status = PreflightStatusFailed
				brief.ContextGaps = append(brief.ContextGaps, ContextGap{Code: "preflight_decision_failed", Field: "decider", Remediation: "Retry after the outbound decider can evaluate all campaign steps."})
				continue
			}
			if decision.PolicyBundleVersion != "" && brief.PolicyBundleVersion == "" {
				brief.PolicyBundleVersion = decision.PolicyBundleVersion
			}
			dryRef := DecisionRef{Type: "dry_run_decision", ID: req.RequestID, Mode: DecisionModeDryRun}
			brief.EventRefs = append(brief.EventRefs, dryRef)
			if len(decision.EvidenceRefs) > 0 {
				brief.EvidenceRefs = append(brief.EvidenceRefs, decision.EvidenceRefs...)
			}
			if decision.Outcome == DecisionAllow {
				continue
			}
			brief.Status = PreflightStatusFailed
			brief.Findings = append(brief.Findings, findingFromDecision(campaign, recipient, step, decision, dryRef))
			brief.ContextGaps = append(brief.ContextGaps, decision.FailClosedReasons...)
		}
	}
	if brief.Status == PreflightStatusPassed {
		brief.Summary = "campaign preflight passed; all campaign recipient and step combinations were evaluated in dry-run mode"
	} else if brief.Summary == "" {
		brief.Summary = "campaign preflight failed; review findings and context gaps before launch"
	}
	return brief, nil
}

func validateCampaign(c CampaignArtifact) []ContextGap {
	var gaps []ContextGap
	if c.CampaignID == "" {
		gaps = append(gaps, ContextGap{Code: "missing_campaign_id", Field: "campaign_id", Remediation: "Provide a campaign ID before preflight."})
	}
	if c.TenantID == "" {
		gaps = append(gaps, ContextGap{Code: "missing_tenant_id", Field: "tenant_id", Remediation: "Provide trusted tenant context before preflight."})
	}
	if c.Schedule.StartsAt.IsZero() {
		gaps = append(gaps, ContextGap{Code: "missing_campaign_start", Field: "schedule.starts_at", Remediation: "Provide a campaign start time before preflight."})
	}
	if len(c.Audience) == 0 {
		gaps = append(gaps, ContextGap{Code: "missing_campaign_audience", Field: "audience", Remediation: "Provide at least one campaign recipient before preflight."})
	}
	for _, recipient := range c.Audience {
		if recipient.RecipientRef == "" {
			gaps = append(gaps, ContextGap{Code: "missing_recipient_ref", Field: "audience.recipient_ref", Remediation: "Provide a recipient reference for every campaign recipient."})
		}
		if recipient.DebtorID == "" {
			gaps = append(gaps, ContextGap{Code: "missing_recipient_debtor", Field: "audience.debtor_id", Remediation: "Provide a debtor reference for every campaign recipient."})
		}
		if recipient.Timezone == "" {
			gaps = append(gaps, ContextGap{Code: "missing_recipient_timezone", Field: "audience.timezone", Remediation: "Provide an IANA timezone for every campaign recipient."})
		}
	}
	if len(c.Steps) == 0 {
		gaps = append(gaps, ContextGap{Code: "missing_campaign_steps", Field: "steps", Remediation: "Provide at least one campaign step before preflight."})
	}
	for _, step := range c.Steps {
		if step.StepID == "" {
			gaps = append(gaps, ContextGap{Code: "missing_step_id", Field: "steps.step_id", Remediation: "Provide a step ID for every campaign step."})
		}
		if step.TemplateID == "" {
			gaps = append(gaps, ContextGap{Code: "missing_template_id", Field: "steps.template_id", Remediation: "Provide a template ID for every campaign step."})
		}
		if step.Channel == "" {
			gaps = append(gaps, ContextGap{Code: "missing_step_channel", Field: "steps.channel", Remediation: "Provide an outbound channel for every campaign step."})
		}
		if step.TextTemplate == "" {
			gaps = append(gaps, ContextGap{Code: "missing_step_template", Field: "steps.text_template", Remediation: "Provide message content for every campaign step."})
		}
	}
	return gaps
}

func expandCampaignProposal(c CampaignArtifact, recipient CampaignRecipient, step CampaignStep) OutboundActionProposal {
	return OutboundActionProposal{
		Kind:                     ActionSendOutboundUtterance,
		ProposalID:               dryRunProposalID(c.CampaignID, recipient.RecipientRef, step.StepID),
		CampaignID:               c.CampaignID,
		StepID:                   step.StepID,
		TemplateID:               step.TemplateID,
		DebtorID:                 recipient.DebtorID,
		Channel:                  step.Channel,
		RecipientRef:             recipient.RecipientRef,
		Text:                     renderCampaignTemplate(step.TextTemplate, recipient),
		ProposedAt:               c.Schedule.StartsAt.Add(step.SendOffset),
		DebtorTimezone:           recipient.Timezone,
		ContactPartyRelationship: recipient.Relationship,
		AuthorizedChannels:       recipient.ChannelRefs,
		PaymentTarget:            step.PaymentTarget,
	}
}

func findingFromDecision(c CampaignArtifact, recipient CampaignRecipient, step CampaignStep, decision Decision, dryRef DecisionRef) PreflightFinding {
	finding := PreflightFinding{
		CampaignID:          c.CampaignID,
		StepID:              step.StepID,
		TemplateID:          step.TemplateID,
		RecipientRef:        recipient.RecipientRef,
		PolicyBundleVersion: decision.PolicyBundleVersion,
		ContextGaps:         decision.FailClosedReasons,
		DryRunRefs:          []DecisionRef{dryRef},
		EvidenceRefs:        decision.EvidenceRefs,
		DraftSuggestion:     decision.DraftSuggestion,
	}
	if len(decision.Violations) > 0 {
		finding.RuleCode = decision.Violations[0].RuleCode
		finding.Rationale = decision.Violations[0].Rationale
		finding.Remediation = decision.Violations[0].Remediation
	} else if len(decision.FailClosedReasons) > 0 {
		finding.RuleCode = decision.FailClosedReasons[0].Code
		finding.Remediation = decision.FailClosedReasons[0].Remediation
	}
	return finding
}

func renderCampaignTemplate(template string, recipient CampaignRecipient) string {
	replacer := strings.NewReplacer(
		"{{debtor_id}}", recipient.DebtorID,
		"{{recipient_ref}}", recipient.RecipientRef,
	)
	return replacer.Replace(template)
}

func dryRunProposalID(campaignID, recipientRef, stepID string) string {
	return fmt.Sprintf("%s/%s/%s", campaignID, recipientRef, stepID)
}

func dryRunDecisionID(campaignID, recipientRef, stepID string) string {
	return fmt.Sprintf("dry-run:%s", dryRunProposalID(campaignID, recipientRef, stepID))
}
