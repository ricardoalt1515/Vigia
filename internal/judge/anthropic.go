package judge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

const (
	// recordVerdictToolName is the forced tool name the Anthropic judge
	// requires a structured verdict through.
	recordVerdictToolName = "record_verdict"

	// perAttemptTimeout is the per-call deadline handed to the SDK via
	// option.WithRequestTimeout: Haiku's p99 for a short transcript is a
	// few seconds, so 8s comfortably covers the tail without letting one
	// hung attempt block the evaluation transaction indefinitely.
	perAttemptTimeout = 8 * time.Second

	// maxRetries bounds the SDK's built-in retry-on-transient-error
	// behavior (429/5xx/timeout) to a small, fixed count — never
	// unbounded.
	maxRetries = 2

	// overallCeiling is the hard ceiling on the whole Evaluate call
	// (all attempts + backoff combined), enforced via a child
	// context.WithTimeout so a residual failure fails closed to HITL
	// quickly rather than hanging the caller's transaction.
	overallCeiling = 15 * time.Second

	// maxTokens caps the model's response; a verdict is a small
	// structured object, never a long completion.
	maxTokens = 1024
)

// AnthropicJudge is the production Judge implementation: it calls the
// official anthropic-sdk-go at temperature 0 with a pinned model, forces a
// structured verdict through the record_verdict tool, and re-validates the
// tool's input against the same JSON schema handed to the model — the
// model's own claims are never trusted.
type AnthropicJudge struct {
	client        anthropic.Client
	modelID       string
	hitlThreshold float64
}

// NewAnthropicJudge constructs an AnthropicJudge. opts are forwarded to
// anthropic.NewClient — tests inject option.WithHTTPClient(&http.Client{
// Transport: fakeRoundTripper}) here to exercise the real SDK's request
// marshaling against a canned response, with no live network call.
func NewAnthropicJudge(apiKey string, modelID anthropic.Model, hitlThreshold float64, opts ...option.RequestOption) *AnthropicJudge {
	clientOpts := append([]option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(maxRetries),
		option.WithRequestTimeout(perAttemptTimeout),
	}, opts...)

	return &AnthropicJudge{
		client:        anthropic.NewClient(clientOpts...),
		modelID:       string(modelID),
		hitlThreshold: hitlThreshold,
	}
}

func (a *AnthropicJudge) Evaluate(ctx context.Context, in JudgeInput) (JudgeResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, overallCeiling)
	defer cancel()

	rubric := in.Rubric
	if rubric.Version == "" {
		rubric = LoadRubric()
	}

	req := a.buildRequest(rubric, in.Utterances)

	msg, err := a.client.Messages.New(ctx, req)
	if err != nil {
		wrapped := fmt.Errorf("%w: %v", ErrTransport, err)
		a.logCall(start, rubric, JudgeResult{}, 0, 0, wrapped)
		return JudgeResult{}, wrapped
	}

	result, err := a.mapResponse(msg, rubric)
	a.logCall(start, rubric, result, msg.Usage.CacheReadInputTokens, msg.Usage.CacheCreationInputTokens, err)
	return result, err
}

// logCall emits the single, fixed-field slog line design.md's observability
// section specifies: no transcript text, no rationale body, no PII.
func (a *AnthropicJudge) logCall(start time.Time, rubric Rubric, result JudgeResult, cacheReadTokens, cacheCreationTokens int64, err error) {
	attrs := []any{
		"code", "MX-REDECO-05",
		"model_id", a.modelID,
		"rubric_version", rubric.Version,
		"latency_ms", time.Since(start).Milliseconds(),
		"cache_read_tokens", cacheReadTokens,
		"cache_creation_tokens", cacheCreationTokens,
	}
	if err != nil {
		attrs = append(attrs, "err", errorTaxonomyLabel(err))
	} else {
		attrs = append(attrs,
			"outcome", string(result.Outcome),
			"confidence", fmt.Sprintf("%.4f", result.Confidence),
			"requires_hitl", result.Outcome == OutcomeBlock,
			"err", "",
		)
	}
	slog.Info("judge.call", attrs...)
}

// errorTaxonomyLabel maps a judge error to its fail-closed taxonomy label
// for the observability line, without leaking the underlying transport
// error's message (which may embed request details).
func errorTaxonomyLabel(err error) string {
	switch {
	case errors.Is(err, ErrTransport):
		return "transport"
	case errors.Is(err, ErrLowConfidence):
		return "low_confidence"
	case errors.Is(err, ErrSchemaInvalid):
		return "schema_invalid"
	case errors.Is(err, ErrMalformedOutput):
		return "malformed_output"
	default:
		return "unknown"
	}
}

