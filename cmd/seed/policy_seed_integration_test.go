package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// TestSeedPolicyCatalogAndBundleIntegration calls SeedPolicyCatalogAndBundle
// against a real Postgres instance and asserts the seven-rule catalog +
// single active bundle snapshot spec scenarios ("Seed creates catalog rows
// for all seven rules", "Seed produces one active bundle snapshotting all
// seven rules"), plus idempotency on re-run.
//
// Requires:
//   - DATABASE_URL env var pointing to a migrated Postgres instance
//   - Running in non-short mode
//
// Skip pattern mirrors devdata_integration_test.go.
func TestSeedPolicyCatalogAndBundleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for the policy seed integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	queries := vigiaDB.New(pool)
	bundleStore := postgres.NewPolicyBundleStoreFromPool(pool)

	// A uniquely-suffixed slug guarantees a brand-new tenant every run, so
	// the "no prior active bundle" branch is genuinely exercised on every
	// test execution — the shared dev Postgres instance persists tenants
	// across runs, so a fixed slug would make created1 depend on whatever
	// state a prior test run left behind.
	slug := fmt.Sprintf("policy-seed-integration-test-%d", time.Now().UnixNano())
	tenant, err := queries.CreateTenant(ctx, vigiaDB.CreateTenantParams{
		Slug:   slug,
		Name:   "Policy Seed Integration Test Tenant",
		Status: "active",
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tenantIDStr := uuidToString(tenant.ID)
	effectiveDate := time.Now().UTC()

	// First run — seeds the catalog and creates the active bundle.
	created1, err := SeedPolicyCatalogAndBundle(ctx, queries, queries, bundleStore, tenant.ID, tenantIDStr, effectiveDate)
	if err != nil {
		t.Fatalf("first SeedPolicyCatalogAndBundle call: %v", err)
	}

	wantCodes := map[string]string{
		"MX-REDECO-03": "medium",
		"MX-REDECO-04": "high",
		"MX-REDECO-05": "high",
		"MX-REDECO-06": "high",
		"MX-REDECO-07": "high",
		"MX-REDECO-10": "high",
		"MX-REDECO-11": "high",
	}

	// Assert the policy_rules catalog: exactly one row per rule code, each
	// with a non-null code/title/description/severity, MX-REDECO-03 at
	// medium severity distinct from the six others at high.
	rows, err := pool.Query(ctx, `
		SELECT code, title, description, severity FROM policy_rules WHERE code = ANY($1)
	`, codesOf(wantCodes))
	if err != nil {
		t.Fatalf("query policy_rules: %v", err)
	}
	seenCodes := map[string]bool{}
	for rows.Next() {
		var code, title, description, severity string
		if err := rows.Scan(&code, &title, &description, &severity); err != nil {
			rows.Close()
			t.Fatalf("scan policy_rules row: %v", err)
		}
		seenCodes[code] = true
		if title == "" || description == "" {
			t.Errorf("policy_rules row %s: title/description must not be empty", code)
		}
		if wantSeverity, ok := wantCodes[code]; ok && severity != wantSeverity {
			t.Errorf("policy_rules row %s severity = %q, want %q", code, severity, wantSeverity)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate policy_rules: %v", err)
	}
	rows.Close()
	for code := range wantCodes {
		if !seenCodes[code] {
			t.Errorf("policy_rules row for %s not found after seed", code)
		}
	}

	// Assert exactly one active bundle for this tenant, snapshotting all
	// seven rule codes with non-null LegalBasis and EffectiveDate.
	activeBundle, err := queries.GetActiveBundleByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("get active bundle: %v", err)
	}
	if activeBundle.Name != redecoBaselineBundleName {
		t.Errorf("active bundle name = %q, want %q", activeBundle.Name, redecoBaselineBundleName)
	}

	bundleRules, err := queries.ListPolicyBundleRulesByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("list policy bundle rules: %v", err)
	}
	if len(bundleRules) != 7 {
		t.Fatalf("policy_bundle_rules snapshot rows = %d, want 7", len(bundleRules))
	}
	snapshotCodes := map[string]bool{}
	for _, r := range bundleRules {
		snapshotCodes[r.Code] = true
		if r.LegalBasis == "" {
			t.Errorf("policy_bundle_rules row %s: LegalBasis must not be empty", r.Code)
		}
		if !r.EffectiveDate.Valid {
			t.Errorf("policy_bundle_rules row %s: EffectiveDate must not be null", r.Code)
		}
	}
	for code := range wantCodes {
		if !snapshotCodes[code] {
			t.Errorf("policy_bundle_rules snapshot missing rule code %s", code)
		}
	}

	// Second run — idempotent: catalog rows still exactly one per code
	// (no UNIQUE(code) violation), and no second bundle version is created
	// (guarded by the existing active bundle).
	created2, err := SeedPolicyCatalogAndBundle(ctx, queries, queries, bundleStore, tenant.ID, tenantIDStr, effectiveDate)
	if err != nil {
		t.Fatalf("second SeedPolicyCatalogAndBundle call: %v", err)
	}
	if !created1 {
		t.Error("first run: created = false, want true (no prior active bundle)")
	}
	if created2 {
		t.Error("second run: created = true, want false (idempotent — active bundle already exists)")
	}

	var ruleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM policy_rules WHERE code = ANY($1)`, codesOf(wantCodes)).Scan(&ruleCount); err != nil {
		t.Fatalf("count policy_rules: %v", err)
	}
	if ruleCount != 7 {
		t.Errorf("policy_rules count after re-run = %d, want 7 (idempotent upsert, no duplicates)", ruleCount)
	}

	activeBundleAfter, err := queries.GetActiveBundleByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("get active bundle after re-run: %v", err)
	}
	if activeBundleAfter.ID != activeBundle.ID {
		t.Errorf("active bundle id changed after re-run: %v -> %v, want unchanged (idempotent, no re-seed stacking)", activeBundle.ID, activeBundleAfter.ID)
	}
}

func codesOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
