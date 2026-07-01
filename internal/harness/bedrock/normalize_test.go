package bedrock

import (
	"encoding/json"
	"testing"
)

func TestBuildRequestBody(t *testing.T) {
	body, err := buildRequestBody("what is the collections status?", 512)
	if err != nil {
		t.Fatalf("buildRequestBody() error = %v", err)
	}

	var decoded struct {
		AnthropicVersion string `json:"anthropic_version"`
		MaxTokens        int    `json:"max_tokens"`
		System           string `json:"system"`
		Messages         []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(body) error = %v", err)
	}

	if decoded.AnthropicVersion == "" {
		t.Error("anthropic_version is empty, want a Bedrock Claude Messages API version string")
	}
	if decoded.MaxTokens != 512 {
		t.Errorf("max_tokens = %d, want 512", decoded.MaxTokens)
	}
	if decoded.System == "" {
		t.Error("system is empty, want the adapter-owned envelope instruction")
	}
	if len(decoded.Messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(decoded.Messages))
	}
	if decoded.Messages[0].Role != "user" {
		t.Errorf("messages[0].role = %q, want %q", decoded.Messages[0].Role, "user")
	}
	if len(decoded.Messages[0].Content) != 1 || decoded.Messages[0].Content[0].Text != "what is the collections status?" {
		t.Errorf("messages[0].content = %+v, want single text block equal to input", decoded.Messages[0].Content)
	}
}

func fakeClaudeResponseBody(t *testing.T, text string, inputTokens, outputTokens int) []byte {
	t.Helper()
	resp := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal(resp) error = %v", err)
	}
	return b
}

func TestParseResponse(t *testing.T) {
	t.Run("tool call envelope", func(t *testing.T) {
		envelope := `{"plan":"call the lookup tool","tool_call":{"name":"lookup_case","input":{"case_id":"c-1"}},"final_output":""}`
		body := fakeClaudeResponseBody(t, envelope, 10, 20)

		out, usage, err := parseResponse(body)
		if err != nil {
			t.Fatalf("parseResponse() error = %v", err)
		}
		if out.ToolCall == nil {
			t.Fatal("ToolCall is nil, want populated tool call")
		}
		if out.ToolCall.Name != "lookup_case" {
			t.Errorf("ToolCall.Name = %q, want %q", out.ToolCall.Name, "lookup_case")
		}
		if out.ToolCall.Input["case_id"] != "c-1" {
			t.Errorf("ToolCall.Input[case_id] = %v, want %q", out.ToolCall.Input["case_id"], "c-1")
		}
		if out.FinalOutput != "" {
			t.Errorf("FinalOutput = %q, want empty", out.FinalOutput)
		}
		if usage.InputTokens != 10 || usage.OutputTokens != 20 {
			t.Errorf("usage = %+v, want {10 20}", usage)
		}
	})

	t.Run("final output envelope", func(t *testing.T) {
		envelope := `{"plan":"","tool_call":{"name":"","input":{}},"final_output":"case resolved"}`
		body := fakeClaudeResponseBody(t, envelope, 5, 7)

		out, usage, err := parseResponse(body)
		if err != nil {
			t.Fatalf("parseResponse() error = %v", err)
		}
		if out.FinalOutput != "case resolved" {
			t.Errorf("FinalOutput = %q, want %q", out.FinalOutput, "case resolved")
		}
		if out.ToolCall != nil {
			t.Errorf("ToolCall = %+v, want nil", out.ToolCall)
		}
		if usage.InputTokens != 5 || usage.OutputTokens != 7 {
			t.Errorf("usage = %+v, want {5 7}", usage)
		}
	})

	t.Run("non-envelope fallback", func(t *testing.T) {
		body := fakeClaudeResponseBody(t, "just some plain prose, not JSON at all", 1, 2)

		out, usage, err := parseResponse(body)
		if err != nil {
			t.Fatalf("parseResponse() error = %v", err)
		}
		if out.FinalOutput != "just some plain prose, not JSON at all" {
			t.Errorf("FinalOutput = %q, want the whole concatenated text", out.FinalOutput)
		}
		if out.Plan != "" {
			t.Errorf("Plan = %q, want empty (zero-valued)", out.Plan)
		}
		if out.ToolCall != nil {
			t.Errorf("ToolCall = %+v, want nil (zero-valued)", out.ToolCall)
		}
		if usage.InputTokens != 1 || usage.OutputTokens != 2 {
			t.Errorf("usage = %+v, want {1 2}", usage)
		}
	})
}
