// Package observability centralizes OpenTelemetry attribute names for Vigia's
// agentic/GenAI paths. It deliberately uses stable string attributes instead of
// tying product code to a specific semconv Go package version.
package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/ricardoalt1515/vigia"

func StartJudgeSpan(ctx context.Context, modelID, rubricVersion string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, "vigia.judge.evaluate",
		trace.WithAttributes(
			attribute.String("gen_ai.system", "anthropic"),
			attribute.String("gen_ai.operation.name", "judge"),
			attribute.String("gen_ai.request.model", modelID),
			attribute.String("vigia.rubric.version", rubricVersion),
		))
}

func RecordJudgeResult(span trace.Span, outcome string, confidence float64, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64, err error) {
	if span == nil {
		return
	}
	span.SetAttributes(
		attribute.String("vigia.judge.outcome", outcome),
		attribute.Float64("vigia.judge.confidence", confidence),
		attribute.Int64("gen_ai.usage.input_tokens", inputTokens),
		attribute.Int64("gen_ai.usage.output_tokens", outputTokens),
		attribute.Int64("gen_ai.usage.cache_read_input_tokens", cacheReadTokens),
		attribute.Int64("gen_ai.usage.cache_creation_input_tokens", cacheCreationTokens),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}
	span.SetStatus(codes.Ok, "")
}
