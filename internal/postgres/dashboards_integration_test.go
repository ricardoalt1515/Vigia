package postgres_test

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// requireAppPool connects through APP_DATABASE_URL (the restricted
// vigia_app role, which has no BypassRLS attribute), following the
// TestDespachoRLSIsolationAcrossTenants precedent. Both dashboard aggregates
// rely entirely on RLS for tenant scoping (no explicit tenant_id filter in
// their SQL, matching CountOutOfHoursEvaluations), so reading through the
// owner pool (which bypasses RLS) would silently return every tenant's
// rows and this must be exercised through the non-bypass role to be a real
// test of tenant isolation.
func requireAppPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	appDatabaseURL := os.Getenv("APP_DATABASE_URL")
	if appDatabaseURL == "" {
		t.Skip("APP_DATABASE_URL (a role without BypassRLS) is required for the dashboard aggregate tests")
	}
	appPool, err := pgxpool.New(ctx, appDatabaseURL)
	if err != nil {
		t.Fatalf("connect app database: %v", err)
	}
	t.Cleanup(appPool.Close)
	return appPool
}

// seedEvaluation inserts a minimal evaluations header row for an interaction
// (setup only, via the owner pool which bypasses RLS). overall_outcome is a
// placeholder value: DashboardByDespacho/DashboardByCause key off
// detector_result_rows.outcome, never evaluations.overall_outcome, so its
// exact value does not affect either aggregate.
func seedEvaluation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, interactionID string) (evaluationID string) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO evaluations (tenant_id, interaction_event_id, overall_outcome)
		VALUES ($1, $2, 'pass')
		RETURNING id
	`, tenantID, interactionID).Scan(&evaluationID); err != nil {
		t.Fatalf("seed evaluation: %v", err)
	}
	return evaluationID
}

// seedDetectorResultRow inserts one detector_result_rows child with the
// given rule code and outcome ('pass'/'fail'/'warn'/'review').
func seedDetectorResultRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, interactionID, detectorCode, outcome string) {
	t.Helper()
	severity := "low"
	switch outcome {
	case "fail":
		severity = "high"
	case "warn":
		severity = "medium"
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO detector_result_rows (tenant_id, interaction_event_id, detector_code, outcome, severity, result_payload)
		VALUES ($1, $2, $3, $4, $5, '{}'::jsonb)
	`, tenantID, interactionID, detectorCode, outcome, severity); err != nil {
		t.Fatalf("seed detector_result_row: %v", err)
	}
}

