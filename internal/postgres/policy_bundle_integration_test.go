package postgres_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/evaluation"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// seedActivePolicyBundle creates a minimal policy_rules + policy_bundles +
// policy_bundle_rules fixture directly via the owner pool (bypasses RLS,
// same pattern as seedTenantAndDebtor), returning the bundle id.
func seedActivePolicyBundle(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, name, version string) (bundleID string) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO policy_bundles (tenant_id, name, version, status)
		VALUES ($1, $2, $3, 'active')
		RETURNING id
	`, tenantID, name, version).Scan(&bundleID); err != nil {
		t.Fatalf("seed active policy bundle: %v", err)
	}
	return bundleID
}

func seedPolicyRule(t *testing.T, ctx context.Context, pool *pgxpool.Pool, code string) (ruleID string) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		INSERT INTO policy_rules (code, title, description, severity)
		VALUES ($1, $2, $3, 'medium')
		RETURNING id
	`, code, code, code).Scan(&ruleID); err != nil {
		t.Fatalf("seed policy rule: %v", err)
	}
	return ruleID
}

func seedPolicyBundleRule(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, bundleID, ruleID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO policy_bundle_rules (tenant_id, policy_bundle_id, policy_rule_id, effective_date, legal_basis)
		VALUES ($1, $2, $3, current_date, 'test-legal-basis')
	`, tenantID, bundleID, ruleID); err != nil {
		t.Fatalf("seed policy bundle rule: %v", err)
	}
}

func requireDB(t *testing.T) (ctx context.Context, cancel context.CancelFunc, pool *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for the policy bundle integration test")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	var err error
	pool, err = pgxpool.New(ctx, databaseURL)
	if err != nil {
		cancel()
		t.Fatalf("connect database: %v", err)
	}
	return ctx, cancel, pool
}

// TestPolicyBundlesGuardBlocksDirectMutation covers *Policy Bundles and Rule
// Snapshots Are Append-Only* [integration]: owner-conn UPDATE/DELETE against
// policy_bundles and policy_bundle_rules, allowed status-only transitions,
// and the illegal active->draft transition.
func TestPolicyBundlesGuardBlocksDirectMutation(t *testing.T) {
	ctx, cancel, pool := requireDB(t)
	defer cancel()
	defer pool.Close()

	tenantID, _ := seedTenantAndDebtor(t, ctx, pool, "pb-guard")

	t.Run("UPDATE a non-status column on policy_bundles fails, row unchanged", func(t *testing.T) {
		bundleID := seedActivePolicyBundle(t, ctx, pool, tenantID, "guard-update-name", "v1")

		_, err := pool.Exec(ctx, `UPDATE policy_bundles SET name = 'changed' WHERE id = $1`, bundleID)
		if err == nil {
			t.Fatal("expected UPDATE of a non-status column to fail")
		}

		var name string
		if err := pool.QueryRow(ctx, `SELECT name FROM policy_bundles WHERE id = $1`, bundleID).Scan(&name); err != nil {
			t.Fatalf("read back bundle: %v", err)
		}
		if name != "guard-update-name" {
			t.Fatalf("name = %q, want unchanged %q", name, "guard-update-name")
		}
	})

	t.Run("DELETE on policy_bundles fails", func(t *testing.T) {
		bundleID := seedActivePolicyBundle(t, ctx, pool, tenantID, "guard-delete-name", "v1")

		if _, err := pool.Exec(ctx, `DELETE FROM policy_bundles WHERE id = $1`, bundleID); err == nil {
			t.Fatal("expected DELETE on policy_bundles to fail")
		}

		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM policy_bundles WHERE id = $1`, bundleID).Scan(&count); err != nil {
			t.Fatalf("count bundle rows: %v", err)
		}
		if count != 1 {
			t.Fatalf("count = %d, want 1 (row must remain after failed DELETE)", count)
		}
	})

	t.Run("draft to active status transition succeeds with no other column changed", func(t *testing.T) {
		var bundleID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO policy_bundles (tenant_id, name, version, status)
			VALUES ($1, 'guard-transition-name', 'v1', 'draft')
			RETURNING id
		`, tenantID).Scan(&bundleID); err != nil {
			t.Fatalf("seed draft bundle: %v", err)
		}

		if _, err := pool.Exec(ctx, `UPDATE policy_bundles SET status = 'active' WHERE id = $1`, bundleID); err != nil {
			t.Fatalf("expected draft->active transition to succeed: %v", err)
		}

		var status, name, version string
		if err := pool.QueryRow(ctx, `SELECT status, name, version FROM policy_bundles WHERE id = $1`, bundleID).Scan(&status, &name, &version); err != nil {
			t.Fatalf("read back bundle: %v", err)
		}
		if status != "active" || name != "guard-transition-name" || version != "v1" {
			t.Fatalf("(status, name, version) = (%s, %s, %s), want (active, guard-transition-name, v1)", status, name, version)
		}
	})

	t.Run("active to superseded status transition succeeds", func(t *testing.T) {
		bundleID := seedActivePolicyBundle(t, ctx, pool, tenantID, "guard-supersede-name", "v1")

		if _, err := pool.Exec(ctx, `UPDATE policy_bundles SET status = 'superseded' WHERE id = $1`, bundleID); err != nil {
			t.Fatalf("expected active->superseded transition to succeed: %v", err)
		}
	})

	t.Run("active to draft status transition fails", func(t *testing.T) {
		bundleID := seedActivePolicyBundle(t, ctx, pool, tenantID, "guard-illegal-name", "v1")

		if _, err := pool.Exec(ctx, `UPDATE policy_bundles SET status = 'draft' WHERE id = $1`, bundleID); err == nil {
			t.Fatal("expected active->draft transition to fail")
		}
	})

	t.Run("any UPDATE or DELETE on policy_bundle_rules fails", func(t *testing.T) {
		bundleID := seedActivePolicyBundle(t, ctx, pool, tenantID, "guard-rules-name", "v1")
		ruleID := seedPolicyRule(t, ctx, pool, "guard-rules-code-"+bundleID[:8])
		seedPolicyBundleRule(t, ctx, pool, tenantID, bundleID, ruleID)

		if _, err := pool.Exec(ctx, `
			UPDATE policy_bundle_rules SET legal_basis = 'changed' WHERE policy_bundle_id = $1
		`, bundleID); err == nil {
			t.Fatal("expected UPDATE on policy_bundle_rules to fail")
		}
		if _, err := pool.Exec(ctx, `
			DELETE FROM policy_bundle_rules WHERE policy_bundle_id = $1
		`, bundleID); err == nil {
			t.Fatal("expected DELETE on policy_bundle_rules to fail")
		}

		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM policy_bundle_rules WHERE policy_bundle_id = $1`, bundleID).Scan(&count); err != nil {
			t.Fatalf("count rule rows: %v", err)
		}
		if count != 1 {
			t.Fatalf("count = %d, want 1 (row must remain after failed UPDATE/DELETE)", count)
		}
	})
}

