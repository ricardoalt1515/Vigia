package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/config"
	"github.com/ricardoalt1515/vigia/internal/observability"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/ricardoalt1515/vigia/internal/postgres"
	"github.com/ricardoalt1515/vigia/internal/timestamping"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

type runtimePlan struct {
	RegisterLedgerCheckpoint bool
}

func planRuntime(cfg config.Config) runtimePlan {
	return runtimePlan{RegisterLedgerCheckpoint: cfg.RFC3161TSAURL != ""}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	if cfg.AppDatabaseURL == "" {
		return fmt.Errorf("APP_DATABASE_URL is required for cmd/worker so tenant jobs run under the restricted app role")
	}
	shutdownTracing, err := observability.ConfigureTracing(ctx, cfg.OTLPTraceEndpoint)
	if err != nil {
		return err
	}
	defer shutdownTracing(context.Background())

	pool, err := pgxpool.New(ctx, cfg.AppDatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	reportStore := postgres.NewRedecoReportStoreFromPool(pool)
	workers := river.NewWorkers()
	periodicJobs := []*river.PeriodicJob{
		orchestrator.NewComplaintPeriodicJob(),
		orchestrator.NewRedecoMonthlyReportPeriodicJob(),
	}
	river.AddWorker(workers, orchestrator.NewComplaintPollWorker(store, orchestrator.RiverContextTransitionEnqueuer{}, orchestrator.ComplaintJobSettings{}))
	river.AddWorker(workers, orchestrator.NewComplaintTransitionWorker(store, orchestrator.ComplaintJobSettings{}))
	river.AddWorker(workers, orchestrator.NewRedecoMonthlyReportWorker(reportStore, orchestrator.ComplaintJobSettings{}))
	plan := planRuntime(cfg)
	if plan.RegisterLedgerCheckpoint {
		checkpointer := postgres.NewMerkleCheckpointerFromPool(pool, timestamping.RFC3161Client{URL: cfg.RFC3161TSAURL}, cfg.RFC3161TSAURL)
		river.AddWorker(workers, orchestrator.NewLedgerCheckpointWorker(checkpointer))
		periodicJobs = append(periodicJobs, orchestrator.NewLedgerCheckpointPeriodicJob())
	}

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 1},
		},
		Workers:      workers,
		PeriodicJobs: periodicJobs,
	})
	if err != nil {
		return err
	}

	if err = client.Start(ctx); err != nil {
		return err
	}

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-sigCtx.Done()
	log.Println("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return client.Stop(shutdownCtx)
}
