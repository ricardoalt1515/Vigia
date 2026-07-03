package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/judge"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// TestEvaluationServiceJudgeBlockPersistsInOneTransaction covers *Judge
// verdict, HITL flag, and evidence fields persist in one transaction*:
// evaluating with FakeJudge BLOCK must write one evaluations row carrying
// requires_hitl/judge_model_id/rubric_version consistent with the verdict,
// one judge detector_result_rows child carrying score/confidence, and the
// corresponding evidence_records row — all inside the same
// tenantdb.WithTenantTx call as the evaluation header and detector rows.
func TestEvaluationServiceJudgeBlockPersistsInOneTransaction(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "eval-judge-block")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "eval-judge/block")

	store := postgres.NewEvaluationStoreFromPool(pool)
	svc := evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: passDetector{}},
		},
		Judges: []evaluation.NamedJudge{
			{Code: "MX-REDECO-05", Judge: judge.FakeJudge{}},
		},
		Rubric: judge.LoadRubric(),
		Store:  store,
	}

	got, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantID,
		InteractionEventID: interactionID,
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC),
			DebtorTimezone: "America/Mexico_City",
		},
		Utterances: []judge.Utterance{
			{Speaker: "agent", Text: "Si no pagas, vamos a tu casa."},
		},
	})
	if err != nil {
		t.Fatalf("EvaluateInteraction: %v", err)
	}
	if got.OverallOutcome != "fail" {
		t.Fatalf("OverallOutcome = %q, want fail (HARD BLOCK)", got.OverallOutcome)
	}

	var requiresHITL bool
	var judgeModelID, rubricVersion string
	if err := pool.QueryRow(ctx, `
		SELECT requires_hitl, judge_model_id, rubric_version FROM evaluations WHERE id = $1
	`, string(got.ID)).Scan(&requiresHITL, &judgeModelID, &rubricVersion); err != nil {
		t.Fatalf("read evaluations row: %v", err)
	}
	if !requiresHITL {
		t.Fatal("evaluations.requires_hitl = false, want true for a confident BLOCK")
	}
	if judgeModelID == "" {
		t.Fatal("evaluations.judge_model_id is empty, want the fake judge's model id")
	}
	if rubricVersion != judge.RubricVersion {
		t.Fatalf("evaluations.rubric_version = %q, want %q", rubricVersion, judge.RubricVersion)
	}

	var judgeRowCount int
	var confidence, score *string
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM detector_result_rows
		WHERE evaluation_id = (SELECT id FROM evaluations WHERE id = $1) AND detector_code = 'MX-REDECO-05'
	`, string(got.ID)).Scan(&judgeRowCount); err != nil {
		t.Fatalf("count judge detector_result_rows: %v", err)
	}
	if judgeRowCount != 1 {
		t.Fatalf("judge detector_result_rows count = %d, want 1", judgeRowCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT confidence::text, score::text FROM detector_result_rows
		WHERE evaluation_id = (SELECT id FROM evaluations WHERE id = $1) AND detector_code = 'MX-REDECO-05'
	`, string(got.ID)).Scan(&confidence, &score); err != nil {
		t.Fatalf("read judge detector_result_rows confidence/score: %v", err)
	}
	if confidence == nil {
		t.Fatal("judge detector_result_rows.confidence is NULL, want the fake judge's confidence")
	}

	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_records WHERE evaluation_id = $1`, string(got.ID)).Scan(&evidenceCount); err != nil {
		t.Fatalf("count evidence_records: %v", err)
	}
	if evidenceCount != 1 {
		t.Fatalf("evidence_records count = %d, want 1", evidenceCount)
	}
}

// passDetector always passes, so the judge's BLOCK verdict is what drives
// overall_outcome in the test above (isolating the judge fold from the
// detector fold).
type passDetector struct{}

func (passDetector) Evaluate(_ detection.Interaction) detection.Result {
	return detection.Result{Outcome: detection.OutcomePass, Rationale: "inside window"}
}