// buildRequest assembles the Anthropic request: the stable prefix (system
// instructions + rubric + the record_verdict tool schema) carries
// cache_control on every block per ADR-10; the volatile transcript is the
// last, uncached content block per ADR-11 (never concatenated into the
// system/instruction portion).
func (a *AnthropicJudge) buildRequest(rubric Rubric, utterances []Utterance) anthropic.MessageNewParams {
	ephemeralCache := anthropic.NewCacheControlEphemeralParam()

	systemBlock := anthropic.TextBlockParam{
		Text:         systemPromptTemplate,
		CacheControl: ephemeralCache,
	}
	rubricBlock := anthropic.TextBlockParam{
		Text:         rubric.Prompt,
		CacheControl: ephemeralCache,
	}

	tool := anthropic.ToolParam{
		Name:         recordVerdictToolName,
		Description:  param.NewOpt("Record the structured MX-REDECO-05 tone/threat verdict for this transcript."),
		InputSchema:  verdictInputSchemaParam(),
		CacheControl: ephemeralCache,
	}

	transcriptBlock := anthropic.NewTextBlock(BuildTranscriptBlock(utterances))

	return anthropic.MessageNewParams{
		Model:       anthropic.Model(a.modelID),
		MaxTokens:   maxTokens,
		Temperature: param.NewOpt(0.0),
		System:      []anthropic.TextBlockParam{systemBlock, rubricBlock},
		Tools:       []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice:  anthropic.ToolChoiceParamOfTool(recordVerdictToolName),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(transcriptBlock),
		},
	}
}

// verdictInputSchemaParam adapts the embedded verdict.v1.json document (the
// same artifact used for re-validation in schema.go) into the SDK's
// ToolInputSchemaParam shape.
func verdictInputSchemaParam() anthropic.ToolInputSchemaParam {
	doc := VerdictInputSchemaMap()

	schema := anthropic.ToolInputSchemaParam{
		Properties: doc["properties"],
	}
	if required, ok := doc["required"].([]any); ok {
		for _, r := range required {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}
	if additional, ok := doc["additionalProperties"]; ok {
		if schema.ExtraFields == nil {
			schema.ExtraFields = map[string]any{}
		}
		schema.ExtraFields["additionalProperties"] = additional
	}
	return schema
}

// mapResponse extracts and validates the record_verdict tool_use block from
// msg, quantizes confidence to 4 decimals (the determinism control — see
// design.md's "Confidence determinism" decision), and folds a
// below-threshold confidence into ErrLowConfidence so the caller fails
// closed to HITL.
func (a *AnthropicJudge) mapResponse(msg *anthropic.Message, rubric Rubric) (JudgeResult, error) {
	var toolInput []byte
	for _, block := range msg.Content {
		if block.Type == "tool_use" && block.Name == recordVerdictToolName {
			toolInput = []byte(block.Input)
			break
		}
	}
	if toolInput == nil {
		return JudgeResult{}, fmt.Errorf("%w: no record_verdict tool_use block in response", ErrMalformedOutput)
	}

	verdict, err := validateVerdict(toolInput)
	if err != nil {
		return JudgeResult{}, err
	}

	confidence := quantizeConfidence(verdict.Confidence)
	if confidence < a.hitlThreshold {
		return JudgeResult{}, fmt.Errorf("%w: confidence %.4f below threshold %.4f", ErrLowConfidence, confidence, a.hitlThreshold)
	}

	return JudgeResult{
		Outcome:       Outcome(verdict.Outcome),
		Confidence:    confidence,
		Rationale:     verdict.Rationale,
		RubricVersion: rubric.Version,
		JudgeModelID:  a.modelID,
	}, nil
}

// quantizeConfidence rounds to 4 decimal places — the fixed precision the
// hashed evidence body and the detector_result_rows numeric(5,4) column
// both expect (see design.md's confidence determinism decision).
func quantizeConfidence(c float64) float64 {
	return math.Round(c*10000) / 10000
}

var _ Judge = (*AnthropicJudge)(nil)

// IsTransportError reports whether err is (or wraps) the judge package's
// transport error, for callers that want to distinguish it from schema or
// low-confidence failures without importing errors.Is boilerplate.
func IsTransportError(err error) bool {
	return errors.Is(err, ErrTransport)
}
