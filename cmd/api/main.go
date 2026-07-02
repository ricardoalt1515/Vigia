package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/auth"
	"github.com/ricardoalt1515/vigia/internal/config"
	"github.com/ricardoalt1515/vigia/internal/httpapi"
	"github.com/ricardoalt1515/vigia/internal/postgres"
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

	keyStore := postgres.NewTenantAPIKeyStoreFromPool(pool)
	reader := postgres.NewInteractionReaderFromPool(pool)
	summary := postgres.NewSummaryReaderFromPool(pool)
	evidence := postgres.NewEvidenceReaderFromPool(pool)
	server := httpapi.NewServer(auth.NewAuthenticator(keyStore, time.Now), reader, summary, evidence)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return http.ListenAndServe(addr, server)
}