// interactionAttributedTo seeds one evaluated interaction attributed to
// despachoID (nil for the unattributed bucket) with a single detector
// result row carrying the given outcome. All writes go through the owner
// pool (setup only, bypasses RLS).
func interactionAttributedTo(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, debtorID string, despachoID *string, transcriptRef, detectorCode, outcome string) {
	t.Helper()
	var interactionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone, despacho_id)
		VALUES ($1, $2, 'phone', 'outbound', 'recorded', now(), $3, 'America/Mexico_City', $4)
		RETURNING id
	`, tenantID, debtorID, transcriptRef, despachoID).Scan(&interactionID); err != nil {
		t.Fatalf("create interaction: %v", err)
	}
	seedEvaluation(t, ctx, pool, tenantID, interactionID)
	seedDetectorResultRow(t, ctx, pool, tenantID, interactionID, detectorCode, outcome)
}

func floatsClose(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// TestDashboardByDespachoRanksAndUnattributed covers the compliance-dashboards
// spec's "Endpoint ranks despachos by violation rate" and "Interactions with
// no despacho attribution are reported under an explicit unattributed
// bucket" [integration] scenarios.
func TestDashboardByDespachoRanksAndUnattributed(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()
	appPool := requireAppPool(t, ctx)

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "dash-despacho-rank")
	highViolation := seedDespacho(t, ctx, pool, tenantID, "dash-despacho-rank/high")
	lowViolation := seedDespacho(t, ctx, pool, tenantID, "dash-despacho-rank/low")

	// highViolation: 2 evaluated interactions, both fail -> rate 1.0.
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, &highViolation, "dash/high-1", "MX-REDECO-04", "fail")
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, &highViolation, "dash/high-2", "MX-REDECO-04", "fail")

	// lowViolation: 2 evaluated interactions, 1 fail + 1 pass -> rate 0.5.
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, &lowViolation, "dash/low-1", "MX-REDECO-04", "fail")
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, &lowViolation, "dash/low-2", "MX-REDECO-04", "pass")

	// Unattributed: 1 evaluated interaction, pass -> rate 0.0. Must appear
	// under the explicit synthetic bucket, never dropped or folded.
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, nil, "dash/unattributed-1", "MX-REDECO-04", "pass")

	reader := postgres.NewDashboardReaderFromPool(appPool)
	rates, err := reader.ByDespacho(ctx, tenantID)
	if err != nil {
		t.Fatalf("ByDespacho: %v", err)
	}
	if len(rates) != 3 {
		t.Fatalf("rates len = %d, want 3; rates = %#v", len(rates), rates)
	}

	// Descending violation-rate order: high (1.0), low (0.5), unattributed (0.0).
	if rates[0].DespachoID == nil || *rates[0].DespachoID != highViolation {
		t.Fatalf("rates[0] = %#v, want highViolation despacho first (rate 1.0)", rates[0])
	}
	if !floatsClose(rates[0].ViolationRate, 1.0) || rates[0].Total != 2 || rates[0].Violations != 2 {
		t.Fatalf("rates[0] = %#v, want total=2 violations=2 rate=1.0", rates[0])
	}

	if rates[1].DespachoID == nil || *rates[1].DespachoID != lowViolation {
		t.Fatalf("rates[1] = %#v, want lowViolation despacho second (rate 0.5)", rates[1])
	}
	if !floatsClose(rates[1].ViolationRate, 0.5) || rates[1].Total != 2 || rates[1].Violations != 1 {
		t.Fatalf("rates[1] = %#v, want total=2 violations=1 rate=0.5", rates[1])
	}

	unattributed := rates[2]
	if unattributed.DespachoID != nil {
		t.Fatalf("unattributed DespachoID = %v, want nil", *unattributed.DespachoID)
	}
	if unattributed.DespachoName != "unattributed" {
		t.Fatalf("unattributed DespachoName = %q, want %q", unattributed.DespachoName, "unattributed")
	}
	if unattributed.Total != 1 || unattributed.Violations != 0 || !floatsClose(unattributed.ViolationRate, 0.0) {
		t.Fatalf("unattributed = %#v, want total=1 violations=0 rate=0.0", unattributed)
	}
}

// TestDashboardByDespachoTieBreaksByName covers the design's tie-break rule:
// despachos tied on violation_rate MUST order by despacho_name ascending.
func TestDashboardByDespachoTieBreaksByName(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()
	appPool := requireAppPool(t, ctx)

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "dash-despacho-tie")
	// Names deliberately seeded out of alphabetical order so a correct
	// tie-break can be distinguished from insertion/creation order.
	zebra := seedDespacho(t, ctx, pool, tenantID, "dash-despacho-tie/zebra")
	alpha := seedDespacho(t, ctx, pool, tenantID, "dash-despacho-tie/alpha")

	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, &zebra, "dash/tie-zebra", "MX-REDECO-04", "fail")
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, &alpha, "dash/tie-alpha", "MX-REDECO-04", "fail")

	reader := postgres.NewDashboardReaderFromPool(appPool)
	rates, err := reader.ByDespacho(ctx, tenantID)
	if err != nil {
		t.Fatalf("ByDespacho: %v", err)
	}
	if len(rates) != 2 {
		t.Fatalf("rates len = %d, want 2", len(rates))
	}
	if rates[0].DespachoID == nil || *rates[0].DespachoID != alpha {
		t.Fatalf("rates[0] = %#v, want alpha despacho first (name tie-break ascending)", rates[0])
	}
	if rates[1].DespachoID == nil || *rates[1].DespachoID != zebra {
		t.Fatalf("rates[1] = %#v, want zebra despacho second", rates[1])
	}
}

// TestDashboardByDespachoRLSIsolation covers "By-despacho aggregate is
// tenant-isolated" [integration].
func TestDashboardByDespachoRLSIsolation(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()
	appPool := requireAppPool(t, ctx)

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "dash-despacho-rls-a")
	tenantB, debtorB := seedTenantAndDebtor(t, ctx, pool, "dash-despacho-rls-b")
	despachoA := seedDespacho(t, ctx, pool, tenantA, "dash-despacho-rls-a/despacho")
	despachoB := seedDespacho(t, ctx, pool, tenantB, "dash-despacho-rls-b/despacho")

	interactionAttributedTo(t, ctx, pool, tenantA, debtorA, &despachoA, "dash/rls-a", "MX-REDECO-04", "fail")
	interactionAttributedTo(t, ctx, pool, tenantB, debtorB, &despachoB, "dash/rls-b", "MX-REDECO-04", "fail")

	reader := postgres.NewDashboardReaderFromPool(appPool)
	ratesA, err := reader.ByDespacho(ctx, tenantA)
	if err != nil {
		t.Fatalf("ByDespacho tenant A: %v", err)
	}
	if len(ratesA) != 1 {
		t.Fatalf("tenant A rates len = %d, want 1 (must not include tenant B's despacho)", len(ratesA))
	}
	if ratesA[0].DespachoID == nil || *ratesA[0].DespachoID != despachoA {
		t.Fatalf("tenant A rates[0] = %#v, want despachoA", ratesA[0])
	}
}

// TestDashboardByCauseBreakdown covers "Endpoint breaks down violations by
// rule code" and "Endpoint reports MX-REDECO-03 warn activity separately
// from violations" [integration].
func TestDashboardByCauseBreakdown(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()
	appPool := requireAppPool(t, ctx)

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "dash-cause")

	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, nil, "dash-cause/04-fail-1", "MX-REDECO-04", "fail")
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, nil, "dash-cause/04-fail-2", "MX-REDECO-04", "fail")
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, nil, "dash-cause/06-fail", "MX-REDECO-06", "fail")
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, nil, "dash-cause/03-warn", "MX-REDECO-03", "warn")
	// A 'review' row must never be counted as a violation.
	interactionAttributedTo(t, ctx, pool, tenantID, debtorID, nil, "dash-cause/05-review", "MX-REDECO-05", "review")

	reader := postgres.NewDashboardReaderFromPool(appPool)
	counts, err := reader.ByCause(ctx, tenantID)
	if err != nil {
		t.Fatalf("ByCause: %v", err)
	}

	byCode := make(map[string]struct{ violations, warnings int64 })
	for _, c := range counts {
		byCode[c.RuleCode] = struct{ violations, warnings int64 }{c.Violations, c.Warnings}
	}

	if got := byCode["MX-REDECO-04"]; got.violations != 2 || got.warnings != 0 {
		t.Fatalf("MX-REDECO-04 = %+v, want violations=2 warnings=0", got)
	}
	if got := byCode["MX-REDECO-06"]; got.violations != 1 || got.warnings != 0 {
		t.Fatalf("MX-REDECO-06 = %+v, want violations=1 warnings=0", got)
	}
	if got := byCode["MX-REDECO-03"]; got.violations != 0 || got.warnings != 1 {
		t.Fatalf("MX-REDECO-03 = %+v, want violations=0 warnings=1 (warn rows must not inflate violations)", got)
	}
	if got := byCode["MX-REDECO-05"]; got.violations != 0 {
		t.Fatalf("MX-REDECO-05 (review outcome) violations = %d, want 0 (review rows are not violations)", got.violations)
	}
}

// TestDashboardByCauseRLSIsolation covers "By-cause aggregate is
// tenant-isolated" [integration].
func TestDashboardByCauseRLSIsolation(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()
	appPool := requireAppPool(t, ctx)

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "dash-cause-rls-a")
	tenantB, debtorB := seedTenantAndDebtor(t, ctx, pool, "dash-cause-rls-b")

	interactionAttributedTo(t, ctx, pool, tenantA, debtorA, nil, "dash-cause-rls-a/04-fail", "MX-REDECO-04", "fail")
	interactionAttributedTo(t, ctx, pool, tenantB, debtorB, nil, "dash-cause-rls-b/04-fail-1", "MX-REDECO-04", "fail")
	interactionAttributedTo(t, ctx, pool, tenantB, debtorB, nil, "dash-cause-rls-b/04-fail-2", "MX-REDECO-04", "fail")

	reader := postgres.NewDashboardReaderFromPool(appPool)
	countsA, err := reader.ByCause(ctx, tenantA)
	if err != nil {
		t.Fatalf("ByCause tenant A: %v", err)
	}
	for _, c := range countsA {
		if c.RuleCode == "MX-REDECO-04" && c.Violations != 1 {
			t.Fatalf("tenant A MX-REDECO-04 violations = %d, want 1 (must not include tenant B's rows)", c.Violations)
		}
	}
}
