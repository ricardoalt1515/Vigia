package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ricardoalt1515/vigia/internal/core"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/postgres"
)

// redecoBaselineBundleName is the single active policy bundle CreateBundleVersion
// seeds all seven REDECO rules into (issue #7 Design Decision "catalog + bundle
// seeding idempotency").
const redecoBaselineBundleName = "redeco-baseline"

// policyRuleCatalogEntry describes one seeded policy_rules row plus the
// policy_bundle_rules snapshot provenance (LegalBasis) it carries when
// bundled, per docs/regulatory-ruleset.md.
type policyRuleCatalogEntry struct {
	Code        string
	Title       string
	Description string
	Severity    string
	LegalBasis  string
}

// policyRuleCatalog returns the seven REDECO rules this change seeds: the two
// existing (MX-REDECO-04, MX-REDECO-05) plus the five new deterministic
// detectors (MX-REDECO-06, MX-REDECO-07, MX-REDECO-10, MX-REDECO-11,
// MX-REDECO-03). MX-REDECO-03's severity is medium (warn-level catalog
// action); the other six are high (hard-block/LLM-judge catalog actions).
func policyRuleCatalog() []policyRuleCatalogEntry {
	return []policyRuleCatalogEntry{
		{
			Code:        "MX-REDECO-03",
			Title:       "Disclosure of UNE/complaints-unit contact",
			Description: "Provide the UNE / complaints-unit contact (address/email/phone) and that a complaint can be filed in REDECO.",
			Severity:    string(core.SeverityMedium),
			LegalBasis:  "REDECO cause catalog",
		},
		{
			Code:        "MX-REDECO-04",
			Title:       "Authorized contact hours",
			Description: "Contact only on business days, 08:00-21:00 in the debtor's timezone.",
			Severity:    string(core.SeverityHigh),
			LegalBasis:  "REDECO / current disposition",
		},
		{
			Code:        "MX-REDECO-05",
			Title:       "Prohibited tone",
			Description: "Prohibit threats, offense, intimidation, harassment.",
			Severity:    string(core.SeverityHigh),
			LegalBasis:  "REDECO causes; CONDUSEF material",
		},
		{
			Code:        "MX-REDECO-06",
			Title:       "Third-party contact",
			Description: "Prohibit collection management with persons who are not the user/debtor/co-debtor/aval/obligado solidario.",
			Severity:    string(core.SeverityHigh),
			LegalBasis:  "REDECO causes / disposition",
		},
		{
			Code:        "MX-REDECO-07",
			Title:       "Protected population",
			Description: "Prohibit management with minors or elderly persons, unless the elderly person is the debtor.",
			Severity:    string(core.SeverityHigh),
			LegalBasis:  "Disposition / REDECO causes",
		},
		{
			Code:        "MX-REDECO-10",
			Title:       "Payment routing",
			Description: "The despacho may not directly receive payment; payments/agreements must be made/received by the financial entity.",
			Severity:    string(core.SeverityHigh),
			LegalBasis:  "Arts. 121, 126, 132",
		},
		{
			Code:        "MX-REDECO-11",
			Title:       "Authorized channel",
			Description: "Prohibit contact at address/phone/email other than the one provided by the entity or debtor/aval.",
			Severity:    string(core.SeverityHigh),
			LegalBasis:  "Disposition",
		},
	}
}

// PolicyRuleUpserter is the minimal port SeedPolicyCatalogAndBundle needs to
// idempotently seed the policy_rules catalog.
type PolicyRuleUpserter interface {
	UpsertPolicyRule(ctx context.Context, arg vigiaDB.UpsertPolicyRuleParams) (vigiaDB.PolicyRule, error)
}

// ActiveBundleChecker resolves whether a tenant already has an active policy
// bundle, guarding CreateBundleVersion from stacking v2, v3... on every seed
// re-run.
type ActiveBundleChecker interface {
	GetActiveBundleByTenant(ctx context.Context, tenantID pgtype.UUID) (vigiaDB.PolicyBundle, error)
}

// BundleVersionCreator creates a new active policy bundle version with its
// rule-snapshot rows. Satisfied by *postgres.PolicyBundleStore in production.
type BundleVersionCreator interface {
	CreateBundleVersion(ctx context.Context, tenantID, name string, rules []postgres.BundleRuleInput) (core.PolicyBundle, error)
}

// SeedPolicyCatalogAndBundle idempotently seeds the policy_rules catalog (all
// seven REDECO rules, always upserted so title/description/severity stay
// current) and, only when the tenant has no active policy bundle yet,
// snapshots all seven rules into one new active "redeco-baseline" bundle
// version. Returns created=true when a new bundle version was created.
func SeedPolicyCatalogAndBundle(
	ctx context.Context,
	upserter PolicyRuleUpserter,
	checker ActiveBundleChecker,
	bundleCreator BundleVersionCreator,
	tenantID pgtype.UUID,
	tenantIDStr string,
	effectiveDate time.Time,
) (bool, error) {
	catalog := policyRuleCatalog()
	ruleIDByCode := make(map[string]string, len(catalog))
	for _, entry := range catalog {
		row, err := upserter.UpsertPolicyRule(ctx, vigiaDB.UpsertPolicyRuleParams{
			Code:        entry.Code,
			Title:       entry.Title,
			Description: entry.Description,
			Severity:    entry.Severity,
		})
		if err != nil {
			return false, fmt.Errorf("upsert policy rule %s: %w", entry.Code, err)
		}
		ruleIDByCode[entry.Code] = uuidToString(row.ID)
	}

	if _, err := checker.GetActiveBundleByTenant(ctx, tenantID); err == nil {
		// An active bundle already exists for this tenant: the catalog rows
		// above are refreshed, but the bundle snapshot is left untouched.
		return false, nil
	} else if !isNotFound(err) {
		return false, fmt.Errorf("get active bundle for tenant: %w", err)
	}

	rules := make([]postgres.BundleRuleInput, 0, len(catalog))
	for _, entry := range catalog {
		rules = append(rules, postgres.BundleRuleInput{
			PolicyRuleID:  ruleIDByCode[entry.Code],
			EffectiveDate: effectiveDate,
			LegalBasis:    entry.LegalBasis,
		})
	}

	if _, err := bundleCreator.CreateBundleVersion(ctx, tenantIDStr, redecoBaselineBundleName, rules); err != nil {
		return false, fmt.Errorf("create bundle version: %w", err)
	}
	return true, nil
}
