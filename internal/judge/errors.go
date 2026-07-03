package judge

import "errors"

// The judge error taxonomy lets evaluation.Service attribute a fail-closed
// rationale to exactly what went wrong, without depending on judge's
// internal types. Every one of these MUST route the calling evaluation to
// requires_hitl = true — never a silent pass (spec: "Judge Fails Closed to
// requires_hitl on Every Uncertain Path").
var (
	// ErrTransport covers network failures, timeouts, and transient
	// transport errors (HTTP 429/5xx) that exhausted the judge client's
	// bounded retry budget.
	ErrTransport = errors.New("judge: transport error")
	// ErrMalformedOutput covers a response with no (or an unparseable)
	// record_verdict tool_use block.
	ErrMalformedOutput = errors.New("judge: malformed output")
	// ErrSchemaInvalid covers a tool input that fails strict JSON-schema
	// validation, or that is schema-valid but semantically inconsistent
	// (e.g. an effectively empty rationale).
	ErrSchemaInvalid = errors.New("judge: schema-invalid output")
	// ErrLowConfidence covers a schema-valid, semantically consistent
	// verdict whose confidence is below the configured HITL threshold.
	ErrLowConfidence = errors.New("judge: confidence below HITL threshold")
)
