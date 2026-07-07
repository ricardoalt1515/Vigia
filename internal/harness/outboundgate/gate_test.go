package outboundgate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/outbound"
)

type spyDecider struct {
	calls    []outbound.DecisionRequest
	decision outbound.Decision
	err      error
}

func (s *spyDecider) Decide(ctx context.Context, req outbound.DecisionRequest) (outbound.Decision, error) {
	s.calls = append(s.calls, req)
	return s.decision, s.err
}

type spyFallback struct {
	calls    []harness.ToolCall
	decision harness.PermissionDecision
}

type spyRecorder struct {
	calls    []outbound.DecisionRequest
	recorded outbound.RecordedDecision
	err      error
}

func (s *spyRecorder) Record(_ context.Context, req outbound.DecisionRequest, _ outbound.Decision) (outbound.RecordedDecision, error) {
	s.calls = append(s.calls, req)
	if s.err != nil {
		return outbound.RecordedDecision{}, s.err
	}
	return s.recorded, nil
}

func (s *spyFallback) Decide(ctx context.Context, call harness.ToolCall) harness.PermissionDecision {
	s.calls = append(s.calls, call)
	return s.decision
}

func TestGateMapsSendOutboundUtteranceToEnforcementDecision(t *testing.T) {
	decider := &spyDecider{decision: outbound.Decision{
		ID: "decision-1", Mode: outbound.DecisionModeEnforcement, Outcome: outbound.DecisionDeny,
		Reason: "blocked", PolicyBundleVersion: "v1",
		Violations: []outbound.RuleViolation{{RuleCode: "MX-REDECO-02"}},
	}}
	fallback := &spyFallback{decision: harness.PermissionDecision{Kind: harness.PermissionAllowed, Reason: "fallback"}}
	gate := NewGate(Config{TenantID: "tenant-1", ActorID: "agent-1", Decider: decider, Fallback: fallback})

	decision := gate.Decide(context.Background(), harness.ToolCall{Name: "send_outbound_utterance", Input: validSendInput()})

	if decision.Kind != harness.PermissionDenied {
		t.Fatalf("permission kind = %q, want %q", decision.Kind, harness.PermissionDenied)
	}
	if len(decider.calls) != 1 {
		t.Fatalf("decider calls = %d, want 1", len(decider.calls))
	}
	gotReq := decider.calls[0]
	if gotReq.Mode != outbound.DecisionModeEnforcement || gotReq.TenantID != "tenant-1" || gotReq.ActorID != "agent-1" {
		t.Fatalf("request context = (%q, %q, %q), want enforcement tenant-1 agent-1", gotReq.Mode, gotReq.TenantID, gotReq.ActorID)
	}
	if gotReq.Proposal.Kind != outbound.ActionSendOutboundUtterance || gotReq.Proposal.Channel != core.InteractionChannelMessage || gotReq.Proposal.PaymentTarget != "creditor" {
		t.Fatalf("proposal mapping = %+v", gotReq.Proposal)
	}
	if got := decision.Metadata["decision_id"]; got != "decision-1" {
		t.Fatalf("metadata decision_id = %v, want decision-1", got)
	}
	if len(fallback.calls) != 0 {
		t.Fatalf("fallback calls = %d, want 0 for send outbound", len(fallback.calls))
	}
}

func TestGateDeniesSendWhenDeciderUnavailableWithMetadata(t *testing.T) {
	tests := []struct {
		name    string
		decider Decider
		want    string
	}{
		{name: "decider missing", want: "outbound_decider_unconfigured"},
		{name: "decider error", decider: &spyDecider{err: context.Canceled}, want: "outbound_decider_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := NewGate(Config{TenantID: "tenant-1", ActorID: "agent-1", Decider: tt.decider})

			decision := gate.Decide(context.Background(), harness.ToolCall{Name: "send_outbound_utterance", Input: validSendInput()})

			if decision.Kind != harness.PermissionDenied {
				t.Fatalf("permission kind = %q, want denied", decision.Kind)
			}
			if got := decision.Metadata["action_kind"]; got != string(outbound.ActionSendOutboundUtterance) {
				t.Fatalf("metadata action_kind = %v, want send_outbound_utterance", got)
			}
			if got := decision.Metadata["proposal_id"]; got != "proposal-1" {
				t.Fatalf("metadata proposal_id = %v, want proposal-1", got)
			}
			codes, ok := decision.Metadata["fail_closed_codes"].([]string)
			if !ok || len(codes) != 1 || codes[0] != tt.want {
				t.Fatalf("fail_closed_codes = %v, want [%s]", decision.Metadata["fail_closed_codes"], tt.want)
			}
		})
	}
}

