package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/postgres"
)

func TestRunRequiresTenantID(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), nil, fakeCheckpointer{}, &out)
	if code != exitUsageOrOperational {
		t.Fatalf("exit = %d, want %d", code, exitUsageOrOperational)
	}
	if !strings.Contains(out.String(), "-tenant-id is required") {
		t.Fatalf("output = %q, want tenant-id error", out.String())
	}
}

func TestRunCreatesCheckpoint(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), []string{"-tenant-id", "tenant-1"}, fakeCheckpointer{result: postgres.MerkleCheckpointResult{TenantID: "tenant-1", FirstSeq: 1, LastSeq: 3, RecordCount: 3, RootHash: "abc", TSAURL: "https://tsa.example.test"}}, &out)
	if code != exitCreated {
		t.Fatalf("exit = %d, want %d", code, exitCreated)
	}
	if !strings.Contains(out.String(), "checkpoint created: tenant=tenant-1 seq=1..3 records=3 root=abc") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunReportsCheckpointError(t *testing.T) {
	var out bytes.Buffer
	boom := errors.New("broken chain")
	code := run(context.Background(), []string{"-tenant-id", "tenant-1"}, fakeCheckpointer{err: boom}, &out)
	if code != exitUsageOrOperational {
		t.Fatalf("exit = %d, want %d", code, exitUsageOrOperational)
	}
	if !strings.Contains(out.String(), "broken chain") {
		t.Fatalf("output = %q, want error", out.String())
	}
}

type fakeCheckpointer struct {
	result postgres.MerkleCheckpointResult
	err    error
}

func (f fakeCheckpointer) CreateCheckpoint(context.Context, string) (postgres.MerkleCheckpointResult, error) {
	return f.result, f.err
}
