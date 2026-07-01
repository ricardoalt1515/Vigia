package bedrock

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/ricardoalt1515/vigia/internal/harness"
	"github.com/ricardoalt1515/vigia/internal/harness/caseflow"
)

// noopGate always allows any proposed tool call. It is never exercised on the failure path
// tested here (the failing agent's Generate call errors before any tool is proposed), but a
// non-nil PermissionGate is still required to build a caseflow.Orchestrator.
type noopGate struct{}

func (noopGate) Decide(_ context.Context, _ harness.ToolCall) harness.PermissionDecision {
	return harness.PermissionDecision{Kind: harness.PermissionAllowed}
}

// TestOrchestrator_BedrockAdapterErrorReachesFailureReason wires a real *bedrock.Provider —
// backed by a fake invoker returning a Bedrock SDK throttling exception — into a real
// caseflow.Orchestrator run, and asserts the normalized adapter error surfaces on
// CaseBrief.FailureReason.
//
// This closes a coverage gap left by two isolated test suites: provider_test.go proves error
// normalization at the Provider level alone, and caseflow's own tests prove generic error
// propagation using non-bedrock fakes. Neither proves the two compose: that a Bedrock SDK
// exception raised by Provider.Generate propagates through harness.Runtime.RunStep and
// caseflow.runAgent all the way to CaseBrief.FailureReason, with the raw AWS SDK exception
// type and message never crossing the adapter boundary.
//
// No live AWS SDK client or network call is made — client is a fakeInvoker.
//
// Satisfies: openspec/changes/issue-22-bedrock-claude-provider/specs/harness-bedrock-provider/
// spec.md § "Normalized adapter errors reach the orchestrator failure-reason path".
func TestOrchestrator_BedrockAdapterErrorReachesFailureReason(t *testing.T) {
	const failingAgent = "PolicyExplainer" // first agent in caseflow.AllAgentDefinitions() order

	fake := &fakeInvoker{
		fn: func(_ context.Context, _ *bedrockruntime.InvokeModelInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			return nil, &types.ThrottlingException{Message: aws("too many requests")}
		},
	}

	bedrockProvider := &Provider{
		client:    fake,
		modelID:   "anthropic.claude-3-sonnet",
		maxTokens: 256,
		agentName: failingAgent,
	}

	// Only failingAgent's Generate must ever be invoked: the orchestrator stops at the first
	// agent failure, so no later agent's factory slot should be requested or called.
	factory := func(agentName string) harness.ModelProvider {
		if agentName != failingAgent {
			t.Fatalf("factory called for agent %q; only %q should run in this scenario", agentName, failingAgent)
		}
		return bedrockProvider
	}

	orch, err := caseflow.NewOrchestrator(factory, harness.ToolRegistry{}, noopGate{}, caseflow.AllAgentDefinitions())
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	brief, err := orch.Run(context.Background(), "CASE-SYN-001")
	if err != nil {
		t.Fatalf("Run: unexpected transport error: %v", err)
	}

	if brief.Status != caseflow.CaseStatusIncomplete {
		t.Errorf("Status = %q, want %q", brief.Status, caseflow.CaseStatusIncomplete)
	}
	if brief.FailedAgent != failingAgent {
		t.Errorf("FailedAgent = %q, want %q", brief.FailedAgent, failingAgent)
	}
	if !strings.Contains(brief.FailureReason, ErrThrottled.Error()) {
		t.Errorf("FailureReason = %q, want it to contain normalized adapter message %q", brief.FailureReason, ErrThrottled.Error())
	}
	if strings.Contains(brief.FailureReason, "ThrottlingException") {
		t.Errorf("FailureReason leaks raw AWS SDK exception type: %q", brief.FailureReason)
	}
	if strings.Contains(brief.FailureReason, "too many requests") {
		t.Errorf("FailureReason leaks raw AWS SDK exception message: %q", brief.FailureReason)
	}
}
