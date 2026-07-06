package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/config"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/ricardoalt1515/vigia/internal/postgres"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := postgres.NewComplaintCaseStoreFromPool(pool)
	reportStore := postgres.NewRedecoReportStoreFromPool(pool)
	workers := river.NewWorkers()
	river.AddWorker(workers, orchestrator.NewComplaintPollWorker(store, orchestrator.RiverContextTransitionEnqueuer{}, orchestrator.ComplaintJobSettings{}))
	river.AddWorker(workers, orchestrator.NewComplaintTransitionWorker(store, orchestrator.ComplaintJobSettings{}))
	river.AddWorker(workers, orchestrator.NewRedecoMonthlyReportWorker(reportStore, orchestrator.ComplaintJobSettings{}))

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 1},
		},
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{
			orchestrator.NewComplaintPeriodicJob(),
			orchestrator.NewRedecoMonthlyReportPeriodicJob(),
		},
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
