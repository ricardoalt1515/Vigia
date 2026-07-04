package evaluation_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/judge"
)

// fakeDetector returns a canned detection.Result regardless of input.
// calls is an optional *int counter (nil-safe) so tests can assert a
// detector was never invoked (e.g. a tenant check short-circuiting before
// the pipeline runs).
type fakeDetector struct {
	result detection.Result
	calls  *int
}

func (f fakeDetector) Evaluate(in detection.Interaction) detection.Result {
	if f.calls != nil {
		*f.calls++
	}
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

	t.Run("warn outcome maps to warn severity medium and does not flip overall outcome", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "contact-hours", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomePass}}},
				{Code: "MX-REDECO-03", Detector: fakeDetector{result: detection.Result{
					Outcome:   detection.OutcomeWarn,
					Rationale: "disclosure not stated",
				}}},
			},
			Store: store,
		}

		got, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-warn",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.OverallOutcome != "pass" {
			t.Fatalf("OverallOutcome = %q, want %q — a warn row alone must not flip overall outcome", got.OverallOutcome, "pass")
		}
		var warnRow *evaluation.DetectorResultInput
		for i, row := range store.calls[0].DetectorResults {
			if row.DetectorCode == "MX-REDECO-03" {
				warnRow = &store.calls[0].DetectorResults[i]
			}
		}
		if warnRow == nil {
			t.Fatal("no detector result row carries the MX-REDECO-03 code")
		}
		if warnRow.Outcome != core.DetectorOutcomeWarn {
			t.Errorf("Outcome = %q, want %q", warnRow.Outcome, core.DetectorOutcomeWarn)
		}
		if warnRow.Severity != core.SeverityMedium {
			t.Errorf("Severity = %q, want %q", warnRow.Severity, core.SeverityMedium)
		}
	})

	t.Run("warn row coexisting with a hard-block row yields overall fail", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "MX-REDECO-06", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomeBlock}}},
				{Code: "MX-REDECO-03", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomeWarn}}},
			},
			Store: store,
		}

		got, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-warn-and-block",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.OverallOutcome != "fail" {
			t.Fatalf("OverallOutcome = %q, want %q — driven by the hard-block detector", got.OverallOutcome, "fail")
		}
		if len(store.calls[0].DetectorResults) != 2 {
			t.Fatalf("DetectorResults len = %d, want 2 (one fail row + one warn row)", len(store.calls[0].DetectorResults))
		}
	})

	t.Run("MX-REDECO-07 block sets requires_hitl true", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "MX-REDECO-07", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomeBlock}}, RequiresHITL: true},
			},
			Store: store,
		}

		_, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-hitl",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !store.calls[0].RequiresHITL {
			t.Fatalf("call.RequiresHITL = %v, want true", store.calls[0].RequiresHITL)
		}
	})

	t.Run("other detectors' blocks do not set requires_hitl", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "MX-REDECO-06", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomeBlock}}},
				{Code: "MX-REDECO-07", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomePass}}, RequiresHITL: true},
			},
			Store: store,
		}

		_, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-no-hitl",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store.calls[0].RequiresHITL {
			t.Fatalf("call.RequiresHITL = %v, want false — only MX-REDECO-07's own block sets it", store.calls[0].RequiresHITL)
		}
	})

	t.Run("no detectors configured returns error without persisting", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := evaluation.Service{
			Detectors: nil,
			Store:     store,
		}

		_, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-e",
			Interaction:        interaction,
		})
		if !errors.Is(err, evaluation.ErrNoDetectors) {
			t.Fatalf("err = %v, want ErrNoDetectors", err)
		}
		if len(store.calls) != 0 {
			t.Fatalf("CreateEvaluation calls = %d, want 0 (no fabricated evaluation)", len(store.calls))
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

// slowJudge blocks until ctx is done or its delay elapses, whichever comes
// first, for exercising the *Judge timeout sets requires_hitl, never a
// silent pass* scenario without a real network or clock dependency.
type slowJudge struct{ delay time.Duration }

func (s slowJudge) Evaluate(ctx context.Context, in judge.JudgeInput) (judge.JudgeResult, error) {
	select {
	case <-time.After(s.delay):
		return judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: 0.99}, nil
	case <-ctx.Done():
		// Provenance (rubric/model attempted) is still reported on a timeout,
		// mirroring AnthropicJudge's fail-closed behavior: a HITL row from a
		// judge timeout must not lose evidence of what was attempted.
		return judge.JudgeResult{RubricVersion: judge.RubricVersion, JudgeModelID: "slow-judge-v1"}, ctx.Err()
	}
}

