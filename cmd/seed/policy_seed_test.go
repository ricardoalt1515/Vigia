package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ricardoalt1515/vigia/internal/core"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// --- fake PolicyRuleUpserter ---

type fakePolicyRuleUpserter struct {
	upserted []vigiaDB.UpsertPolicyRuleParams
	nextID   byte
	err      error
}

func (f *fakePolicyRuleUpserter) UpsertPolicyRule(ctx context.Context, arg vigiaDB.UpsertPolicyRuleParams) (vigiaDB.PolicyRule, error) {
	f.upserted = append(f.upserted, arg)
	if f.err != nil {
		return vigiaDB.PolicyRule{}, f.err
	}
	f.nextID++
	return vigiaDB.PolicyRule{
		ID:          pgtype.UUID{Bytes: [16]byte{50, f.nextID}, Valid: true},
		Code:        arg.Code,
		Title:       arg.Title,
		Description: arg.Description,
		Severity:    arg.Severity,
	}, nil
}

// --- fake ActiveBundleChecker ---

type fakeActiveBundleChecker struct {
	hasActive bool
	err       error
}

func (f *fakeActiveBundleChecker) GetActiveBundleByTenant(ctx context.Context, tenantID pgtype.UUID) (vigiaDB.PolicyBundle, error) {
	if f.err != nil {
		return vigiaDB.PolicyBundle{}, f.err
	}
	if f.hasActive {
		return vigiaDB.PolicyBundle{ID: pgtype.UUID{Bytes: [16]byte{99}, Valid: true}, TenantID: tenantID, Status: "active"}, nil
	}
	return vigiaDB.PolicyBundle{}, pgx.ErrNoRows
}

// --- fake BundleVersionCreator ---

type fakeBundleVersionCreatorCall struct {
	tenantID string
	name     string
	rules    []postgres.BundleRuleInput
}

type fakeBundleVersionCreator struct {
	calls []fakeBundleVersionCreatorCall
	err   error
}

func (f *fakeBundleVersionCreator) CreateBundleVersion(ctx context.Context, tenantID, name string, rules []postgres.BundleRuleInput) (core.PolicyBundle, error) {
	f.calls = append(f.calls, fakeBundleVersionCreatorCall{tenantID: tenantID, name: name, rules: rules})
	if f.err != nil {
		return core.PolicyBundle{}, f.err
	}
	return core.PolicyBundle{ID: "bundle-fake", TenantID: core.ID(tenantID), Name: name, Version: "v1", Status: "active"}, nil
}

func TestSeedPolicyCatalogAndBundle(t *testing.T) {
	ctx := context.Background()
	tenantID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	tenantIDStr := uuidToString(tenantID)
	effectiveDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("no active bundle: seeds all seven catalog rows and creates one bundle", func(t *testing.T) {
		upserter := &fakePolicyRuleUpserter{}
		checker := &fakeActiveBundleChecker{hasActive: false}
		bundleCreator := &fakeBundleVersionCreator{}

		created, err := SeedPolicyCatalogAndBundle(ctx, upserter, checker, bundleCreator, tenantID, tenantIDStr, effectiveDate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !created {
			t.Error("created = false, want true (no prior active bundle)")
		}

		if len(upserter.upserted) != 7 {
			t.Fatalf("upserted rule count = %d, want 7", len(upserter.upserted))
		}
		severityByCode := map[string]string{}
		for _, arg := range upserter.upserted {
			severityByCode[arg.Code] = arg.Severity
			if arg.Title == "" || arg.Description == "" {
				t.Errorf("rule %s: title/description must not be empty", arg.Code)
			}
		}
		wantCodes := []string{"MX-REDECO-03", "MX-REDECO-04", "MX-REDECO-05", "MX-REDECO-06", "MX-REDECO-07", "MX-REDECO-10", "MX-REDECO-11"}
		for _, code := range wantCodes {
			if _, ok := severityByCode[code]; !ok {
				t.Errorf("missing catalog row for %s", code)
			}
		}
		if severityByCode["MX-REDECO-03"] != string(core.SeverityMedium) {
			t.Errorf("MX-REDECO-03 severity = %q, want %q", severityByCode["MX-REDECO-03"], core.SeverityMedium)
		}
		for _, code := range []string{"MX-REDECO-04", "MX-REDECO-05", "MX-REDECO-06", "MX-REDECO-07", "MX-REDECO-10", "MX-REDECO-11"} {
			if severityByCode[code] != string(core.SeverityHigh) {
				t.Errorf("%s severity = %q, want %q", code, severityByCode[code], core.SeverityHigh)
			}
		}

		if len(bundleCreator.calls) != 1 {
			t.Fatalf("CreateBundleVersion calls = %d, want 1", len(bundleCreator.calls))
		}
		call := bundleCreator.calls[0]
		if call.tenantID != tenantIDStr {
			t.Errorf("CreateBundleVersion tenantID = %q, want %q", call.tenantID, tenantIDStr)
		}
		if call.name != redecoBaselineBundleName {
			t.Errorf("CreateBundleVersion name = %q, want %q", call.name, redecoBaselineBundleName)
		}
		if len(call.rules) != 7 {
			t.Fatalf("CreateBundleVersion rules = %d, want 7", len(call.rules))
		}
		for _, r := range call.rules {
			if r.PolicyRuleID == "" {
				t.Error("bundle rule PolicyRuleID must not be empty")
			}
			if r.LegalBasis == "" {
				t.Error("bundle rule LegalBasis must not be empty")
			}
			if r.EffectiveDate.IsZero() {
				t.Error("bundle rule EffectiveDate must not be zero")
			}
		}
	})

	t.Run("active bundle already exists: rules still upserted, no new bundle created", func(t *testing.T) {
		upserter := &fakePolicyRuleUpserter{}
		checker := &fakeActiveBundleChecker{hasActive: true}
		bundleCreator := &fakeBundleVersionCreator{}

		created, err := SeedPolicyCatalogAndBundle(ctx, upserter, checker, bundleCreator, tenantID, tenantIDStr, effectiveDate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if created {
			t.Error("created = true, want false (active bundle already exists)")
		}
		if len(upserter.upserted) != 7 {
			t.Errorf("upserted rule count = %d, want 7 (catalog upsert always runs)", len(upserter.upserted))
		}
		if len(bundleCreator.calls) != 0 {
			t.Errorf("CreateBundleVersion calls = %d, want 0 (guarded by existing active bundle)", len(bundleCreator.calls))
		}
	})
}
