package evaluation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
)

// fakeBundleResolver is a table-driven test double for
// evaluation.BundleResolver: it returns a canned (version, id, found, err)
// tuple regardless of tenantID, so tests can drive every branch of the
// stamping seam without a database.
type fakeBundleResolver struct {
	version string
	id      string
	found   bool
	err     error
}

func (f fakeBundleResolver) ActiveBundle(_ context.Context, _ string) (string, string, bool, error) {
	return f.version, f.id, f.found, f.err
}

// TestServiceStampsResolvedBundleVersion covers *Evaluations Are Stamped
// With the Resolved Bundle Version* (unit half): an active bundle resolves
// to real version+id on CreateEvaluationInput; a not-found or nil resolver
// keeps today's ""/nil sentinel; a resolver error must not hard-fail the
// evaluation (Design Decision 3).
func TestServiceStampsResolvedBundleVersion(t *testing.T) {
	interaction := detection.Interaction{
		OccurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		DebtorTimezone: "America/Mexico_City",
	}
	newPassingService := func(store *fakeEvaluationStore, resolver evaluation.BundleResolver) evaluation.Service {
		return evaluation.Service{
			Detectors: []evaluation.NamedDetector{
				{Code: "contact-hours", Detector: fakeDetector{result: detection.Result{Outcome: detection.OutcomePass}}},
			},
			Store:    store,
			Resolver: resolver,
		}
	}

	t.Run("active bundle stamps real version and id", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := newPassingService(store, fakeBundleResolver{version: "v2", id: "bundle-2", found: true})

		_, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-bundle-1",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		call := store.calls[0]
		if call.PolicyBundleVersion != "v2" {
			t.Fatalf("PolicyBundleVersion = %q, want %q", call.PolicyBundleVersion, "v2")
		}
		if call.PolicyBundleID == nil || *call.PolicyBundleID != "bundle-2" {
			t.Fatalf("PolicyBundleID = %v, want pointer to %q", call.PolicyBundleID, "bundle-2")
		}
	})

	t.Run("not found resolver keeps empty sentinel", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := newPassingService(store, fakeBundleResolver{found: false})

		_, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-bundle-2",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		call := store.calls[0]
		if call.PolicyBundleVersion != "" {
			t.Fatalf("PolicyBundleVersion = %q, want empty sentinel", call.PolicyBundleVersion)
		}
		if call.PolicyBundleID != nil {
			t.Fatalf("PolicyBundleID = %v, want nil", call.PolicyBundleID)
		}
	})

	t.Run("nil resolver keeps empty sentinel (pre-#6 behavior)", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := newPassingService(store, nil)

		_, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-bundle-3",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		call := store.calls[0]
		if call.PolicyBundleVersion != "" {
			t.Fatalf("PolicyBundleVersion = %q, want empty sentinel", call.PolicyBundleVersion)
		}
		if call.PolicyBundleID != nil {
			t.Fatalf("PolicyBundleID = %v, want nil", call.PolicyBundleID)
		}
	})

	t.Run("resolver error does not hard-fail the evaluation", func(t *testing.T) {
		store := &fakeEvaluationStore{}
		svc := newPassingService(store, fakeBundleResolver{err: errors.New("resolver transport error")})

		got, err := svc.EvaluateInteraction(context.Background(), evaluation.EvaluateInteractionInput{
			TenantID:           "tenant-a",
			InteractionEventID: "interaction-bundle-4",
			Interaction:        interaction,
		})
		if err != nil {
			t.Fatalf("EvaluateInteraction must not hard-fail on a resolver error: %v", err)
		}
		if got.OverallOutcome != "pass" {
			t.Fatalf("OverallOutcome = %q, want pass despite the resolver error", got.OverallOutcome)
		}
		call := store.calls[0]
		if call.PolicyBundleVersion != "" {
			t.Fatalf("PolicyBundleVersion = %q, want empty sentinel on resolver error", call.PolicyBundleVersion)
		}
		if call.PolicyBundleID != nil {
			t.Fatalf("PolicyBundleID = %v, want nil on resolver error", call.PolicyBundleID)
		}
	})
}
