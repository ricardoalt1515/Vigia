package outbound

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/judge"
)

type stubContextResolver struct {
	context AuthorityContext
	err     error
}

func (r stubContextResolver) Resolve(context.Context, string, OutboundActionProposal) (AuthorityContext, error) {
	if r.err != nil {
		return AuthorityContext{}, r.err
	}
	return r.context, nil
}

type stubBundleResolver struct {
	bundle ResolvedPolicyBundle
	err    error
}

func (r stubBundleResolver) ResolveActiveBundle(context.Context, string) (ResolvedPolicyBundle, error) {
	if r.err != nil {
		return ResolvedPolicyBundle{}, r.err
	}
	return r.bundle, nil
}

type spyJudge struct {
	calls  []judge.JudgeInput
	result judge.JudgeResult
	err    error
}

func (j *spyJudge) Evaluate(ctx context.Context, in judge.JudgeInput) (judge.JudgeResult, error) {
	j.calls = append(j.calls, in)
	if j.err != nil {
		return judge.JudgeResult{}, j.err
	}
	return j.result, nil
}

func TestDeciderDeniesFailClosedAuthorityGaps(t *testing.T) {
	tests := []struct {
		name         string
		request      DecisionRequest
		context      AuthorityContext
		contextErr   error
		bundleErr    error
		wantGapCode  string
		wantGapField string
	}{
		{
			name:         "missing debtor timezone",
			request:      validRequest(),
			context:      AuthorityContext{},
			wantGapCode:  "missing_debtor_timezone",
			wantGapField: "debtor_timezone",
		},
		{
			name: "invalid debtor timezone",
			request: validRequestWith(func(req *DecisionRequest) {
				req.Proposal.ProposedAt = time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)
			}),
			context: validAuthorityContextWith(func(ctx *AuthorityContext) {
				ctx.DebtorTimezone = "America/Nope"
			}),
			wantGapCode:  "invalid_debtor_timezone",
			wantGapField: "debtor_timezone",
		},
		{
			name:         "unknown active policy bundle",
			request:      validRequest(),
			context:      validAuthorityContext(),
			bundleErr:    ErrPolicyBundleNotFound,
			wantGapCode:  "unknown_policy_bundle",
			wantGapField: "policy_bundle",
		},
		{
			name:         "context resolver unavailable",
			request:      validRequest(),
			contextErr:   errors.New("resolver unavailable"),
			wantGapCode:  "authority_context_unresolved",
			wantGapField: "authority_context",
		},
		{
			name: "zero proposed time",
			request: validRequestWith(func(req *DecisionRequest) {
				req.Proposal.ProposedAt = time.Time{}
			}),
			context: validAuthorityContextWith(func(ctx *AuthorityContext) {
				ctx.ProposedAt = time.Time{}
			}),
			wantGapCode:  "missing_proposed_at",
			wantGapField: "proposed_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := newTestDecider(tt.context, tt.contextErr, tt.bundleErr).Decide(context.Background(), tt.request)
			if err != nil {
				t.Fatalf("Decide returned error: %v", err)
			}
			if decision.Outcome != DecisionDeny {
				t.Fatalf("outcome = %q, want %q", decision.Outcome, DecisionDeny)
			}
			if len(decision.FailClosedReasons) != 1 {
				t.Fatalf("fail-closed reasons = %d, want 1", len(decision.FailClosedReasons))
			}
			gap := decision.FailClosedReasons[0]
			if gap.Code != tt.wantGapCode || gap.Field != tt.wantGapField {
				t.Fatalf("gap = (%q, %q), want (%q, %q)", gap.Code, gap.Field, tt.wantGapCode, tt.wantGapField)
			}
		})
	}
}

func TestDeciderUsesRequestIDAsDecisionCorrelation(t *testing.T) {
	req := validRequestWith(func(req *DecisionRequest) {
		req.RequestID = "request-123"
	})

	decision, err := newTestDecider(validAuthorityContext(), nil, nil).Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.ID != "request-123" {
		t.Fatalf("decision ID = %q, want request-123", decision.ID)
	}
	metadata := decision.Metadata()
	if got := metadata["decision_id"]; got != "request-123" {
		t.Fatalf("metadata decision_id = %v, want request-123", got)
	}
	if got := metadata["decision_mode"]; got != string(DecisionModeEnforcement) {
		t.Fatalf("metadata decision_mode = %v, want enforcement", got)
	}
	if got := metadata["action_kind"]; got != string(ActionSendOutboundUtterance) {
		t.Fatalf("metadata action_kind = %v, want send_outbound_utterance", got)
	}
	if got := metadata["proposal_id"]; got != "proposal-1" {
		t.Fatalf("metadata proposal_id = %v, want proposal-1", got)
	}
}