// TestPolicyBundlesGuardBlocksTruncate covers the TRUNCATE half of *Policy
// Bundles and Rule Snapshots Are Append-Only* [integration]: TRUNCATE on
// either table fails and rows remain intact.
func TestPolicyBundlesGuardBlocksTruncate(t *testing.T) {
	ctx, cancel, pool := requireDB(t)
	defer cancel()
	defer pool.Close()

	tenantID, _ := seedTenantAndDebtor(t, ctx, pool, "pb-truncate")
	bundleID := seedActivePolicyBundle(t, ctx, pool, tenantID, "truncate-name", "v1")
	ruleID := seedPolicyRule(t, ctx, pool, "truncate-code-"+bundleID[:8])
	seedPolicyBundleRule(t, ctx, pool, tenantID, bundleID, ruleID)

	if _, err := pool.Exec(ctx, `TRUNCATE policy_bundles`); err == nil {
		t.Fatal("expected TRUNCATE policy_bundles to fail")
	}
	if _, err := pool.Exec(ctx, `TRUNCATE policy_bundle_rules`); err == nil {
		t.Fatal("expected TRUNCATE policy_bundle_rules to fail")
	}

	var bundleCount, ruleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM policy_bundles WHERE id = $1`, bundleID).Scan(&bundleCount); err != nil {
		t.Fatalf("count bundle rows: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM policy_bundle_rules WHERE policy_bundle_id = $1`, bundleID).Scan(&ruleCount); err != nil {
		t.Fatalf("count rule rows: %v", err)
	}
	if bundleCount != 1 || ruleCount != 1 {
		t.Fatalf("(bundleCount, ruleCount) = (%d, %d), want (1, 1) after failed TRUNCATE", bundleCount, ruleCount)
	}
}

