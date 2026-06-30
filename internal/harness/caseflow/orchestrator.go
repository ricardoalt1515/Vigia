package caseflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// ErrAgentFailed is the sentinel returned internally when an agent cannot produce a valid handoff.
// Callers of Run receive a CaseBrief with CaseStatusIncomplete instead; this error is not exposed.
var ErrAgentFailed = errors.New("agent failed")

// ProviderFactory creates a fresh ModelProvider for each named agent.
type ProviderFactory func(agentName string) harness.ModelProvider

// AgentDefinition is the static configuration for one domain agent.
type AgentDefinition struct {
	Name          string
	Instructions  string
	ToolAllowlist []string
	Budget        harness.Budget // MaxModelAttempts MUST be 1 (enforced by NewOrchestrator guard)
	MaxSteps      int            // outer-loop cap; harness.Budget.MaxModelAttempts is per-RunStep
	Validator     harness.Validator
	DecodeHandoff func(finalOutput string) (HandoffArtifact, error)
}

// validateAgentDefinitions enforces the construction-time guard: every AgentDefinition must have
// Budget.MaxModelAttempts == 1. With >1, RunStep re-sends IDENTICAL input on validation failure with
// no feedback, silently consuming the scripted corrected output and breaking the orchestrator retry
// contract. Forcing 1 ensures RunStep returns ValidationFailed immediately so the orchestrator
// owns the only retry loop (Lock 1 + Lock 3 from design).
func validateAgentDefinitions(defs []AgentDefinition) error {
	for _, def := range defs {
		if def.Budget.MaxModelAttempts != 1 {
			return fmt.Errorf("agent %q: Budget.MaxModelAttempts must be 1, got %d", def.Name, def.Budget.MaxModelAttempts)
		}
	}
	return nil
}

// filterRegistry returns a new ToolRegistry containing only tools whose names appear in allowlist.
// Out-of-allowlist calls reach the runtime's "tool not found" path and return StepStatusToolNotFound.
func filterRegistry(full harness.ToolRegistry, allowlist []string) harness.ToolRegistry {
	filtered := make(harness.ToolRegistry, len(allowlist))
	for _, name := range allowlist {
		if tool, ok := full[name]; ok {
			filtered[name] = tool
		}
	}
	return filtered
}

// buildInput constructs the delimiter-sectioned input string for one RunStep call.
// Tool observations MUST already be JSON-marshaled strings (constraint #4).
func buildInput(instructions, caseID string, priorHandoffs []HandoffArtifact, observations []string, feedback string) string {
	var sb strings.Builder

	sb.WriteString("<instructions>\n")
	sb.WriteString(instructions)
	sb.WriteString("\n</instructions>\n")

	sb.WriteString("<approved_input>\n")
	sb.WriteString("case_id: ")
	sb.WriteString(caseID)
	sb.WriteString("\n</approved_input>\n")

	sb.WriteString("<prior_handoffs>\n")
	if len(priorHandoffs) == 0 {
		sb.WriteString("[]")
	} else {
		// Marshal the slice as JSON. On error, fall back to empty array marker.
		b, err := json.Marshal(priorHandoffs)
		if err != nil {
			sb.WriteString("[]")
		} else {
			sb.Write(b)
		}
	}
	sb.WriteString("\n</prior_handoffs>\n")

	sb.WriteString("<tool_observations>\n")
	for _, obs := range observations {
		sb.WriteString(obs)
		sb.WriteString("\n")
	}
	sb.WriteString("</tool_observations>\n")

	if feedback != "" {
		sb.WriteString("<validation_feedback>\n")
		sb.WriteString(feedback)
		sb.WriteString("\n</validation_feedback>\n")
	}

	return sb.String()
}

// extractFeedback scans events in reverse for the last EventValidationFailure and returns
// its Data["error"] string. Returns empty string if none found.
func extractFeedback(events []harness.Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type == harness.EventValidationFailure {
			if msg, ok := ev.Data["error"].(string); ok {
				return msg
			}
		}
	}
	return ""
}

