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
	// Detectors mirror cmd/api/main.go's and cmd/seed/main.go's real
	// production wiring (issue #7: MX-REDECO-06/07/10/11), so this
	// integration test genuinely proves the detector-input snapshot
	// plumbing end to end, not a stand-in pipeline.
	evaluator := evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "MX-REDECO-04", Detector: detection.ContactHoursDetector{
				Window: detection.Window{StartHour: 8, EndHour: 21},
			}},
			{Code: "MX-REDECO-06", Detector: detection.ThirdPartyContactDetector{}},
			{Code: "MX-REDECO-07", Detector: detection.ProtectedPopulationDetector{}, RequiresHITL: true},
			{Code: "MX-REDECO-11", Detector: detection.AuthorizedChannelDetector{}},
			{Code: "MX-REDECO-10", Detector: detection.PaymentRoutingDetector{}},
			{Code: "MX-REDECO-03", Detector: detection.DisclosureDetector{}},
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

	// Assert exactly eleven interaction events for this tenant (the fixture
	// transcript_refs, including the out-of-hours fixture, the
	// threatening/neutral transcript fixtures (issue #4), and the five
	// issue #7 new-detector demo fixtures), each snapshotting the debtor's
	// timezone (spec "Seeded interactions snapshot the debtor's timezone").
	events, err := queries.ListInteractionEventsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list interaction events: %v", err)
	}
	fixtureRefs := map[string]bool{
		"seed/demo/call-01":                      false,
		"seed/demo/message-01":                   false,
		"seed/demo/email-01":                     false,
		"seed/demo/call-02-after-hours":          false,
		"seed/demo/call-03-threatening":          false,
		"seed/demo/call-04-neutral":              false,
		"seed/demo/call-05-third-party":          false,
		"seed/demo/call-06-protected-minor":      false,
		"seed/demo/call-07-unauthorized-channel": false,
		"seed/demo/message-08-payment-routing":   false,
		"seed/demo/call-09-disclosure-warn":      false,
	}
	var threateningID, neutralID pgtype.UUID
	var compliantID, thirdPartyID, protectedMinorID, unauthorizedChannelID, paymentRoutingID, disclosureWarnID pgtype.UUID
	for _, e := range events {
		if e.TranscriptRef != nil {
			fixtureRefs[*e.TranscriptRef] = true
			switch *e.TranscriptRef {
			case "seed/demo/call-03-threatening":
				threateningID = e.ID
			case "seed/demo/call-04-neutral":
				neutralID = e.ID
			case "seed/demo/call-01":
				// The first fixture is compliant for every issue #7
				// detector (relationship debtor, authorized channel,
				// creditor recipient, adult DOB, disclosure stated) — used
				// below to prove the PASS path end to end.
				compliantID = e.ID
			case "seed/demo/call-05-third-party":
				thirdPartyID = e.ID
			case "seed/demo/call-06-protected-minor":
				protectedMinorID = e.ID
			case "seed/demo/call-07-unauthorized-channel":
				unauthorizedChannelID = e.ID
			case "seed/demo/message-08-payment-routing":
				paymentRoutingID = e.ID
			case "seed/demo/call-09-disclosure-warn":
				disclosureWarnID = e.ID
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
	if len(events) != 11 {
		t.Errorf("interaction_events count = %d, want 11", len(events))
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

	// Issue #7 (judgment-day fix): prove the detector-input snapshot
	// columns actually flow from cmd/seed's fixtures through
	// CreateInteractionEvent into detection.Interaction, not silently
	// zero-valued. Without this plumbing, every one of MX-REDECO-06/07/10/11
	// would fail closed to BLOCK on every interaction, regardless of the
	// fixture's real relationship/channel/recipient/DOB data.
	detectorOutcome := func(t *testing.T, interactionID pgtype.UUID, code string) string {
		t.Helper()
		var outcome string
		if err := pool.QueryRow(ctx, `
			SELECT dr.outcome
			FROM detector_result_rows dr
			JOIN evaluations e ON e.id = dr.evaluation_id
			WHERE e.interaction_event_id = $1 AND dr.detector_code = $2
		`, interactionID, code).Scan(&outcome); err != nil {
			t.Fatalf("read detector_result_rows outcome for %s on interaction %s: %v", code, uuidToString(interactionID), err)
		}
		return outcome
	}

	// (a) The compliant fixture (relationship=debtor, its own channel
	// authorized, recipient=creditor, adult DOB, disclosure stated) PASSES
	// all four new hard-block detectors AND the MX-REDECO-03 disclosure
	// detector.
	for _, code := range []string{"MX-REDECO-06", "MX-REDECO-07", "MX-REDECO-11", "MX-REDECO-10", "MX-REDECO-03"} {
		if got := detectorOutcome(t, compliantID, code); got != "pass" {
			t.Errorf("compliant fixture %s outcome = %q, want pass", code, got)
		}
	}

	// (b) Each violation-demo fixture BLOCKs (persisted as "fail") on
	// exactly the detector it targets.
	if got := detectorOutcome(t, thirdPartyID, "MX-REDECO-06"); got != "fail" {
		t.Errorf("third-party demo fixture MX-REDECO-06 outcome = %q, want fail", got)
	}
	if got := detectorOutcome(t, protectedMinorID, "MX-REDECO-07"); got != "fail" {
		t.Errorf("protected-minor demo fixture MX-REDECO-07 outcome = %q, want fail", got)
	}
	if got := detectorOutcome(t, unauthorizedChannelID, "MX-REDECO-11"); got != "fail" {
		t.Errorf("unauthorized-channel demo fixture MX-REDECO-11 outcome = %q, want fail", got)
	}
	if got := detectorOutcome(t, paymentRoutingID, "MX-REDECO-10"); got != "fail" {
		t.Errorf("payment-routing demo fixture MX-REDECO-10 outcome = %q, want fail", got)
	}

	// (d) Issue #7 (MX-REDECO-03, warn-level): the disclosure-warn demo
	// fixture emits a `warn` (not `fail`) detector_result_rows entry. The
	// end-to-end "warn-only stays overall pass" / "warn+block coexists as
	// overall fail" guarantees are already proven deterministically at the
	// Service level (internal/evaluation/service_test.go, PR2a) with a fake
	// EvaluationStore — mirroring the MX-REDECO-07 HITL precedent (2a.9) —
	// so this fixture-based check intentionally stays scoped to the
	// detector-row outcome only, since this fixture's overall_outcome also
	// depends on the wall-clock-relative contact-hours detector and would be
	// flaky if asserted here.
	if got := detectorOutcome(t, disclosureWarnID, "MX-REDECO-03"); got != "warn" {
		t.Errorf("disclosure-warn demo fixture MX-REDECO-03 outcome = %q, want warn", got)
	}

	// The third-party demo fixture also emits a coexisting MX-REDECO-03
	// warn (disclosure not stated), proving a warn row coexisting with a
	// hard-block row still yields overall fail, driven by the block (spec
	// "A warn row coexisting with a hard-block row yields overall fail").
	// Unlike the pass-path assertion above, this fail assertion is safe from
	// wall-clock flakiness: ANY detector block forces overall fail
	// regardless of what the other detectors (including contact-hours) do.
	if got := detectorOutcome(t, thirdPartyID, "MX-REDECO-03"); got != "warn" {
		t.Errorf("third-party demo fixture MX-REDECO-03 outcome = %q, want warn", got)
	}
	var thirdPartyOverallOutcome string
	if err := pool.QueryRow(ctx, `
		SELECT overall_outcome FROM evaluations WHERE interaction_event_id = $1
	`, thirdPartyID).Scan(&thirdPartyOverallOutcome); err != nil {
		t.Fatalf("read third-party interaction evaluation: %v", err)
	}
	if thirdPartyOverallOutcome != "fail" {
		t.Errorf("third-party interaction overall_outcome = %q, want fail (driven by the MX-REDECO-06 block, not the coexisting warn)", thirdPartyOverallOutcome)
	}

	// The protected-population BLOCK must set requires_hitl=true on its
	// evaluation (spec "MX-REDECO-07 blocks require Human-in-the-Loop").
	var protectedMinorHITL bool
	if err := pool.QueryRow(ctx, `
		SELECT requires_hitl FROM evaluations WHERE interaction_event_id = $1
	`, protectedMinorID).Scan(&protectedMinorHITL); err != nil {
		t.Fatalf("read protected-minor interaction evaluation: %v", err)
	}
	if !protectedMinorHITL {
		t.Error("protected-minor interaction requires_hitl = false, want true (MX-REDECO-07 block)")
	}

	// (c) Re-evaluation reproducibility: GetInteractionForReEvaluation (the
	// real adapter used by Service.ReEvaluateInteraction, issue #6) MUST map
	// the SAME detector-input snapshot the original evaluation used, not a
	// zero-valued Interaction. Re-running the four new detectors directly
	// against the adapter's returned Interaction MUST reproduce the exact
	// same pass outcomes persisted above for the compliant fixture.
	lookup := postgres.NewInteractionLookupAdapterFromPool(pool)
	reInput, reInputFound, err := lookup.GetInteractionForReEvaluation(ctx, uuidToString(tenant.ID), uuidToString(compliantID))
	if err != nil {
		t.Fatalf("GetInteractionForReEvaluation: %v", err)
	}
	if !reInputFound {
		t.Fatal("GetInteractionForReEvaluation: compliant interaction not found")
	}
	if reInput.Interaction.Channel == "" {
		t.Error("GetInteractionForReEvaluation: Channel is empty, want the snapshotted channel")
	}
	if reInput.Interaction.ContactPartyRelationship != "debtor" {
		t.Errorf("GetInteractionForReEvaluation: ContactPartyRelationship = %q, want %q", reInput.Interaction.ContactPartyRelationship, "debtor")
	}
	if len(reInput.Interaction.AuthorizedChannels) == 0 {
		t.Error("GetInteractionForReEvaluation: AuthorizedChannels is empty, want the snapshotted list")
	}
	if reInput.Interaction.PaymentRecipient != "creditor" {
		t.Errorf("GetInteractionForReEvaluation: PaymentRecipient = %q, want %q", reInput.Interaction.PaymentRecipient, "creditor")
	}
	if reInput.Interaction.ContactedPartyDOB == nil {
		t.Error("GetInteractionForReEvaluation: ContactedPartyDOB is nil, want the debtor's snapshotted date of birth")
	}
	reEvalDetectors := map[string]detection.Detector{
		"MX-REDECO-06": detection.ThirdPartyContactDetector{},
		"MX-REDECO-07": detection.ProtectedPopulationDetector{},
		"MX-REDECO-11": detection.AuthorizedChannelDetector{},
		"MX-REDECO-10": detection.PaymentRoutingDetector{},
	}
	for code, d := range reEvalDetectors {
		result := d.Evaluate(reInput.Interaction)
		if result.Outcome != detection.OutcomePass {
			t.Errorf("re-evaluated %s on the adapter's snapshot = %q, want pass (reproducibility)", code, result.Outcome)
		}
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
