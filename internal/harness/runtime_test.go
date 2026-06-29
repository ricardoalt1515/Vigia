package harness

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type queuedModelProvider struct {
	outputs []ModelOutput
	calls   int
}

func (p *queuedModelProvider) Generate(ctx context.Context, request ModelRequest) (ModelOutput, error) {
	p.calls++
	if len(p.outputs) == 0 {
		return ModelOutput{}, errors.New("unexpected model call")
	}
	out := p.outputs[0]
	p.outputs = p.outputs[1:]
	return out, nil
}

type gateFunc func(context.Context, ToolCall) PermissionDecision

func (f gateFunc) Decide(ctx context.Context, call ToolCall) PermissionDecision { return f(ctx, call) }

type validatorFunc func(ModelOutput) error

func (f validatorFunc) Validate(out ModelOutput) error { return f(out) }

type spyTool struct {
	calls  int
	result ToolResult
}

func (t *spyTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	t.calls++
	return t.result, nil
}

func alwaysValid(ModelOutput) error { return nil }

func eventTypes(events []Event) []EventType {
	types := make([]EventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func countEvents(events []Event, typ EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == typ {
			count++
		}
	}
	return count
}

func TestRunStepAllowedReadToolRecordsEvents(t *testing.T) {
	model := &queuedModelProvider{outputs: []ModelOutput{{
		Plan: "Inspect the case summary before answering.",
		ToolCall: &ToolCall{
			Name:  "read_case",
			Input: map[string]any{"case_id": "case-123"},
		},
	}}}
	tool := &spyTool{result: ToolResult{Status: ToolStatusSuccess, Output: map[string]any{"summary": "ready"}}}
	runtime := Runtime{
		Model: model,
		Tools: ToolRegistry{"read_case": tool},
		Permissions: gateFunc(func(ctx context.Context, call ToolCall) PermissionDecision {
			return PermissionDecision{Kind: PermissionAllowed, Reason: "read-only tool"}
		}),
		Validator: validatorFunc(alwaysValid),
		Budget:    Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
	}

	result, err := runtime.RunStep(context.Background(), StepInput{Input: "summarize case"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if result.Status != StepStatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, StepStatusCompleted)
	}
	if tool.calls != 1 {
		t.Fatalf("tool calls = %d, want 1", tool.calls)
	}
	if result.ToolResult.Status != ToolStatusSuccess {
		t.Fatalf("tool result status = %q, want %q", result.ToolResult.Status, ToolStatusSuccess)
	}
	wantEvents := []EventType{EventAgentStarted, EventPlanCreated, EventToolProposed, EventPermissionDecision, EventToolResult, EventAgentCompleted}
	if got := eventTypes(result.Events); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("events = %#v, want %#v", got, wantEvents)
	}
}

