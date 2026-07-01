package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// TestSeedDevDataIntegration calls SeedDevData twice against a real Postgres instance and
// asserts that exactly one tenant, one debtor, and four interaction_events exist after both
// runs, proving idempotency end-to-end. It also proves the WU7 spec scenarios: the demo
// debtor has a non-empty IANA timezone, seeded interactions snapshot that timezone, and at
// least one seeded interaction evaluates to BLOCK.
//
// Requires:
//   - DATABASE_URL env var pointing to a migrated Postgres instance
//   - Running in non-short mode (go test -run TestSeedDevDataIntegration, not go test -short)
//
// Skip pattern mirrors internal/db/rls_isolation_test.go.
func TestSeedDevDataIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for the seed integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
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

	params := DevDataParams{
		Slug:       "integration-test-demo",
		Name:       "Integration Test Tenant",
		DebtorRef:  "debtor-integration-001",
		DebtorName: "Test Debtor (integration)",
		Label:      "integration-test",
	}

	// First run — creates all entities.
	result1, err := SeedDevData(ctx, queries, issuer, evaluator, params)
	if err != nil {
		t.Fatalf("first SeedDevData call: %v", err)
	}
	if result1.PlaintextKey == "" {
		t.Error("first run: PlaintextKey should not be empty")
	}

	// Second run — idempotent, only a new API key.
	result2, err := SeedDevData(ctx, queries, issuer, evaluator, params)
	if err != nil {
		t.Fatalf("second SeedDevData call: %v", err)
	}
	if result2.PlaintextKey == "" {
		t.Error("second run: PlaintextKey should not be empty")
	}

	// Assert exactly one tenant with the expected slug.
	tenant, err := queries.GetTenantBySlug(ctx, params.Slug)
	if err != nil {
		t.Fatalf("get tenant after seed: %v", err)
	}
	if tenant.Slug != params.Slug {
		t.Errorf("tenant.Slug = %q, want %q", tenant.Slug, params.Slug)
	}

	// Assert exactly one debtor with the expected external_ref, and that it
	// has a non-empty IANA timezone (spec "Seeded demo debtor has a
	// timezone").
	debtors, err := queries.ListDebtorsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list debtors: %v", err)
	}
	found := 0
	var debtorTimezone string
	for _, d := range debtors {
		if d.ExternalRef == params.DebtorRef {
			found++
			debtorTimezone = d.Timezone
		}
	}
	if found != 1 {
		t.Errorf("debtors with external_ref %q = %d, want 1", params.DebtorRef, found)
	}
	if debtorTimezone == "" {
		t.Error("debtor timezone should not be empty")
	}
	if _, err := time.LoadLocation(debtorTimezone); err != nil {
		t.Errorf("debtor timezone %q should be a valid IANA zone: %v", debtorTimezone, err)
	}

	// Assert exactly four interaction events for this tenant (the fixture
	// transcript_refs, including the out-of-hours fixture), each snapshotting
	// the debtor's timezone (spec "Seeded interactions snapshot the debtor's
	// timezone").
	events, err := queries.ListInteractionEventsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list interaction events: %v", err)
	}
	fixtureRefs := map[string]bool{
		"seed/demo/call-01":             false,
		"seed/demo/message-01":          false,
		"seed/demo/email-01":            false,
		"seed/demo/call-02-after-hours": false,
	}
	for _, e := range events {
		if e.TranscriptRef != nil {
			fixtureRefs[*e.TranscriptRef] = true
		}
		if e.DebtorTimezone != debtorTimezone {
			t.Errorf("interaction %s debtor_timezone = %q, want %q", uuidToString(e.ID), e.DebtorTimezone, debtorTimezone)
		}
	}
	for ref, seen := range fixtureRefs {
		if !seen {
			t.Errorf("interaction event with transcript_ref %q not found after seed", ref)
		}
	}
	if len(events) != 4 {
		t.Errorf("interaction_events count = %d, want 4", len(events))
	}

	// Assert at least one seeded interaction evaluates to BLOCK (spec "Seed
	// includes at least one out-of-hours interaction").
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", uuidToString(tenant.ID)); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT overall_outcome FROM evaluations
		WHERE interaction_event_id = ANY(
			SELECT id FROM interaction_events WHERE tenant_id = $1
		)
	`, tenant.ID)
	if err != nil {
		t.Fatalf("query evaluations: %v", err)
	}
	defer rows.Close()
	sawBlock := false
	outcomeCount := 0
	for rows.Next() {
		var outcome string
		if err := rows.Scan(&outcome); err != nil {
			t.Fatalf("scan evaluation outcome: %v", err)
		}
		outcomeCount++
		if outcome == "fail" {
			sawBlock = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate evaluations: %v", err)
	}
	if outcomeCount == 0 {
		t.Fatal("expected at least one persisted evaluation after seeding")
	}
	if !sawBlock {
		t.Error("expected at least one seeded interaction to evaluate to BLOCK (fail)")
	}
}
