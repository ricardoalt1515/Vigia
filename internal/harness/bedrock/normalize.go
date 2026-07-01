package bedrock

import (
	"encoding/json"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// anthropicVersion pins the Bedrock Claude Messages API request shape this adapter builds.
const anthropicVersion = "bedrock-2023-05-31"

// systemEnvelopeInstruction is the adapter-owned system prompt that asks Claude to reply with a
// strict JSON envelope. It never leaves this package; caseflow and Domain Agent code never see it.
const systemEnvelopeInstruction = `You are a Vigía Domain Agent running inside a deterministic workflow. ` +
	`Reply with a single strict JSON object and nothing else, matching exactly this shape: ` +
	`{"plan":"","tool_call":{"name":"","input":{}},"final_output":""}. ` +
	`Use "plan" for your visible reasoning, "tool_call" when you need to invoke a tool (leave "name" ` +
	`empty otherwise), and "final_output" for your finished answer when no further tool call is needed.`

// Usage carries token accounting extracted from a Bedrock Claude response.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// claudeMessage is one entry in the Bedrock Claude Messages API "messages" array.
type claudeMessage struct {
	Role    string              `json:"role"`
	Content []claudeContentText `json:"content"`
}

// claudeContentText is a single text content block inside a Claude message.
type claudeContentText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// claudeRequestBody is the Bedrock Claude Messages API request shape this adapter builds.
type claudeRequestBody struct {
	AnthropicVersion string          `json:"anthropic_version"`
	MaxTokens        int             `json:"max_tokens"`
	System           string          `json:"system"`
	Messages         []claudeMessage `json:"messages"`
}

// buildRequestBody builds a valid Bedrock Claude Messages API request from a ModelRequest's Input,
// carrying the adapter-owned system envelope instruction.
func buildRequestBody(input string, maxTokens int) ([]byte, error) {
	req := claudeRequestBody{
		AnthropicVersion: anthropicVersion,
		MaxTokens:        maxTokens,
		System:           systemEnvelopeInstruction,
		Messages: []claudeMessage{
			{
				Role: "user",
				Content: []claudeContentText{
					{Type: "text", Text: input},
				},
			},
		},
	}
	return json.Marshal(req)
}

// claudeResponseBody is the Bedrock Claude Messages API response shape this adapter parses.
type claudeResponseBody struct {
	Content []claudeContentText `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// modelOutputEnvelope is the strict JSON reply shape this adapter instructs Claude to produce.
type modelOutputEnvelope struct {
	Plan     string `json:"plan"`
	ToolCall struct {
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"tool_call"`
	FinalOutput string `json:"final_output"`
}

// parseResponse decodes a Bedrock Claude Messages API response into harness.ModelOutput and Usage.
// It first concatenates all text content blocks, then attempts a strict envelope unmarshal; on
// failure it falls back to treating the whole concatenated text as FinalOutput.
func parseResponse(body []byte) (harness.ModelOutput, Usage, error) {
	var resp claudeResponseBody
	if err := json.Unmarshal(body, &resp); err != nil {
		return harness.ModelOutput{}, Usage{}, err
	}

	var text string
	for _, block := range resp.Content {
		text += block.Text
	}

	usage := Usage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}

	var envelope modelOutputEnvelope
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		// Fallback: non-envelope text becomes FinalOutput wholesale.
		return harness.ModelOutput{FinalOutput: text}, usage, nil
	}

	out := harness.ModelOutput{
		Plan:        envelope.Plan,
		FinalOutput: envelope.FinalOutput,
	}
	if envelope.ToolCall.Name != "" {
		out.ToolCall = &harness.ToolCall{
			Name:  envelope.ToolCall.Name,
			Input: envelope.ToolCall.Input,
		}
	}
	return out, usage, nil
}
