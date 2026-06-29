package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// TestWorkerIntegration boots a River client against a migrated Postgres,
// inserts one NoopJob, waits for it to reach the completed state, then stops
// the client cleanly. It is skipped when running with -short or when
// DATABASE_URL is absent, mirroring the skip pattern from rls_isolation_test.go.
func TestWorkerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping River integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for the River integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &NoopWorker{})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 1},
		},
		Workers: workers,
	})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}

	if err = client.Start(ctx); err != nil {
		t.Fatalf("start river client: %v", err)
	}

	insertResult, err := client.Insert(ctx, NoopJob{}, nil)
	if err != nil {
		t.Fatalf("insert NoopJob: %v", err)
	}

	jobID := insertResult.Job.ID

	// Poll until the job reaches completed state (or timeout fires via ctx).
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		row := pool.QueryRow(ctx,
			"SELECT state FROM river_job WHERE id = $1", jobID)
		var state string
		if err := row.Scan(&state); err != nil {
			t.Fatalf("query river_job state: %v", err)
		}
		if state == "completed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify final state.
	row := pool.QueryRow(ctx,
		"SELECT state FROM river_job WHERE id = $1", jobID)
	var finalState string
	if err := row.Scan(&finalState); err != nil {
		t.Fatalf("query final river_job state: %v", err)
	}
	if finalState != "completed" {
		t.Errorf("job %d: got state %q, want %q", jobID, finalState, "completed")
	}

	shutdownCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := client.Stop(shutdownCtx); err != nil {
		t.Fatalf("stop river client: %v", err)
	}
}
