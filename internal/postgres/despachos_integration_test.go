package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
)

// seedDespacho creates a despachos row via the owner pool (setup only,
// bypasses RLS — same pattern as seedTenantAndDebtor).
func seedDespacho(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, externalRef string) (despachoID string) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO despachos (tenant_id, external_ref, display_name)
		VALUES ($1, $2, $3)
		RETURNING id
	`, tenantID, externalRef, externalRef).Scan(&despachoID); err != nil {
		t.Fatalf("seed despacho: %v", err)
	}
	return despachoID
}

// TestDespachoRLSIsolationAcrossTenants covers the despacho-registry spec's
// "Despacho row is visible only to its owning tenant" [integration]
// scenario, following the restricted-role APP_DATABASE_URL pattern
// established in internal/db/rls_isolation_test.go.
func TestDespachoRLSIsolationAcrossTenants(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	appDatabaseURL := os.Getenv("APP_DATABASE_URL")
	if appDatabaseURL == "" {
		t.Skip("APP_DATABASE_URL (a role without BypassRLS) is required for the despacho RLS isolation test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantA, _ := seedTenantAndDebtor(t, ctx, pool, "despacho-rls-a")
	tenantB, _ := seedTenantAndDebtor(t, ctx, pool, "despacho-rls-b")
	seedDespacho(t, ctx, pool, tenantA, "despacho-rls/tenant-a")
	despachoB := seedDespacho(t, ctx, pool, tenantB, "despacho-rls/tenant-b")

	appPool, err := pgxpool.New(ctx, appDatabaseURL)
	if err != nil {
		t.Fatalf("connect app database: %v", err)
	}
	defer appPool.Close()

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin app tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantA); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}

	rows, err := tx.Query(ctx, `SELECT id FROM despachos WHERE tenant_id = $1`, tenantB)
	if err != nil {
		t.Fatalf("query tenant B despachos under tenant A context: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("tenant B's despacho row was readable under tenant A's RLS context")
	}
	rows.Close()

	var visibleIDs []string
	rows, err = tx.Query(ctx, `SELECT id FROM despachos`)
	if err != nil {
		t.Fatalf("query all visible despachos under tenant A context: %v", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan despacho id: %v", err)
		}
		visibleIDs = append(visibleIDs, id)
	}
	rows.Close()
	for _, id := range visibleIDs {
		if id == despachoB {
			t.Fatal("tenant B's despacho id appeared in tenant A's unscoped despachos query")
		}
	}
}

// TestDespachoRequiresTenant covers "Despacho row cannot be created without a
// tenant" [integration]: an insert omitting tenant_id must fail with a
// not-null constraint violation.
func TestDespachoRequiresTenant(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `
		INSERT INTO despachos (external_ref, display_name)
		VALUES ('no-tenant-despacho', 'no-tenant-despacho')
	`)
	if err == nil {
		t.Fatal("expected not-null constraint violation inserting despacho without tenant_id, got nil error")
	}
}

// TestDespachoOneTenantManyDespachos covers "A tenant can have multiple
// despachos" [integration]: the 1-tenant-to-N cardinality.
func TestDespachoOneTenantManyDespachos(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, _ := seedTenantAndDebtor(t, ctx, pool, "despacho-cardinality")
	first := seedDespacho(t, ctx, pool, tenantID, "despacho-cardinality/first")
	second := seedDespacho(t, ctx, pool, tenantID, "despacho-cardinality/second")
	if first == second {
		t.Fatal("expected two distinct despacho ids for the same tenant")
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM despachos WHERE tenant_id = $1`, tenantID).Scan(&count); err != nil {
		t.Fatalf("count despachos for tenant: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 despachos for tenant, got %d", count)
	}
}

// TestInteractionDespachoFKBackwardCompat covers "Existing interactions
// remain valid after the FK is added" [integration]: a pre-existing-shaped
// interaction insert (despacho_id omitted) must still succeed with the
// column NULL.
func TestInteractionDespachoFKBackwardCompat(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "despacho-fk-backcompat")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "despacho-fk-backcompat/interaction")

	var despachoID *string
	if err := pool.QueryRow(ctx, `SELECT despacho_id FROM interaction_events WHERE id = $1`, interactionID).Scan(&despachoID); err != nil {
		t.Fatalf("read despacho_id: %v", err)
	}
	if despachoID != nil {
		t.Fatalf("expected NULL despacho_id for pre-existing-shaped interaction, got %v", *despachoID)
	}
}

