package postgres_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/outbound"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// requireDatabaseURL skips the calling test in -short mode or when
// DATABASE_URL is unset, matching the repo's existing integration test
// convention (see evaluation_integration_test.go).
func requireDatabaseURL(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for the evidence ledger integration test")
	}
	return databaseURL
}

// evaluateOnce runs one evaluation for a tenant/interaction pair through the
// real ledger append path (evaluation.Service -> EvaluationStore.CreateEvaluation).
func evaluateOnce(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, interactionID string) core.Evaluation {
	t.Helper()
	store := postgres.NewEvaluationStoreFromPool(pool)
	svc := evaluation.Service{
		Detectors: []evaluation.NamedDetector{{Code: "contact-hours", Detector: fakeBlockDetector{}}},
		Store:     store,
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
	return got
}

func fetchEvidenceRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, evaluationID string) (seq int64, prevHash, hash, overallOutcome string) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		SELECT seq, prev_hash, hash, overall_outcome
		FROM evidence_records WHERE tenant_id = $1 AND evaluation_id = $2
	`, tenantID, evaluationID).Scan(&seq, &prevHash, &hash, &overallOutcome); err != nil {
		t.Fatalf("read evidence_records row: %v", err)
	}
	return
}

func countEvidenceRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, evaluationID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_records WHERE evaluation_id = $1`, evaluationID).Scan(&count); err != nil {
		t.Fatalf("count evidence_records: %v", err)
	}
	return count
}

// TestEvidenceAppendProducesExactlyOneRecord covers *Successful evaluation
// produces exactly one evidence record* and *First record for a tenant uses
// the genesis prev_hash*.
func TestOutboundAuthorityDecisionRecorderWritesBlockedEvidenceChain(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "outbound-authority")
	bundleID := seedActivePolicyBundle(t, ctx, pool, tenantID, "outbound-authority", "redeco-2026.1")

	recorder := postgres.NewOutboundDecisionRecorderFromPool(pool)
	recorded, err := recorder.Record(ctx, outbound.DecisionRequest{
		TenantID: tenantID,
		ActorID:  "agent-1",
		Mode:     outbound.DecisionModeEnforcement,
		Proposal: outbound.OutboundActionProposal{
			Kind:       outbound.ActionSendOutboundUtterance,
			ProposalID: "proposal-1",
			DebtorID:   debtorID,
			Channel:    core.InteractionChannelMessage,
			Text:       "bounded proposed utterance is not stored in evidence metadata",
			ProposedAt: time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC),
		},
		RequestID: "decision-1",
	}, outbound.Decision{
		ID:                  "decision-1",
		Mode:                outbound.DecisionModeEnforcement,
		ActionKind:          outbound.ActionSendOutboundUtterance,
		ProposalID:          "proposal-1",
		Outcome:             outbound.DecisionDeny,
		Reason:              "third-party contact rule blocked the outbound proposal",
		PolicyBundleID:      bundleID,
		PolicyBundleVersion: "redeco-2026.1",
		CheckedAt:           time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC),
		Violations: []outbound.RuleViolation{{
			RuleCode:    "MX-REDECO-06",
			Severity:    core.SeverityHigh,
			Rationale:   "third-party contact is not authorized",
			Remediation: "Contact only the debtor or authorized third party.",
		}},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(recorded.EventRefs) != 1 || recorded.EventRefs[0].Type != "interaction_event" || recorded.EventRefs[0].Mode != outbound.DecisionModeEnforcement {
		t.Fatalf("event refs = %+v, want enforcement interaction_event", recorded.EventRefs)
	}
	if len(recorded.EvidenceRefs) != 1 || recorded.EvidenceRefs[0].Type != "evidence_record" || recorded.EvidenceRefs[0].Mode != outbound.DecisionModeEnforcement {
		t.Fatalf("evidence refs = %+v, want enforcement evidence_record", recorded.EvidenceRefs)
	}

	var status, transcriptRef string
	if err := pool.QueryRow(ctx, `SELECT status, transcript_ref FROM interaction_events WHERE id = $1`, recorded.EventRefs[0].ID).Scan(&status, &transcriptRef); err != nil {
		t.Fatalf("read interaction_event: %v", err)
	}
	if status != "blocked_before_send" {
		t.Fatalf("interaction status = %q, want blocked_before_send", status)
	}
	if transcriptRef != "outbound:decision/decision-1/proposal/proposal-1" {
		t.Fatalf("transcript_ref = %q, want decision/proposal correlation", transcriptRef)
	}

	var outcome, policyVersion string
	if err := pool.QueryRow(ctx, `
		SELECT e.overall_outcome, er.policy_bundle_version
		FROM evidence_records er
		JOIN evaluations e ON e.id = er.evaluation_id
		WHERE er.id = $1
	`, recorded.EvidenceRefs[0].ID).Scan(&outcome, &policyVersion); err != nil {
		t.Fatalf("read evidence chain: %v", err)
	}
	if outcome != "fail" || policyVersion != "redeco-2026.1" {
		t.Fatalf("chain = (%s, %s), want (fail, redeco-2026.1)", outcome, policyVersion)
	}

	rows, err := pool.Query(ctx, `
		SELECT dr.detector_code, dr.result_payload->>'rationale'
		FROM evidence_records er
		JOIN evaluations e ON e.id = er.evaluation_id
		JOIN detector_result_rows dr ON dr.evaluation_id = e.id
		WHERE er.id = $1
	`, recorded.EvidenceRefs[0].ID)
	if err != nil {
		t.Fatalf("read detector rows: %v", err)
	}
	defer rows.Close()
	seen := map[string]string{}
	for rows.Next() {
		var code, rationale string
		if err := rows.Scan(&code, &rationale); err != nil {
			t.Fatalf("scan detector row: %v", err)
		}
		seen[code] = rationale
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate detector rows: %v", err)
	}
	if _, ok := seen["MX-REDECO-06"]; !ok {
		t.Fatalf("detector rows = %+v, want MX-REDECO-06", seen)
	}
	authority := seen["authority_decision"]
	for _, want := range []string{"decision_id=decision-1", "decision_mode=enforcement", "decision_outcome=deny", "action_kind=send_outbound_utterance", "proposal_id=proposal-1"} {
		if !strings.Contains(authority, want) {
			t.Fatalf("authority_decision rationale = %q, want %q", authority, want)
		}
	}
}

