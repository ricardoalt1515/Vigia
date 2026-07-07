package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRecordJudgeResultEmitsTokenUsageAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)

	ctx, span := StartJudgeSpan(context.Background(), "claude-test", "rubric-v1")
	_ = ctx
	RecordJudgeResult(span, "pass", 0.875, 1000, 100, 800, 50, nil)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	attrs := spans[0].Attributes
	assertInt64Attr(t, attrs, "gen_ai.usage.input_tokens", 1000)
	assertInt64Attr(t, attrs, "gen_ai.usage.output_tokens", 100)
	assertInt64Attr(t, attrs, "gen_ai.usage.cache_read_input_tokens", 800)
	assertInt64Attr(t, attrs, "gen_ai.usage.cache_creation_input_tokens", 50)
}

func assertInt64Attr(t *testing.T, attrs []attribute.KeyValue, key string, want int64) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			if got := attr.Value.AsInt64(); got != want {
				t.Fatalf("%s = %d, want %d", key, got, want)
			}
			return
		}
	}
	t.Fatalf("missing attribute %s in %#v", key, attrs)
}