func TestDeciderDerivesStableDecisionIDWhenRequestIDMissing(t *testing.T) {
	decision, err := newTestDecider(validAuthorityContext(), nil, nil).Decide(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.ID != "proposal-1" {
		t.Fatalf("decision ID = %q, want proposal-1", decision.ID)
	}
}

func TestDeciderDeniesRecipientChannelThirdPartyAndPaymentViolationsBeforeJudge(t *testing.T) {
	tests := []struct {
		name         string
		context      AuthorityContext
		wantRule     string
		wantGapCode  string
		wantGapField string
	}{
		{
			name: "ambiguous recipient fails closed",
			context: validAuthorityContextWith(func(ctx *AuthorityContext) {
				ctx.RecipientAmbiguous = true
			}),
			wantGapCode:  "ambiguous_recipient",
			wantGapField: "recipient_ref",
		},
		{
			name: "ambiguous channel fails closed",
			context: validAuthorityContextWith(func(ctx *AuthorityContext) {
				ctx.ChannelAmbiguous = true
			}),
			wantGapCode:  "ambiguous_channel",
			wantGapField: "channel",
		},
		{
			name: "unauthorized third party blocks",
			context: validAuthorityContextWith(func(ctx *AuthorityContext) {
				ctx.ContactPartyRelationship = "third_party"
			}),
			wantRule: "MX-REDECO-06",
		},
		{
			name: "unauthorized channel blocks",
			context: validAuthorityContextWith(func(ctx *AuthorityContext) {
				ctx.AuthorizedChannels = []string{string(core.InteractionChannelCall)}
			}),
			wantRule: "MX-REDECO-11",
		},
		{
			name: "payment routing blocks",
			context: validAuthorityContextWith(func(ctx *AuthorityContext) {
				ctx.PaymentRecipient = "collector"
			}),
			wantRule: "MX-REDECO-10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			judgeSpy := &spyJudge{result: judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99, Rationale: "safe"}}
			decision, err := newTestDeciderWithJudge(tt.context, nil, nil, judgeSpy).Decide(context.Background(), validRequest())
			if err != nil {
				t.Fatalf("Decide returned error: %v", err)
			}
			if decision.Outcome != DecisionDeny {
				t.Fatalf("outcome = %q, want %q", decision.Outcome, DecisionDeny)
			}
			if len(judgeSpy.calls) != 0 {
				t.Fatalf("judge calls = %d, want 0 for deterministic block", len(judgeSpy.calls))
			}
			if tt.wantRule != "" {
				if len(decision.Violations) != 1 || decision.Violations[0].RuleCode != tt.wantRule {
					t.Fatalf("violations = %+v, want one %s", decision.Violations, tt.wantRule)
				}
				if decision.Violations[0].Remediation == "" {
					t.Fatalf("remediation is empty for violation %+v", decision.Violations[0])
				}
				return
			}
			if len(decision.FailClosedReasons) != 1 {
				t.Fatalf("fail-closed reasons = %d, want 1", len(decision.FailClosedReasons))
			}
			gap := decision.FailClosedReasons[0]
			if gap.Code != tt.wantGapCode || gap.Field != tt.wantGapField || gap.Remediation == "" {
				t.Fatalf("gap = %+v, want code=%s field=%s with remediation", gap, tt.wantGapCode, tt.wantGapField)
			}
		})
	}
}