func TestOutboundAuthorityContextResolverUsesTenantScopedDebtorTimezone(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "outbound-resolver-a")
	tenantB, _ := seedTenantAndDebtor(t, ctx, pool, "outbound-resolver-b")
	resolver := postgres.NewOutboundAuthorityContextResolverFromPool(pool)

	authority, err := resolver.Resolve(ctx, tenantA, outbound.OutboundActionProposal{
		DebtorID:                 debtorA,
		Channel:                  core.InteractionChannelMessage,
		ProposedAt:               time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC),
		DebtorTimezone:           "Etc/UTC",
		ContactPartyRelationship: "debtor",
		AuthorizedChannels:       []string{"message"},
		PaymentTarget:            "creditor",
	})
	if err != nil {
		t.Fatalf("Resolve tenant A debtor: %v", err)
	}
	if authority.TenantID != tenantA || authority.DebtorID != debtorA || authority.DebtorTimezone != "America/Mexico_City" {
		t.Fatalf("authority = %+v, want tenant-scoped DB debtor timezone", authority)
	}

	if _, err := resolver.Resolve(ctx, tenantB, outbound.OutboundActionProposal{DebtorID: debtorA, Channel: core.InteractionChannelMessage, ProposedAt: time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)}); err == nil {
		t.Fatal("expected cross-tenant debtor lookup to fail")
	}
}

