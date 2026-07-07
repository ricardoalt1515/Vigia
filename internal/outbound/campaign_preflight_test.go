package outbound

import (
	"context"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/judge"
)

func TestPreflightFailsNonCompliantCompleteCampaignWithActionableBrief(t *testing.T) {
	judgeSpy := &spyJudge{result: judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99, Rationale: "safe"}}
	decider := Decider{
		ContextResolver: campaignContextResolver{},
		BundleResolver:  stubBundleResolver{bundle: ResolvedPolicyBundle{ID: "bundle-1", Version: "policy-v4"}},
		ContactWindow:   detection.Window{StartHour: 8, EndHour: 21},
		ToneJudge:       judgeSpy,
		ToneRubric:      judge.Rubric{Version: "mx-redeco-05.tone-threat.v1", Prompt: "judge tone and threats"},
		Recorder: &spyDecisionRecorder{recorded: RecordedDecision{
			EventRefs:    []DecisionRef{{Type: "interaction_event", ID: "enforcement-event", Mode: DecisionModeEnforcement}},
			EvidenceRefs: []DecisionRef{{Type: "evidence_record", ID: "enforcement-evidence", Mode: DecisionModeEnforcement}},
		}},
	}
	service := PreflightService{Decider: decider}

	brief, err := service.Run(context.Background(), CampaignArtifact{
		CampaignID: "campaign-1",
		Name:       "July outreach",
		TenantID:   "tenant-1",
		ActorID:    "planner-1",
		Audience: []CampaignRecipient{{
			RecipientRef: "recipient-third-party",
			DebtorID:     "debtor-1",
			Relationship: "third_party",
			ChannelRefs:  []string{string(core.InteractionChannelMessage)},
			Timezone:     "America/Mexico_City",
		}},
		Steps: []CampaignStep{{
			StepID:        "step-1",
			TemplateID:    "template-1",
			Channel:       core.InteractionChannelMessage,
			TextTemplate:  "Buen día, le comparto información de su cuenta.",
			SendOffset:    2 * time.Hour,
			PaymentTarget: "creditor",
		}},
		Schedule: CampaignSchedule{StartsAt: time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC), Timezone: "America/Mexico_City"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if brief.Status != PreflightStatusFailed {
		t.Fatalf("status = %q, want %q", brief.Status, PreflightStatusFailed)
	}
	if brief.PolicyBundleVersion != "policy-v4" {
		t.Fatalf("policy bundle version = %q, want policy-v4", brief.PolicyBundleVersion)
	}
	if len(brief.Findings) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(brief.Findings), brief.Findings)
	}
	finding := brief.Findings[0]
	if finding.CampaignID != "campaign-1" || finding.StepID != "step-1" || finding.TemplateID != "template-1" || finding.RecipientRef != "recipient-third-party" {
		t.Fatalf("finding component = %+v, want campaign/step/template/recipient refs", finding)
	}
	if finding.RuleCode != "MX-REDECO-06" || finding.PolicyBundleVersion != "policy-v4" || finding.Remediation == "" {
		t.Fatalf("finding compliance details = %+v, want rule, bundle, remediation", finding)
	}
	if len(finding.DryRunRefs) != 1 || finding.DryRunRefs[0].Type != "dry_run_decision" || finding.DryRunRefs[0].Mode != DecisionModeDryRun {
		t.Fatalf("dry-run refs = %+v, want dry_run_decision in dry_run mode", finding.DryRunRefs)
	}
	if len(finding.EvidenceRefs) != 0 || len(brief.EvidenceRefs) != 0 {
		t.Fatalf("evidence refs = finding %+v brief %+v, want no enforcement evidence refs for dry-run", finding.EvidenceRefs, brief.EvidenceRefs)
	}
	if len(brief.EventRefs) != 1 || brief.EventRefs[0].Type != "dry_run_decision" || brief.EventRefs[0].Mode != DecisionModeDryRun {
		t.Fatalf("brief event refs = %+v, want dry-run decision refs", brief.EventRefs)
	}
	if len(judgeSpy.calls) != 0 {
		t.Fatalf("judge calls = %d, want 0 because deterministic failure short-circuits", len(judgeSpy.calls))
	}
}