// thresholdJudge is a minimal test double that makes its own HITL-threshold
// decision (mirroring where AnthropicJudge/FakeJudge decide it), so the
// *HITL threshold is configurable without a code change* scenario can prove
// evaluation.Service's fail-closed folding reacts correctly to whichever
// threshold the Judge implementation used — without evaluation.Service
// itself knowing about threshold values.
type thresholdJudge struct {
	confidence float64
	threshold  float64
}

func (j thresholdJudge) Evaluate(_ context.Context, _ judge.JudgeInput) (judge.JudgeResult, error) {
	if j.confidence < j.threshold {
		// Provenance is still reported below-threshold, mirroring
		// AnthropicJudge's fail-closed behavior.
		return judge.JudgeResult{RubricVersion: judge.RubricVersion, JudgeModelID: "threshold-judge-v1"}, judge.ErrLowConfidence
	}
	return judge.JudgeResult{Outcome: judge.OutcomePass, Confidence: j.confidence, RubricVersion: judge.RubricVersion, JudgeModelID: "threshold-judge-v1"}, nil
}

func passingDetectorService(store evaluation.EvaluationStore, judges ...evaluation.NamedJudge) evaluation.Service {
	return evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomePass, Rationale: "inside window"}}},
		},
		Judges: judges,
		Store:  store,
	}
}

func evaluateWithJudge(t *testing.T, ctx context.Context, svc evaluation.Service) (core.Evaluation, *fakeEvaluationStore) {
	t.Helper()
	store := svc.Store.(*fakeEvaluationStore)
	got, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           "tenant-a",
		InteractionEventID: "interaction-judge",
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
			DebtorTimezone: "America/Mexico_City",
		},
		Utterances: []judge.Utterance{{Speaker: "agent", Text: "Le recordamos su pago."}},
	})
	if err != nil {
		t.Fatalf("EvaluateInteraction returned unexpected error: %v", err)
	}
	return got, store
}

