package judge

import _ "embed"

// RubricVersion is the pinned MX-REDECO-05 tone/threat rubric version. It is
// the single source of truth threaded to JudgeResult, the Evaluation, and
// the evidence body — bump it (and the embedded file) together whenever the
// rubric text changes.
const RubricVersion = "mx-redeco-05.tone-threat.v1"

//go:embed rubric/mx-redeco-05.v1.md
var rubricPrompt string

// LoadRubric returns the resolved, versioned MX-REDECO-05 rubric. It is
// loaded once at construction, not per call (design.md's evaluation wiring
// note).
func LoadRubric() Rubric {
	return Rubric{
		Version: RubricVersion,
		Prompt:  rubricPrompt,
	}
}
