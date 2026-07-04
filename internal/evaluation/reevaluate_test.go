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
// known (tenantID, interactionID) pair, found=false otherwise (covers both
// unknown and foreign-tenant interaction ids, mirroring the real
// tenant-scoped SQL lookup).
type fakeInteractionLookup struct {
	byInteractionID map[string]evaluation.ReEvaluationInput // key: tenantID+"|"+interactionID
	err             error
	calls           int
}

func (f *fakeInteractionLookup) GetInteractionForReEvaluation(_ context.Context, tenantID, interactionID string) (evaluation.ReEvaluationInput, bool, error) {
	f.calls++
	if f.err != nil {
		return evaluation.ReEvaluationInput{}, false, f.err
	}
	in, found := f.byInteractionID[tenantID+"|"+interactionID]
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
	return reEvaluationServiceWithDetector(store, interactions, bundles, fakeDetector{result: detection.Result{Outcome: detection.OutcomePass, Rationale: "inside window"}})
}

func reEvaluationServiceWithDetector(store evaluation.EvaluationStore, interactions evaluation.InteractionLookup, bundles evaluation.BundleVersionLookup, detector fakeDetector) evaluation.Service {
	return evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: detector},
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
	interactions := &fakeInteractionLookup{byInteractionID: map[string]evaluation.ReEvaluationInput{
		"tenant-a|interaction-1": {TenantID: "tenant-a", Interaction: interaction},
	}}
	bundles := fakeBundleVersionLookup{versions: map[string]string{
		"tenant-a|bundle-v1": "v1",
	}}

	store := &fakeEvaluationStore{}
	svc := reEvaluationService(store, interactions, bundles)

	got1, err := svc.ReEvaluateInteraction(context.Background(), "tenant-a", "interaction-1", "bundle-v1")
	if err != nil {
		t.Fatalf("ReEvaluateInteraction (first call): %v", err)
	}
	got2, err := svc.ReEvaluateInteraction(context.Background(), "tenant-a", "interaction-1", "bundle-v1")
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
	interactions := &fakeInteractionLookup{byInteractionID: map[string]evaluation.ReEvaluationInput{
		"tenant-a|interaction-1": {TenantID: "tenant-a", Interaction: interaction},
	}}
	bundles := fakeBundleVersionLookup{versions: map[string]string{}} // no bundle known at all

	store := &fakeEvaluationStore{}
	svc := reEvaluationService(store, interactions, bundles)

	_, err := svc.ReEvaluateInteraction(context.Background(), "tenant-a", "interaction-1", "unknown-bundle")
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
	interactions := &fakeInteractionLookup{byInteractionID: map[string]evaluation.ReEvaluationInput{}}
	bundles := fakeBundleVersionLookup{versions: map[string]string{"tenant-a|bundle-v1": "v1"}}

	store := &fakeEvaluationStore{}
	svc := reEvaluationService(store, interactions, bundles)

	_, err := svc.ReEvaluateInteraction(context.Background(), "tenant-a", "unknown-interaction", "bundle-v1")
	if !errors.Is(err, evaluation.ErrInteractionNotFound) {
		t.Fatalf("err = %v, want ErrInteractionNotFound", err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("Store.CreateEvaluation was called %d times, want 0 on unknown interaction id", len(store.calls))
	}
}

// TestServiceReEvaluateInteractionCrossTenantNeverRunsPipeline covers the
// judgment-day finding that the tenant check must precede any re-evaluation
// work: an interactionID that exists but belongs to a DIFFERENT tenant than
// the authenticated caller must resolve to ErrInteractionNotFound WITHOUT
// ever invoking a detector (and, by the same code path, without ever
// invoking the judge) — sending a foreign tenant's transcript to an
// external LLM judge before the tenant check is a data leak.
func TestServiceReEvaluateInteractionCrossTenantNeverRunsPipeline(t *testing.T) {
	interaction := detection.Interaction{
		OccurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		DebtorTimezone: "America/Mexico_City",
	}
	// interaction-1 exists, but only for tenant-a. tenant-b requesting it is
	// the cross-tenant case; the fake mirrors the real tenant-scoped SQL
	// lookup, which simply has no row for (tenant-b, interaction-1).
	interactions := &fakeInteractionLookup{byInteractionID: map[string]evaluation.ReEvaluationInput{
		"tenant-a|interaction-1": {TenantID: "tenant-a", Interaction: interaction},
	}}
	bundles := fakeBundleVersionLookup{versions: map[string]string{"tenant-a|bundle-v1": "v1"}}

	store := &fakeEvaluationStore{}
	var detectorCalls int
	detector := fakeDetector{result: detection.Result{Outcome: detection.OutcomePass}, calls: &detectorCalls}
	svc := reEvaluationServiceWithDetector(store, interactions, bundles, detector)

	_, err := svc.ReEvaluateInteraction(context.Background(), "tenant-b", "interaction-1", "bundle-v1")
	if !errors.Is(err, evaluation.ErrInteractionNotFound) {
		t.Fatalf("err = %v, want ErrInteractionNotFound", err)
	}
	if detectorCalls != 0 {
		t.Fatalf("detector was invoked %d times, want 0 (tenant check must precede pipeline execution)", detectorCalls)
	}
	if len(store.calls) != 0 {
		t.Fatalf("Store.CreateEvaluation was called %d times, want 0 on cross-tenant interaction id", len(store.calls))
	}
}
