package bedrock

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/ricardoalt1515/vigia/internal/harness"
)

func TestProviderGenerate_Success(t *testing.T) {
	envelope := `{"plan":"","tool_call":{"name":"","input":{}},"final_output":"all good"}`
	respBody := fakeClaudeResponseBody(t, envelope, 3, 4)

	var recordedUsage Usage
	var recordedAgent string
	reporterCalled := false

	fake := &fakeInvoker{
		fn: func(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			return &bedrockruntime.InvokeModelOutput{Body: respBody}, nil
		},
	}

	p := &Provider{
		client:    fake,
		modelID:   "anthropic.claude-3-sonnet",
		maxTokens: 256,
		agentName: "triage_agent",
		reportUsage: func(agentName string, usage Usage) {
			reporterCalled = true
			recordedAgent = agentName
			recordedUsage = usage
		},
	}

	out, err := p.Generate(context.Background(), harness.ModelRequest{Input: "assess this case"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if out.FinalOutput != "all good" {
		t.Errorf("FinalOutput = %q, want %q", out.FinalOutput, "all good")
	}
	if !reporterCalled {
		t.Fatal("usage reporter was not called")
	}
	if recordedAgent != "triage_agent" {
		t.Errorf("reporter agent = %q, want %q", recordedAgent, "triage_agent")
	}
	if recordedUsage != (Usage{InputTokens: 3, OutputTokens: 4}) {
		t.Errorf("reporter usage = %+v, want {3 4}", recordedUsage)
	}
}

func TestProviderGenerate_NoReporterConfigured(t *testing.T) {
	envelope := `{"plan":"","tool_call":{"name":"","input":{}},"final_output":"fine"}`
	respBody := fakeClaudeResponseBody(t, envelope, 1, 1)

	fake := &fakeInvoker{
		fn: func(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			return &bedrockruntime.InvokeModelOutput{Body: respBody}, nil
		},
	}

	p := &Provider{
		client:    fake,
		modelID:   "anthropic.claude-3-sonnet",
		maxTokens: 256,
		agentName: "triage_agent",
		// reportUsage intentionally nil.
	}

	out, err := p.Generate(context.Background(), harness.ModelRequest{Input: "assess this case"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if out.FinalOutput != "fine" {
		t.Errorf("FinalOutput = %q, want %q", out.FinalOutput, "fine")
	}
}

func TestProviderGenerate_SDKError(t *testing.T) {
	fake := &fakeInvoker{
		fn: func(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			return nil, &types.ThrottlingException{Message: aws("slow down")}
		},
	}

	p := &Provider{
		client:    fake,
		modelID:   "anthropic.claude-3-sonnet",
		maxTokens: 256,
		agentName: "triage_agent",
	}

	_, err := p.Generate(context.Background(), harness.ModelRequest{Input: "assess this case"})
	if err == nil {
		t.Fatal("Generate() error = nil, want non-nil normalized error")
	}
	if !errors.Is(err, ErrThrottled) {
		t.Errorf("Generate() error = %v, want errors.Is match for ErrThrottled", err)
	}
}
