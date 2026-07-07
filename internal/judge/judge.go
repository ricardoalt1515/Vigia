// Package judge implements the LLM-judge seam for MX-REDECO-05 tone/threat
// detection: a network-bound, ctx-aware, fallible port distinct from
// detection.Detector. The Anthropic implementation, the deterministic fake,
// the embedded rubric artifact, the embedded output schema, and all
// validation live behind the Judge interface exported here.
package judge

import "context"

// Outcome is the judge-seam vocabulary (mirrors detection.Outcome's shape
// but is a distinct type). Persistence maps block -> core "fail"/high,
// pass -> "pass".
type Outcome string

const (
	OutcomePass  Outcome = "pass"
	OutcomeBlock Outcome = "block"
)

// Utterance is one speaker turn of the transcript the judge reads.
type Utterance struct {
	Speaker string
	Text    string
}

// Rubric is the resolved, versioned MX-REDECO-05 tone/threat rubric.
type Rubric struct {
	// Version is the pinned rubric_version string, e.g.
	// "mx-redeco-05.tone-threat.v1".
	Version string
	// Prompt is the embedded rubric body (go:embed), part of the cached
	// stable prefix.
	Prompt string
}

// JudgeInput is everything the judge needs to decide. No IDs, no tenant —
// the judge is a pure decision over transcript + rubric; identity stays in
// evaluation.Service.
type JudgeInput struct {
	Utterances []Utterance
	Rubric     Rubric
}

// JudgeResult is a schema-validated verdict. Confidence is already
// quantized to 4 decimals (see the Anthropic judge's confidence
// determinism). RubricVersion/JudgeModelID are echoed so the caller records
// exactly what produced the verdict.
type JudgeResult struct {
	Outcome                  Outcome
	Confidence               float64
	Rationale                string
	RubricVersion            string
	JudgeModelID             string
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
}

// Judge is the network-bound, fallible seam. Evaluate MUST honor ctx
// deadlines and return an error on any failure the caller must fail-closed
// on. Judge is deliberately distinct from detection.Detector, which is
// contractually pure (no ctx, no error, no I/O).
type Judge interface {
	Evaluate(ctx context.Context, in JudgeInput) (JudgeResult, error)
}

// NamedJudge pairs a Judge with the stable detector_code its result row
// carries (e.g. "MX-REDECO-05").
type NamedJudge struct {
	Code  string
	Judge Judge
}
