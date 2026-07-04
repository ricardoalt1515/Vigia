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

// TestListInteractionsDoesNotFanOutAcrossDetectorResults covers *Two
// detector results for one evaluation do not fan out*: an evaluation with a
// contact-hours detector row AND the MX-REDECO-05 judge row for the same
// evaluation_id must appear exactly once in the returned interactions list.
func TestListInteractionsDoesNotFanOutAcrossDetectorResults(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "interq-fanout")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "interq/fanout")

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
	if _, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantID,
		InteractionEventID: interactionID,
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC),
			DebtorTimezone: "America/Mexico_City",
		},
		Utterances: []judge.Utterance{{Speaker: "agent", Text: "Le recordamos su pago."}},
	}); err != nil {
		t.Fatalf("EvaluateInteraction: %v", err)
	}

	reader := postgres.NewInteractionReaderFromPool(pool)
	items, err := reader.ListInteractions(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListInteractions: %v", err)
	}

	var matches int
	for _, item := range items {
		if item.ID == interactionID {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("interaction %s appeared %d times in the list, want exactly 1 (no fan-out)", interactionID, matches)
	}
}

// TestListInteractionsWorstSeverityWinsOnDisagreement covers
// *Worst-severity-wins when detector and judge disagree*: a PASS
// contact-hours result plus a BLOCK judge result for the same interaction
// must surface the more severe BLOCK outcome/reason.
func TestListInteractionsWorstSeverityWinsOnDisagreement(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "interq-worst")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "interq/worst")

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
	if _, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantID,
		InteractionEventID: interactionID,
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC),
			DebtorTimezone: "America/Mexico_City",
		},
		// FakeJudge blocks on this threat keyword; contact-hours passes.
		Utterances: []judge.Utterance{{Speaker: "agent", Text: "Si no pagas, vamos a tu casa."}},
	}); err != nil {
		t.Fatalf("EvaluateInteraction: %v", err)
	}

	reader := postgres.NewInteractionReaderFromPool(pool)
	items, err := reader.ListInteractions(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListInteractions: %v", err)
	}

	var found bool
	for _, item := range items {
		if item.ID != interactionID {
			continue
		}
		found = true
		if item.Outcome == nil || *item.Outcome != "BLOCK" {
			t.Fatalf("Outcome = %v, want BLOCK (the judge's severity wins)", item.Outcome)
		}
	}
	if !found {
		t.Fatalf("interaction %s not found in the list", interactionID)
	}
}

// TestListInteractionsCarriesThreatHITLFlag covers *Interactions DTO
// carries a threat/HITL flag*.
func TestListInteractionsCarriesThreatHITLFlag(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "interq-hitl")
	flaggedInteraction := seedInteraction(t, ctx, pool, tenantID, debtorID, "interq/flagged")
	unflaggedInteraction := seedInteraction(t, ctx, pool, tenantID, debtorID, "interq/unflagged")

	store := postgres.NewEvaluationStoreFromPool(pool)
	svc := evaluation.Service{
		Detectors: []evaluation.NamedDetector{{Code: "contact-hours", Detector: passDetector{}}},
		Judges:    []evaluation.NamedJudge{{Code: "MX-REDECO-05", Judge: judge.FakeJudge{}}},
		Rubric:    judge.LoadRubric(),
		Store:     store,
	}

	if _, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantID,
		InteractionEventID: flaggedInteraction,
		Interaction: detection.Interaction{
			OccurredAt: time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC), DebtorTimezone: "America/Mexico_City",
		},
		Utterances: []judge.Utterance{{Speaker: "agent", Text: "Si no pagas, vamos a tu casa."}},
	}); err != nil {
		t.Fatalf("EvaluateInteraction (flagged): %v", err)
	}
	if _, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantID,
		InteractionEventID: unflaggedInteraction,
		Interaction: detection.Interaction{
			OccurredAt: time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC), DebtorTimezone: "America/Mexico_City",
		},
		Utterances: []judge.Utterance{{Speaker: "agent", Text: "Le recordamos su pago."}},
	}); err != nil {
		t.Fatalf("EvaluateInteraction (unflagged): %v", err)
	}

	reader := postgres.NewInteractionReaderFromPool(pool)
	items, err := reader.ListInteractions(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListInteractions: %v", err)
	}

	byID := map[string]bool{}
	for _, item := range items {
		if item.RequiresHITL != nil {
			byID[item.ID] = *item.RequiresHITL
		}
	}
	if hitl, ok := byID[flaggedInteraction]; !ok || !hitl {
		t.Fatalf("RequiresHITL for flagged interaction = %v (ok=%v), want true", hitl, ok)
	}
	if hitl, ok := byID[unflaggedInteraction]; !ok || hitl {
		t.Fatalf("RequiresHITL for unflagged interaction = %v (ok=%v), want false", hitl, ok)
	}
}