func TestOutboundAuthorityDecisionRecorderRollsBackInteractionWhenEvidenceFails(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "outbound-rollback")
	recorder := postgres.NewOutboundDecisionRecorderFromPool(pool)
	_, err = recorder.Record(ctx, outbound.DecisionRequest{
		TenantID:  tenantID,
		Mode:      outbound.DecisionModeEnforcement,
		Proposal:  outbound.OutboundActionProposal{Kind: outbound.ActionSendOutboundUtterance, ProposalID: "proposal-rollback", DebtorID: debtorID, Channel: core.InteractionChannelMessage, ProposedAt: time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC)},
		RequestID: "decision-rollback",
	}, outbound.Decision{
		ID:                  "decision-rollback",
		Mode:                outbound.DecisionModeEnforcement,
		ActionKind:          outbound.ActionSendOutboundUtterance,
		ProposalID:          "proposal-rollback",
		Outcome:             outbound.DecisionDeny,
		PolicyBundleID:      "00000000-0000-0000-0000-000000000001",
		PolicyBundleVersion: "missing-bundle",
		CheckedAt:           time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC),
		Violations:          []outbound.RuleViolation{{RuleCode: "MX-REDECO-06", Severity: core.SeverityHigh, Rationale: "blocked"}},
	})
	if err == nil {
		t.Fatal("expected Record to fail on missing policy bundle FK")
	}
	var interactions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interaction_events WHERE tenant_id = $1 AND transcript_ref = 'outbound:decision/decision-rollback/proposal/proposal-rollback'`, tenantID).Scan(&interactions); err != nil {
		t.Fatalf("count interaction_events: %v", err)
	}
	if interactions != 0 {
		t.Fatalf("interaction_events persisted after failed evidence transaction = %d, want 0", interactions)
	}
}

func TestEvidenceAppendProducesExactlyOneRecord(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-single")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/single")

	got := evaluateOnce(t, ctx, pool, tenantID, interactionID)

	if n := countEvidenceRows(t, ctx, pool, string(got.ID)); n != 1 {
		t.Fatalf("evidence_records rows for evaluation = %d, want 1", n)
	}

	seq, prevHash, _, _ := fetchEvidenceRow(t, ctx, pool, tenantID, string(got.ID))
	if seq != 1 {
		t.Fatalf("seq = %d, want 1", seq)
	}
	if prevHash != ledger.GenesisPrevHash {
		t.Fatalf("prev_hash = %q, want empty genesis sentinel", prevHash)
	}

	// A second insert attempt for the same evaluation_id must fail on
	// UNIQUE (tenant_id, evaluation_id).
	_, err = pool.Exec(ctx, `
		INSERT INTO evidence_records (tenant_id, interaction_event_id, evaluation_id, seq,
			prev_hash, hash, overall_outcome, policy_bundle_version, inputs_digest, created_at)
		VALUES ($1, $2, $3, $4, 'x', 'y', 'pass', '', 'z', now())
	`, tenantID, interactionID, string(got.ID), seq+1)
	if err == nil {
		t.Fatal("expected duplicate evaluation_id insert to fail on UNIQUE(tenant_id, evaluation_id)")
	}
}

// TestEvidenceAppendSharesEvaluationTransaction is a code-review assertion
// (the persistence hook lives inside CreateEvaluation's existing
// tenantdb.WithTenantTx closure — see internal/postgres/adapters.go) backed
// by the forced-failure case below: if any write in that closure fails, none
// of the three writes (evaluations, detector_result_rows, evidence_records)
// persist.
func TestEvidenceAppendSharesEvaluationTransaction(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-shared-tx")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/shared-tx")

	// Force a mid-transaction failure: a detector code containing an
	// embedded NUL byte is rejected by Postgres ("invalid byte sequence for
	// encoding UTF8: 0x00", SQLSTATE 22021) when the detector_result_rows
	// insert runs, aborting the whole transaction before the evidence
	// append or chain-head update are reached. Because all writes share one
	// tenantdb.WithTenantTx call, the abort rolls back the evaluations
	// header too — proving the three writes are transactionally coupled
	// regardless of which statement is the one that fails.
	store := postgres.NewEvaluationStoreFromPool(pool)
	_, err = store.CreateEvaluation(ctx, evaluation.CreateEvaluationInput{
		TenantID:           tenantID,
		InteractionEventID: interactionID,
		OverallOutcome:     "fail",
		DetectorResults: []evaluation.DetectorResultInput{
			{DetectorCode: "bad\x00code", Outcome: core.DetectorOutcomeFail, Severity: core.SeverityHigh, Rationale: "forced failure"},
		},
	})
	if err == nil {
		t.Fatal("expected CreateEvaluation to fail on the forced detector-row insert error")
	}

	var evalCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evaluations WHERE interaction_event_id = $1`, interactionID).Scan(&evalCount); err != nil {
		t.Fatalf("count evaluations: %v", err)
	}
	if evalCount != 0 {
		t.Fatalf("evaluations rows = %d, want 0 (rollback must remove the header too)", evalCount)
	}

	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_records WHERE interaction_event_id = $1`, interactionID).Scan(&evidenceCount); err != nil {
		t.Fatalf("count evidence_records: %v", err)
	}
	if evidenceCount != 0 {
		t.Fatalf("evidence_records rows = %d, want 0", evidenceCount)
	}

	var headExists bool
	if err := pool.QueryRow(ctx, `SELECT exists(SELECT 1 FROM ledger_chain_heads WHERE tenant_id = $1)`, tenantID).Scan(&headExists); err != nil {
		t.Fatalf("check ledger_chain_heads: %v", err)
	}
	if headExists {
		t.Fatal("ledger_chain_heads row was created despite the transaction rolling back")
	}

	// The next successful evaluation for this tenant must still receive
	// seq = 1 with no gap from the failed attempt.
	interaction2 := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/shared-tx-2")
	got := evaluateOnce(t, ctx, pool, tenantID, interaction2)
	seq, prevHash, _, _ := fetchEvidenceRow(t, ctx, pool, tenantID, string(got.ID))
	if seq != 1 {
		t.Fatalf("seq after failed attempt = %d, want 1 (no gap)", seq)
	}
	if prevHash != ledger.GenesisPrevHash {
		t.Fatalf("prev_hash after failed attempt = %q, want genesis", prevHash)
	}
}

// TestEvidenceChainLinkageAndSequence covers *Subsequent records chain to
// the previous hash* and *Sequence has no gaps under normal operation*.
func TestEvidenceChainLinkageAndSequence(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-chain")

	const k = 4
	var lastHash string
	for i := 0; i < k; i++ {
		interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/chain")
		got := evaluateOnce(t, ctx, pool, tenantID, interactionID)
		seq, prevHash, hash, _ := fetchEvidenceRow(t, ctx, pool, tenantID, string(got.ID))
		if seq != int64(i+1) {
			t.Fatalf("record %d: seq = %d, want %d", i, seq, i+1)
		}
		if i == 0 {
			if prevHash != ledger.GenesisPrevHash {
				t.Fatalf("record 0: prev_hash = %q, want genesis", prevHash)
			}
		} else if prevHash != lastHash {
			t.Fatalf("record %d: prev_hash = %q, want %q (previous hash)", i, prevHash, lastHash)
		}
		lastHash = hash
	}

	// Sequence has no gaps or duplicates.
	rows, err := pool.Query(ctx, `SELECT seq FROM evidence_records WHERE tenant_id = $1 ORDER BY seq ASC`, tenantID)
	if err != nil {
		t.Fatalf("query seqs: %v", err)
	}
	defer rows.Close()
	var seqs []int64
	for rows.Next() {
		var s int64
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan seq: %v", err)
		}
		seqs = append(seqs, s)
	}
	if len(seqs) != k {
		t.Fatalf("len(seqs) = %d, want %d", len(seqs), k)
	}
	for i, s := range seqs {
		if s != int64(i+1) {
			t.Fatalf("seqs = %v, want 1..%d with no gaps", seqs, k)
		}
	}
}

// TestEvidenceConcurrentAppendsSameTenantNeverFork covers *Concurrent
// appends for one tenant never fork the chain*.
func TestEvidenceConcurrentAppendsSameTenantNeverFork(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-concurrent")
	interactionA := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/concurrent-a")
	interactionB := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/concurrent-b")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		evaluateOnce(t, ctx, pool, tenantID, interactionA)
	}()
	go func() {
		defer wg.Done()
		evaluateOnce(t, ctx, pool, tenantID, interactionB)
	}()
	wg.Wait()

	var lastSeq int64
	if err := pool.QueryRow(ctx, `SELECT last_seq FROM ledger_chain_heads WHERE tenant_id = $1`, tenantID).Scan(&lastSeq); err != nil {
		t.Fatalf("read ledger_chain_heads: %v", err)
	}
	if lastSeq != 2 {
		t.Fatalf("last_seq = %d, want 2", lastSeq)
	}

	verifier := postgres.NewChainVerifierFromPool(pool)
	result, err := verifier.VerifyChain(ctx, tenantID)
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if !result.OK || result.Count != 2 {
		t.Fatalf("VerifyChain = %+v, want OK with 2 records (no fork)", result)
	}
}

// TestEvidenceConcurrentAppendsDifferentTenantsIndependent covers
// *Concurrent appends across different tenants proceed independently*.
func TestEvidenceConcurrentAppendsDifferentTenantsIndependent(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "evid-indep-a")
	tenantB, debtorB := seedTenantAndDebtor(t, ctx, pool, "evid-indep-b")
	interactionA := seedInteraction(t, ctx, pool, tenantA, debtorA, "evid/indep-a")
	interactionB := seedInteraction(t, ctx, pool, tenantB, debtorB, "evid/indep-b")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		evaluateOnce(t, ctx, pool, tenantA, interactionA)
	}()
	go func() {
		defer wg.Done()
		evaluateOnce(t, ctx, pool, tenantB, interactionB)
	}()
	wg.Wait()

	verifierA := postgres.NewChainVerifierFromPool(pool)
	resultA, err := verifierA.VerifyChain(ctx, tenantA)
	if err != nil {
		t.Fatalf("VerifyChain tenant A: %v", err)
	}
	if !resultA.OK || resultA.Count != 1 {
		t.Fatalf("tenant A VerifyChain = %+v, want OK with 1 record", resultA)
	}

	resultB, err := verifierA.VerifyChain(ctx, tenantB)
	if err != nil {
		t.Fatalf("VerifyChain tenant B: %v", err)
	}
	if !resultB.OK || resultB.Count != 1 {
		t.Fatalf("tenant B VerifyChain = %+v, want OK with 1 record", resultB)
	}
}

// TestEvidenceRecordsAreWriteOnceAgainstOwnerConnection covers *Direct SQL
// UPDATE against evidence_records fails* and *Direct SQL DELETE against
// evidence_records fails*, exercised via the DB owner connection (the same
// role the application uses over DATABASE_URL) to prove the trigger is
// role-independent, not merely an app-layer convention.
func TestEvidenceRecordsAreWriteOnceAgainstOwnerConnection(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-writeonce")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/writeonce")
	got := evaluateOnce(t, ctx, pool, tenantID, interactionID)

	_, err = pool.Exec(ctx, `UPDATE evidence_records SET hash = 'tampered' WHERE evaluation_id = $1`, string(got.ID))
	if err == nil {
		t.Fatal("expected UPDATE against evidence_records to fail")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("UPDATE error = %v, want append-only exception", err)
	}
	_, _, hashAfterUpdate, _ := fetchEvidenceRow(t, ctx, pool, tenantID, string(got.ID))
	if hashAfterUpdate == "tampered" {
		t.Fatal("row was mutated despite the trigger rejecting the UPDATE")
	}

	_, err = pool.Exec(ctx, `DELETE FROM evidence_records WHERE evaluation_id = $1`, string(got.ID))
	if err == nil {
		t.Fatal("expected DELETE against evidence_records to fail")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("DELETE error = %v, want append-only exception", err)
	}
	if n := countEvidenceRows(t, ctx, pool, string(got.ID)); n != 1 {
		t.Fatalf("evidence_records rows after failed DELETE = %d, want 1 (still exists)", n)
	}
}

// TestEvidenceRecordsWriteOnceSurvivesSessionReplicationRole covers the gap
// left by ENABLE ORIGIN (the trigger default): a session that sets
// session_replication_role = replica (e.g. logical-replication or restore
// tooling) must still be blocked, because the migration marks both
// evidence_records triggers ENABLE ALWAYS.
func TestEvidenceRecordsWriteOnceSurvivesSessionReplicationRole(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-replica-role")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/replica-role")
	got := evaluateOnce(t, ctx, pool, tenantID, interactionID)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SET session_replication_role = replica`); err != nil {
		t.Fatalf("set session_replication_role: %v", err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, `SET session_replication_role = origin`); err != nil {
			t.Fatalf("reset session_replication_role: %v", err)
		}
	}()

	_, err = conn.Exec(ctx, `UPDATE evidence_records SET hash = 'tampered' WHERE evaluation_id = $1`, string(got.ID))
	if err == nil {
		t.Fatal("expected UPDATE against evidence_records to fail even under session_replication_role = replica")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("UPDATE error = %v, want append-only exception", err)
	}

	_, err = conn.Exec(ctx, `DELETE FROM evidence_records WHERE evaluation_id = $1`, string(got.ID))
	if err == nil {
		t.Fatal("expected DELETE against evidence_records to fail even under session_replication_role = replica")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("DELETE error = %v, want append-only exception", err)
	}

	if n := countEvidenceRows(t, ctx, pool, string(got.ID)); n != 1 {
		t.Fatalf("evidence_records rows after failed UPDATE/DELETE under replica role = %d, want 1 (still exists)", n)
	}
}

