package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
)

// TestSeedDevDataIntegration calls SeedDevData twice against a real Postgres instance and
// asserts that exactly one tenant, one debtor, and three interaction_events exist after both
// runs, proving idempotency end-to-end.
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

	params := DevDataParams{
		Slug:       "integration-test-demo",
		Name:       "Integration Test Tenant",
		DebtorRef:  "debtor-integration-001",
		DebtorName: "Test Debtor (integration)",
		Label:      "integration-test",
	}

	// First run — creates all entities.
	result1, err := SeedDevData(ctx, queries, issuer, params)
	if err != nil {
		t.Fatalf("first SeedDevData call: %v", err)
	}
	if result1.PlaintextKey == "" {
		t.Error("first run: PlaintextKey should not be empty")
	}

	// Second run — idempotent, only a new API key.
	result2, err := SeedDevData(ctx, queries, issuer, params)
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

	// Assert exactly one debtor with the expected external_ref.
	debtors, err := queries.ListDebtorsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list debtors: %v", err)
	}
	found := 0
	for _, d := range debtors {
		if d.ExternalRef == params.DebtorRef {
			found++
		}
	}
	if found != 1 {
		t.Errorf("debtors with external_ref %q = %d, want 1", params.DebtorRef, found)
	}

	// Assert exactly three interaction events for this tenant (the fixture transcript_refs).
	events, err := queries.ListInteractionEventsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list interaction events: %v", err)
	}
	fixtureRefs := map[string]bool{
		"seed/demo/call-01":    false,
		"seed/demo/message-01": false,
		"seed/demo/email-01":   false,
	}
	for _, e := range events {
		if e.TranscriptRef != nil {
			fixtureRefs[*e.TranscriptRef] = true
		}
	}
	for ref, seen := range fixtureRefs {
		if !seen {
			t.Errorf("interaction event with transcript_ref %q not found after seed", ref)
		}
	}
	if len(events) != 3 {
		t.Errorf("interaction_events count = %d, want 3", len(events))
	}
}
