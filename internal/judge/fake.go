package judge

import (
	"context"
	"strings"
)

// fakeJudgeModelID is the model id the FakeJudge echoes on JudgeResult, so
// callers that record judge_model_id in tests/seed get a stable,
// recognizably-fake value rather than a real Anthropic snapshot.
const fakeJudgeModelID = "fake-judge-v1"

// threatKeywords is the small, pinned keyword set FakeJudge scans for.
// These markers are deliberately used only by synthetic fixtures/tests —
// the fake is deterministic and does NOT reproduce the real Anthropic
// judge's language understanding.
var threatKeywords = []string{
	"vamos a tu casa",
	"te vamos a",
	"amenaza",
	"vamos a la fuerza",
}

// FakeJudge is a deterministic, no-network, no-key stand-in for the
// Anthropic judge. It decides purely by scanning Utterances for a small
// pinned threat-keyword set — never by any instruction-like text embedded
// in the transcript, which is why an injection attempt cannot flip its
// verdict: the fake never "reads" the transcript as instructions in the
// first place, only as a keyword-scan target (structurally the same
// guarantee the real judge gets from schema re-validation).
type FakeJudge struct {
	// ForceErr makes Evaluate return a transport-style error, for
	// exercising the fail-closed path without a real network.
	ForceErr bool
	// ForceMalformed makes Evaluate return a schema-invalid-output error,
	// for exercising the fail-closed path without a real network.
	ForceMalformed bool
}

func (f FakeJudge) Evaluate(ctx context.Context, in JudgeInput) (JudgeResult, error) {
	rubricVersion := in.Rubric.Version
	if rubricVersion == "" {
		rubricVersion = RubricVersion
	}
	// attempted carries the rubric/model provenance for every failure path
	// below, so a caller (evaluation.Service) can record what was attempted
	// even when the verdict itself fails closed to requires_hitl.
	attempted := JudgeResult{RubricVersion: rubricVersion, JudgeModelID: fakeJudgeModelID}

	if err := ctx.Err(); err != nil {
		return attempted, err
	}
	if f.ForceErr {
		return attempted, ErrTransport
	}
	if f.ForceMalformed {
		return attempted, ErrSchemaInvalid
	}

	if containsThreat(in.Utterances) {
		return JudgeResult{
			Outcome:       OutcomeBlock,
			Confidence:    0.95,
			Rationale:     "fake judge: transcript contains a pinned threat-keyword match",
			RubricVersion: rubricVersion,
			JudgeModelID:  fakeJudgeModelID,
		}, nil
	}

	return JudgeResult{
		Outcome:       OutcomePass,
		Confidence:    0.90,
		Rationale:     "fake judge: no pinned threat-keyword match found",
		RubricVersion: rubricVersion,
		JudgeModelID:  fakeJudgeModelID,
	}, nil
}

// containsThreat scans every utterance's text for any pinned threat
// keyword, case-insensitively. It never inspects Speaker, and it never
// treats any text as an instruction to follow — only as data to
// pattern-match, which is what keeps an embedded "ignore your
// instructions" string from having any effect on the verdict.
func containsThreat(utterances []Utterance) bool {
	for _, u := range utterances {
		lower := strings.ToLower(u.Text)
		for _, kw := range threatKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

var _ Judge = FakeJudge{}
