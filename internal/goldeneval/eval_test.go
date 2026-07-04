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
	if result.JudgeModelID != "fake-judge-v1" {
		t.Fatalf("JudgeModelID = %q, want fake judge provenance", result.JudgeModelID)
	}
	for rule, ruleResult := range result.ByRule {
		if ruleResult.Total < 2 {
			t.Fatalf("rule %s evaluated %d outcomes, want compliant and violating golden cases", rule, ruleResult.Total)
		}
		if !ruleResult.HasPass || !ruleResult.HasHardBlock {
			t.Fatalf("rule %s coverage = %+v, want pass and hard_block human labels", rule, ruleResult)
		}
		if ruleResult.ChanceCorrectedAgreement != 1.0 {
			t.Fatalf("rule %s chance-corrected agreement = %.4f, want 1.0", rule, ruleResult.ChanceCorrectedAgreement)
		}
	}
}

func TestEvaluateReportsChanceCorrectedAgreementByRule(t *testing.T) {
	cases := labtools.CaseStore{
		"CASE-1": goldenCase("CASE-1", "2024-03-15T12:00:00-06:00", "hello", "pass"),
		"CASE-2": goldenCase("CASE-2", "2024-03-15T23:30:00-06:00", "hello", "hard_block"),
	}

	result, err := Evaluate(context.Background(), cases, Options{Threshold: 1.0})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	rule := result.ByRule["MX-REDECO-04"]
	if rule.Agreement != 1.0 || rule.ChanceCorrectedAgreement != 1.0 {
		t.Fatalf("rule result = %+v, want perfect raw and chance-corrected agreement", rule)
	}
}

func TestEvaluateUsesMarginalDistributionsForChanceCorrection(t *testing.T) {
	cases := labtools.CaseStore{
		"CASE-1": goldenCase("CASE-1", "2024-03-15T12:00:00-06:00", "hello", "pass"),
		"CASE-2": goldenCase("CASE-2", "2024-03-15T12:00:00-06:00", "hello", "pass"),
		"CASE-3": goldenCase("CASE-3", "2024-03-15T12:00:00-06:00", "hello", "hard_block"),
	}

	result, err := Evaluate(context.Background(), cases, Options{Threshold: -1})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	rule := result.ByRule["MX-REDECO-04"]
	if rule.Agreement != float64(2)/float64(3) {
		t.Fatalf("raw agreement = %.4f, want 0.6667", rule.Agreement)
	}
	if rule.ChanceCorrectedAgreement != 0 {
		t.Fatalf("chance-corrected agreement = %.4f, want 0 for majority-class predictions", rule.ChanceCorrectedAgreement)
	}
}

func TestEvaluateFailsWhenChanceCorrectedAgreementDropsBelowThreshold(t *testing.T) {
	cases := labtools.CaseStore{
		"CASE-1": goldenCase("CASE-1", "2024-03-15T12:00:00-06:00", "hello", "pass"),
		"CASE-2": goldenCase("CASE-2", "2024-03-15T23:30:00-06:00", "hello", "pass"),
	}

	result, err := Evaluate(context.Background(), cases, Options{Threshold: 0.5})
	if err == nil {
		t.Fatal("Evaluate() error = nil, want threshold error")
	}
	if result.ByRule["MX-REDECO-04"].ChanceCorrectedAgreement >= 0.5 {
		t.Fatalf("rule result = %+v, want chance-corrected agreement below threshold", result.ByRule["MX-REDECO-04"])
	}
}

func TestEvaluateRequiresDriftReevaluationWhenJudgeModelOrRubricChanges(t *testing.T) {
	cases := labtools.CaseStore{
		"CASE-1": goldenCase("CASE-1", "2024-03-15T12:00:00-06:00", "hello", "pass"),
	}

	result, err := Evaluate(context.Background(), cases, Options{
		Threshold:             -1,
		ExpectedJudgeModelID:  "previous-model",
		ExpectedRubricVersion: "previous-rubric",
	})
	if err == nil {
		t.Fatal("Evaluate() error = nil, want drift re-evaluation error")
	}
	if !result.DriftReevaluationRequired {
		t.Fatalf("DriftReevaluationRequired = false, want true")
	}
}

func TestEvaluateAllowsMinimumThreshold(t *testing.T) {
	cases := mismatchingCaseStore()

	result, err := Evaluate(context.Background(), cases, Options{Threshold: -1})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Agreement != 0 {
		t.Fatalf("Agreement = %.4f, want 0", result.Agreement)
	}
}

func TestEvaluateRejectsInvalidThreshold(t *testing.T) {
	for _, threshold := range []float64{-1.1, 1.1} {
		_, err := Evaluate(context.Background(), labtools.CaseStore{}, Options{Threshold: threshold})
		if err == nil {
			t.Fatalf("Evaluate(... Threshold: %.1f) error = nil, want error", threshold)
		}
	}
}

func TestEvaluateFailsBelowThreshold(t *testing.T) {
	cases := mismatchingCaseStore()

	result, err := Evaluate(context.Background(), cases, Options{Threshold: 1.0})
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

func goldenCase(id, occurredAt, collectorText, outcome string) labtools.SyntheticCase {
	return labtools.SyntheticCase{
		CaseID:         id,
		OccurredAt:     occurredAt,
		DebtorTimezone: "America/Mexico_City",
		Transcript: []labtools.Utterance{
			{Speaker: "collector", Text: collectorText},
		},
		DetectorResults: []labtools.DetectorResult{
			{RuleCode: "MX-REDECO-04", DetectorKind: "deterministic", Outcome: outcome},
		},
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
