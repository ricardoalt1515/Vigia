package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/riverqueue/river"
)

func TestLedgerCheckpointWorkerCreatesForAllTenantsAndIgnoresNoNewEvidence(t *testing.T) {
	store := &fakeLedgerCheckpointStore{
		tenants: []string{"tenant-a", "tenant-b", "tenant-c"},
		errors:  map[string]error{"tenant-b": ledger.ErrNoNewCheckpointRecords},
	}
	worker := NewLedgerCheckpointWorker(store)

	if err := worker.Work(context.Background(), &river.Job[LedgerCheckpointArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	want := []string{"tenant-a", "tenant-b", "tenant-c"}
	if !reflect.DeepEqual(store.created, want) {
		t.Fatalf("created tenants = %#v, want %#v", store.created, want)
	}
}

func TestLedgerCheckpointWorkerReturnsTenantErrors(t *testing.T) {
	boom := errors.New("tsa unavailable")
	store := &fakeLedgerCheckpointStore{tenants: []string{"tenant-a"}, errors: map[string]error{"tenant-a": boom}}
	worker := NewLedgerCheckpointWorker(store)

	err := worker.Work(context.Background(), &river.Job[LedgerCheckpointArgs]{})
	if !errors.Is(err, boom) {
		t.Fatalf("Work error = %v, want %v", err, boom)
	}
}

func TestLedgerCheckpointWorkerCanRunOneTenant(t *testing.T) {
	store := &fakeLedgerCheckpointStore{tenants: []string{"tenant-a", "tenant-b"}}
	worker := NewLedgerCheckpointWorker(store)

	if err := worker.Work(context.Background(), &river.Job[LedgerCheckpointArgs]{Args: LedgerCheckpointArgs{TenantID: "tenant-b"}}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	want := []string{"tenant-b"}
	if !reflect.DeepEqual(store.created, want) {
		t.Fatalf("created tenants = %#v, want %#v", store.created, want)
	}
}

type fakeLedgerCheckpointStore struct {
	tenants []string
	errors  map[string]error
	created []string
}

func (s *fakeLedgerCheckpointStore) ListMerkleCheckpointTenants(context.Context) ([]string, error) {
	return s.tenants, nil
}

func (s *fakeLedgerCheckpointStore) CreateMerkleCheckpoint(_ context.Context, tenantID string) error {
	s.created = append(s.created, tenantID)
	if s.errors != nil {
		return s.errors[tenantID]
	}
	return nil
}