func TestRuntimeWrapsSendToolExecutionWithOutboundGate(t *testing.T) {
	model := &queuedModel{output: harness.ModelOutput{ToolCall: &harness.ToolCall{Name: "send_outbound_utterance", Input: validSendInput()}}}
	sendTool := &spyTool{result: harness.ToolResult{Status: harness.ToolStatusSuccess}}
	decider := &spyDecider{decision: outbound.Decision{ID: "decision-1", Mode: outbound.DecisionModeEnforcement, ActionKind: outbound.ActionSendOutboundUtterance, ProposalID: "proposal-1", Outcome: outbound.DecisionDeny, Reason: "blocked"}}
	runtime := Runtime(harness.Runtime{
		Model: model,
		Tools: harness.ToolRegistry{"send_outbound_utterance": sendTool},
		Permissions: permissionGateFunc(func(context.Context, harness.ToolCall) harness.PermissionDecision {
			return harness.PermissionDecision{Kind: harness.PermissionAllowed}
		}),
		Validator: validatorFunc(func(harness.ModelOutput) error { return nil }),
		Budget:    harness.Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
	}, Config{TenantID: "tenant-1", ActorID: "agent-1", Decider: decider})

	result, err := runtime.RunStep(context.Background(), harness.StepInput{Input: "send"})
	if err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	if result.Status != harness.StepStatusPermissionDenied {
		t.Fatalf("status = %q, want permission denied", result.Status)
	}
	if sendTool.calls != 0 {
		t.Fatalf("send tool executed %d times, want 0", sendTool.calls)
	}
	if len(decider.calls) != 1 || decider.calls[0].Mode != outbound.DecisionModeEnforcement {
		t.Fatalf("decider calls = %+v, want one enforcement decision", decider.calls)
	}
}

func TestGateRecordsInvalidSendSchemaWhenEnoughContextExists(t *testing.T) {
	recorder := &spyRecorder{recorded: outbound.RecordedDecision{
		EventRefs:    []outbound.DecisionRef{{Type: "interaction_event", ID: "interaction-1", Mode: outbound.DecisionModeEnforcement}},
		EvidenceRefs: []outbound.DecisionRef{{Type: "evidence_record", ID: "evidence-1", Mode: outbound.DecisionModeEnforcement}},
	}}
	gate := NewGate(Config{TenantID: "tenant-1", ActorID: "agent-1", Recorder: recorder})

	decision := gate.Decide(context.Background(), harness.ToolCall{Name: "send_outbound_utterance", Input: validSendInputWith(func(input map[string]any) { input["channel"] = "fax" })})

	if decision.Kind != harness.PermissionDenied {
		t.Fatalf("permission kind = %q, want denied", decision.Kind)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("recorder calls = %d, want 1", len(recorder.calls))
	}
	if recorder.calls[0].TenantID != "tenant-1" || recorder.calls[0].ActorID != "agent-1" || recorder.calls[0].Mode != outbound.DecisionModeEnforcement {
		t.Fatalf("record request = %+v, want tenant/actor enforcement", recorder.calls[0])
	}
	if got := decision.Metadata["evidence_refs"]; got == nil {
		t.Fatalf("evidence_refs metadata missing after schema denial recording: %+v", decision.Metadata)
	}
}

func TestGateInvalidSendSchemaSurfacesRecorderFailures(t *testing.T) {
	recorder := &spyRecorder{err: errors.New("database unavailable")}
	gate := NewGate(Config{TenantID: "tenant-1", ActorID: "agent-1", Recorder: recorder})

	decision := gate.Decide(context.Background(), harness.ToolCall{Name: "send_outbound_utterance", Input: validSendInputWith(func(input map[string]any) { input["channel"] = "fax" })})

	if decision.Kind != harness.PermissionDenied {
		t.Fatalf("permission kind = %q, want denied", decision.Kind)
	}
	codes, ok := decision.Metadata["fail_closed_codes"].([]string)
	if !ok || !containsString(codes, "decision_recording_failed") {
		t.Fatalf("fail_closed_codes = %#v, want decision_recording_failed", decision.Metadata["fail_closed_codes"])
	}
	if got := decision.Metadata["evidence_refs"]; got != nil {
		t.Fatalf("evidence_refs = %v, want absent when recording failed", got)
	}
}