// TestInteractionCanReferenceSameTenantDespacho covers "An interaction can be
// attributed to a despacho of the same tenant" [integration].
func TestInteractionCanReferenceSameTenantDespacho(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "despacho-fk-samet")
	despachoID := seedDespacho(t, ctx, pool, tenantID, "despacho-fk-samet/despacho")

	var interactionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone, despacho_id)
		VALUES ($1, $2, 'phone', 'outbound', 'recorded', now(), $3, 'America/Mexico_City', $4)
		RETURNING id
	`, tenantID, debtorID, "despacho-fk-samet/interaction", despachoID).Scan(&interactionID); err != nil {
		t.Fatalf("create interaction attributed to despacho: %v", err)
	}

	var readDespachoID string
	if err := pool.QueryRow(ctx, `SELECT despacho_id FROM interaction_events WHERE id = $1`, interactionID).Scan(&readDespachoID); err != nil {
		t.Fatalf("read despacho_id: %v", err)
	}
	if readDespachoID != despachoID {
		t.Fatalf("expected despacho_id %s, got %s", despachoID, readDespachoID)
	}
}

// TestInteractionCannotReferenceOtherTenantDespacho covers "An interaction
// cannot reference a despacho from a different tenant" [integration]: the
// composite FK must reject a cross-tenant despacho_id.
func TestInteractionCannotReferenceOtherTenantDespacho(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantA, _ := seedTenantAndDebtor(t, ctx, pool, "despacho-fk-crosst-a")
	tenantB, debtorB := seedTenantAndDebtor(t, ctx, pool, "despacho-fk-crosst-b")
	despachoA := seedDespacho(t, ctx, pool, tenantA, "despacho-fk-crosst/a")

	_, err = pool.Exec(ctx, `
		INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone, despacho_id)
		VALUES ($1, $2, 'phone', 'outbound', 'recorded', now(), $3, 'America/Mexico_City', $4)
	`, tenantB, debtorB, "despacho-fk-crosst/interaction", despachoA)
	if err == nil {
		t.Fatal("expected tenant-consistency FK violation referencing another tenant's despacho, got nil error")
	}
}

// TestDespachoRoundTrip covers "Despacho type round-trips through the
// generated data layer" [unit]: a Despacho value's ID/TenantID/ExternalRef
// and display name survive the sqlc-generated create/read path unchanged.
func TestDespachoRoundTrip(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, _ := seedTenantAndDebtor(t, ctx, pool, "despacho-roundtrip")
	tenantUUID, err := parseTestUUID(tenantID)
	if err != nil {
		t.Fatalf("parse tenant uuid: %v", err)
	}

	q := vigiaDB.New(pool)
	created, err := q.CreateDespacho(ctx, vigiaDB.CreateDespachoParams{
		TenantID:    tenantUUID,
		ExternalRef: "despacho-roundtrip/ref",
		DisplayName: "Despacho Roundtrip",
	})
	if err != nil {
		t.Fatalf("create despacho: %v", err)
	}

	got, err := q.GetDespachoByTenant(ctx, vigiaDB.GetDespachoByTenantParams{
		TenantID: tenantUUID,
		ID:       created.ID,
	})
	if err != nil {
		t.Fatalf("get despacho by tenant: %v", err)
	}

	if got.ID != created.ID {
		t.Fatalf("expected ID %v, got %v", created.ID, got.ID)
	}
	if got.TenantID != tenantUUID {
		t.Fatalf("expected TenantID %v, got %v", tenantUUID, got.TenantID)
	}
	if got.ExternalRef != "despacho-roundtrip/ref" {
		t.Fatalf("expected ExternalRef %q, got %q", "despacho-roundtrip/ref", got.ExternalRef)
	}
	if got.DisplayName != "Despacho Roundtrip" {
		t.Fatalf("expected DisplayName %q, got %q", "Despacho Roundtrip", got.DisplayName)
	}
}
