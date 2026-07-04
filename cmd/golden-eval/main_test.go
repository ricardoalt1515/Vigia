package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/goldeneval"
)

func TestRunExitsZeroAndPrintsGateSummary(t *testing.T) {
	var stdout, stderr strings.Builder
	exitCode := run(nil, &stdout, &stderr, func(context.Context, goldeneval.Options) (goldeneval.Result, error) {
		return resultFixture(), nil
	})

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"golden-set agreement", "model=model-v1", "rubric=rubric-v1", "chance_corrected=1.0000"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q, want substring %q", out, want)
		}
	}
}

func TestRunExitsOneOnThresholdFailure(t *testing.T) {
	var stdout, stderr strings.Builder
	exitCode := run(nil, &stdout, &stderr, func(context.Context, goldeneval.Options) (goldeneval.Result, error) {
		return resultFixture(), goldeneval.ErrBelowThreshold
	})

	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), goldeneval.ErrBelowThreshold.Error()) {
		t.Fatalf("stderr = %q, want threshold error", stderr.String())
	}
}

func TestRunExitsOneOnDriftFailure(t *testing.T) {
	var stdout, stderr strings.Builder
	exitCode := run(nil, &stdout, &stderr, func(context.Context, goldeneval.Options) (goldeneval.Result, error) {
		return resultFixture(), goldeneval.ErrDriftReevaluationRequired
	})

	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), goldeneval.ErrDriftReevaluationRequired.Error()) {
		t.Fatalf("stderr = %q, want drift error", stderr.String())
	}
}

func TestRunExitsTwoOnOperationalFailure(t *testing.T) {
	var stdout, stderr strings.Builder
	exitCode := run(nil, &stdout, &stderr, func(context.Context, goldeneval.Options) (goldeneval.Result, error) {
		return goldeneval.Result{}, errors.New("boom")
	})

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunExitsTwoOnInvalidFlags(t *testing.T) {
	var stdout, stderr strings.Builder
	exitCode := run([]string{"-threshold", "not-a-number"}, &stdout, &stderr, func(context.Context, goldeneval.Options) (goldeneval.Result, error) {
		t.Fatal("evaluator should not run when flags are invalid")
		return goldeneval.Result{}, nil
	})

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
}

func resultFixture() goldeneval.Result {
	return goldeneval.Result{
		Total:         1,
		Matched:       1,
		Agreement:     1,
		JudgeModelID:  "model-v1",
		RubricVersion: "rubric-v1",
		ByRule: map[string]goldeneval.RuleResult{
			"MX-REDECO-04": {
				RuleCode:                 "MX-REDECO-04",
				Total:                    1,
				Matched:                  1,
				Agreement:                1,
				ChanceCorrectedAgreement: 1,
				HasPass:                  true,
			},
		},
	}
}
