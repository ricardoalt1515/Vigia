package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

func TestRedecoReportStoreRebuildsMonthlyPenalizations(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "redeco-report")
	despachoID := seedDespacho(t, ctx, pool, tenantID, "redeco-report-despacho")
	interactionID := seedInteractionWithDespacho(t, ctx, pool, tenantID, debtorID, despachoID)
	caseID := seedComplaintCaseForReport(t, ctx, pool, tenantID, interactionID, "MX-REDECO-05", "escalated", "redeco-report-escalated", time.Date(2026, time.June, 18, 10, 0, 0, 0, time.UTC))
	seedStalePenalization(t, ctx, pool, tenantID, despachoID, caseID, 2026, 6)
	openInteractionID := seedInteractionWithDespacho(t, ctx, pool, tenantID, debtorID, despachoID)
	openCaseID := seedComplaintCaseForReport(t, ctx, pool, tenantID, openInteractionID, "MX-REDECO-03", "open", "redeco-report-open", time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC))
	seedStalePenalization(t, ctx, pool, tenantID, despachoID, openCaseID, 2026, 6)

	reportPool := pool
	if appDatabaseURL := os.Getenv("APP_DATABASE_URL"); appDatabaseURL != "" {
		appPool, err := pgxpool.New(ctx, appDatabaseURL)
		if err != nil {
			t.Fatalf("connect app database: %v", err)
		}
		defer appPool.Close()
		reportPool = appPool
	}
	store := postgres.NewRedecoReportStoreFromPool(reportPool)
	report, err := store.GenerateRedecoMonthlyReport(ctx, tenantID, orchestrator.RedecoReportPeriod{Year: 2026, Month: time.June})
	if err != nil {
		t.Fatalf("GenerateRedecoMonthlyReport: %v", err)
	}
	if len(report.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(report.Entries))
	}
	entry := report.Entries[0]
	if entry.Channel != "phone" || entry.Cause != "MX-REDECO-05" || entry.Status != "escalated" || entry.Resolution != "escalated" || entry.Penalization != "penalized" {
		t.Fatalf("entry = %#v", entry)
	}
	if !strings.Contains(string(report.CSV), "phone,MX-REDECO-05,escalated,escalated,penalized") {
		t.Fatalf("CSV = %q", string(report.CSV))
	}

	var count int
	var penalization string
	if err := pool.QueryRow(ctx, `
		SELECT count(*), max(penalization)
		FROM despacho_penalizations
		WHERE tenant_id = $1 AND period_year = 2026 AND period_month = 6
	`, tenantID).Scan(&count, &penalization); err != nil {
		t.Fatalf("query penalizations: %v", err)
	}
	if count != 1 || penalization != "penalized" {
		t.Fatalf("penalization registry count=%d penalization=%q, want one penalized row", count, penalization)
	}
}

func seedInteractionWithDespacho(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, debtorID, despachoID string) string {
	t.Helper()
	var interactionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO interaction_events (tenant_id, debtor_id, despacho_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone)
		VALUES ($1, $2, $3, 'phone', 'outbound', 'recorded', '2026-06-02T15:04:00Z', 'redeco-report', 'America/Mexico_City')
		RETURNING id
	`, tenantID, debtorID, despachoID).Scan(&interactionID); err != nil {
		t.Fatalf("create interaction: %v", err)
	}
	return interactionID
}

func seedComplaintCaseForReport(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, interactionID, cause, state, idempotencyKey string, closedAt time.Time) string {
	t.Helper()
	var caseID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO complaint_cases (
			tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
			calendar_version, resolved_at, idempotency_key, updated_at
		) VALUES ($1, $2, $3, $4, '2026-06-02T15:04:00Z', '2026-06-16T15:04:00Z', 'mx-lft-art-74-2026a', NULL, $5, $6)
		RETURNING id
	`, tenantID, interactionID, cause, state, idempotencyKey, closedAt).Scan(&caseID); err != nil {
		t.Fatalf("create complaint case: %v", err)
	}
	return caseID
}

func seedStalePenalization(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, despachoID, caseID string, year, month int) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO despacho_penalizations (
			tenant_id, despacho_id, complaint_case_id, period_year, period_month,
			penalization, resolution, source_state
		) VALUES ($1, $2, $3, $4, $5, 'cleared', 'resolved', 'resolved')
	`, tenantID, despachoID, caseID, year, month); err != nil {
		t.Fatalf("create stale penalization: %v", err)
	}
}
