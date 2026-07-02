package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// TestEvaluationStoreIntegration proves EvaluationStore.CreateEvaluation writes
// an evaluations header + a linked detector_result_rows child inside one
// tenantdb.WithTenantTx call, that the header is RLS-scoped to its tenant,
// and that pre-existing (issue #1-style) detector_result_rows with no
// evaluation_id remain queryable.
//
// Requires DATABASE_URL; skips under -short.
func TestEvaluationStoreIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for the evaluation store integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "eval-integration")

	// A pre-existing (issue #1-style) detector_result_rows row with no
	// evaluation_id must remain queryable after this migration.
	legacyInteractionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "eval/legacy")
	seedLegacyDetectorResultRow(t, ctx, pool, tenantID, legacyInteractionID)

	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "eval/new")

	store := postgres.NewEvaluationStoreFromPool(pool)
	svc := evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: fakeBlockDetector{}},
		},
		Store: store,
	}

	got, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantID,
		InteractionEventID: interactionID,
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC),
			DebtorTimezone: "UTC",
		},
	})
	if err != nil {
		t.Fatalf("EvaluateInteraction: %v", err)
	}
	if got.OverallOutcome != "fail" {
		t.Fatalf("OverallOutcome = %q, want %q", got.OverallOutcome, "fail")
	}
	if got.ID == "" {
		t.Fatal("evaluation ID should not be empty")
	}

	// Assert the evaluations header is readable under the tenant's RLS
	// context and carries the composite FK to the interaction.
	var (
		headerTenantID           string
		headerInteractionEventID string
		headerOutcome            string
	)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT tenant_id, interaction_event_id, overall_outcome
		FROM evaluations WHERE id = $1
	`, string(got.ID)).Scan(&headerTenantID, &headerInteractionEventID, &headerOutcome); err != nil {
		t.Fatalf("read evaluations header: %v", err)
	}
	if headerTenantID != tenantID || headerInteractionEventID != interactionID || headerOutcome != "fail" {
		t.Fatalf("header = (%s, %s, %s), want (%s, %s, fail)", headerTenantID, headerInteractionEventID, headerOutcome, tenantID, interactionID)
	}

	// Assert the child detector_result_rows row is linked via evaluation_id
	// and carries outcome + rationale.
	var (
		childEvaluationID *string
		childOutcome      string
		childCode         string
	)
	if err := tx.QueryRow(ctx, `
		SELECT evaluation_id, outcome, detector_code
		FROM detector_result_rows WHERE evaluation_id = $1
	`, string(got.ID)).Scan(&childEvaluationID, &childOutcome, &childCode); err != nil {
		t.Fatalf("read detector_result_rows child: %v", err)
	}
	if childEvaluationID == nil || *childEvaluationID != string(got.ID) {
		t.Fatalf("child evaluation_id = %v, want %s", childEvaluationID, got.ID)
	}
	if childOutcome != string(core.DetectorOutcomeFail) || childCode != "contact-hours" {
		t.Fatalf("child = (%s, %s), want (fail, contact-hours)", childOutcome, childCode)
	}

	// Assert the pre-existing detector_result_rows row (no evaluation_id)
	// remains queryable and valid.
	var legacyEvaluationID *string
	if err := tx.QueryRow(ctx, `
		SELECT evaluation_id FROM detector_result_rows
		WHERE interaction_event_id = $1
	`, legacyInteractionID).Scan(&legacyEvaluationID); err != nil {
		t.Fatalf("read legacy detector_result_rows: %v", err)
	}
	if legacyEvaluationID != nil {
		t.Fatalf("legacy row evaluation_id = %v, want nil", legacyEvaluationID)
	}
}

// TestEvaluationRLSIsolationAcrossTenants proves evaluations/summary reads
// are actually enforced by Postgres RLS, not just readable via the owner
// pool (which bypasses RLS as the table owner). It creates evaluations for
// two tenants via the owner pool, then reads back under tenant A's RLS
// context through a restricted role: tenant B's evaluations rows must not
// be visible, and CountOutOfHoursEvaluations must only count tenant A's
// fail evaluations.
//
// Requires DATABASE_URL and APP_DATABASE_URL (a role without BypassRLS);
// skips when either is unavailable, matching internal/db/rls_isolation_test.go.
func TestEvaluationRLSIsolationAcrossTenants(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL RLS integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	appDatabaseURL := os.Getenv("APP_DATABASE_URL")
	if databaseURL == "" || appDatabaseURL == "" {
		t.Skip("DATABASE_URL and APP_DATABASE_URL are required for PostgreSQL RLS integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "eval-rls-a")
	tenantB, debtorB := seedTenantAndDebtor(t, ctx, pool, "eval-rls-b")

	interactionA := seedInteraction(t, ctx, pool, tenantA, debtorA, "eval-rls/tenant-a")
	interactionB := seedInteraction(t, ctx, pool, tenantB, debtorB, "eval-rls/tenant-b")

	// Seed evaluations for both tenants via the owner pool (setup only —
	// the owner role bypasses RLS, so this does not itself prove isolation).
	store := postgres.NewEvaluationStoreFromPool(pool)
	svc := evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: fakeBlockDetector{}},
		},
		Store: store,
	}
	evalA, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantA,
		InteractionEventID: interactionA,
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC),
			DebtorTimezone: "UTC",
		},
	})
	if err != nil {
		t.Fatalf("EvaluateInteraction tenant A: %v", err)
	}
	if _, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantB,
		InteractionEventID: interactionB,
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC),
			DebtorTimezone: "UTC",
		},
	}); err != nil {
		t.Fatalf("EvaluateInteraction tenant B: %v", err)
	}

	appPool, err := pgxpool.New(ctx, appDatabaseURL)
	if err != nil {
		t.Fatalf("connect app database: %v", err)
	}
	defer appPool.Close()

	// (a) Tenant B's evaluations rows must not be readable under tenant A's
	// RLS context, even though tenant B unquestionably has a row.
	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin app tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantA); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}

	rows, err := tx.Query(ctx, `SELECT tenant_id FROM evaluations WHERE interaction_event_id = $1`, interactionB)
	if err != nil {
		t.Fatalf("query tenant B evaluation under tenant A context: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("tenant B's evaluation row was readable under tenant A's RLS context")
	}
	rows.Close()

	var visibleTenantID string
	if err := tx.QueryRow(ctx, `SELECT tenant_id FROM evaluations WHERE interaction_event_id = $1`, interactionA).Scan(&visibleTenantID); err != nil {
		t.Fatalf("query tenant A evaluation under tenant A context: %v", err)
	}
	if visibleTenantID != tenantA {
		t.Fatalf("visible tenant_id = %s, want %s", visibleTenantID, tenantA)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback tx: %v", err)
	}

	// (b) CountOutOfHoursEvaluations must only count tenant A's fail
	// evaluations, never tenant B's, when read through the restricted role.
	summaryReader := postgres.NewSummaryReaderFromPool(appPool)
	count, err := summaryReader.CountOutOfHours(ctx, tenantA)
	if err != nil {
		t.Fatalf("CountOutOfHours tenant A: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountOutOfHours(tenantA) = %d, want 1 (only tenant A's fail evaluation, not tenant B's)", count)
	}
	if evalA.OverallOutcome != "fail" {
		t.Fatalf("sanity check: evalA.OverallOutcome = %q, want fail", evalA.OverallOutcome)
	}
}

type fakeBlockDetector struct{}

func (fakeBlockDetector) Evaluate(_ detection.Interaction) detection.Result {
	return detection.Result{Outcome: detection.OutcomeBlock, Rationale: "outside window (test fixture)"}
}

func seedTenantAndDebtor(t *testing.T, ctx context.Context, pool *pgxpool.Pool, suffix string) (tenantID, debtorID string) {
	t.Helper()
	slug := suffix + "-" + time.Now().Format("150405.000000000")
	if err := pool.QueryRow(ctx, `
		INSERT INTO tenants (slug, name, status)
		VALUES ($1, $2, 'active')
		RETURNING id
	`, slug, slug).Scan(&tenantID); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO debtors (tenant_id, external_ref, display_name, timezone)
		VALUES ($1, $2, $3, 'America/Mexico_City')
		RETURNING id
	`, tenantID, suffix+"-debtor", suffix+"-debtor").Scan(&debtorID); err != nil {
		t.Fatalf("create debtor: %v", err)
	}
	return tenantID, debtorID
}

func seedInteraction(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, debtorID, transcriptRef string) (interactionID string) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone)
		VALUES ($1, $2, 'phone', 'outbound', 'recorded', now(), $3, 'America/Mexico_City')
		RETURNING id
	`, tenantID, debtorID, transcriptRef).Scan(&interactionID); err != nil {
		t.Fatalf("create interaction: %v", err)
	}
	return interactionID
}

func seedLegacyDetectorResultRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, interactionEventID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO detector_result_rows (tenant_id, interaction_event_id, detector_code, outcome, severity, result_payload)
		VALUES ($1, $2, 'legacy-detector', 'pass', 'low', '{}'::jsonb)
	`, tenantID, interactionEventID); err != nil {
		t.Fatalf("create legacy detector_result_row: %v", err)
	}
}
