package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

func TestComplaintStoreRejectsNonHumanHumanReviewIDBeforeDatabaseAccess(t *testing.T) {
	reviewID := "11111111-1111-1111-1111-111111111111"
	store := postgres.NewComplaintCaseStore(nil)

	got, err := store.ApplyTransition(context.Background(), orchestrator.ApplyComplaintTransitionInput{
		TenantID:        "not-parsed-before-kind-validation",
		ComplaintCaseID: "not-parsed-before-kind-validation",
		Kind:            orchestrator.TransitionRequestReview,
		HumanReviewID:   &reviewID,
	})
	if err == nil || !strings.Contains(err.Error(), "only valid for approve or override") {
		t.Fatalf("ApplyTransition request_review with human review id error = %v, want validation rejection", err)
	}
	if got.Applied {
		t.Fatalf("ApplyTransition request_review with human review id applied = true, want false")
	}
}

func TestComplaintStoreCreatesCaseIdempotentlyAndAppendsOpenEvidenceOnce(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "complaint-create")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "complaint/create")
	store := postgres.NewComplaintCaseStoreFromPool(pool)
	openedAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)
	slaDueAt := openedAt.AddDate(0, 0, 14)

	first, err := store.CreateComplaintCase(ctx, orchestrator.CreateComplaintCaseInput{
		TenantID:        tenantID,
		InteractionID:   interactionID,
		RedecoCause:     "improper_contact",
		OpenedAt:        openedAt,
		SLADueAt:        slaDueAt,
		CalendarVersion: "mx-lft-art-74-2026a",
		IdempotencyKey:  "idem-create-1",
	})
	if err != nil {
		t.Fatalf("CreateComplaintCase first: %v", err)
	}
	second, err := store.CreateComplaintCase(ctx, orchestrator.CreateComplaintCaseInput{
		TenantID:        tenantID,
		InteractionID:   interactionID,
		RedecoCause:     "improper_contact",
		OpenedAt:        openedAt,
		SLADueAt:        slaDueAt,
		CalendarVersion: "mx-lft-art-74-2026a",
		IdempotencyKey:  "idem-create-1",
	})
	if err != nil {
		t.Fatalf("CreateComplaintCase retry: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("retry returned case %q, want original %q", second.ID, first.ID)
	}
	if second.Created {
		t.Fatal("retry reported Created=true, want idempotent existing case")
	}

	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_records WHERE tenant_id = $1 AND complaint_case_id = $2`, tenantID, first.ID).Scan(&evidenceCount); err != nil {
		t.Fatalf("count complaint evidence: %v", err)
	}
	if evidenceCount != 1 {
		t.Fatalf("complaint evidence rows = %d, want 1", evidenceCount)
	}
	_ = debtorID
}

func TestComplaintStoreTransitionNoOpDoesNotAppendEvidence(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "complaint-noop")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "complaint/noop")
	store := postgres.NewComplaintCaseStoreFromPool(pool)
	openedAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)
	created, err := store.CreateComplaintCase(ctx, orchestrator.CreateComplaintCaseInput{
		TenantID: tenantID, InteractionID: interactionID, RedecoCause: "improper_contact",
		OpenedAt: openedAt, SLADueAt: openedAt.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "idem-noop-1",
	})
	if err != nil {
		t.Fatalf("CreateComplaintCase: %v", err)
	}

	got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: tenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionApprove, Now: openedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ApplyTransition invalid approve: %v", err)
	}
	if got.Applied {
		t.Fatal("invalid approve from open applied, want CAS no-op")
	}

	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_records WHERE tenant_id = $1 AND complaint_case_id = $2`, tenantID, created.ID).Scan(&evidenceCount); err != nil {
		t.Fatalf("count complaint evidence: %v", err)
	}
	if evidenceCount != 1 {
		t.Fatalf("complaint evidence rows after no-op = %d, want only open evidence", evidenceCount)
	}
}

