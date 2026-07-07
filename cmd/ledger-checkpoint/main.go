// Command ledger-checkpoint creates an RFC 3161-timestamped Merkle checkpoint
// for one tenant's evidence records that have not yet been checkpointed.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/config"
	"github.com/ricardoalt1515/vigia/internal/postgres"
	"github.com/ricardoalt1515/vigia/internal/timestamping"
)

type Checkpointer interface {
	CreateCheckpoint(ctx context.Context, tenantID string) (postgres.MerkleCheckpointResult, error)
}

const (
	exitCreated            = 0
	exitUsageOrOperational = 2
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], nil, os.Stdout))
}

func run(ctx context.Context, args []string, checkpointer Checkpointer, out io.Writer) int {
	flags := flag.NewFlagSet("ledger-checkpoint", flag.ContinueOnError)
	flags.SetOutput(out)
	tenantID := flags.String("tenant-id", "", "tenant UUID to checkpoint (required)")
	tsaURL := flags.String("tsa-url", "", "RFC 3161 TSA URL (defaults to RFC3161_TSA_URL)")
	if err := flags.Parse(args); err != nil {
		return exitUsageOrOperational
	}
	if *tenantID == "" {
		fmt.Fprintln(out, "error: -tenant-id is required")
		return exitUsageOrOperational
	}

	if checkpointer == nil {
		cfg, err := config.LoadFromEnv()
		if err != nil {
			fmt.Fprintf(out, "error: load config: %v\n", err)
			return exitUsageOrOperational
		}
		url := *tsaURL
		if url == "" {
			url = cfg.RFC3161TSAURL
		}
		if url == "" {
			fmt.Fprintln(out, "error: -tsa-url or RFC3161_TSA_URL is required")
			return exitUsageOrOperational
		}
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(out, "error: connect database: %v\n", err)
			return exitUsageOrOperational
		}
		defer pool.Close()
		checkpointer = postgres.NewMerkleCheckpointerFromPool(pool, timestamping.RFC3161Client{URL: url}, url)
	}

	checkpoint, err := checkpointer.CreateCheckpoint(ctx, *tenantID)
	if err != nil {
		fmt.Fprintf(out, "error: create checkpoint: %v\n", err)
		return exitUsageOrOperational
	}
	fmt.Fprintf(out, "checkpoint created: tenant=%s seq=%d..%d records=%d root=%s tsa=%s\n", checkpoint.TenantID, checkpoint.FirstSeq, checkpoint.LastSeq, checkpoint.RecordCount, checkpoint.RootHash, checkpoint.TSAURL)
	return exitCreated
}