func TestDeciderDeniesWhenRequiredResolversAreNotConfigured(t *testing.T) {
	tests := []struct {
		name        string
		decider     Decider
		wantGapCode string
	}{
		{
			name: "missing authority context resolver",
			decider: Decider{
				BundleResolver: stubBundleResolver{bundle: ResolvedPolicyBundle{ID: "bundle-1", Version: "v1"}},
				ToneJudge:      &spyJudge{result: judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99, Rationale: "safe"}},
			},
			wantGapCode: "authority_context_unresolved",
		},
		{
			name: "missing bundle resolver",
			decider: Decider{
				ContextResolver: stubContextResolver{context: validAuthorityContext()},
				ToneJudge:       &spyJudge{result: judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99, Rationale: "safe"}},
			},
			wantGapCode: "policy_bundle_unresolved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := tt.decider.Decide(context.Background(), validRequest())
			if err != nil {
				t.Fatalf("Decide returned error: %v", err)
			}
			if decision.Outcome != DecisionDeny {
				t.Fatalf("outcome = %q, want %q", decision.Outcome, DecisionDeny)
			}
			if len(decision.FailClosedReasons) != 1 || decision.FailClosedReasons[0].Code != tt.wantGapCode {
				t.Fatalf("fail-closed reasons = %+v, want %s", decision.FailClosedReasons, tt.wantGapCode)
			}
			if got := decision.Metadata()["proposal_id"]; got != validRequest().Proposal.ProposalID {
				t.Fatalf("metadata proposal_id = %v, want %s", got, validRequest().Proposal.ProposalID)
			}
		})
	}
}

func TestDeciderDeniesWhenRequiredJudgeIsNotConfigured(t *testing.T) {
	decision, err := newTestDeciderWithJudge(validAuthorityContext(), nil, nil, nil).Decide(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.Outcome != DecisionDeny {
		t.Fatalf("outcome = %q, want %q", decision.Outcome, DecisionDeny)
	}
	if len(decision.FailClosedReasons) != 1 || decision.FailClosedReasons[0].Code != "judge_unavailable" {
		t.Fatalf("fail-closed reasons = %+v, want judge_unavailable", decision.FailClosedReasons)
	}
}

func TestDeciderUsesJudgeOnlyForSemanticToneThreat(t *testing.T) {
	tests := []struct {
		name        string
		result      judge.JudgeResult
		err         error
		wantOutcome DecisionOutcome
		wantRule    string
		wantGapCode string
		wantDraft   bool
	}{
		{
			name:        "safe semantic result allows",
			result:      judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.95, Rationale: "professional tone"},
			wantOutcome: DecisionAllow,
		},
		{
			name:        "threatening semantic result denies with draft suggestion",
			result:      judge.JudgeResult{Outcome: judge.OutcomeBlock, Confidence: 0.91, Rationale: "coercive threat"},
			wantOutcome: DecisionDeny,
			wantRule:    "MX-REDECO-05",
			wantDraft:   true,
		},
		{
			name:        "judge error fails closed",
			err:         errors.New("judge unavailable"),
			wantOutcome: DecisionDeny,
			wantGapCode: "judge_unavailable",
		},
		{
			name:        "inconclusive judge result fails closed",
			result:      judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.20, Rationale: "uncertain"},
			wantOutcome: DecisionDeny,
			wantGapCode: "judge_inconclusive",
		},
		{
			name:        "invalid judge outcome fails closed",
			result:      judge.JudgeResult{Outcome: judge.Outcome("unknown"), Confidence: 0.95, Rationale: "malformed"},
			wantOutcome: DecisionDeny,
			wantGapCode: "judge_invalid_result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			judgeSpy := &spyJudge{result: tt.result, err: tt.err}
			decision, err := newTestDeciderWithJudge(validAuthorityContext(), nil, nil, judgeSpy).Decide(context.Background(), validRequest())
			if err != nil {
				t.Fatalf("Decide returned error: %v", err)
			}
			if len(judgeSpy.calls) != 1 {
				t.Fatalf("judge calls = %d, want 1 after deterministic checks pass", len(judgeSpy.calls))
			}
			if gotText := judgeSpy.calls[0].Utterances[0].Text; gotText != validRequest().Proposal.Text {
				t.Fatalf("judge text = %q, want proposal text", gotText)
			}
			if decision.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %q, want %q", decision.Outcome, tt.wantOutcome)
			}
			if tt.wantRule != "" && (len(decision.Violations) != 1 || decision.Violations[0].RuleCode != tt.wantRule) {
				t.Fatalf("violations = %+v, want %s", decision.Violations, tt.wantRule)
			}
			if tt.wantGapCode != "" && (len(decision.FailClosedReasons) != 1 || decision.FailClosedReasons[0].Code != tt.wantGapCode) {
				t.Fatalf("fail-closed reasons = %+v, want %s", decision.FailClosedReasons, tt.wantGapCode)
			}
			if tt.wantDraft {
				if decision.DraftSuggestion == nil || !decision.DraftSuggestion.DraftOnly || decision.DraftSuggestion.BasedOn != validRequest().Proposal.ProposalID {
					t.Fatalf("draft suggestion = %+v, want draft-only based on proposal", decision.DraftSuggestion)
				}
			}
		})
	}
}