func TestComplaintStoreTenantPredicateBlocksCrossTenantTransition(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "complaint-tenant-a")
	tenantB, _ := seedTenantAndDebtor(t, ctx, pool, "complaint-tenant-b")
	interactionID := seedInteraction(t, ctx, pool, tenantA, debtorA, "complaint/tenant")
	store := postgres.NewComplaintCaseStoreFromPool(pool)
	openedAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)
	created, err := store.CreateComplaintCase(ctx, orchestrator.CreateComplaintCaseInput{
		TenantID: tenantA, InteractionID: interactionID, RedecoCause: "improper_contact",
		OpenedAt: openedAt, SLADueAt: openedAt.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "idem-tenant-1",
	})
	if err != nil {
		t.Fatalf("CreateComplaintCase: %v", err)
	}

	got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: tenantB, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionRequestReview, Now: openedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ApplyTransition with wrong tenant: %v", err)
	}
	if got.Applied {
		t.Fatal("cross-tenant transition applied, want tenant predicate no-op")
	}
}

func TestComplaintStoreApproveRequiresHumanReviewID(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	created, reviewExpiresAt := createAwaitingReviewComplaintCase(t, ctx, pool, store, "complaint-missing-review")

	got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: created.TenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionApprove,
		Now: reviewExpiresAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("ApplyTransition approve without human review: %v", err)
	}
	if got.Applied {
		t.Fatal("approve without human review applied, want no-op")
	}
	assertComplaintCaseState(t, ctx, pool, created.TenantID, created.ID, string(orchestrator.ComplaintStateAwaitingReview))
	assertComplaintEvidenceCount(t, ctx, pool, created.TenantID, created.ID, 2)
}

func TestComplaintStoreApproveRejectsWrongCaseHumanReviewID(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	caseA, reviewExpiresAt := createAwaitingReviewComplaintCase(t, ctx, pool, store, "complaint-wrong-review-a")
	caseB, _ := createAwaitingReviewComplaintCase(t, ctx, pool, store, "complaint-wrong-review-b")
	wrongReviewID := insertHumanReview(t, ctx, pool, caseB.TenantID, caseB.ID, "approve", "wrong-case-reviewer")

	got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: caseA.TenantID, ComplaintCaseID: caseA.ID, Kind: orchestrator.TransitionApprove,
		Now: reviewExpiresAt.Add(-time.Hour), HumanReviewID: &wrongReviewID,
	})
	if err != nil {
		t.Fatalf("ApplyTransition approve with wrong-case human review: %v", err)
	}
	if got.Applied {
		t.Fatal("approve with wrong-case human review applied, want no-op")
	}
	assertComplaintCaseState(t, ctx, pool, caseA.TenantID, caseA.ID, string(orchestrator.ComplaintStateAwaitingReview))
	assertHumanReviewUnprocessed(t, ctx, pool, wrongReviewID)
	assertComplaintEvidenceCount(t, ctx, pool, caseA.TenantID, caseA.ID, 2)
}

func TestComplaintStoreApproveRejectsOverrideHumanReviewDecision(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	created, reviewExpiresAt := createAwaitingReviewComplaintCase(t, ctx, pool, store, "complaint-approve-override-mismatch")
	reviewID := insertHumanReview(t, ctx, pool, created.TenantID, created.ID, "override", "mismatch-reviewer")

	got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: created.TenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionApprove,
		Now: reviewExpiresAt.Add(-time.Hour), HumanReviewID: &reviewID,
	})
	if err != nil {
		t.Fatalf("ApplyTransition approve with override human review: %v", err)
	}
	if got.Applied {
		t.Fatal("approve with override human review applied, want decision-kind mismatch no-op")
	}
	assertComplaintCaseState(t, ctx, pool, created.TenantID, created.ID, string(orchestrator.ComplaintStateAwaitingReview))
	assertHumanReviewUnprocessed(t, ctx, pool, reviewID)
	assertComplaintEvidenceCount(t, ctx, pool, created.TenantID, created.ID, 2)
}