// TestEvidenceRecordsTruncateBlocked covers the row-trigger blind spot:
// row-level BEFORE UPDATE OR DELETE triggers never fire on TRUNCATE, so a
// dedicated statement-level BEFORE TRUNCATE trigger is required to keep the
// ledger append-only.
func TestEvidenceRecordsTruncateBlocked(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-truncate")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/truncate")
	got := evaluateOnce(t, ctx, pool, tenantID, interactionID)

	_, err = pool.Exec(ctx, `TRUNCATE evidence_records`)
	if err == nil {
		t.Fatal("expected TRUNCATE against evidence_records to fail")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("TRUNCATE error = %v, want append-only exception", err)
	}

	if n := countEvidenceRows(t, ctx, pool, string(got.ID)); n != 1 {
		t.Fatalf("evidence_records rows after failed TRUNCATE = %d, want 1 (still exists)", n)
	}
}

// TestVerifyChainDetectsTamperedFields covers *VerifyChain detects a
// tampered overall_outcome / inputs_digest / prev_hash / seq* against real
// Postgres. Because the write-once trigger blocks UPDATE unconditionally
// (even for the owner role), tampering is simulated the way a pre-ledger
// chain audit would encounter it: disable the trigger for the duration of
// the direct UPDATE, then re-enable it, and assert the store-backed
// VerifyChain adapter reports a break at the correct seq.
func TestVerifyChainDetectsTamperedFields(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tests := []struct {
		name        string
		column      string
		value       any
		wantBreakAt int64
		wantReason  string
	}{
		{name: "tampered overall_outcome", column: "overall_outcome", value: "tampered", wantBreakAt: 2, wantReason: "hash mismatch"},
		{name: "tampered inputs_digest", column: "inputs_digest", value: "tampered-digest", wantBreakAt: 2, wantReason: "hash mismatch"},
		{name: "tampered prev_hash", column: "prev_hash", value: "tampered-prev-hash", wantBreakAt: 2, wantReason: "prev_hash linkage"},
		{name: "tampered seq", column: "seq", value: int64(99), wantBreakAt: 3, wantReason: "seq gap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-tamper-"+strings.ReplaceAll(tt.column, "_", "-"))
			for i := 0; i < 3; i++ {
				interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/tamper-"+tt.column+"-"+time.Now().Format("150405.000000000"))
				evaluateOnce(t, ctx, pool, tenantID, interactionID)
			}

			// Tamper the record with seq = 2 directly, bypassing the
			// write-once trigger for the duration of this statement only.
			tamperEvidenceRecordBypassingTrigger(t, ctx, pool, tenantID, tt.column, tt.value)

			verifier := postgres.NewChainVerifierFromPool(pool)
			result, err := verifier.VerifyChain(ctx, tenantID)
			if err != nil {
				t.Fatalf("VerifyChain: %v", err)
			}
			if result.OK {
				t.Fatal("VerifyChain OK = true, want tampering detected")
			}
			if result.BreakAtSeq != tt.wantBreakAt {
				t.Fatalf("BreakAtSeq = %d, want %d", result.BreakAtSeq, tt.wantBreakAt)
			}
			if result.BreakReason != tt.wantReason {
				t.Fatalf("BreakReason = %q, want %q", result.BreakReason, tt.wantReason)
			}
		})
	}
}

