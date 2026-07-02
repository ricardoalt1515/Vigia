package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/ledger"
)

var errBoom = errors.New("boom")

type fakeChainVerifier struct {
	result ledger.VerifyResult
	err    error
}

func (f *fakeChainVerifier) VerifyChain(ctx context.Context, tenantID string) (ledger.VerifyResult, error) {
	return f.result, f.err
}

func TestRunIntactChain(t *testing.T) {
	store := &fakeChainVerifier{result: ledger.VerifyResult{OK: true, Count: 3}}
	var out bytes.Buffer

	code := run(context.Background(), []string{"-tenant-id", "11111111-1111-1111-1111-111111111111"}, store, &out)

	if code != 0 {
		t.Fatalf("run() exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "intact") {
		t.Fatalf("output = %q, want it to mention the chain is intact", out.String())
	}
}

func TestRunBrokenChain(t *testing.T) {
	store := &fakeChainVerifier{result: ledger.VerifyResult{OK: false, Count: 3, BreakAtSeq: 2, BreakReason: "hash mismatch"}}
	var out bytes.Buffer

	code := run(context.Background(), []string{"-tenant-id", "11111111-1111-1111-1111-111111111111"}, store, &out)

	if code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "BROKEN") {
		t.Fatalf("output = %q, want it to report the chain as BROKEN", out.String())
	}
	if !strings.Contains(out.String(), "2") || !strings.Contains(out.String(), "hash mismatch") {
		t.Fatalf("output = %q, want it to name the first-break seq and reason", out.String())
	}
}

func TestRunMissingTenantIDFlag(t *testing.T) {
	store := &fakeChainVerifier{result: ledger.VerifyResult{OK: true}}
	var out bytes.Buffer

	code := run(context.Background(), []string{}, store, &out)

	if code != 2 {
		t.Fatalf("run() exit code = %d, want 2 (usage error)", code)
	}
}

func TestRunVerifyChainError(t *testing.T) {
	store := &fakeChainVerifier{err: errBoom}
	var out bytes.Buffer

	code := run(context.Background(), []string{"-tenant-id", "11111111-1111-1111-1111-111111111111"}, store, &out)

	if code != 2 {
		t.Fatalf("run() exit code = %d, want 2 (operational error)", code)
	}
}