func TestComplaintStoreOverrideRejectsApproveHumanReviewDecision(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	created, reviewExpiresAt := createAwaitingReviewComplaintCase(t, ctx, pool, store, "complaint-override-approve-mismatch")
	reviewID := insertHumanReview(t, ctx, pool, created.TenantID, created.ID, "approve", "mismatch-reviewer")

	got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: created.TenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionOverride,
		Now: reviewExpiresAt.Add(-time.Hour), HumanReviewID: &reviewID,
	})
	if err != nil {
		t.Fatalf("ApplyTransition override with approve human review: %v", err)
	}
	if got.Applied {
		t.Fatal("override with approve human review applied, want decision-kind mismatch no-op")
	}
	assertComplaintCaseState(t, ctx, pool, created.TenantID, created.ID, string(orchestrator.ComplaintStateAwaitingReview))
	assertHumanReviewUnprocessed(t, ctx, pool, reviewID)
	assertComplaintEvidenceCount(t, ctx, pool, created.TenantID, created.ID, 2)
}

func TestComplaintStoreApproveProcessesWinnerAndSupersedesDuplicateReviews(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	created, reviewExpiresAt := createAwaitingReviewComplaintCase(t, ctx, pool, store, "complaint-review-bookkeeping")
	winnerID := insertHumanReview(t, ctx, pool, created.TenantID, created.ID, "approve", "winner")
	duplicateID := insertHumanReview(t, ctx, pool, created.TenantID, created.ID, "approve", "duplicate")

	got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: created.TenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionApprove,
		Now: reviewExpiresAt.Add(-time.Hour), HumanReviewID: &winnerID,
	})
	if err != nil {
		t.Fatalf("ApplyTransition approve with same-case human review: %v", err)
	}
	if !got.Applied || got.Case.State != string(orchestrator.ComplaintStateResolved) {
		t.Fatalf("transition result = %+v, want resolved", got)
	}
	assertHumanReviewProcessed(t, ctx, pool, winnerID)
	assertHumanReviewSuperseded(t, ctx, pool, duplicateID)
	assertComplaintEvidenceCount(t, ctx, pool, created.TenantID, created.ID, 3)
}

func TestComplaintStoreLateApprovalAfterEscalationDoesNotResolveOrProcessReview(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	dbNow := databaseNow(t, ctx, pool)
	reviewExpiresAt := dbNow.Add(-time.Hour)
	created, reviewExpiresAt := createAwaitingReviewComplaintCaseWithReviewExpiresAt(t, ctx, pool, store, "complaint-late-review", reviewExpiresAt)
	escalated, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: created.TenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionTTLExpired,
		Now: reviewExpiresAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ApplyTransition ttl_expired: %v", err)
	}
	if !escalated.Applied || escalated.Case.State != string(orchestrator.ComplaintStateEscalated) {
		t.Fatalf("ttl transition result = %+v, want escalated", escalated)
	}
	lateReviewID := forceInsertHumanReview(t, ctx, pool, created.TenantID, created.ID, "approve", "late-reviewer")

	got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: created.TenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionApprove,
		Now: reviewExpiresAt.Add(2 * time.Hour), HumanReviewID: &lateReviewID,
	})
	if err != nil {
		t.Fatalf("ApplyTransition late approve: %v", err)
	}
	if got.Applied {
		t.Fatal("late approval applied after escalation, want no-op")
	}
	assertComplaintCaseState(t, ctx, pool, created.TenantID, created.ID, string(orchestrator.ComplaintStateEscalated))
	assertHumanReviewUnprocessed(t, ctx, pool, lateReviewID)
	assertComplaintEvidenceCount(t, ctx, pool, created.TenantID, created.ID, 3)
}

