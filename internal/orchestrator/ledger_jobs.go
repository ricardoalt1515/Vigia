package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/riverqueue/river"
)

const LedgerCheckpointJobKind = "ledger_checkpoint"

type LedgerCheckpointArgs struct {
	TenantID string `json:"tenant_id,omitempty" river:"unique"`
}

func (LedgerCheckpointArgs) Kind() string { return LedgerCheckpointJobKind }

func (a LedgerCheckpointArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type LedgerCheckpointStore interface {
	ListMerkleCheckpointTenants(ctx context.Context) ([]string, error)
	CreateMerkleCheckpoint(ctx context.Context, tenantID string) error
}

type LedgerCheckpointWorker struct {
	river.WorkerDefaults[LedgerCheckpointArgs]
	store LedgerCheckpointStore
}

func NewLedgerCheckpointWorker(store LedgerCheckpointStore) *LedgerCheckpointWorker {
	return &LedgerCheckpointWorker{store: store}
}

func (w *LedgerCheckpointWorker) Work(ctx context.Context, job *river.Job[LedgerCheckpointArgs]) error {
	if job.Args.TenantID != "" {
		return w.createForTenant(ctx, job.Args.TenantID)
	}

	tenantIDs, err := w.store.ListMerkleCheckpointTenants(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, tenantID := range tenantIDs {
		if err := w.createForTenant(ctx, tenantID); err != nil {
			errs = append(errs, fmt.Errorf("checkpoint tenant %s: %w", tenantID, err))
		}
	}
	return errors.Join(errs...)
}

func (w *LedgerCheckpointWorker) createForTenant(ctx context.Context, tenantID string) error {
	err := w.store.CreateMerkleCheckpoint(ctx, tenantID)
	if errors.Is(err, ledger.ErrNoNewCheckpointRecords) {
		return nil
	}
	return err
}

func NewLedgerCheckpointPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) { return LedgerCheckpointArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: false},
	)
}
