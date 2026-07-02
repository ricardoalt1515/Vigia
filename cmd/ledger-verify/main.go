// Command ledger-verify is an operator binary that runs the evidence
// ledger's store-backed VerifyChain for one tenant and reports whether the
// chain is intact or where it broke. It follows cmd/seed's testable
// run(ctx, args) seam style.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/config"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// ChainVerifier is the minimal port run() needs: verify one tenant's chain.
// internal/postgres.ChainVerifier implements this against real Postgres;
// tests supply a fake.
type ChainVerifier interface {
	VerifyChain(ctx context.Context, tenantID string) (ledger.VerifyResult, error)
}

// Exit codes: 0 intact, 1 broken chain, 2 usage/operational error. The
// split lets a caller (CI, a future daily job) shell out and branch on
// integrity vs. infrastructure failure.
const (
	exitIntact             = 0
	exitBroken             = 1
	exitUsageOrOperational = 2
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], nil, os.Stdout))
}

// run resolves the -tenant-id flag, verifies the tenant's chain via store
// (constructing a real Postgres-backed ChainVerifier when store is nil —
// i.e. only in main()), and writes a human-readable report to out.
func run(ctx context.Context, args []string, store ChainVerifier, out io.Writer) int {
	flags := flag.NewFlagSet("ledger-verify", flag.ContinueOnError)
	flags.SetOutput(out)
	tenantID := flags.String("tenant-id", "", "tenant UUID to verify (required)")
	if err := flags.Parse(args); err != nil {
		return exitUsageOrOperational
	}
	if *tenantID == "" {
		fmt.Fprintln(out, "error: -tenant-id is required")
		return exitUsageOrOperational
	}

	if store == nil {
		cfg, err := config.LoadFromEnv()
		if err != nil {
			fmt.Fprintf(out, "error: load config: %v\n", err)
			return exitUsageOrOperational
		}
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(out, "error: connect database: %v\n", err)
			return exitUsageOrOperational
		}
		defer pool.Close()
		store = postgres.NewChainVerifierFromPool(pool)
	}

	result, err := store.VerifyChain(ctx, *tenantID)
	if err != nil {
		fmt.Fprintf(out, "error: verify chain: %v\n", err)
		return exitUsageOrOperational
	}

	if result.OK {
		fmt.Fprintf(out, "chain intact: tenant=%s records=%d\n", *tenantID, result.Count)
		return exitIntact
	}

	fmt.Fprintf(out, "chain BROKEN: tenant=%s first_break_seq=%d expected_seq=%d reason=%s\n", *tenantID, result.BreakAtSeq, result.ExpectedSeq, result.BreakReason)
	return exitBroken
}