func TestComplaintStoreNonHumanTransitionsRejectHumanReviewID(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	openedAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)

	t.Run("request_review", func(t *testing.T) {
		tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "complaint-request-review-human-id")
		interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "complaint/request-review-human-id")
		created, err := store.CreateComplaintCase(ctx, orchestrator.CreateComplaintCaseInput{
			TenantID: tenantID, InteractionID: interactionID, RedecoCause: "improper_contact",
			OpenedAt: openedAt, SLADueAt: openedAt.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "idem-request-review-human-id",
		})
		if err != nil {
			t.Fatalf("CreateComplaintCase: %v", err)
		}
		reviewID := forceInsertHumanReview(t, ctx, pool, tenantID, created.ID, "approve", "arbitrary-request-reviewer")

		got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
			TenantID: tenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionRequestReview,
			Now: openedAt.Add(time.Hour), ReviewExpiresAt: openedAt.AddDate(0, 0, 5), HumanReviewID: &reviewID,
		})
		if err == nil {
			t.Fatalf("ApplyTransition request_review with human review id err = nil, want rejection")
		}
		if got.Applied {
			t.Fatalf("ApplyTransition request_review with human review id applied = true, want false")
		}
		assertComplaintCaseState(t, ctx, pool, tenantID, created.ID, string(orchestrator.ComplaintStateOpen))
		assertComplaintEvidenceCount(t, ctx, pool, tenantID, created.ID, 1)
		assertNoComplaintEvidenceHumanReviewID(t, ctx, pool, tenantID, created.ID)
		assertHumanReviewUnprocessed(t, ctx, pool, reviewID)
	})

	t.Run("ttl_expired", func(t *testing.T) {
		created, _ := createAwaitingReviewComplaintCase(t, ctx, pool, store, "complaint-ttl-human-id")
		reviewID := forceInsertHumanReview(t, ctx, pool, created.TenantID, created.ID, "approve", "arbitrary-ttl-reviewer")

		got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
			TenantID: created.TenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionTTLExpired,
			Now: openedAt.Add(2 * time.Hour), HumanReviewID: &reviewID,
		})
		if err == nil {
			t.Fatalf("ApplyTransition ttl_expired with human review id err = nil, want rejection")
		}
		if got.Applied {
			t.Fatalf("ApplyTransition ttl_expired with human review id applied = true, want false")
		}
		assertComplaintCaseState(t, ctx, pool, created.TenantID, created.ID, string(orchestrator.ComplaintStateAwaitingReview))
		assertComplaintEvidenceCount(t, ctx, pool, created.TenantID, created.ID, 2)
		assertNoComplaintEvidenceHumanReviewID(t, ctx, pool, created.TenantID, created.ID)
		assertHumanReviewUnprocessed(t, ctx, pool, reviewID)
	})

	t.Run("sla_breach", func(t *testing.T) {
		tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "complaint-sla-human-id")
		interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "complaint/sla-human-id")
		created, err := store.CreateComplaintCase(ctx, orchestrator.CreateComplaintCaseInput{
			TenantID: tenantID, InteractionID: interactionID, RedecoCause: "improper_contact",
			OpenedAt: openedAt, SLADueAt: openedAt.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "idem-sla-human-id",
		})
		if err != nil {
			t.Fatalf("CreateComplaintCase: %v", err)
		}
		reviewID := forceInsertHumanReview(t, ctx, pool, tenantID, created.ID, "override", "arbitrary-sla-reviewer")

		got, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
			TenantID: tenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionSLABreach,
			Now: openedAt.Add(24 * time.Hour), HumanReviewID: &reviewID,
		})
		if err == nil {
			t.Fatalf("ApplyTransition sla_breach with human review id err = nil, want rejection")
		}
		if got.Applied {
			t.Fatalf("ApplyTransition sla_breach with human review id applied = true, want false")
		}
		assertComplaintCaseState(t, ctx, pool, tenantID, created.ID, string(orchestrator.ComplaintStateOpen))
		assertComplaintEvidenceCount(t, ctx, pool, tenantID, created.ID, 1)
		assertNoComplaintEvidenceHumanReviewID(t, ctx, pool, tenantID, created.ID)
		assertHumanReviewUnprocessed(t, ctx, pool, reviewID)
	})
}

