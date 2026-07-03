package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/core"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// TestEvidenceJudgeBodyRoundTripsThroughDBReconstruction is the load-bearing
// gate-fix regression test (design.md's CRITICAL note): a chain of a
// judge-less record followed by a judged record must both re-verify through
// ChainVerifier.VerifyChain and EvidenceReader.GetEvidencePackage ->
// VerifyPackage, proving evidenceRowToRecord correctly reconstructs
// Body.Judge from the three evidence_records columns (nil when NULL,
// populated verbatim when set) rather than always reconstructing nil.
func TestEvidenceJudgeBodyRoundTripsThroughDBReconstruction(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "evid-judge-roundtrip")
	store := postgres.NewEvaluationStoreFromPool(pool)

	// Record 1: judge-less, exactly like a #2/#3 evaluation.
	judgelessInteraction := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid-judge/judgeless")
	judgelessEval, err := store.CreateEvaluation(ctx, evaluation.CreateEvaluationInput{
		TenantID:           tenantID,
		InteractionEventID: judgelessInteraction,
		OverallOutcome:     "pass",
		DetectorResults: []evaluation.DetectorResultInput{
			{DetectorCode: "contact-hours", Outcome: core.DetectorOutcomePass, Severity: core.SeverityLow, Rationale: "within window"},
		},
	})
	if err != nil {
		t.Fatalf("CreateEvaluation (judge-less): %v", err)
	}

	// Record 2: judged, using the fake judge's fixed verdict shape
	// end-to-end through CreateEvaluation.
	confidence := 0.95
	judgedInteraction := seedInteraction(t, ctx, pool, tenantID, debtorID, "evid-judge/judged")
	judgedEval, err := store.CreateEvaluation(ctx, evaluation.CreateEvaluationInput{
		TenantID:           tenantID,
		InteractionEventID: judgedInteraction,
		OverallOutcome:     "fail",
		RequiresHITL:       true,
		JudgeModelID:       "claude-haiku-4-5-20251001",
		RubricVersion:      "mx-redeco-05.tone-threat.v1",
		JudgeConfidence:    &confidence,
		DetectorResults: []evaluation.DetectorResultInput{
			{DetectorCode: "MX-REDECO-05", Outcome: core.DetectorOutcomeFail, Severity: core.SeverityCritical, Rationale: "threatening language detected", Confidence: &confidence},
		},
	})
	if err != nil {
		t.Fatalf("CreateEvaluation (judged): %v", err)
	}

	verifier := postgres.NewChainVerifierFromPool(pool)
	chainResult, err := verifier.VerifyChain(ctx, tenantID)
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if !chainResult.OK || chainResult.Count != 2 {
		t.Fatalf("VerifyChain = %+v, want OK with 2 records (judge-less then judged)", chainResult)
	}

	reader := postgres.NewEvidenceReaderFromPool(pool)

	judgelessPkg, err := reader.GetEvidencePackage(ctx, tenantID, judgelessInteraction)
	if err != nil {
		t.Fatalf("GetEvidencePackage (judge-less): %v", err)
	}
	if result := ledger.VerifyPackage(judgelessPkg); !result.OK {
		t.Fatalf("VerifyPackage (judge-less) = %+v, want OK", result)
	}

	judgedPkg, err := reader.GetEvidencePackage(ctx, tenantID, judgedInteraction)
	if err != nil {
		t.Fatalf("GetEvidencePackage (judged): %v", err)
	}
	if result := ledger.VerifyPackage(judgedPkg); !result.OK {
		t.Fatalf("VerifyPackage (judged) = %+v, want OK — the judge sub-object must survive the DB round-trip", result)
	}

	_ = judgelessEval
	_ = judgedEval

	// Tamper the stored judge_confidence column directly (bypassing the
	// write-once trigger, mirroring evidence_integration_test.go's
	// tamperEvidenceRecordBypassingTrigger) and assert a hash mismatch is
	// reported at that seq (the gate-fix regression case).
	tamperJudgeConfidenceBypassingTrigger(t, ctx, pool, tenantID, string(judgedEval.ID), "0.8000")

	tamperedResult, err := verifier.VerifyChain(ctx, tenantID)
	if err != nil {
		t.Fatalf("VerifyChain after tamper: %v", err)
	}
	if tamperedResult.OK {
		t.Fatal("VerifyChain OK = true after tampering judge_confidence, want tampering detected")
	}
}

// tamperJudgeConfidenceBypassingTrigger disables the write-once trigger for
// the duration of a single UPDATE of judge_confidence on the record with the
// given evaluation_id, then re-enables it (see
// evidence_integration_test.go's tamperEvidenceRecordBypassingTrigger for
// the same pattern keyed by seq).
func tamperJudgeConfidenceBypassingTrigger(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, evaluationID, value string) {
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
		if _, err := conn.Exec(ctx, `ALTER TABLE evidence_records ENABLE ALWAYS TRIGGER evidence_records_no_update_delete`); err != nil {
			t.Fatalf("re-enable trigger: %v", err)
		}
	}()

	if _, err := conn.Exec(ctx, `
		UPDATE evidence_records SET judge_confidence = $1
		WHERE tenant_id = $2 AND evaluation_id = $3
	`, value, tenantID, evaluationID); err != nil {
		t.Fatalf("tamper judge_confidence: %v", err)
	}
}