// TestServiceJudgeTimeoutSetsRequiresHITLNeverSilentPass covers *Judge
// timeout sets requires_hitl, never a silent pass*.
func TestServiceJudgeTimeoutSetsRequiresHITLNeverSilentPass(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := passingDetectorService(store, evaluation.NamedJudge{Code: "MX-REDECO-05", Judge: slowJudge{delay: 200 * time.Millisecond}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, store = evaluateWithJudge(t, ctx, svc)

	call := store.calls[0]
	if !call.RequiresHITL {
		t.Fatal("RequiresHITL = false, want true after a judge timeout")
	}
	if call.OverallOutcome == "pass" {
		t.Fatalf("OverallOutcome = %q, want it not to be a silent pass after a judge timeout", call.OverallOutcome)
	}
	if !judgeRowRationaleContains(call, "timed out") && !judgeRowRationaleContains(call, "deadline") {
		t.Fatalf("no MX-REDECO-05 detector result row rationale mentions the timeout: %+v", call.DetectorResults)
	}
	if call.JudgeModelID == "" || call.RubricVersion == "" {
		t.Fatalf("call = %+v, want JudgeModelID/RubricVersion recorded even on a judge timeout", call)
	}
}

// TestServiceJudgeTransportErrorSetsRequiresHITL covers *Judge transport
// error sets requires_hitl*.
func TestServiceJudgeTransportErrorSetsRequiresHITL(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := passingDetectorService(store, evaluation.NamedJudge{Code: "MX-REDECO-05", Judge: judge.FakeJudge{ForceErr: true}})

	_, store = evaluateWithJudge(t, context.Background(), svc)

	call := store.calls[0]
	if !call.RequiresHITL {
		t.Fatal("RequiresHITL = false, want true after a judge transport error")
	}
	if !judgeRowRationaleContains(call, "transport") {
		t.Fatalf("no MX-REDECO-05 detector result row rationale references the transport failure: %+v", call.DetectorResults)
	}
	if call.JudgeModelID == "" || call.RubricVersion == "" {
		t.Fatalf("call = %+v, want JudgeModelID/RubricVersion recorded even on a judge transport error", call)
	}
}

// TestServiceMalformedJudgeOutputSetsRequiresHITLNeverPass covers
// *Malformed judge output sets requires_hitl, never a pass*.
func TestServiceMalformedJudgeOutputSetsRequiresHITLNeverPass(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := passingDetectorService(store, evaluation.NamedJudge{Code: "MX-REDECO-05", Judge: judge.FakeJudge{ForceMalformed: true}})

	got, store := evaluateWithJudge(t, context.Background(), svc)

	call := store.calls[0]
	if !call.RequiresHITL {
		t.Fatal("RequiresHITL = false, want true after malformed judge output")
	}
	if got.OverallOutcome == "pass" || call.OverallOutcome == "pass" {
		t.Fatalf("OverallOutcome = %q, want it not to be PASS after malformed judge output", call.OverallOutcome)
	}
	if !judgeRowRationaleContains(call, "malformed") && !judgeRowRationaleContains(call, "invalid") {
		t.Fatalf("no MX-REDECO-05 detector result row rationale states malformed/invalid: %+v", call.DetectorResults)
	}
	if call.JudgeModelID == "" || call.RubricVersion == "" {
		t.Fatalf("call = %+v, want JudgeModelID/RubricVersion recorded even on malformed judge output", call)
	}
}

// TestServiceLowConfidenceSetsRequiresHITL covers *Confidence below
// threshold sets requires_hitl*.
func TestServiceLowConfidenceSetsRequiresHITL(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := passingDetectorService(store, evaluation.NamedJudge{Code: "MX-REDECO-05", Judge: thresholdJudge{confidence: 0.5, threshold: 0.75}})

	_, store = evaluateWithJudge(t, context.Background(), svc)

	call := store.calls[0]
	if !call.RequiresHITL {
		t.Fatal("RequiresHITL = false, want true when confidence is below the configured threshold")
	}
	if !judgeRowRationaleContains(call, "threshold") && !judgeRowRationaleContains(call, "confidence") {
		t.Fatalf("no MX-REDECO-05 detector result row rationale states below-threshold: %+v", call.DetectorResults)
	}
	if call.JudgeModelID == "" || call.RubricVersion == "" {
		t.Fatalf("call = %+v, want JudgeModelID/RubricVersion recorded even below the confidence threshold", call)
	}
}

// TestServiceConfidentBlockIsHardBlockAndRequiresHITL covers *Confident
// threat verdict is a HARD BLOCK and also sets requires_hitl*.
func TestServiceConfidentBlockIsHardBlockAndRequiresHITL(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := passingDetectorService(store, evaluation.NamedJudge{
		Code: "MX-REDECO-05",
		Judge: fixedJudge{result: judge.JudgeResult{
			Outcome: judge.OutcomeBlock, Confidence: 0.95, Rationale: "threatening language",
			RubricVersion: judge.RubricVersion, JudgeModelID: "fake-model",
		}},
	})

	got, store := evaluateWithJudge(t, context.Background(), svc)

	if got.OverallOutcome != "fail" {
		t.Fatalf("OverallOutcome = %q, want fail (HARD BLOCK) for a confident threat verdict", got.OverallOutcome)
	}
	call := store.calls[0]
	if !call.RequiresHITL {
		t.Fatal("RequiresHITL = false, want true even for a confident HARD BLOCK (MX-REDECO-05 mandates human review)")
	}
	if call.JudgeModelID != "fake-model" || call.RubricVersion != judge.RubricVersion {
		t.Fatalf("call = %+v, want JudgeModelID/RubricVersion echoed from the judge result", call)
	}
	if call.JudgeConfidence == nil || *call.JudgeConfidence != 0.95 {
		t.Fatalf("JudgeConfidence = %v, want 0.95", call.JudgeConfidence)
	}
}

// TestServiceConfidentPassDoesNotSetRequiresHITL covers *Confident neutral
// verdict passes without requires_hitl*.
func TestServiceConfidentPassDoesNotSetRequiresHITL(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := passingDetectorService(store, evaluation.NamedJudge{
		Code: "MX-REDECO-05",
		Judge: fixedJudge{result: judge.JudgeResult{
			Outcome: judge.OutcomePass, Confidence: 0.9, Rationale: "neutral reminder",
			RubricVersion: judge.RubricVersion, JudgeModelID: "fake-model",
		}},
	})

	got, store := evaluateWithJudge(t, context.Background(), svc)

	if got.OverallOutcome != "pass" {
		t.Fatalf("OverallOutcome = %q, want pass (unblocked by the judge step)", got.OverallOutcome)
	}
	if store.calls[0].RequiresHITL {
		t.Fatal("RequiresHITL = true, want false from the judge step alone for a confident pass")
	}
}

// TestServiceHITLThresholdConfigurableWithoutCodeChange covers *HITL
// threshold is configurable without a code change*.
func TestServiceHITLThresholdConfigurableWithoutCodeChange(t *testing.T) {
	const fixedConfidence = 0.8

	highThresholdStore := &fakeEvaluationStore{}
	highThresholdSvc := passingDetectorService(highThresholdStore, evaluation.NamedJudge{
		Code: "MX-REDECO-05", Judge: thresholdJudge{confidence: fixedConfidence, threshold: 0.9},
	})
	_, highThresholdStore = evaluateWithJudge(t, context.Background(), highThresholdSvc)
	if !highThresholdStore.calls[0].RequiresHITL {
		t.Fatal("RequiresHITL = false under the higher threshold, want true (same confidence, no source change)")
	}

	lowThresholdStore := &fakeEvaluationStore{}
	lowThresholdSvc := passingDetectorService(lowThresholdStore, evaluation.NamedJudge{
		Code: "MX-REDECO-05", Judge: thresholdJudge{confidence: fixedConfidence, threshold: 0.5},
	})
	_, lowThresholdStore = evaluateWithJudge(t, context.Background(), lowThresholdSvc)
	if lowThresholdStore.calls[0].RequiresHITL {
		t.Fatal("RequiresHITL = true under the lower threshold, want false (same confidence, no source change)")
	}
}

// TestServiceWiresJudgeAsDistinctTypedStep covers *Evaluation service wires
// the judge as a distinct typed step* and *Deterministic fake judge
// implements the same port*.
func TestServiceWiresJudgeAsDistinctTypedStep(t *testing.T) {
	store := &fakeEvaluationStore{}
	fj := judge.FakeJudge{}
	svc := passingDetectorService(store, evaluation.NamedJudge{Code: "MX-REDECO-05", Judge: fj})

	_, store = evaluateWithJudge(t, context.Background(), svc)

	call := store.calls[0]
	// One detector row (contact-hours) + one judge row (MX-REDECO-05),
	// proving the judge ran through its own typed step, not folded into
	// the detector loop.
	if len(call.DetectorResults) != 2 {
		t.Fatalf("DetectorResults len = %d, want 2 (one detector + one judge)", len(call.DetectorResults))
	}
	var sawJudgeRow bool
	for _, row := range call.DetectorResults {
		if row.DetectorCode == "MX-REDECO-05" {
			sawJudgeRow = true
		}
	}
	if !sawJudgeRow {
		t.Fatal("no detector result row carries the judge's MX-REDECO-05 code")
	}
}

// TestServiceRejectsMultipleJudges covers the multi-judge clobber guard:
// today the header fields (judgeModelID/rubricVersion/judgeConfidence) are
// last-judge-wins across s.Judges, so more than one configured judge would
// silently drop provenance for every judge but the last. Real multi-judge
// support is issue #7; until then EvaluateInteraction fails fast instead.
func TestServiceRejectsMultipleJudges(t *testing.T) {
	store := &fakeEvaluationStore{}
	svc := passingDetectorService(store,
		evaluation.NamedJudge{Code: "MX-REDECO-05", Judge: judge.FakeJudge{}},
		evaluation.NamedJudge{Code: "MX-REDECO-06", Judge: judge.FakeJudge{}},
	)

	_, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
		TenantID:           "tenant-a",
		InteractionEventID: "interaction-judge",
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
			DebtorTimezone: "America/Mexico_City",
		},
		Utterances: []judge.Utterance{{Speaker: "agent", Text: "Le recordamos su pago."}},
	})
	if !errors.Is(err, evaluation.ErrMultipleJudgesNotSupported) {
		t.Fatalf("err = %v, want ErrMultipleJudgesNotSupported", err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("store.calls = %d, want 0 (multi-judge guard must reject before persisting)", len(store.calls))
	}
}

// fixedJudge returns a canned JudgeResult regardless of input.
type fixedJudge struct{ result judge.JudgeResult }

func (f fixedJudge) Evaluate(_ context.Context, _ judge.JudgeInput) (judge.JudgeResult, error) {
	return f.result, nil
}

func judgeRowRationaleContains(call evaluation.CreateEvaluationInput, substr string) bool {
	for _, row := range call.DetectorResults {
		if row.DetectorCode != "MX-REDECO-05" {
			continue
		}
		if strings.Contains(strings.ToLower(row.Rationale), strings.ToLower(substr)) {
			return true
		}
	}
	return false
}