func TestDeciderContactHoursAndCompliantAllow(t *testing.T) {
	tests := []struct {
		name        string
		proposedAt  time.Time
		wantOutcome DecisionOutcome
		wantRule    string
	}{
		{
			name:        "out of hours denies",
			proposedAt:  time.Date(2026, 7, 6, 3, 30, 0, 0, time.UTC),
			wantOutcome: DecisionDeny,
			wantRule:    "MX-REDECO-02",
		},
		{
			name:        "compliant proposal allows",
			proposedAt:  time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC),
			wantOutcome: DecisionAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorityContext := validAuthorityContext()
			authorityContext.ProposedAt = tt.proposedAt
			req := validRequest()
			req.Proposal.ProposedAt = tt.proposedAt

			decision, err := newTestDecider(authorityContext, nil, nil).Decide(context.Background(), req)
			if err != nil {
				t.Fatalf("Decide returned error: %v", err)
			}
			if decision.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %q, want %q", decision.Outcome, tt.wantOutcome)
			}
			if decision.PolicyBundleVersion != "v1" {
				t.Fatalf("policy bundle version = %q, want v1", decision.PolicyBundleVersion)
			}
			if tt.wantRule != "" && decision.Violations[0].RuleCode != tt.wantRule {
				t.Fatalf("rule code = %q, want %q", decision.Violations[0].RuleCode, tt.wantRule)
			}
			if tt.wantRule == "" && (len(decision.Violations) != 0 || len(decision.FailClosedReasons) != 0) {
				t.Fatalf("allowed decision has violations=%d gaps=%d", len(decision.Violations), len(decision.FailClosedReasons))
			}
		})
	}
}

func TestDeciderRecordsBlockedEnforcementDecision(t *testing.T) {
	recorder := &spyDecisionRecorder{recorded: RecordedDecision{
		EventRefs:    []DecisionRef{{Type: "interaction_event", ID: "interaction-1", Mode: DecisionModeEnforcement}},
		EvidenceRefs: []DecisionRef{{Type: "evidence_record", ID: "evidence-1", Mode: DecisionModeEnforcement}},
	}}
	decider := newTestDeciderWithJudge(validAuthorityContextWith(func(ctx *AuthorityContext) {
		ctx.ContactPartyRelationship = "third_party"
	}), nil, nil, &spyJudge{result: judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99, Rationale: "safe"}})
	decider.Recorder = recorder
	req := validRequestWith(func(req *DecisionRequest) {
		req.RequestID = "decision-1"
	})

	decision, err := decider.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.Outcome != DecisionDeny {
		t.Fatalf("outcome = %q, want deny", decision.Outcome)
	}
	if recorder.calls != 1 {
		t.Fatalf("recorder calls = %d, want 1", recorder.calls)
	}
	if recorder.lastReq.RequestID != "decision-1" || recorder.lastDecision.ID != "decision-1" {
		t.Fatalf("recorder correlation = request %q decision %q, want decision-1", recorder.lastReq.RequestID, recorder.lastDecision.ID)
	}
	metadata := decision.Metadata()
	if got := metadata["event_refs"]; got == nil {
		t.Fatal("metadata event_refs missing")
	}
	if got := metadata["evidence_refs"]; got == nil {
		t.Fatal("metadata evidence_refs missing")
	}
}