// TestListInteractionsDistinguishesPolicyBundleVersionNullFromEmpty covers
// *Console Surfaces the Judging Bundle Version* [integration]: an evaluated
// interaction under a real bundle version surfaces that version string; an
// unevaluated interaction surfaces nil (never a fabricated empty string);
// an evaluated interaction with no active bundle at evaluation time
// surfaces the empty-string sentinel, distinct from nil.
func TestListInteractionsDistinguishesPolicyBundleVersionNullFromEmpty(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "interq-bundleversion")

	if _, err := pool.Exec(ctx, `
		INSERT INTO policy_bundles (tenant_id, name, version, status)
		VALUES ($1, 'interq-bundleversion-policy', 'v2', 'active')
	`, tenantID); err != nil {
		t.Fatalf("seed active policy bundle: %v", err)
	}

	evaluatedWithBundleID := seedInteraction(t, ctx, pool, tenantID, debtorID, "interq/bundleversion/with-bundle")
	unevaluatedID := seedInteraction(t, ctx, pool, tenantID, debtorID, "interq/bundleversion/unevaluated")
	evaluatedNoBundleID := seedInteraction(t, ctx, pool, tenantID, debtorID, "interq/bundleversion/no-bundle")

	resolver := postgres.NewBundleResolverAdapterFromPool(pool)
	store := postgres.NewEvaluationStoreFromPool(pool)
	svcWithResolver := evaluation.Service{
		Detectors: []evaluation.NamedDetector{{Code: "contact-hours", Detector: passDetector{}}},
		Store:     store,
		Resolver:  resolver,
	}
	svcNoResolver := evaluation.Service{
		Detectors: []evaluation.NamedDetector{{Code: "contact-hours", Detector: passDetector{}}},
		Store:     store,
	}

	if _, err := svcWithResolver.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantID,
		InteractionEventID: evaluatedWithBundleID,
		Interaction:        detection.Interaction{OccurredAt: time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC), DebtorTimezone: "America/Mexico_City"},
	}); err != nil {
		t.Fatalf("EvaluateInteraction (with bundle): %v", err)
	}
	if _, err := svcNoResolver.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantID,
		InteractionEventID: evaluatedNoBundleID,
		Interaction:        detection.Interaction{OccurredAt: time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC), DebtorTimezone: "America/Mexico_City"},
	}); err != nil {
		t.Fatalf("EvaluateInteraction (no bundle): %v", err)
	}

	reader := postgres.NewInteractionReaderFromPool(pool)
	items, err := reader.ListInteractions(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListInteractions: %v", err)
	}

	byID := map[string]*string{}
	for _, item := range items {
		byID[item.ID] = item.PolicyBundleVersion
	}

	withBundle, ok := byID[evaluatedWithBundleID]
	if !ok || withBundle == nil || *withBundle != "v2" {
		t.Fatalf("PolicyBundleVersion for evaluated-with-bundle interaction = %v (ok=%v), want pointer to v2", withBundle, ok)
	}
	unevaluated, ok := byID[unevaluatedID]
	if !ok || unevaluated != nil {
		t.Fatalf("PolicyBundleVersion for unevaluated interaction = %v (ok=%v), want nil", unevaluated, ok)
	}
	noBundle, ok := byID[evaluatedNoBundleID]
	if !ok || noBundle == nil || *noBundle != "" {
		t.Fatalf("PolicyBundleVersion for evaluated-no-bundle interaction = %v (ok=%v), want pointer to empty string", noBundle, ok)
	}
}
