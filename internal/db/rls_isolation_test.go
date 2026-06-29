package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ricardoalt1515/vigia/internal/auth"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
)

func TestRLSIsolationForCurrentTenantInteractions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL RLS integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	appDatabaseURL := os.Getenv("APP_DATABASE_URL")
	if databaseURL == "" || appDatabaseURL == "" {
		t.Skip("DATABASE_URL and APP_DATABASE_URL are required for PostgreSQL RLS integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close(ctx)

	tenantA := createTenantForRLS(t, ctx, db, "rls-a")
	tenantB := createTenantForRLS(t, ctx, db, "rls-b")
	debtorA := createDebtorForRLS(t, ctx, db, tenantA, "debtor-a")
	debtorB := createDebtorForRLS(t, ctx, db, tenantB, "debtor-b")
	createInteractionForRLS(t, ctx, db, tenantA, debtorA, "tenant-a-call")
	createInteractionForRLS(t, ctx, db, tenantB, debtorB, "tenant-b-call")

	appDB, err := pgx.Connect(ctx, appDatabaseURL)
	if err != nil {
		t.Fatalf("connect app database: %v", err)
	}
	defer appDB.Close(ctx)

	tx, err := appDB.Begin(ctx)
	if err != nil {
		t.Fatalf("begin app tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantA); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}

	queries := vigiaDB.New(tx)
	items, err := queries.ListCurrentTenantInteractions(ctx, 20)
	if err != nil {
		t.Fatalf("list current tenant interactions: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one interaction for tenant A")
	}
	for _, item := range items {
		if uuidString(item.TenantID) != tenantA {
			t.Fatalf("RLS returned tenant %s row while scoped to %s", uuidString(item.TenantID), tenantA)
		}
	}
}

func createTenantForRLS(t *testing.T, ctx context.Context, db *pgx.Conn, suffix string) string {
	t.Helper()
	var id string
	slug := suffix + "-" + auth.HashAPIKey(time.Now().String())[:12]
	if err := db.QueryRow(ctx, `
		INSERT INTO tenants (slug, name, status)
		VALUES ($1, $2, 'active')
		RETURNING id
	`, slug, slug).Scan(&id); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return id
}

func createDebtorForRLS(t *testing.T, ctx context.Context, db *pgx.Conn, tenantID, externalRef string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(ctx, `
		INSERT INTO debtors (tenant_id, external_ref, display_name)
		VALUES ($1, $2, $3)
		RETURNING id
	`, tenantID, externalRef, externalRef).Scan(&id); err != nil {
		t.Fatalf("create debtor: %v", err)
	}
	return id
}

func createInteractionForRLS(t *testing.T, ctx context.Context, db *pgx.Conn, tenantID, debtorID, transcriptRef string) {
	t.Helper()
	if _, err := db.Exec(ctx, `
		INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref)
		VALUES ($1, $2, 'phone', 'outbound', 'recorded', now(), $3)
	`, tenantID, debtorID, transcriptRef); err != nil {
		t.Fatalf("create interaction: %v", err)
	}
}

func uuidString(id pgtype.UUID) string {
	return id.String()
}
