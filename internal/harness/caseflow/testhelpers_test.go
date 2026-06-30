package caseflow_test

import (
	"context"
	"errors"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// caseflowQueuedProvider is a queued fake ModelProvider for caseflow tests.
// queuedModelProvider in package harness is unexported; this is the caseflow-local equivalent.
type caseflowQueuedProvider struct {
	outputs []harness.ModelOutput
	calls   int
}

func (p *caseflowQueuedProvider) Generate(_ context.Context, _ harness.ModelRequest) (harness.ModelOutput, error) {
	p.calls++
	if len(p.outputs) == 0 {
		return harness.ModelOutput{}, errors.New("unexpected model call: queue empty")
	}
	out := p.outputs[0]
	p.outputs = p.outputs[1:]
	return out, nil
}

// perAgentProvider routes model calls to per-agent queues.
type perAgentProvider struct {
	queues map[string]*caseflowQueuedProvider
}

// factory returns the per-agent queue for the given agent name; panics on unknown name.
func (p *perAgentProvider) factory(name string) harness.ModelProvider {
	q, ok := p.queues[name]
	if !ok {
		panic("perAgentProvider: no queue for agent " + name)
	}
	return q
}

// recordingProvider wraps another provider and records the Input strings it receives.
type recordingProvider struct {
	inputs   []string
	delegate harness.ModelProvider
}

func (r *recordingProvider) Generate(ctx context.Context, req harness.ModelRequest) (harness.ModelOutput, error) {
	r.inputs = append(r.inputs, req.Input)
	return r.delegate.Generate(ctx, req)
}

// gateAll returns a PermissionGate that always returns the given decision kind.
type alwaysGate struct {
	kind harness.PermissionDecisionKind
}

func (g alwaysGate) Decide(_ context.Context, _ harness.ToolCall) harness.PermissionDecision {
	return harness.PermissionDecision{Kind: g.kind}
}

func gateAll(kind harness.PermissionDecisionKind) harness.PermissionGate {
	return alwaysGate{kind: kind}
}

// vFunc adapts a plain function to harness.Validator — test-only mirror of validatorFunc.
type vFuncAdapter struct {
	fn func(harness.ModelOutput) error
}

func (a vFuncAdapter) Validate(out harness.ModelOutput) error { return a.fn(out) }

func vFunc(fn func(harness.ModelOutput) error) harness.Validator {
	return vFuncAdapter{fn: fn}
}

// acceptAll is a Validator that accepts any output.
var acceptAll harness.Validator = vFuncAdapter{fn: func(harness.ModelOutput) error { return nil }}

// noopTool is a Tool that always succeeds and returns a configurable output map.
type noopTool struct{ output map[string]any }

func (t noopTool) Execute(_ context.Context, _ harness.ToolCall) (harness.ToolResult, error) {
	return harness.ToolResult{Status: harness.ToolStatusSuccess, Output: t.output}, nil
}

// fakeRegistry builds a ToolRegistry where each name maps to a noopTool with the given output.
func fakeRegistry(names []string, output map[string]any) harness.ToolRegistry {
	reg := make(harness.ToolRegistry, len(names))
	for _, name := range names {
		reg[name] = noopTool{output: output}
	}
	return reg
}

// allToolsRegistry builds a registry with noopTool (empty output) for the given names.
func allToolsRegistry(names ...string) harness.ToolRegistry {
	return fakeRegistry(names, map[string]any{})
}
