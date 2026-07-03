package judge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ricardoalt1515/vigia/internal/judge"
)

// capturedRequest is the minimal shape of an outgoing Anthropic /v1/messages
// request body this test suite inspects.
type capturedRequest struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	System      []struct {
		Type         string `json:"type"`
		Text         string `json:"text"`
		CacheControl *struct {
			Type string `json:"type"`
		} `json:"cache_control"`
	} `json:"system"`
	Tools []struct {
		Name         string `json:"name"`
		CacheControl *struct {
			Type string `json:"type"`
		} `json:"cache_control"`
		InputSchema map[string]any `json:"input_schema"`
	} `json:"tools"`
	ToolChoice struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"tool_choice"`
	Messages []struct {
		Role    string `json:"role"`
		Content []struct {
			Type         string `json:"type"`
			Text         string `json:"text"`
			CacheControl *struct {
				Type string `json:"type"`
			} `json:"cache_control"`
		} `json:"content"`
	} `json:"messages"`
}

// fakeRoundTripper intercepts the SDK's real HTTP request marshaling and
// returns a canned response, without ever hitting the network. respond is
// called once per attempt (so retry tests can vary the response by call
// count).
type fakeRoundTripper struct {
	calls    int32
	captured []capturedRequest
	respond  func(callNum int, req capturedRequest) (*http.Response, error)
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	n := atomic.AddInt32(&f.calls, 1)

	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
	}

	var captured capturedRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &captured); err != nil {
			return nil, err
		}
	}
	f.captured = append(f.captured, captured)

	return f.respond(int(n), captured)
}

func jsonResponse(status int, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func validToolUseMessage(input any) map[string]any {
	inputJSON, _ := json.Marshal(input)
	return map[string]any{
		"id":   "msg_test",
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{
				"type":  "tool_use",
				"id":    "toolu_test",
				"name":  "record_verdict",
				"input": json.RawMessage(inputJSON),
			},
		},
		"model":         string(anthropic.ModelClaudeHaiku4_5_20251001),
		"stop_reason":   "tool_use",
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":                10,
			"output_tokens":               10,
			"cache_read_input_tokens":     5,
			"cache_creation_input_tokens": 0,
		},
	}
}

func newTestAnthropicJudge(t *testing.T, rt http.RoundTripper) *judge.AnthropicJudge {
	t.Helper()
	return judge.NewAnthropicJudge(
		"test-api-key",
		anthropic.ModelClaudeHaiku4_5_20251001,
		0.75,
		option.WithHTTPClient(&http.Client{Transport: rt}),
	)
}

func testInput() judge.JudgeInput {
	return judge.JudgeInput{
		Utterances: []judge.Utterance{
			{Speaker: "agent", Text: "Le recordamos su pago pendiente."},
		},
		Rubric: judge.LoadRubric(),
	}
}

// TestAnthropicJudgeBuildsRequestAtTemperatureZeroWithPinnedModel covers
// *Anthropic request is built at temperature 0 with a pinned model*.
func TestAnthropicJudgeBuildsRequestAtTemperatureZeroWithPinnedModel(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(_ int, _ capturedRequest) (*http.Response, error) {
			return jsonResponse(200, validToolUseMessage(map[string]any{
				"outcome": "pass", "confidence": 0.9, "rationale": "neutral reminder",
			}))
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	_, err := aj.Evaluate(context.Background(), testInput())
	if err != nil {
		t.Fatalf("Evaluate returned unexpected error: %v", err)
	}
	if len(rt.captured) == 0 {
		t.Fatal("no request was captured")
	}
	req := rt.captured[0]
	if req.Temperature != 0 {
		t.Fatalf("request temperature = %v, want 0", req.Temperature)
	}
	if req.Model != string(anthropic.ModelClaudeHaiku4_5_20251001) {
		t.Fatalf("request model = %q, want pinned constant %q", req.Model, anthropic.ModelClaudeHaiku4_5_20251001)
	}
}

// TestAnthropicJudgeCachesStablePrefixNotTranscript covers *Stable prefix
// carries cache_control; transcript does not*.
func TestAnthropicJudgeCachesStablePrefixNotTranscript(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(_ int, _ capturedRequest) (*http.Response, error) {
			return jsonResponse(200, validToolUseMessage(map[string]any{
				"outcome": "pass", "confidence": 0.9, "rationale": "neutral reminder",
			}))
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	_, err := aj.Evaluate(context.Background(), testInput())
	if err != nil {
		t.Fatalf("Evaluate returned unexpected error: %v", err)
	}
	req := rt.captured[0]

	// System instructions and the rubric are sent as two separate cached
	// text blocks (not concatenated into one string), and neither embeds
	// the transcript: the transcript is the volatile, later, uncached
	// message content block asserted below.
	if len(req.System) != 2 {
		t.Fatalf("len(system) = %d, want 2 (instructions block + rubric block)", len(req.System))
	}
	for i, block := range req.System {
		if block.CacheControl == nil {
			t.Fatalf("system block %d has no cache_control, want ephemeral", i)
		}
		if strings.Contains(block.Text, "<transcript>") {
			t.Fatalf("system block %d contains <transcript>; the transcript must never be embedded in the system prompt: %q", i, block.Text)
		}
	}
	rubric := judge.LoadRubric()
	if req.System[1].Text != rubric.Prompt {
		t.Fatalf("system block 1 = %q, want the rubric prompt text", req.System[1].Text)
	}

	if len(req.Tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(req.Tools))
	}
	if req.Tools[0].CacheControl == nil {
		t.Fatal("record_verdict tool has no cache_control, want ephemeral")
	}
	if req.Tools[0].Name != "record_verdict" {
		t.Fatalf("tool name = %q, want record_verdict", req.Tools[0].Name)
	}

	if len(req.Messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(req.Messages))
	}
	msg := req.Messages[0]
	if len(msg.Content) != 1 {
		t.Fatalf("len(message content) = %d, want 1 (the transcript block)", len(msg.Content))
	}
	transcriptBlock := msg.Content[0]
	if transcriptBlock.CacheControl != nil {
		t.Fatal("transcript content block carries cache_control, want none (it is the volatile part)")
	}
	if !strings.Contains(transcriptBlock.Text, "<transcript>") {
		t.Fatalf("transcript block does not contain <transcript>: %q", transcriptBlock.Text)
	}
}

// TestAnthropicJudgeForcesRecordVerdictToolChoice covers the tool_choice
// assertion from *Anthropic request is built at temperature 0 with a pinned
// model* / the request-construction scenarios: tool_choice MUST force
// record_verdict.
func TestAnthropicJudgeForcesRecordVerdictToolChoice(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(_ int, _ capturedRequest) (*http.Response, error) {
			return jsonResponse(200, validToolUseMessage(map[string]any{
				"outcome": "pass", "confidence": 0.9, "rationale": "neutral reminder",
			}))
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	_, err := aj.Evaluate(context.Background(), testInput())
	if err != nil {
		t.Fatalf("Evaluate returned unexpected error: %v", err)
	}
	req := rt.captured[0]
	if req.ToolChoice.Type != "tool" || req.ToolChoice.Name != "record_verdict" {
		t.Fatalf("tool_choice = %+v, want {type: tool, name: record_verdict}", req.ToolChoice)
	}
}

func TestAnthropicJudgeMapsValidToolUseToJudgeResult(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(_ int, _ capturedRequest) (*http.Response, error) {
			return jsonResponse(200, validToolUseMessage(map[string]any{
				"outcome": "block", "confidence": 0.9512345, "rationale": "threatening language",
			}))
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	got, err := aj.Evaluate(context.Background(), testInput())
	if err != nil {
		t.Fatalf("Evaluate returned unexpected error: %v", err)
	}
	if got.Outcome != judge.OutcomeBlock {
		t.Fatalf("Outcome = %q, want block", got.Outcome)
	}
	// Confidence determinism: quantized to 4 decimals.
	if got.Confidence != 0.9512 {
		t.Fatalf("Confidence = %v, want 0.9512 (quantized to 4 decimals)", got.Confidence)
	}
	if got.Rationale != "threatening language" {
		t.Fatalf("Rationale = %q, want %q", got.Rationale, "threatening language")
	}
	if got.RubricVersion != judge.RubricVersion {
		t.Fatalf("RubricVersion = %q, want %q", got.RubricVersion, judge.RubricVersion)
	}
	if got.JudgeModelID != string(anthropic.ModelClaudeHaiku4_5_20251001) {
		t.Fatalf("JudgeModelID = %q, want pinned model constant", got.JudgeModelID)
	}
}

func TestAnthropicJudgeMissingToolBlockIsMalformedOutput(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(_ int, _ capturedRequest) (*http.Response, error) {
			return jsonResponse(200, map[string]any{
				"id":   "msg_test",
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{"type": "text", "text": "I cannot comply with tool use."},
				},
				"model":         string(anthropic.ModelClaudeHaiku4_5_20251001),
				"stop_reason":   "end_turn",
				"stop_sequence": nil,
				"usage":         map[string]any{"input_tokens": 1, "output_tokens": 1},
			})
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	got, err := aj.Evaluate(context.Background(), testInput())
	if !errors.Is(err, judge.ErrMalformedOutput) {
		t.Fatalf("error = %v, want wrapping judge.ErrMalformedOutput", err)
	}
	if got.RubricVersion == "" || got.JudgeModelID == "" {
		t.Fatalf("got = %+v, want RubricVersion/JudgeModelID recorded even on malformed output (attempted provenance)", got)
	}
}

func TestAnthropicJudgeSchemaInvalidToolInputIsSchemaInvalid(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(_ int, _ capturedRequest) (*http.Response, error) {
			return jsonResponse(200, validToolUseMessage(map[string]any{
				"outcome": "maybe", "confidence": 0.9, "rationale": "ambiguous",
			}))
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	got, err := aj.Evaluate(context.Background(), testInput())
	if !errors.Is(err, judge.ErrSchemaInvalid) {
		t.Fatalf("error = %v, want wrapping judge.ErrSchemaInvalid", err)
	}
	if got.RubricVersion == "" || got.JudgeModelID == "" {
		t.Fatalf("got = %+v, want RubricVersion/JudgeModelID recorded even on schema-invalid output (attempted provenance)", got)
	}
}

func TestAnthropicJudgeBelowThresholdConfidenceIsLowConfidence(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(_ int, _ capturedRequest) (*http.Response, error) {
			return jsonResponse(200, validToolUseMessage(map[string]any{
				"outcome": "pass", "confidence": 0.5, "rationale": "uncertain",
			}))
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	got, err := aj.Evaluate(context.Background(), testInput())
	if !errors.Is(err, judge.ErrLowConfidence) {
		t.Fatalf("error = %v, want wrapping judge.ErrLowConfidence", err)
	}
	if got.RubricVersion == "" || got.JudgeModelID == "" {
		t.Fatalf("got = %+v, want RubricVersion/JudgeModelID recorded even below the confidence threshold (attempted provenance)", got)
	}
}

// TestAnthropicJudgeRetriesTransientFailureWithinBudget covers *Judge-client
// layer bounds timeout and retry*: a transient 503 within the retry budget
// eventually succeeds.
func TestAnthropicJudgeRetriesTransientFailureWithinBudget(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(callNum int, _ capturedRequest) (*http.Response, error) {
			if callNum < 2 {
				return jsonResponse(503, map[string]any{
					"type":  "error",
					"error": map[string]any{"type": "overloaded_error", "message": "overloaded"},
				})
			}
			return jsonResponse(200, validToolUseMessage(map[string]any{
				"outcome": "pass", "confidence": 0.9, "rationale": "neutral reminder",
			}))
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	got, err := aj.Evaluate(context.Background(), testInput())
	if err != nil {
		t.Fatalf("Evaluate returned unexpected error after transient failures within budget: %v", err)
	}
	if got.Outcome != judge.OutcomePass {
		t.Fatalf("Outcome = %q, want pass", got.Outcome)
	}
	if rt.calls < 2 {
		t.Fatalf("calls = %d, want at least 2 (one retry after the first 503)", rt.calls)
	}
}

// TestAnthropicJudgeGivesUpAfterExhaustingRetryBudget covers the "gives up"
// half of *Judge-client layer bounds timeout and retry*: a failure that
// persists past the bounded retry count returns an error rather than
// retrying indefinitely.
func TestAnthropicJudgeGivesUpAfterExhaustingRetryBudget(t *testing.T) {
	rt := &fakeRoundTripper{
		respond: func(_ int, _ capturedRequest) (*http.Response, error) {
			return jsonResponse(503, map[string]any{
				"type":  "error",
				"error": map[string]any{"type": "overloaded_error", "message": "overloaded"},
			})
		},
	}
	aj := newTestAnthropicJudge(t, rt)

	start := time.Now()
	got, err := aj.Evaluate(context.Background(), testInput())
	elapsed := time.Since(start)

	if !errors.Is(err, judge.ErrTransport) {
		t.Fatalf("error = %v, want wrapping judge.ErrTransport", err)
	}
	if got.RubricVersion == "" || got.JudgeModelID == "" {
		t.Fatalf("got = %+v, want RubricVersion/JudgeModelID recorded even on a transport error (attempted provenance)", got)
	}
	// Bounded: must not spin forever. Generous ceiling for CI jitter.
	if elapsed > 20*time.Second {
		t.Fatalf("Evaluate took %v, want it to give up well within the bounded retry/timeout budget", elapsed)
	}
}
