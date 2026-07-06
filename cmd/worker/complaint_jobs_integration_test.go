package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func TestComplaintTransitionUniqueOptsDedupeDuplicateReviews(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping River integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		requireRiverDatabaseURLOrSkip(t)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, orchestrator.NewComplaintTransitionWorker(&dedupeTestTransitionStore{}, orchestrator.ComplaintJobSettings{}))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	caseID := uuid.NewString()
	firstReviewID := uuid.NewString()
	secondReviewID := uuid.NewString()
	first := orchestrator.ComplaintTransitionArgs{TenantID: uuid.NewString(), ComplaintCaseID: caseID, TransitionKind: string(orchestrator.TransitionApprove), HumanReviewID: &firstReviewID}
	second := orchestrator.ComplaintTransitionArgs{TenantID: first.TenantID, ComplaintCaseID: caseID, TransitionKind: string(orchestrator.TransitionApprove), HumanReviewID: &secondReviewID}

	if _, err := client.Insert(ctx, first, nil); err != nil {
		t.Fatalf("insert first transition: %v", err)
	}
	if _, err := client.Insert(ctx, second, nil); err != nil {
		t.Fatalf("insert duplicate transition: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
SELECT count(*)
FROM river_job
WHERE kind = $1
  AND args->>'complaint_case_id' = $2
  AND args->>'transition_kind' = $3`, orchestrator.ComplaintTransitionJobKind, caseID, string(orchestrator.TransitionApprove)).Scan(&count); err != nil {
		t.Fatalf("count duplicate jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("duplicate approve reviews inserted %d jobs, want 1", count)
	}
}

type dedupeTestTransitionStore struct{}

func (*dedupeTestTransitionStore) ApplyTransition(context.Context, orchestrator.ApplyComplaintTransitionInput) (orchestrator.ApplyComplaintTransitionResult, error) {
	return orchestrator.ApplyComplaintTransitionResult{}, nil
}

func requireRiverDatabaseURLOrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("BUILDKITE") != "" {
		t.Fatalf("DATABASE_URL is required in CI for the River unique-options integration test")
	}
	t.Skip("DATABASE_URL is required for the River integration test")
}