func TestGateInvalidSendSchemaWithoutLedgerIdentifiersIsExplicitlyUnrecordable(t *testing.T) {
	recorder := &spyRecorder{}
	gate := NewGate(Config{TenantID: "tenant-1", ActorID: "agent-1", Recorder: recorder})

	decision := gate.Decide(context.Background(), harness.ToolCall{Name: "send_outbound_utterance", Input: map[string]any{"proposal_id": "proposal-1"}})

	if decision.Kind != harness.PermissionDenied {
		t.Fatalf("permission kind = %q, want denied", decision.Kind)
	}
	if len(recorder.calls) != 0 {
		t.Fatalf("recorder calls = %d, want 0 without debtor/proposed_at", len(recorder.calls))
	}
	codes, ok := decision.Metadata["fail_closed_codes"].([]string)
	if !ok || !containsString(codes, "decision_recording_unavailable") {
		t.Fatalf("fail_closed_codes = %#v, want decision_recording_unavailable", decision.Metadata["fail_closed_codes"])
	}
}

func TestGateDelegatesNonSendToolsToFallback(t *testing.T) {
	decider := &spyDecider{}
	fallback := &spyFallback{decision: harness.PermissionDecision{Kind: harness.PermissionDenied, Reason: "fallback denied"}}
	gate := NewGate(Config{TenantID: "tenant-1", ActorID: "agent-1", Decider: decider, Fallback: fallback})

	decision := gate.Decide(context.Background(), harness.ToolCall{Name: "read_case", Input: map[string]any{"case_id": "case-1"}})

	if decision.Kind != harness.PermissionDenied || decision.Reason != "fallback denied" {
		t.Fatalf("decision = %+v, want fallback denied", decision)
	}
	if len(fallback.calls) != 1 {
		t.Fatalf("fallback calls = %d, want 1", len(fallback.calls))
	}
	if len(decider.calls) != 0 {
		t.Fatalf("decider calls = %d, want 0 for non-send tool", len(decider.calls))
	}
}

func TestGateDeniesInvalidSendSchemaBeforeDecider(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{name: "missing required fields", input: map[string]any{"proposal_id": "proposal-1"}},
		{name: "unsupported channel", input: validSendInputWith(func(input map[string]any) { input["channel"] = "fax" })},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decider := &spyDecider{}
			fallback := &spyFallback{decision: harness.PermissionDecision{Kind: harness.PermissionAllowed}}
			gate := NewGate(Config{TenantID: "tenant-1", ActorID: "agent-1", Decider: decider, Fallback: fallback})

			decision := gate.Decide(context.Background(), harness.ToolCall{Name: "send_outbound_utterance", Input: tt.input})

			if decision.Kind != harness.PermissionDenied {
				t.Fatalf("permission kind = %q, want denied", decision.Kind)
			}
			if got := decision.Metadata["fail_closed_codes"]; got == nil {
				t.Fatalf("fail_closed_codes metadata missing: %+v", decision.Metadata)
			}
			if got := decision.Metadata["action_kind"]; got != string(outbound.ActionSendOutboundUtterance) {
				t.Fatalf("metadata action_kind = %v, want send_outbound_utterance", got)
			}
			if got := decision.Metadata["proposal_id"]; tt.input["proposal_id"] != nil && got != tt.input["proposal_id"] {
				t.Fatalf("metadata proposal_id = %v, want %v", got, tt.input["proposal_id"])
			}
			if len(decider.calls) != 0 {
				t.Fatalf("decider calls = %d, want 0 on invalid schema", len(decider.calls))
			}
		})
	}
}

type permissionGateFunc func(context.Context, harness.ToolCall) harness.PermissionDecision

func (f permissionGateFunc) Decide(ctx context.Context, call harness.ToolCall) harness.PermissionDecision {
	return f(ctx, call)
}

type queuedModel struct {
	output harness.ModelOutput
}

func (m *queuedModel) Generate(context.Context, harness.ModelRequest) (harness.ModelOutput, error) {
	return m.output, nil
}

type validatorFunc func(harness.ModelOutput) error

func (f validatorFunc) Validate(output harness.ModelOutput) error { return f(output) }

type spyTool struct {
	calls  int
	result harness.ToolResult
}

func (s *spyTool) Execute(context.Context, harness.ToolCall) (harness.ToolResult, error) {
	s.calls++
	return s.result, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validSendInput() map[string]any {
	return validSendInputWith(func(map[string]any) {})
}

func validSendInputWith(update func(map[string]any)) map[string]any {
	input := map[string]any{
		"proposal_id":    "proposal-1",
		"case_id":        "case-1",
		"debtor_id":      "debtor-1",
		"channel":        string(core.InteractionChannelMessage),
		"recipient_ref":  "recipient-1",
		"text":           "Buen día, le comparto información de su cuenta.",
		"proposed_at":    time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"payment_target": "creditor",
	}
	update(input)
	return input
}
