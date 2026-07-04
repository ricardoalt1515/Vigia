package evaluation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
)

// fakeInteractionLookup is a table-driven test double for
// evaluation.InteractionLookup: returns a canned ReEvaluationInput for a
// known interactionID, found=false otherwise.
type fakeInteractionLookup struct {
	byInteractionID map[string]evaluation.ReEvaluationInput
	err             error
}

func (f fakeInteractionLookup) GetInteractionForReEvaluation(_ context.Context, interactionID string) (evaluation.ReEvaluationInput, bool, error) {
	if f.err != nil {
		return evaluation.ReEvaluationInput{}, false, f.err
	}
	in, found := f.byInteractionID[interactionID]
	return in, found, nil
}

// fakeBundleVersionLookup is a table-driven test double for
// evaluation.BundleVersionLookup: returns a canned version for a known
// (tenantID, policyBundleID) pair, found=false otherwise (covers both
// unknown and foreign-tenant bundle ids).
type fakeBundleVersionLookup struct {
	versions map[string]string // key: tenantID+"|"+policyBundleID
	err      error
}

func (f fakeBundleVersionLookup) BundleVersionByID(_ context.Context, tenantID, policyBundleID string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	version, found := f.versions[tenantID+"|"+policyBundleID]
	return version, found, nil
}

func reEvaluationService(store evaluation.EvaluationStore, interactions evaluation.InteractionLookup, bundles evaluation.BundleVersionLookup) evaluation.Service {
	return evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomePass, Rationale: "inside window"}}},
		},
		Store:        store,
		Interactions: interactions,
		Bundles:      bundles,
	}
}

// TestServiceReEvaluateInteractionStampsHistoricalVersion covers
// *Reproducible Re-Evaluation Against a Specific Bundle Version* (unit
// half): re-running the same interaction + historical bundle id twice
// produces identical outcome + stamped version/id (determinism), and it
// never calls Store.CreateEvaluation (non-persisting).
func TestServiceReEvaluateInteractionStampsHistoricalVersion(t *testing.T) {
	interaction := detection.Interaction{
		OccurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		DebtorTimezone: "America/Mexico_City",
	}
	interactions := fakeInteractionLookup{byInteractionID: map[string]evaluation.ReEvaluationInput{
		"interaction-1": {TenantID: "tenant-a", Interaction: interaction},
	}}
	bundles := fakeBundleVersionLookup{versions: map[string]string{
		"tenant-a|bundle-v1": "v1",
	}}

	store := &fakeEvaluationStore{}
	svc := reEvaluationService(store, interactions, bundles)

	got1, err := svc.ReEvaluateInteraction(context.Background(), "interaction-1", "bundle-v1")
	if err != nil {
		t.Fatalf("ReEvaluateInteraction (first call): %v", err)
	}
	got2, err := svc.ReEvaluateInteraction(context.Background(), "interaction-1", "bundle-v1")
	if err != nil {
		t.Fatalf("ReEvaluateInteraction (second call): %v", err)
	}

	if got1.OverallOutcome != got2.OverallOutcome {
		t.Fatalf("OverallOutcome differs across identical reruns: %q vs %q", got1.OverallOutcome, got2.OverallOutcome)
	}
	if got1.OverallOutcome != "pass" {
		t.Fatalf("OverallOutcome = %q, want pass", got1.OverallOutcome)
	}
	if got1.PolicyBundleVersion != "v1" || got2.PolicyBundleVersion != "v1" {
		t.Fatalf("PolicyBundleVersion = (%q, %q), want (v1, v1)", got1.PolicyBundleVersion, got2.PolicyBundleVersion)
	}
	if got1.PolicyBundleID == nil || *got1.PolicyBundleID != "bundle-v1" {
		t.Fatalf("PolicyBundleID = %v, want pointer to %q", got1.PolicyBundleID, "bundle-v1")
	}
	if got2.PolicyBundleID == nil || *got2.PolicyBundleID != *got1.PolicyBundleID {
		t.Fatalf("PolicyBundleID differs across identical reruns: %v vs %v", got1.PolicyBundleID, got2.PolicyBundleID)
	}
	if len(store.calls) != 0 {
		t.Fatalf("Store.CreateEvaluation was called %d times, want 0 (ReEvaluateInteraction must not persist)", len(store.calls))
	}
}

// TestServiceReEvaluateInteractionUnknownBundleFails covers *Re-evaluation
// against an unknown bundle id fails*: an unknown or foreign-tenant
// policyBundleID returns a defined error and creates no evaluation.
func TestServiceReEvaluateInteractionUnknownBundleFails(t *testing.T) {
	interaction := detection.Interaction{
		OccurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		DebtorTimezone: "America/Mexico_City",
	}
	interactions := fakeInteractionLookup{byInteractionID: map[string]evaluation.ReEvaluationInput{
		"interaction-1": {TenantID: "tenant-a", Interaction: interaction},
	}}
	bundles := fakeBundleVersionLookup{versions: map[string]string{}} // no bundle known at all

	store := &fakeEvaluationStore{}
	svc := reEvaluationService(store, interactions, bundles)

	_, err := svc.ReEvaluateInteraction(context.Background(), "interaction-1", "unknown-bundle")
	if !errors.Is(err, evaluation.ErrPolicyBundleNotFound) {
		t.Fatalf("err = %v, want ErrPolicyBundleNotFound", err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("Store.CreateEvaluation was called %d times, want 0 on unknown bundle id", len(store.calls))
	}
}

// TestServiceReEvaluateInteractionUnknownInteractionFails covers the
// interaction-not-found half of the same requirement.
func TestServiceReEvaluateInteractionUnknownInteractionFails(t *testing.T) {
	interactions := fakeInteractionLookup{byInteractionID: map[string]evaluation.ReEvaluationInput{}}
	bundles := fakeBundleVersionLookup{versions: map[string]string{"tenant-a|bundle-v1": "v1"}}

	store := &fakeEvaluationStore{}
	svc := reEvaluationService(store, interactions, bundles)

	_, err := svc.ReEvaluateInteraction(context.Background(), "unknown-interaction", "bundle-v1")
	if !errors.Is(err, evaluation.ErrInteractionNotFound) {
		t.Fatalf("err = %v, want ErrInteractionNotFound", err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("Store.CreateEvaluation was called %d times, want 0 on unknown interaction id", len(store.calls))
	}
}
