package goldeneval

import (
	"context"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
)

func TestRunPassesWhenAgreementMeetsThreshold(t *testing.T) {
	result, err := Run(context.Background(), Options{Threshold: 1.0})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Total == 0 {
		t.Fatal("Run() evaluated zero golden results")
	}
	if result.Agreement != 1.0 {
		t.Fatalf("Agreement = %.4f, want 1.0; mismatches=%v", result.Agreement, result.Mismatches)
	}
}

func TestEvaluateAllowsZeroThreshold(t *testing.T) {
	cases := mismatchingCaseStore()

	result, err := Evaluate(cases, Options{Threshold: 0})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Agreement != 0 {
		t.Fatalf("Agreement = %.4f, want 0", result.Agreement)
	}
}

func TestEvaluateRejectsInvalidThreshold(t *testing.T) {
	for _, threshold := range []float64{-0.1, 1.1} {
		_, err := Evaluate(labtools.CaseStore{}, Options{Threshold: threshold})
		if err == nil {
			t.Fatalf("Evaluate(... Threshold: %.1f) error = nil, want error", threshold)
		}
	}
}

func TestEvaluateFailsBelowThreshold(t *testing.T) {
	cases := mismatchingCaseStore()

	result, err := Evaluate(cases, Options{Threshold: 1.0})
	if err == nil {
		t.Fatal("Evaluate() error = nil, want threshold error")
	}
	if result.Agreement != 0 {
		t.Fatalf("Agreement = %.4f, want 0", result.Agreement)
	}
	if len(result.Mismatches) != 1 {
		t.Fatalf("Mismatches len = %d, want 1", len(result.Mismatches))
	}
}

func mismatchingCaseStore() labtools.CaseStore {
	return labtools.CaseStore{
		"CASE-1": {
			CaseID:         "CASE-1",
			OccurredAt:     "2024-03-15T12:00:00-06:00",
			DebtorTimezone: "America/Mexico_City",
			Transcript: []labtools.Utterance{
				{Speaker: "collector", Text: "Good afternoon."},
			},
			DetectorResults: []labtools.DetectorResult{
				{RuleCode: "MX-REDECO-04", DetectorKind: "deterministic", Outcome: "hard_block"},
			},
		},
	}
}
