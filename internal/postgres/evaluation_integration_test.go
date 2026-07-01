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
