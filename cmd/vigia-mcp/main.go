package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
	"github.com/ricardoalt1515/vigia/internal/mcp"
)

func main() {
	if err := run(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "vigia-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	cases, _, err := labtools.Load()
	if err != nil {
		return fmt.Errorf("load synthetic fixtures: %w", err)
	}
	server := mcp.NewServer(mcp.Config{
		Authenticator: mcp.StaticBearerAuthenticator{
			Token:    os.Getenv("VIGIA_MCP_API_KEY"),
			TenantID: os.Getenv("VIGIA_MCP_TENANT_ID"),
			KeyID:    os.Getenv("VIGIA_MCP_KEY_ID"),
		},
		Index:     mcp.NewSyntheticIndex(cases),
		AuditSink: &mcp.MemoryAuditSink{},
	})
	return server.ServeJSONLines(ctx, stdin, stdout)
}