// runAgent drives one agent through its bounded RunStep loop.
// It returns (handoff, "", nil) on success, or ("", reason, ErrAgentFailed) on failure.
// observations and events are loop-local and never passed to other agents (Lock 7).
func runAgent(ctx context.Context, def AgentDefinition, rt harness.Runtime, caseID string, priorHandoffs []HandoffArtifact) (HandoffArtifact, string, error) {
	var observations []string
	var feedback string
	retried := false

	for step := 0; step < def.MaxSteps; step++ {
		input := buildInput(def.Instructions, caseID, priorHandoffs, observations, feedback)
		result, err := rt.RunStep(ctx, harness.StepInput{Input: input})
		if err != nil {
			return nil, err.Error(), ErrAgentFailed
		}

		switch result.Status {
		case harness.StepStatusCompleted:
			if result.FinalOutput != "" {
				// Synthesis step: decode the handoff artifact.
				handoff, decodeErr := def.DecodeHandoff(result.FinalOutput)
				if decodeErr != nil {
					// Treat decode failure as a validation failure — retry once.
					synthFeedback := decodeErr.Error()
					if retried {
						return nil, synthFeedback, ErrAgentFailed
					}
					retried = true
					feedback = synthFeedback
					step-- // Don't consume this step from the outer cap for the retry.
					continue
				}
				return handoff, "", nil
			}
			// Tool step: marshal ToolResult.Output and accumulate.
			if result.ToolResult.Output != nil {
				marshaled, marshalErr := json.Marshal(result.ToolResult.Output)
				if marshalErr != nil {
					marshaled = []byte("<marshal_error>")
				}
				observations = append(observations, string(marshaled))
			}
			// Continue to next step.

		case harness.StepStatusValidationFailed:
			newFeedback := extractFeedback(result.Events)
			if !retried {
				retried = true
				feedback = newFeedback
				step-- // Retry without consuming the outer cap.
				continue
			}
			// Second failure — stop this agent.
			return nil, newFeedback, ErrAgentFailed

		case harness.StepStatusPermissionDenied:
			return nil, "permission denied: " + result.ToolResult.Reason, ErrAgentFailed

		case harness.StepStatusToolNotFound:
			return nil, "tool not found: " + result.ToolResult.Reason, ErrAgentFailed

		case harness.StepStatusBudgetExceeded:
			return nil, "budget exceeded", ErrAgentFailed

		case harness.StepStatusApprovalRequired:
			return nil, "approval required: " + result.ToolResult.Reason, ErrAgentFailed
		}
	}

	return nil, "max steps exceeded", ErrAgentFailed
}

// Orchestrator runs all agents in fixed order and assembles a CaseBrief.
type Orchestrator struct {
	factory      ProviderFactory
	fullRegistry harness.ToolRegistry
	gate         harness.PermissionGate
	defs         []AgentDefinition
}

// NewOrchestrator constructs an Orchestrator, enforcing the MaxModelAttempts==1 guard on all defs.
func NewOrchestrator(factory ProviderFactory, registry harness.ToolRegistry, gate harness.PermissionGate, defs []AgentDefinition) (*Orchestrator, error) {
	if err := validateAgentDefinitions(defs); err != nil {
		return nil, err
	}
	return &Orchestrator{
		factory:      factory,
		fullRegistry: registry,
		gate:         gate,
		defs:         defs,
	}, nil
}

// Run executes all agent definitions in their fixed declared order.
// Each agent gets a fresh Runtime and isolated context (Lock 7).
// Returns a CaseBrief describing the terminal state.
func (o *Orchestrator) Run(ctx context.Context, caseID string) (CaseBrief, error) {
	brief := CaseBrief{CaseID: caseID}
	var priorHandoffs []HandoffArtifact

	for _, def := range o.defs {
		rt := harness.Runtime{
			Model:       o.factory(def.Name),
			Tools:       filterRegistry(o.fullRegistry, def.ToolAllowlist),
			Permissions: o.gate,
			Validator:   def.Validator,
			Budget:      def.Budget,
		}

		handoff, reason, err := runAgent(ctx, def, rt, caseID, priorHandoffs)
		if err != nil {
			brief.Status = CaseStatusIncomplete
			brief.FailedAgent = def.Name
			brief.FailureReason = reason
			return brief, nil
		}

		brief.Stages = append(brief.Stages, StageEntry{AgentName: def.Name, Handoff: handoff})
		priorHandoffs = append(priorHandoffs, handoff)
	}

	brief.Status = CaseStatusComplete
	return brief, nil
}