// tamperEvidenceRecordBypassingTrigger disables the write-once trigger for
// the duration of a single UPDATE against the record with seq = 2 for the
// given tenant, then re-enables it. Session-scoped
// "ALTER TABLE ... DISABLE TRIGGER" is used (per design.md's suggested
// mechanism) rather than session_replication_role, so only this specific
// trigger is bypassed and RLS/other triggers remain active.
func tamperEvidenceRecordBypassingTrigger(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, column string, value any) {
	t.Helper()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `ALTER TABLE evidence_records DISABLE TRIGGER evidence_records_no_update_delete`); err != nil {
		t.Fatalf("disable trigger: %v", err)
	}
	defer func() {
		// Restore with ENABLE ALWAYS (not plain ENABLE): the migration created
		// this trigger as ENABLE ALWAYS so it fires under session_replication_role
		// = replica. A plain ENABLE would silently downgrade it to ORIGIN and
		// leak that weakened state into other tests sharing this database.
		if _, err := conn.Exec(ctx, `ALTER TABLE evidence_records ENABLE ALWAYS TRIGGER evidence_records_no_update_delete`); err != nil {
			t.Fatalf("re-enable trigger: %v", err)
		}
	}()

	query := `UPDATE evidence_records SET ` + column + ` = $1 WHERE tenant_id = $2 AND seq = 2`
	if _, err := conn.Exec(ctx, query, value, tenantID); err != nil {
		t.Fatalf("tamper record: %v", err)
	}
}