func TestPreflightPassesCompliantCompleteCampaignAndEvaluatesEveryRecipientStep(t *testing.T) {
	spy := &requestSpyDecider{decision: Decision{Outcome: DecisionAllow, PolicyBundleVersion: "policy-v4"}}
	service := PreflightService{Decider: spy}
	campaign := CampaignArtifact{
		CampaignID: "campaign-2",
		TenantID:   "tenant-1",
		ActorID:    "planner-1",
		Audience: []CampaignRecipient{
			{RecipientRef: "recipient-1", DebtorID: "debtor-1", Timezone: "America/Mexico_City", ChannelRefs: []string{string(core.InteractionChannelMessage)}},
			{RecipientRef: "recipient-2", DebtorID: "debtor-2", Timezone: "America/Mexico_City", ChannelRefs: []string{string(core.InteractionChannelEmail)}},
		},
		Steps: []CampaignStep{
			{StepID: "step-message", TemplateID: "template-message", Channel: core.InteractionChannelMessage, TextTemplate: "Mensaje {{debtor_id}}", SendOffset: time.Hour, PaymentTarget: "creditor"},
			{StepID: "step-email", TemplateID: "template-email", Channel: core.InteractionChannelEmail, TextTemplate: "Email {{recipient_ref}}", SendOffset: 2 * time.Hour, PaymentTarget: "creditor"},
		},
		Schedule: CampaignSchedule{StartsAt: time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC), Timezone: "America/Mexico_City"},
	}

	brief, err := service.Run(context.Background(), campaign)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if brief.Status != PreflightStatusPassed {
		t.Fatalf("status = %q, want %q", brief.Status, PreflightStatusPassed)
	}
	if len(brief.Findings) != 0 || len(brief.ContextGaps) != 0 {
		t.Fatalf("brief findings=%d gaps=%d, want none", len(brief.Findings), len(brief.ContextGaps))
	}
	if len(spy.requests) != 4 {
		t.Fatalf("decider requests = %d, want every recipient/step pair", len(spy.requests))
	}
	for _, req := range spy.requests {
		if req.Mode != DecisionModeDryRun {
			t.Fatalf("request mode = %q, want dry_run", req.Mode)
		}
		if req.Proposal.CampaignID != "campaign-2" || req.Proposal.StepID == "" || req.Proposal.TemplateID == "" || req.Proposal.ProposedAt.IsZero() {
			t.Fatalf("expanded proposal missing campaign fields: %+v", req.Proposal)
		}
	}
}

func TestPreflightSurfacesCampaignContextGapsSeparately(t *testing.T) {
	service := PreflightService{Decider: &requestSpyDecider{}}
	brief, err := service.Run(context.Background(), CampaignArtifact{
		CampaignID: "campaign-3",
		TenantID:   "tenant-1",
		Audience:   []CampaignRecipient{{RecipientRef: "recipient-1", DebtorID: "debtor-1"}},
		Steps:      []CampaignStep{{StepID: "step-1", TemplateID: "template-1", Channel: core.InteractionChannelMessage, TextTemplate: "Hello", SendOffset: time.Hour}},
		Schedule:   CampaignSchedule{StartsAt: time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if brief.Status != PreflightStatusFailed {
		t.Fatalf("status = %q, want failed", brief.Status)
	}
	if len(brief.ContextGaps) == 0 {
		t.Fatal("context gaps missing, want campaign validation gap")
	}
	if brief.ContextGaps[0].Code != "missing_recipient_timezone" {
		t.Fatalf("gap = %+v, want missing_recipient_timezone", brief.ContextGaps[0])
	}
}

type requestSpyDecider struct {
	requests []DecisionRequest
	decision Decision
}

func (d *requestSpyDecider) Decide(_ context.Context, req DecisionRequest) (Decision, error) {
	d.requests = append(d.requests, req)
	decision := d.decision
	if decision.ID == "" {
		decision.ID = req.RequestID
	}
	if decision.Mode == "" {
		decision.Mode = req.Mode
	}
	if decision.ActionKind == "" {
		decision.ActionKind = req.Proposal.Kind
	}
	if decision.ProposalID == "" {
		decision.ProposalID = req.Proposal.ProposalID
	}
	return decision, nil
}

type campaignContextResolver struct{}

func (campaignContextResolver) Resolve(_ context.Context, tenantID string, p OutboundActionProposal) (AuthorityContext, error) {
	relationship := "debtor"
	if p.RecipientRef == "recipient-third-party" {
		relationship = "third_party"
	}
	return AuthorityContext{
		TenantID:                 tenantID,
		DebtorID:                 p.DebtorID,
		DebtorTimezone:           "America/Mexico_City",
		Channel:                  p.Channel,
		ProposedAt:               p.ProposedAt,
		ContactPartyRelationship: relationship,
		AuthorizedChannels:       []string{string(p.Channel)},
		PaymentRecipient:         p.PaymentTarget,
	}, nil
}
