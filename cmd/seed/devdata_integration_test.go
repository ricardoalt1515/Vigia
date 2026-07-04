package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/judge"
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
		Judges: []evaluation.NamedJudge{
			{Code: "MX-REDECO-05", Judge: judge.FakeJudge{}},
		},
		Rubric: judge.LoadRubric(),
		Store:  postgres.NewEvaluationStoreFromPool(pool),
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

	// Assert exactly six interaction events for this tenant (the fixture
	// transcript_refs, including the out-of-hours fixture and the
	// threatening/neutral transcript fixtures, issue #4), each snapshotting
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
		"seed/demo/call-03-threatening": false,
		"seed/demo/call-04-neutral":     false,
	}
	var threateningID, neutralID pgtype.UUID
	for _, e := range events {
		if e.TranscriptRef != nil {
			fixtureRefs[*e.TranscriptRef] = true
			switch *e.TranscriptRef {
			case "seed/demo/call-03-threatening":
				threateningID = e.ID
			case "seed/demo/call-04-neutral":
				neutralID = e.ID
			}
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
	if len(events) != 6 {
		t.Errorf("interaction_events count = %d, want 6", len(events))
	}

	// Assert the threatening seed transcript's interaction evaluates to a
	// HARD BLOCK with requires_hitl=true (spec "Seed includes a threatening
	// transcript that the judge blocks"), and the neutral seed transcript's
	// interaction evaluates with requires_hitl=false from the judge step
	// alone (spec "Seed includes a neutral transcript that the judge
	// passes").
	var threateningOutcome string
	var threateningHITL bool
	if err := pool.QueryRow(ctx, `
		SELECT overall_outcome, requires_hitl FROM evaluations WHERE interaction_event_id = $1
	`, threateningID).Scan(&threateningOutcome, &threateningHITL); err != nil {
		t.Fatalf("read threatening interaction evaluation: %v", err)
	}
	if threateningOutcome != "fail" {
		t.Errorf("threatening interaction overall_outcome = %q, want fail (HARD BLOCK)", threateningOutcome)
	}
	if !threateningHITL {
		t.Error("threatening interaction requires_hitl = false, want true")
	}

	var neutralHITL bool
	if err := pool.QueryRow(ctx, `
		SELECT requires_hitl FROM evaluations WHERE interaction_event_id = $1
	`, neutralID).Scan(&neutralHITL); err != nil {
		t.Fatalf("read neutral interaction evaluation: %v", err)
	}
	if neutralHITL {
		t.Error("neutral interaction requires_hitl = true, want false from the judge step alone")
	}

	// Re-running seed must not create a duplicate transcript for either
	// fixture (idempotency).
	var threateningTranscriptCount, neutralTranscriptCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interaction_transcripts WHERE interaction_event_id = $1`, threateningID).Scan(&threateningTranscriptCount); err != nil {
		t.Fatalf("count threatening transcripts: %v", err)
	}
	if threateningTranscriptCount != 1 {
		t.Errorf("threatening transcript count = %d, want 1 (no duplicate on re-run)", threateningTranscriptCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interaction_transcripts WHERE interaction_event_id = $1`, neutralID).Scan(&neutralTranscriptCount); err != nil {
		t.Fatalf("count neutral transcripts: %v", err)
	}
	if neutralTranscriptCount != 1 {
		t.Errorf("neutral transcript count = %d, want 1 (no duplicate on re-run)", neutralTranscriptCount)
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

	// Assert every seeded evaluation has a corresponding evidence_records row
	// (issue #3 spec "Seeded evaluations produce evidence records"), with
	// seq starting at 1 for this tenant. No new seed logic is required: the
	// existing seed path (EvaluateInteraction -> CreateEvaluation -> append)
	// produces evidence automatically.
	evidenceRows, err := tx.Query(ctx, `
		SELECT er.seq
		FROM evidence_records er
		WHERE er.tenant_id = $1
		ORDER BY er.seq ASC
	`, tenant.ID)
	if err != nil {
		t.Fatalf("query evidence_records: %v", err)
	}
	defer evidenceRows.Close()
	var seqs []int64
	for evidenceRows.Next() {
		var seq int64
		if err := evidenceRows.Scan(&seq); err != nil {
			t.Fatalf("scan evidence_records seq: %v", err)
		}
		seqs = append(seqs, seq)
	}
	if err := evidenceRows.Err(); err != nil {
		t.Fatalf("iterate evidence_records: %v", err)
	}
	if len(seqs) != outcomeCount {
		t.Fatalf("evidence_records rows = %d, want %d (one per evaluation)", len(seqs), outcomeCount)
	}
	if len(seqs) == 0 || seqs[0] != 1 {
		t.Fatalf("evidence_records seqs = %v, want to start at 1", seqs)
	}
	for i, seq := range seqs {
		if seq != int64(i+1) {
			t.Fatalf("evidence_records seqs = %v, want contiguous 1..%d", seqs, len(seqs))
		}
	}

	// Issue #6 compatibility: seed never configures a BundleResolver, so
	// every seeded evaluation must keep the pre-#6 no-active-bundle sentinel
	// — policy_bundle_version = "" and policy_bundle_id = NULL — and the
	// evidence-ledger/golden-hash assertions above must remain unaffected by
	// migration 00007's additive schema.
	bundleRows, err := tx.Query(ctx, `
		SELECT e.policy_bundle_version, e.policy_bundle_id
		FROM evaluations e
		WHERE e.interaction_event_id = ANY(
			SELECT id FROM interaction_events WHERE tenant_id = $1
		)
	`, tenant.ID)
	if err != nil {
		t.Fatalf("query evaluations policy_bundle columns: %v", err)
	}
	defer bundleRows.Close()
	bundleRowCount := 0
	for bundleRows.Next() {
		bundleRowCount++
		var version string
		var bundleID *string
		if err := bundleRows.Scan(&version, &bundleID); err != nil {
			t.Fatalf("scan evaluations policy_bundle columns: %v", err)
		}
		if version != "" {
			t.Errorf("seeded evaluation policy_bundle_version = %q, want empty sentinel (no BundleResolver configured)", version)
		}
		if bundleID != nil {
			t.Errorf("seeded evaluation policy_bundle_id = %v, want nil (no BundleResolver configured)", *bundleID)
		}
	}
	if err := bundleRows.Err(); err != nil {
		t.Fatalf("iterate evaluations policy_bundle columns: %v", err)
	}
	if bundleRowCount != outcomeCount {
		t.Fatalf("evaluations rows with policy_bundle columns checked = %d, want %d", bundleRowCount, outcomeCount)
	}
}