func TestComplaintStoreTransitionAndEvidenceAppendAtomically(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "complaint-atomic")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "complaint/atomic")
	store := postgres.NewComplaintCaseStoreFromPool(pool)
	openedAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)
	created, err := store.CreateComplaintCase(ctx, orchestrator.CreateComplaintCaseInput{
		TenantID: tenantID, InteractionID: interactionID, RedecoCause: "improper_contact",
		OpenedAt: openedAt, SLADueAt: openedAt.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "idem-atomic-1",
	})
	if err != nil {
		t.Fatalf("CreateComplaintCase: %v", err)
	}

	transitioned, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: tenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionRequestReview,
		Now: openedAt.Add(time.Hour), ReviewExpiresAt: openedAt.AddDate(0, 0, 5),
	})
	if err != nil {
		t.Fatalf("ApplyTransition request_review: %v", err)
	}
	if !transitioned.Applied || transitioned.Case.State != string(orchestrator.ComplaintStateAwaitingReview) {
		t.Fatalf("transition result = %+v, want applied awaiting_review", transitioned)
	}

	verifier := postgres.NewChainVerifierFromPool(pool)
	result, err := verifier.VerifyChain(ctx, tenantID)
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if !result.OK || result.Count != 2 {
		t.Fatalf("VerifyChain() = %+v, want OK with open + request_review complaint evidence", result)
	}
}

func createAwaitingReviewComplaintCase(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *postgres.ComplaintCaseStore, suffix string) (orchestrator.ComplaintCase, time.Time) {
	t.Helper()
	// Approval CAS uses the database clock (`review_expires_at > now()`), so keep
	// successful approval fixtures well in the future relative to the DB clock.
	return createAwaitingReviewComplaintCaseWithReviewExpiresAt(t, ctx, pool, store, suffix, databaseNow(t, ctx, pool).AddDate(1, 0, 0))
}

func createAwaitingReviewComplaintCaseWithReviewExpiresAt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *postgres.ComplaintCaseStore, suffix string, reviewExpiresAt time.Time) (orchestrator.ComplaintCase, time.Time) {
	t.Helper()
	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, suffix)
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "complaint/"+suffix)
	openedAt := time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC)
	created, err := store.CreateComplaintCase(ctx, orchestrator.CreateComplaintCaseInput{
		TenantID: tenantID, InteractionID: interactionID, RedecoCause: "improper_contact",
		OpenedAt: openedAt, SLADueAt: openedAt.AddDate(0, 0, 14), CalendarVersion: "mx-lft-art-74-2026a", IdempotencyKey: "idem-" + suffix,
	})
	if err != nil {
		t.Fatalf("CreateComplaintCase: %v", err)
	}
	transitioned, err := store.ApplyTransition(ctx, orchestrator.ApplyComplaintTransitionInput{
		TenantID: tenantID, ComplaintCaseID: created.ID, Kind: orchestrator.TransitionRequestReview,
		Now: openedAt.Add(time.Hour), ReviewExpiresAt: reviewExpiresAt,
	})
	if err != nil {
		t.Fatalf("ApplyTransition request_review: %v", err)
	}
	if !transitioned.Applied || transitioned.Case.State != string(orchestrator.ComplaintStateAwaitingReview) {
		t.Fatalf("request_review result = %+v, want awaiting_review", transitioned)
	}
	return transitioned.Case, reviewExpiresAt
}

func databaseNow(t *testing.T, ctx context.Context, pool *pgxpool.Pool) time.Time {
	t.Helper()
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		t.Fatalf("query database clock: %v", err)
	}
	return now.UTC().Truncate(time.Microsecond)
}