func TestDeciderMarksBlockedEnforcementDecisionWhenRecordingFails(t *testing.T) {
	recorder := &spyDecisionRecorder{err: errors.New("ledger unavailable")}
	decider := newTestDeciderWithJudge(validAuthorityContextWith(func(ctx *AuthorityContext) {
		ctx.ContactPartyRelationship = "third_party"
	}), nil, nil, &spyJudge{result: judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99, Rationale: "safe"}})
	decider.Recorder = recorder

	decision, err := decider.Decide(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.Outcome != DecisionDeny {
		t.Fatalf("outcome = %q, want deny", decision.Outcome)
	}
	metadata := decision.Metadata()
	codes, ok := metadata["fail_closed_codes"].([]string)
	if !ok || !containsString(codes, "decision_recording_failed") {
		t.Fatalf("fail_closed_codes = %#v, want decision_recording_failed", metadata["fail_closed_codes"])
	}
	if got := metadata["evidence_refs"]; got != nil {
		t.Fatalf("evidence_refs = %v, want absent when recorder failed", got)
	}
}

func TestDeciderDoesNotRecordAllowedOrDryRunDecisions(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode DecisionMode
		rel  string
	}{
		{name: "allowed enforcement", mode: DecisionModeEnforcement, rel: "debtor"},
		{name: "blocked dry run", mode: DecisionModeDryRun, rel: "third_party"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &spyDecisionRecorder{}
			decider := newTestDeciderWithJudge(validAuthorityContextWith(func(ctx *AuthorityContext) {
				ctx.ContactPartyRelationship = tt.rel
			}), nil, nil, &spyJudge{result: judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99, Rationale: "safe"}})
			decider.Recorder = recorder
			req := validRequestWith(func(req *DecisionRequest) {
				req.Mode = tt.mode
			})
			if _, err := decider.Decide(context.Background(), req); err != nil {
				t.Fatalf("Decide returned error: %v", err)
			}
			if recorder.calls != 0 {
				t.Fatalf("recorder calls = %d, want 0", recorder.calls)
			}
		})
	}
}

type spyDecisionRecorder struct {
	calls        int
	lastReq      DecisionRequest
	lastDecision Decision
	recorded     RecordedDecision
	err          error
}

func (r *spyDecisionRecorder) Record(_ context.Context, req DecisionRequest, decision Decision) (RecordedDecision, error) {
	r.calls++
	r.lastReq = req
	r.lastDecision = decision
	if r.err != nil {
		return RecordedDecision{}, r.err
	}
	return r.recorded, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func newTestDecider(ctx AuthorityContext, contextErr, bundleErr error) Decider {
	return newTestDeciderWithJudge(ctx, contextErr, bundleErr, &spyJudge{result: judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99, Rationale: "safe"}})
}

func newTestDeciderWithJudge(ctx AuthorityContext, contextErr, bundleErr error, judgeSpy judge.Judge) Decider {
	return Decider{
		ContextResolver: stubContextResolver{context: ctx, err: contextErr},
		BundleResolver:  stubBundleResolver{bundle: ResolvedPolicyBundle{ID: "bundle-1", Version: "v1"}, err: bundleErr},
		ContactWindow:   detection.Window{StartHour: 8, EndHour: 21},
		ToneJudge:       judgeSpy,
		ToneRubric:      judge.Rubric{Version: "mx-redeco-05.tone-threat.v1", Prompt: "judge tone and threats"},
	}
}

func validRequest() DecisionRequest {
	proposedAt := time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)
	return DecisionRequest{TenantID: "tenant-1", ActorID: "agent-1", Mode: DecisionModeEnforcement, Proposal: OutboundActionProposal{Kind: ActionSendOutboundUtterance, ProposalID: "proposal-1", CaseID: "case-1", DebtorID: "debtor-1", Channel: core.InteractionChannelMessage, RecipientRef: "recipient-1", Text: "Buen día, le comparto información de su cuenta.", ProposedAt: proposedAt}}
}

func validRequestWith(update func(*DecisionRequest)) DecisionRequest {
	req := validRequest()
	update(&req)
	return req
}

func validAuthorityContext() AuthorityContext {
	return AuthorityContext{TenantID: "tenant-1", DebtorID: "debtor-1", DebtorTimezone: "America/Mexico_City", Channel: core.InteractionChannelMessage, ProposedAt: time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC), ContactPartyRelationship: "debtor", AuthorizedChannels: []string{string(core.InteractionChannelMessage)}, PaymentRecipient: "creditor"}
}

func validAuthorityContextWith(update func(*AuthorityContext)) AuthorityContext {
	ctx := validAuthorityContext()
	update(&ctx)
	return ctx
}