// TestCreateBundleVersionSupersedesAndAppends covers *At Most One Active
// Bundle Per Tenant and Name* and *Policy Bundles and Rule Snapshots Are
// Append-Only* [integration]: calling CreateBundleVersion twice for the
// same (tenant, name) produces two bundle rows, the prior marked
// superseded, both rule-snapshot sets intact, and effective_date/legal_basis
// non-null on the new rows.
func TestCreateBundleVersionSupersedesAndAppends(t *testing.T) {
	ctx, cancel, pool := requireDB(t)
	defer cancel()
	defer pool.Close()

	tenantID, _ := seedTenantAndDebtor(t, ctx, pool, "pb-createversion")
	ruleID1 := seedPolicyRule(t, ctx, pool, "cbv-rule-1-"+tenantID[:8])
	ruleID2 := seedPolicyRule(t, ctx, pool, "cbv-rule-2-"+tenantID[:8])

	store := postgres.NewPolicyBundleStoreFromPool(pool)

	first, err := store.CreateBundleVersion(ctx, tenantID, "retention-policy", []postgres.BundleRuleInput{
		{PolicyRuleID: ruleID1, EffectiveDate: time.Now().UTC(), LegalBasis: "art-1"},
	})
	if err != nil {
		t.Fatalf("CreateBundleVersion (first): %v", err)
	}
	if first.Version != "v1" || first.Status != "active" {
		t.Fatalf("first bundle = (version=%s, status=%s), want (v1, active)", first.Version, first.Status)
	}

	second, err := store.CreateBundleVersion(ctx, tenantID, "retention-policy", []postgres.BundleRuleInput{
		{PolicyRuleID: ruleID2, EffectiveDate: time.Now().UTC(), LegalBasis: "art-2"},
	})
	if err != nil {
		t.Fatalf("CreateBundleVersion (second): %v", err)
	}
	if second.Version != "v2" || second.Status != "active" {
		t.Fatalf("second bundle = (version=%s, status=%s), want (v2, active)", second.Version, second.Status)
	}

	var firstStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM policy_bundles WHERE id = $1`, string(first.ID)).Scan(&firstStatus); err != nil {
		t.Fatalf("read back first bundle status: %v", err)
	}
	if firstStatus != "superseded" {
		t.Fatalf("first bundle status = %q, want superseded after second CreateBundleVersion", firstStatus)
	}

	var firstRuleCount, secondRuleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM policy_bundle_rules WHERE policy_bundle_id = $1`, string(first.ID)).Scan(&firstRuleCount); err != nil {
		t.Fatalf("count first bundle rules: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM policy_bundle_rules WHERE policy_bundle_id = $1`, string(second.ID)).Scan(&secondRuleCount); err != nil {
		t.Fatalf("count second bundle rules: %v", err)
	}
	if firstRuleCount != 1 || secondRuleCount != 1 {
		t.Fatalf("(firstRuleCount, secondRuleCount) = (%d, %d), want (1, 1); prior snapshot must remain intact", firstRuleCount, secondRuleCount)
	}

	var effectiveDateNonNull, legalBasisNonNull bool
	if err := pool.QueryRow(ctx, `
		SELECT effective_date IS NOT NULL, legal_basis IS NOT NULL
		FROM policy_bundle_rules WHERE policy_bundle_id = $1
	`, string(second.ID)).Scan(&effectiveDateNonNull, &legalBasisNonNull); err != nil {
		t.Fatalf("read second bundle rule snapshot: %v", err)
	}
	if !effectiveDateNonNull || !legalBasisNonNull {
		t.Fatalf("(effectiveDateNonNull, legalBasisNonNull) = (%t, %t), want (true, true)", effectiveDateNonNull, legalBasisNonNull)
	}

	var activeCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM policy_bundles WHERE tenant_id = $1 AND name = 'retention-policy' AND status = 'active'
	`, tenantID).Scan(&activeCount); err != nil {
		t.Fatalf("count active bundles: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active bundle count = %d, want exactly 1", activeCount)
	}
}

// TestCreateBundleVersionConcurrencySerializesToOneActive covers *At Most
// One Active Bundle Per Tenant and Name* [integration], matching the spec
// scenario's exact acceptance criteria: two goroutines calling
// CreateBundleVersion concurrently for the same (tenant, name) must leave
// exactly one resulting active row and no duplicate (tenant_id, name,
// version); the FOR UPDATE lock on the prior-active row prevents both
// calls from superseding the same row and racing to insert two actives,
// and the non-deferrable partial unique index is the schema-enforced
// safety net — the spec explicitly allows one of the two concurrent
// attempts to FAIL rather than silently corrupt the single-active
// invariant (it does not require both attempts to succeed).
func TestCreateBundleVersionConcurrencySerializesToOneActive(t *testing.T) {
	ctx, cancel, pool := requireDB(t)
	defer cancel()
	defer pool.Close()

	tenantID, _ := seedTenantAndDebtor(t, ctx, pool, "pb-concurrency")
	ruleID := seedPolicyRule(t, ctx, pool, "concurrency-rule-"+tenantID[:8])

	// Seed an initial active bundle so the FOR UPDATE lock has a row to
	// serialize on (mirrors the spec scenario: "a tenant has an active
	// bundle named retention-policy").
	seedActivePolicyBundle(t, ctx, pool, tenantID, "concurrent-policy", "v1")

	store := postgres.NewPolicyBundleStoreFromPool(pool)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := store.CreateBundleVersion(ctx, tenantID, "concurrent-policy", []postgres.BundleRuleInput{
				{PolicyRuleID: ruleID, EffectiveDate: time.Now().UTC(), LegalBasis: "concurrent-basis"},
			})
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	// Per spec: at least one of the two attempts MUST succeed (the tenant
	// still gets its new bundle version), and any attempt that would
	// violate the single-active constraint MUST fail loudly rather than
	// silently succeed — never both silently corrupting the invariant.
	successCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
		}
	}
	if successCount == 0 {
		t.Fatalf("both concurrent CreateBundleVersion calls failed: %v", errs)
	}

	var activeCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM policy_bundles WHERE tenant_id = $1 AND name = 'concurrent-policy' AND status = 'active'
	`, tenantID).Scan(&activeCount); err != nil {
		t.Fatalf("count active bundles: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active bundle count = %d, want exactly 1 after concurrent CreateBundleVersion calls", activeCount)
	}

	var distinctVersions int
	if err := pool.QueryRow(ctx, `
		SELECT count(DISTINCT version) FROM policy_bundles WHERE tenant_id = $1 AND name = 'concurrent-policy'
	`, tenantID).Scan(&distinctVersions); err != nil {
		t.Fatalf("count distinct versions: %v", err)
	}
	var totalRows int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM policy_bundles WHERE tenant_id = $1 AND name = 'concurrent-policy'
	`, tenantID).Scan(&totalRows); err != nil {
		t.Fatalf("count total bundle rows: %v", err)
	}
	if distinctVersions != totalRows {
		t.Fatalf("distinctVersions = %d, totalRows = %d, want equal (no duplicate version numbers)", distinctVersions, totalRows)
	}
}

// TestEvaluationStampingWithActiveBundleAndRLSIsolation covers *Evaluations
// Are Stamped With the Resolved Bundle Version* [integration]: evaluating an
// interaction for a tenant with an active bundle sets
// evaluations.policy_bundle_id and a non-empty version, and tenant A cannot
// resolve tenant B's bundle via BundleResolver.
func TestEvaluationStampingWithActiveBundleAndRLSIsolation(t *testing.T) {
	ctx, cancel, pool := requireDB(t)
	defer cancel()
	defer pool.Close()

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "pb-stamp-a")
	tenantB, _ := seedTenantAndDebtor(t, ctx, pool, "pb-stamp-b")

	bundleAID := seedActivePolicyBundle(t, ctx, pool, tenantA, "stamp-policy", "v1")

	interactionA := seedInteraction(t, ctx, pool, tenantA, debtorA, "stamp/tenant-a")

	resolver := postgres.NewBundleResolverAdapterFromPool(pool)
	evalStore := postgres.NewEvaluationStoreFromPool(pool)
	svc := evaluation.Service{
		Detectors: []evaluation.NamedDetector{
			{Code: "contact-hours", Detector: fakeBlockDetector{}},
		},
		Store:    evalStore,
		Resolver: resolver,
	}

	got, err := svc.EvaluateInteraction(ctx, evaluation.EvaluateInteractionInput{
		TenantID:           tenantA,
		InteractionEventID: interactionA,
		Interaction: detection.Interaction{
			OccurredAt:     time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC),
			DebtorTimezone: "UTC",
		},
	})
	if err != nil {
		t.Fatalf("EvaluateInteraction: %v", err)
	}
	if got.PolicyBundleVersion != "v1" {
		t.Fatalf("PolicyBundleVersion = %q, want %q", got.PolicyBundleVersion, "v1")
	}
	if got.PolicyBundleID == nil || string(*got.PolicyBundleID) != bundleAID {
		t.Fatalf("PolicyBundleID = %v, want pointer to %q", got.PolicyBundleID, bundleAID)
	}

	// Tenant B has no active bundle at all, so resolving under tenant B's
	// context must return found=false, never tenant A's bundle.
	version, id, found, err := resolver.ActiveBundle(ctx, tenantB)
	if err != nil {
		t.Fatalf("ActiveBundle(tenantB): %v", err)
	}
	if found {
		t.Fatalf("ActiveBundle(tenantB) found = true (version=%q, id=%q), want false (tenant B has no bundle, must never see tenant A's)", version, id)
	}
}