func insertHumanReview(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, complaintCaseID, decision, reviewer string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO human_reviews (tenant_id, complaint_case_id, decision, reviewer, notes)
		SELECT $1, $2, $3, $4, ''
		WHERE EXISTS (
			SELECT 1 FROM complaint_cases WHERE tenant_id = $1 AND id = $2 AND state = 'awaiting_review'
		)
		RETURNING id::text`, tenantID, complaintCaseID, decision, reviewer).Scan(&id); err != nil {
		t.Fatalf("insert human review: %v", err)
	}
	return id
}

func forceInsertHumanReview(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, complaintCaseID, decision, reviewer string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO human_reviews (tenant_id, complaint_case_id, decision, reviewer, notes)
		VALUES ($1, $2, $3, $4, '')
		RETURNING id::text`, tenantID, complaintCaseID, decision, reviewer).Scan(&id); err != nil {
		t.Fatalf("force insert human review: %v", err)
	}
	return id
}

func assertComplaintCaseState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, complaintCaseID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT state FROM complaint_cases WHERE tenant_id = $1 AND id = $2`, tenantID, complaintCaseID).Scan(&got); err != nil {
		t.Fatalf("query complaint case state: %v", err)
	}
	if got != want {
		t.Fatalf("complaint case state = %q, want %q", got, want)
	}
}

func assertComplaintEvidenceCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, complaintCaseID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_records WHERE tenant_id = $1 AND complaint_case_id = $2`, tenantID, complaintCaseID).Scan(&got); err != nil {
		t.Fatalf("count complaint evidence: %v", err)
	}
	if got != want {
		t.Fatalf("complaint evidence rows = %d, want %d", got, want)
	}
}

func assertHumanReviewProcessed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, reviewID string) {
	t.Helper()
	var processed, superseded bool
	if err := pool.QueryRow(ctx, `SELECT processed_at IS NOT NULL, superseded_at IS NOT NULL FROM human_reviews WHERE id = $1`, reviewID).Scan(&processed, &superseded); err != nil {
		t.Fatalf("query human review bookkeeping: %v", err)
	}
	if !processed || superseded {
		t.Fatalf("review %s bookkeeping processed=%v superseded=%v, want processed only", reviewID, processed, superseded)
	}
}

func assertHumanReviewSuperseded(t *testing.T, ctx context.Context, pool *pgxpool.Pool, reviewID string) {
	t.Helper()
	var processed, superseded bool
	if err := pool.QueryRow(ctx, `SELECT processed_at IS NOT NULL, superseded_at IS NOT NULL FROM human_reviews WHERE id = $1`, reviewID).Scan(&processed, &superseded); err != nil {
		t.Fatalf("query human review bookkeeping: %v", err)
	}
	if processed || !superseded {
		t.Fatalf("review %s bookkeeping processed=%v superseded=%v, want superseded only", reviewID, processed, superseded)
	}
}

func assertHumanReviewUnprocessed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, reviewID string) {
	t.Helper()
	var processed, superseded bool
	if err := pool.QueryRow(ctx, `SELECT processed_at IS NOT NULL, superseded_at IS NOT NULL FROM human_reviews WHERE id = $1`, reviewID).Scan(&processed, &superseded); err != nil {
		t.Fatalf("query human review bookkeeping: %v", err)
	}
	if processed || superseded {
		t.Fatalf("review %s bookkeeping processed=%v superseded=%v, want unprocessed", reviewID, processed, superseded)
	}
}

func assertNoComplaintEvidenceHumanReviewID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, complaintCaseID string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM evidence_records
		WHERE tenant_id = $1
			AND complaint_case_id = $2
			AND human_review_id IS NOT NULL`, tenantID, complaintCaseID).Scan(&count); err != nil {
		t.Fatalf("count complaint evidence human review ids: %v", err)
	}
	if count != 0 {
		t.Fatalf("complaint evidence rows with human_review_id = %d, want 0", count)
	}
}