func TestRunStepDeniedToolDoesNotExecute(t *testing.T) {
	model := &queuedModelProvider{outputs: []ModelOutput{{ToolCall: &ToolCall{Name: "approve_case"}}}}
	tool := &spyTool{result: ToolResult{Status: ToolStatusSuccess}}
	runtime := Runtime{
		Model: model,
		Tools: ToolRegistry{"approve_case": tool},
		Permissions: gateFunc(func(ctx context.Context, call ToolCall) PermissionDecision {
			return PermissionDecision{Kind: PermissionDenied, Reason: "authority-bearing tool"}
		}),
		Validator: validatorFunc(alwaysValid),
		Budget:    Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
	}

	result, err := runtime.RunStep(context.Background(), StepInput{Input: "approve"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if tool.calls != 0 {
		t.Fatalf("tool calls = %d, want 0", tool.calls)
	}
	if result.Status != StepStatusPermissionDenied {
		t.Fatalf("status = %q, want %q", result.Status, StepStatusPermissionDenied)
	}
	if result.ToolResult.Status != ToolStatusDenied {
		t.Fatalf("tool result status = %q, want %q", result.ToolResult.Status, ToolStatusDenied)
	}
	if result.ToolResult.Reason != "authority-bearing tool" {
		t.Fatalf("tool result reason = %q", result.ToolResult.Reason)
	}
}

func TestRunStepApprovalRequiredToolDoesNotExecute(t *testing.T) {
	model := &queuedModelProvider{outputs: []ModelOutput{{ToolCall: &ToolCall{Name: "send_notice"}}}}
	tool := &spyTool{result: ToolResult{Status: ToolStatusSuccess}}
	runtime := Runtime{
		Model: model,
		Tools: ToolRegistry{"send_notice": tool},
		Permissions: gateFunc(func(ctx context.Context, call ToolCall) PermissionDecision {
			return PermissionDecision{Kind: PermissionApprovalRequired, Reason: "human approval required"}
		}),
		Validator: validatorFunc(alwaysValid),
		Budget:    Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
	}

	result, err := runtime.RunStep(context.Background(), StepInput{Input: "notify"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if tool.calls != 0 {
		t.Fatalf("tool calls = %d, want 0", tool.calls)
	}
	if result.Status != StepStatusApprovalRequired {
		t.Fatalf("status = %q, want %q", result.Status, StepStatusApprovalRequired)
	}
	if result.ToolResult.Status != ToolStatusApprovalRequired {
		t.Fatalf("tool result status = %q, want %q", result.ToolResult.Status, ToolStatusApprovalRequired)
	}
	if result.ToolResult.Reason != "human approval required" {
		t.Fatalf("tool result reason = %q", result.ToolResult.Reason)
	}
}

func TestRunStepInvalidOutputFailsWithoutRepair(t *testing.T) {
	validationErr := errors.New("missing final output or tool call")
	runtime := Runtime{
		Model: &queuedModelProvider{outputs: []ModelOutput{{Plan: "invalid by policy"}}},
		Permissions: gateFunc(func(ctx context.Context, call ToolCall) PermissionDecision {
			return PermissionDecision{Kind: PermissionAllowed}
		}),
		Validator: validatorFunc(func(out ModelOutput) error { return validationErr }),
		Budget:    Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
	}

	result, err := runtime.RunStep(context.Background(), StepInput{Input: "invalid"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if result.Status != StepStatusValidationFailed {
		t.Fatalf("status = %q, want %q", result.Status, StepStatusValidationFailed)
	}
	if result.FinalOutput != "" || result.ToolResult.Status != "" {
		t.Fatalf("runtime repaired invalid output: final=%q tool=%q", result.FinalOutput, result.ToolResult.Status)
	}
	if countEvents(result.Events, EventValidationFailure) != 1 {
		t.Fatalf("validation_failure events = %d, want 1", countEvents(result.Events, EventValidationFailure))
	}
}

func TestRunStepValidationRetryOnce(t *testing.T) {
	model := &queuedModelProvider{outputs: []ModelOutput{
		{Plan: "invalid first response"},
		{FinalOutput: "valid answer"},
	}}
	attempt := 0
	runtime := Runtime{
		Model: model,
		Permissions: gateFunc(func(ctx context.Context, call ToolCall) PermissionDecision {
			return PermissionDecision{Kind: PermissionAllowed}
		}),
		Validator: validatorFunc(func(out ModelOutput) error {
			attempt++
			if attempt == 1 {
				return errors.New("invalid first response")
			}
			return nil
		}),
		Budget: Budget{MaxModelAttempts: 2, MaxToolCalls: 1},
	}

	result, err := runtime.RunStep(context.Background(), StepInput{Input: "retry"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if model.calls != 2 {
		t.Fatalf("model calls = %d, want 2", model.calls)
	}
	if result.Status != StepStatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, StepStatusCompleted)
	}
	if result.FinalOutput != "valid answer" {
		t.Fatalf("final output = %q, want valid answer", result.FinalOutput)
	}
	if countEvents(result.Events, EventValidationFailure) != 1 {
		t.Fatalf("validation_failure events = %d, want 1", countEvents(result.Events, EventValidationFailure))
	}
}

func TestRunStepModelBudgetExceeded(t *testing.T) {
	model := &queuedModelProvider{outputs: []ModelOutput{{FinalOutput: "should not be requested"}}}
	runtime := Runtime{
		Model: model,
		Permissions: gateFunc(func(ctx context.Context, call ToolCall) PermissionDecision {
			return PermissionDecision{Kind: PermissionAllowed}
		}),
		Validator: validatorFunc(alwaysValid),
		Budget:    Budget{MaxModelAttempts: 0, MaxToolCalls: 1},
	}

	result, err := runtime.RunStep(context.Background(), StepInput{Input: "budget"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if model.calls != 0 {
		t.Fatalf("model calls = %d, want 0", model.calls)
	}
	if result.Status != StepStatusBudgetExceeded {
		t.Fatalf("status = %q, want %q", result.Status, StepStatusBudgetExceeded)
	}
	if countEvents(result.Events, EventBudgetExceeded) != 1 {
		t.Fatalf("budget_exceeded events = %d, want 1", countEvents(result.Events, EventBudgetExceeded))
	}
}

func TestRunStepToolBudgetExceeded(t *testing.T) {
	model := &queuedModelProvider{outputs: []ModelOutput{{ToolCall: &ToolCall{Name: "read_case"}}}}
	tool := &spyTool{result: ToolResult{Status: ToolStatusSuccess}}
	runtime := Runtime{
		Model: model,
		Tools: ToolRegistry{"read_case": tool},
		Permissions: gateFunc(func(ctx context.Context, call ToolCall) PermissionDecision {
			return PermissionDecision{Kind: PermissionAllowed, Reason: "allowed read"}
		}),
		Validator: validatorFunc(alwaysValid),
		Budget:    Budget{MaxModelAttempts: 1, MaxToolCalls: 0},
	}

	result, err := runtime.RunStep(context.Background(), StepInput{Input: "read"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if tool.calls != 0 {
		t.Fatalf("tool calls = %d, want 0", tool.calls)
	}
	if result.Status != StepStatusBudgetExceeded {
		t.Fatalf("status = %q, want %q", result.Status, StepStatusBudgetExceeded)
	}
	if countEvents(result.Events, EventBudgetExceeded) != 1 {
		t.Fatalf("budget_exceeded events = %d, want 1", countEvents(result.Events, EventBudgetExceeded))
	}
}