// TestEvidenceRLSIsolationAcrossTenants covers RLS-enforced tenant isolation
// for evidence_records / ledger_chain_heads (design.md's "Integration — RLS
// isolation" testing-strategy row), following the restricted-role
// APP_DATABASE_URL pattern established in internal/db/rls_isolation_test.go
// and internal/postgres/evaluation_integration_test.go. Skips (with a
// documented reason) when APP_DATABASE_URL is not configured, since no
// restricted role is provisioned by default in this dev environment.
func TestEvidenceRLSIsolationAcrossTenants(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	appDatabaseURL := os.Getenv("APP_DATABASE_URL")
	if appDatabaseURL == "" {
		t.Skip("APP_DATABASE_URL (a role without BypassRLS) is required for the evidence RLS isolation test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "evid-rls-a")
	tenantB, debtorB := seedTenantAndDebtor(t, ctx, pool, "evid-rls-b")
	interactionA := seedInteraction(t, ctx, pool, tenantA, debtorA, "evid-rls/tenant-a")
	interactionB := seedInteraction(t, ctx, pool, tenantB, debtorB, "evid-rls/tenant-b")

	// Seed via the owner pool (setup only — the owner role bypasses RLS).
	evaluateOnce(t, ctx, pool, tenantA, interactionA)
	evaluateOnce(t, ctx, pool, tenantB, interactionB)

	appPool, err := pgxpool.New(ctx, appDatabaseURL)
	if err != nil {
		t.Fatalf("connect app database: %v", err)
	}
	defer appPool.Close()

	// (a) Tenant B's evidence_records / ledger_chain_heads rows must not be
	// readable under tenant A's RLS context.
	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin app tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantA); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}

	rows, err := tx.Query(ctx, `SELECT tenant_id FROM evidence_records WHERE interaction_event_id = $1`, interactionB)
	if err != nil {
		t.Fatalf("query tenant B evidence under tenant A context: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("tenant B's evidence_records row was readable under tenant A's RLS context")
	}
	rows.Close()

	var headExists bool
	if err := tx.QueryRow(ctx, `SELECT exists(SELECT 1 FROM ledger_chain_heads WHERE tenant_id = $1)`, tenantB).Scan(&headExists); err != nil {
		t.Fatalf("query tenant B ledger_chain_heads under tenant A context: %v", err)
	}
	if headExists {
		t.Fatal("tenant B's ledger_chain_heads row was readable under tenant A's RLS context")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback tx: %v", err)
	}

	// (b) The store-backed VerifyChain adapter under tenant A's restricted
	// role must see only tenant A's chain.
	verifier := postgres.NewChainVerifierFromPool(appPool)
	resultA, err := verifier.VerifyChain(ctx, tenantA)
	if err != nil {
		t.Fatalf("VerifyChain tenant A (restricted role): %v", err)
	}
	if !resultA.OK || resultA.Count != 1 {
		t.Fatalf("tenant A VerifyChain (restricted role) = %+v, want OK with exactly 1 record (not tenant B's)", resultA)
	}
}

// TestPreMigrationEvaluationHasNoEvidenceRecord covers *Pre-migration
// evaluation has no evidence record*: an evaluations row inserted directly
// (bypassing the ledger append path, simulating a pre-#3 evaluation) must
// not have a matching evidence_records row.
func TestPreMigrationEvaluationHasNoEvidenceRecord(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-premigration")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid/premigration")

	var evaluationID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO evaluations (tenant_id, interaction_event_id, overall_outcome)
		VALUES ($1, $2, 'pass') RETURNING id
	`, tenantID, interactionID).Scan(&evaluationID); err != nil {
		t.Fatalf("insert pre-migration-style evaluation: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_records WHERE evaluation_id = $1`, evaluationID).Scan(&count); err != nil {
		t.Fatalf("count evidence_records: %v", err)
	}
	if count != 0 {
		t.Fatalf("evidence_records rows for pre-migration evaluation = %d, want 0", count)
	}
}
