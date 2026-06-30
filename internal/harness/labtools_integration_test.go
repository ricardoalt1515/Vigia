// Package harness_test provides integration tests for the harness runtime wired with
// labtools fixtures. Using the external test package avoids the import cycle that arises
// when an internal test file in package harness imports a subpackage that imports harness.
package harness_test

import (
	"context"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
)

// staticModelProvider is a minimal ModelProvider for integration tests.
// It returns a single fixed ModelOutput on the first Generate call.
// It is intentionally simpler than the internal queuedModelProvider;
// it is NOT a duplicate — queuedModelProvider is unexported and queues multiple outputs.
type staticModelProvider struct {
	output harness.ModelOutput
	calls  int
}

func (p *staticModelProvider) Generate(_ context.Context, _ harness.ModelRequest) (harness.ModelOutput, error) {
	p.calls++
	return p.output, nil
}

// alwaysValidFunc is a local Validator implementation that accepts any ModelOutput.
type alwaysValidFunc struct{}

func (alwaysValidFunc) Validate(harness.ModelOutput) error { return nil }

// TestLabtoolsIntegration_AuthorityToolDenied verifies that an authority-class tool call
// (append_evidence) is stopped by the LabPermissionGate before the tool impl executes.
// The runtime must return StepStatusPermissionDenied and ToolStatusDenied.
func TestLabtoolsIntegration_AuthorityToolDenied(t *testing.T) {
	cases, rules, err := labtools.Load()
	if err != nil {
		t.Fatalf("labtools.Load() error: %v", err)
	}

	model := &staticModelProvider{output: harness.ModelOutput{
		ToolCall: &harness.ToolCall{Name: "append_evidence", Input: map[string]any{"case_id": "CASE-SYN-001"}},
	}}

	registry := labtools.Registry(cases, rules)
	gate := labtools.NewLabPermissionGate()
	rt := harness.Runtime{
		Model:       model,
		Tools:       registry,
		Permissions: gate,
		Validator:   alwaysValidFunc{},
		Budget:      harness.Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
	}

	result, err := rt.RunStep(context.Background(), harness.StepInput{Input: "attempt authority action"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if result.Status != harness.StepStatusPermissionDenied {
		t.Fatalf("status = %q, want %q", result.Status, harness.StepStatusPermissionDenied)
	}
	if result.ToolResult.Status != harness.ToolStatusDenied {
		t.Fatalf("tool result status = %q, want %q", result.ToolResult.Status, harness.ToolStatusDenied)
	}
	// Gate must have short-circuited: model was called exactly once, gate stopped execution.
	if model.calls != 1 {
		t.Fatalf("model calls = %d, want 1", model.calls)
	}
}

// TestLabtoolsIntegration_ReadCaseCompletes verifies that a read_case tool call
// is allowed by the LabPermissionGate, executes against the fixture store, and
// returns StepStatusCompleted with ToolStatusSuccess and a non-empty tenant_id in output.
func TestLabtoolsIntegration_ReadCaseCompletes(t *testing.T) {
	cases, rules, err := labtools.Load()
	if err != nil {
		t.Fatalf("labtools.Load() error: %v", err)
	}

	model := &staticModelProvider{output: harness.ModelOutput{
		ToolCall: &harness.ToolCall{Name: "read_case", Input: map[string]any{"case_id": "CASE-SYN-001"}},
	}}

	registry := labtools.Registry(cases, rules)
	gate := labtools.NewLabPermissionGate()
	rt := harness.Runtime{
		Model:       model,
		Tools:       registry,
		Permissions: gate,
		Validator:   alwaysValidFunc{},
		Budget:      harness.Budget{MaxModelAttempts: 1, MaxToolCalls: 1},
	}

	result, err := rt.RunStep(context.Background(), harness.StepInput{Input: "read case data"})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if result.Status != harness.StepStatusCompleted {
		t.Fatalf("status = %q, want %q", result.Status, harness.StepStatusCompleted)
	}
	if result.ToolResult.Status != harness.ToolStatusSuccess {
		t.Fatalf("tool result status = %q, want %q; reason: %s",
			result.ToolResult.Status, harness.ToolStatusSuccess, result.ToolResult.Reason)
	}

	// Output must contain tenant_id
	caseMap, ok := result.ToolResult.Output["case"].(map[string]any)
	if !ok {
		t.Fatalf("output missing 'case' key")
	}
	tenantID, _ := caseMap["tenant_id"].(string)
	if tenantID == "" {
		t.Error("output.case.tenant_id is empty; fixture must carry a non-empty tenant_id")
	}
}
