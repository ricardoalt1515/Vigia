package evaluation_test

import (
	"context"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
)

// fakeDetector returns a canned detection.Result regardless of input.
type fakeDetector struct {
	result detection.Result
}

func (f fakeDetector) Evaluate(in detection.Interaction) detection.Result {
	return f.result
}

// fakeEvaluationStore captures the CreateEvaluation call it received.
type fakeEvaluationStore struct {
	calls []evaluation.CreateEvaluationInput
	err   error
}

func (f *fakeEvaluationStore) CreateEvaluation(ctx context.Context, in evaluation.CreateEvaluationInput) (core.Evaluation, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return core.Evaluation{}, f.err
	}
	return core.Evaluation{
		ID:                 "evaluation-1",
		TenantID:           core.ID(in.TenantID),
		InteractionEventID: core.ID(in.InteractionEventID),
		OverallOutcome:     in.OverallOutcome,
	}, nil
}

func TestServiceEvaluateInteraction(t *testing.T) {
	interaction := detection.Interaction{
		OccurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		DebtorTimezone: "America/Mexico_City",
	}

	t.Run("block outcome maps to fail severity high", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "contact-hours", Detector: fakeDetector{result: detection.Result{
					Outcome:   detection.OutcomeBlock,
					Rationale: "outside window",
				}}},
			},
			Store: store,
		}

		got, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-a",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.OverallOutcome != "fail" {
			t.Fatalf("OverallOutcome = %q, want %q", got.OverallOutcome, "fail")
		}
		if len(store.calls) != 1 {
			t.Fatalf("CreateEvaluation calls = %d, want 1", len(store.calls))
		}
		call := store.calls[0]
		if call.TenantID != "tenant-a" || call.InteractionEventID != "interaction-a" {
			t.Fatalf("call = %+v, want tenant-a/interaction-a", call)
		}
		if call.OverallOutcome != "fail" {
			t.Fatalf("call.OverallOutcome = %q, want %q", call.OverallOutcome, "fail")
		}
		if len(call.DetectorResults) != 1 {
			t.Fatalf("DetectorResults len = %d, want 1", len(call.DetectorResults))
		}
		row := call.DetectorResults[0]
		if row.DetectorCode != "contact-hours" {
			t.Errorf("DetectorCode = %q, want %q", row.DetectorCode, "contact-hours")
		}
		if row.Outcome != core.DetectorOutcomeFail {
			t.Errorf("Outcome = %q, want %q", row.Outcome, core.DetectorOutcomeFail)
		}
		if row.Severity != core.SeverityHigh {
			t.Errorf("Severity = %q, want %q", row.Severity, core.SeverityHigh)
		}
		if row.Rationale != "outside window" {
			t.Errorf("Rationale = %q, want %q", row.Rationale, "outside window")
		}
	})

	t.Run("pass outcome maps to pass severity low", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "contact-hours", Detector: fakeDetector{result: detection.Result{
					Outcome:   detection.OutcomePass,
					Rationale: "inside window",
				}}},
			},
			Store: store,
		}

		got, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-b",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.OverallOutcome != "pass" {
			t.Fatalf("OverallOutcome = %q, want %q", got.OverallOutcome, "pass")
		}
		row := store.calls[0].DetectorResults[0]
		if row.Outcome != core.DetectorOutcomePass {
			t.Errorf("Outcome = %q, want %q", row.Outcome, core.DetectorOutcomePass)
		}
		if row.Severity != core.SeverityLow {
			t.Errorf("Severity = %q, want %q", row.Severity, core.SeverityLow)
		}
	})

	t.Run("any block among multiple detectors makes overall outcome fail", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "contact-hours", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomePass}}},
				{Code: "second-detector", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomeBlock}}},
			},
			Store: store,
		}

		got, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-c",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.OverallOutcome != "fail" {
			t.Fatalf("OverallOutcome = %q, want %q when any detector blocks", got.OverallOutcome, "fail")
		}
		if len(store.calls[0].DetectorResults) != 2 {
			t.Fatalf("DetectorResults len = %d, want 2", len(store.calls[0].DetectorResults))
		}
	})

	t.Run("store error propagates", func(t *testing.T) {
		store := &fakeEvaluationStore{err: context.DeadlineExceeded}
		svc := evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "contact-hours", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomePass}}},
			},
			Store: store,
		}

		_, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-d",
			Interaction:        interaction,
		})
		if err == nil {
			t.Fatal("expected error from store to propagate")
		}
	})
}
