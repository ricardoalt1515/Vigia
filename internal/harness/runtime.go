package harness

import (
	"context"
	"errors"
)

// Runtime coordinates one deterministic synthetic agent step.
type Runtime struct {
	Model       ModelProvider
	Tools       ToolRegistry
	Permissions PermissionGate
	Validator   Validator
	Budget      Budget
}

// StepInput carries the visible input for one synthetic runtime step.
type StepInput struct {
	Input string
}

// StepStatus classifies the final state of one runtime step.
type StepStatus string

const (
	StepStatusCompleted        StepStatus = "completed"
	StepStatusValidationFailed StepStatus = "validation_failed"
	StepStatusBudgetExceeded   StepStatus = "budget_exceeded"
	StepStatusPermissionDenied StepStatus = "permission_denied"
	StepStatusApprovalRequired StepStatus = "approval_required"
	StepStatusToolNotFound     StepStatus = "tool_not_found"
)

// StepResult exposes structured state and the accumulated event log.
type StepResult struct {
	Status      StepStatus
	FinalOutput string
	ToolResult  ToolResult
	Events      []Event
}

// RunStep executes one synthetic agent step with explicit validation, permission, and budget gates.
func (r Runtime) RunStep(ctx context.Context, input StepInput) (StepResult, error) {
	if r.Model == nil {
		return StepResult{}, errors.New("harness runtime requires a model provider")
	}
	if r.Validator == nil {
		return StepResult{}, errors.New("harness runtime requires a validator")
	}

	result := StepResult{}
	events := eventRecorder{events: []Event{{Type: EventAgentStarted, Data: map[string]any{"input": input.Input}}}}

	attempts := 0
	for {
		if attempts >= r.Budget.MaxModelAttempts {
			events.add(EventBudgetExceeded, map[string]any{"resource": "model_attempts", "limit": r.Budget.MaxModelAttempts})
			result.Status = StepStatusBudgetExceeded
			result.Events = events.events
			return result, nil
		}
		attempts++

		output, err := r.Model.Generate(ctx, ModelRequest{Input: input.Input})
		if err != nil {
			return StepResult{Events: events.events}, err
		}

		if err := r.Validator.Validate(output); err != nil {
			events.add(EventValidationFailure, map[string]any{"error": err.Error(), "attempt": attempts})
			if attempts >= r.Budget.MaxModelAttempts {
				result.Status = StepStatusValidationFailed
				result.Events = events.events
				return result, nil
			}
			continue
		}

		if output.Plan != "" {
			events.add(EventPlanCreated, map[string]any{"plan": output.Plan})
		}
		if output.ToolCall != nil {
			return r.evaluateTool(ctx, *output.ToolCall, &events)
		}

		result.Status = StepStatusCompleted
		result.FinalOutput = output.FinalOutput
		events.add(EventAgentCompleted, map[string]any{"status": string(result.Status)})
		result.Events = events.events
		return result, nil
	}
}

func (r Runtime) evaluateTool(ctx context.Context, call ToolCall, events *eventRecorder) (StepResult, error) {
	events.add(EventToolProposed, map[string]any{"tool": call.Name})
	if r.Permissions == nil {
		return StepResult{Events: events.events}, errors.New("harness runtime requires a permission gate for tool calls")
	}

	decision := r.Permissions.Decide(ctx, call)
	events.add(EventPermissionDecision, map[string]any{"tool": call.Name, "decision": string(decision.Kind), "reason": decision.Reason})

	switch decision.Kind {
	case PermissionDenied:
		toolResult := ToolResult{Status: ToolStatusDenied, Reason: decision.Reason}
		events.add(EventToolResult, map[string]any{"tool": call.Name, "status": string(toolResult.Status), "reason": toolResult.Reason})
		return StepResult{Status: StepStatusPermissionDenied, ToolResult: toolResult, Events: events.events}, nil
	case PermissionApprovalRequired:
		toolResult := ToolResult{Status: ToolStatusApprovalRequired, Reason: decision.Reason}
		events.add(EventToolResult, map[string]any{"tool": call.Name, "status": string(toolResult.Status), "reason": toolResult.Reason})
		return StepResult{Status: StepStatusApprovalRequired, ToolResult: toolResult, Events: events.events}, nil
	case PermissionAllowed:
		// continue below
	default:
		toolResult := ToolResult{Status: ToolStatusDenied, Reason: "unknown permission decision"}
		events.add(EventToolResult, map[string]any{"tool": call.Name, "status": string(toolResult.Status), "reason": toolResult.Reason})
		return StepResult{Status: StepStatusPermissionDenied, ToolResult: toolResult, Events: events.events}, nil
	}

	if r.Budget.MaxToolCalls <= 0 {
		events.add(EventBudgetExceeded, map[string]any{"resource": "tool_calls", "limit": r.Budget.MaxToolCalls, "tool": call.Name})
		return StepResult{Status: StepStatusBudgetExceeded, Events: events.events}, nil
	}

	tool, ok := r.Tools[call.Name]
	if !ok {
		toolResult := ToolResult{Status: ToolStatusNotFound, Reason: "tool not found"}
		events.add(EventToolResult, map[string]any{"tool": call.Name, "status": string(toolResult.Status), "reason": toolResult.Reason})
		return StepResult{Status: StepStatusToolNotFound, ToolResult: toolResult, Events: events.events}, nil
	}

	toolResult, err := tool.Execute(ctx, call)
	if err != nil {
		return StepResult{Events: events.events}, err
	}
	events.add(EventToolResult, map[string]any{"tool": call.Name, "status": string(toolResult.Status), "reason": toolResult.Reason})
	events.add(EventAgentCompleted, map[string]any{"status": string(StepStatusCompleted)})
	return StepResult{Status: StepStatusCompleted, ToolResult: toolResult, Events: events.events}, nil
}

type eventRecorder struct {
	events []Event
}

func (r *eventRecorder) add(typ EventType, data map[string]any) {
	r.events = append(r.events, Event{Type: typ, Data: data})
}
