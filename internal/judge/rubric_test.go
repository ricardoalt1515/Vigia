package judge_test

import (
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/judge"
)

func TestLoadRubricReturnsEmbeddedPromptAndPinnedVersion(t *testing.T) {
	rubric := judge.LoadRubric()

	if rubric.Version != judge.RubricVersion {
		t.Fatalf("rubric.Version = %q, want judge.RubricVersion %q", rubric.Version, judge.RubricVersion)
	}
	if judge.RubricVersion != "mx-redeco-05.tone-threat.v1" {
		t.Fatalf("judge.RubricVersion = %q, want %q", judge.RubricVersion, "mx-redeco-05.tone-threat.v1")
	}
	if strings.TrimSpace(rubric.Prompt) == "" {
		t.Fatal("rubric.Prompt is empty, want a non-empty embedded rubric body")
	}
}
