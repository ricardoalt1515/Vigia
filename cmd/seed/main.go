package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/auth"
	"github.com/ricardoalt1515/vigia/internal/config"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

const tenantAPIKeyPrefix = "vigia_tenant_"
const randomKeyBytes = 32

type CreateTenantAPIKeyParams struct {
	TenantID string
	KeyHash  string
	Label    string
	Status   string
}

type TenantAPIKeyCreator interface {
	CreateTenantAPIKey(ctx context.Context, params CreateTenantAPIKeyParams) error
}

type IssueTenantAPIKeyParams struct {
	TenantID string
	Label    string
}

type IssuedTenantAPIKey struct {
	PlaintextKey string
}

func IssueTenantAPIKey(ctx context.Context, store TenantAPIKeyCreator, params IssueTenantAPIKeyParams) (IssuedTenantAPIKey, error) {
	if params.TenantID == "" {
		return IssuedTenantAPIKey{}, errors.New("tenant id is required")
	}
	if params.Label == "" {
		return IssuedTenantAPIKey{}, errors.New("label is required")
	}

	plaintext, err := generateTenantAPIKey()
	if err != nil {
		return IssuedTenantAPIKey{}, err
	}
	if err := store.CreateTenantAPIKey(ctx, CreateTenantAPIKeyParams{
		TenantID: params.TenantID,
		KeyHash:  auth.HashAPIKey(plaintext),
		Label:    params.Label,
		Status:   auth.StatusActive,
	}); err != nil {
		return IssuedTenantAPIKey{}, err
	}
	return IssuedTenantAPIKey{PlaintextKey: plaintext}, nil
}

func generateTenantAPIKey() (string, error) {
	buf := make([]byte, randomKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return tenantAPIKeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

type postgresTenantAPIKeyCreator struct {
	queries *vigiaDB.Queries
}

func (s postgresTenantAPIKeyCreator) CreateTenantAPIKey(ctx context.Context, params CreateTenantAPIKeyParams) error {
	tenantID, err := parseUUID(params.TenantID)
	if err != nil {
		return err
	}
	_, err = s.queries.CreateTenantAPIKey(ctx, vigiaDB.CreateTenantAPIKeyParams{
		TenantID: tenantID,
		KeyHash:  params.KeyHash,
		Label:    params.Label,
		Status:   params.Status,
		ExpiresAt: pgtype.Timestamptz{
			Valid: false,
		},
	})
	return err
}

func parseUUID(value string) (pgtype.UUID, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(value); err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse tenant id: %w", err)
	}
	return uuid, nil
}

// defaultKeyIssuer adapts the free IssueTenantAPIKey function to the KeyIssuer interface.
type defaultKeyIssuer struct {
	store TenantAPIKeyCreator
}

func (d defaultKeyIssuer) IssueTenantAPIKey(ctx context.Context, params IssueTenantAPIKeyParams) (IssuedTenantAPIKey, error) {
	return IssueTenantAPIKey(ctx, d.store, params)
}

// routeArgs inspects the first argument to determine the subcommand.
// Returns "dev-data" when args[0] == "dev-data", otherwise "key-issuance".
func routeArgs(args []string) string {
	if len(args) > 0 && args[0] == "dev-data" {
		return "dev-data"
	}
	return "key-issuance"
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	switch routeArgs(args) {
	case "dev-data":
		return runDevData(ctx, args[1:])
	default:
		return runKeyIssuance(ctx, args)
	}
}

func runDevData(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("seed dev-data", flag.ContinueOnError)
	slug := flags.String("slug", "demo", "tenant slug")
	name := flags.String("name", "Demo Tenant", "tenant display name")
	debtorRef := flags.String("debtor-ref", "debtor-001", "debtor external ref")
	debtorName := flags.String("debtor-name", "Juana Pérez (demo)", "debtor display name")
	label := flags.String("label", "local-dev", "API key label")
	timezone := flags.String("timezone", defaultDebtorTimezone, "debtor IANA timezone")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	queries := vigiaDB.New(pool)
	issuer := defaultKeyIssuer{store: postgresTenantAPIKeyCreator{queries: queries}}
	evaluator := evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: detection.ContactHoursDetector{
				Window: detection.Window{StartHour: 8, EndHour: 21},
			}},
		},
		Store: postgres.NewEvaluationStoreFromPool(pool),
	}

	result, err := SeedDevData(ctx, queries, issuer, evaluator, DevDataParams{
		Slug:       *slug,
		Name:       *name,
		DebtorRef:  *debtorRef,
		DebtorName: *debtorName,
		Label:      *label,
		Timezone:   *timezone,
	})
	if err != nil {
		return err
	}

	fmt.Printf("tenant_api_key=%s\n", result.PlaintextKey)
	return nil
}

func runKeyIssuance(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	tenantID := flags.String("tenant-id", "", "tenant UUID to issue an API key for")
	label := flags.String("label", "local-dev", "tenant API key label")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	issued, err := IssueTenantAPIKey(ctx, postgresTenantAPIKeyCreator{queries: vigiaDB.New(pool)}, IssueTenantAPIKeyParams{
		TenantID: *tenantID,
		Label:    *label,
	})
	if err != nil {
		return err
	}

	fmt.Printf("tenant_api_key=%s\n", issued.PlaintextKey)
	return nil
}
