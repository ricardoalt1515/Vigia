package labtools

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
)

var (
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRegex = regexp.MustCompile(`[\+]?[(]?[0-9]{3}[)]?[-\s\.]?[0-9]{3}[-\s\.]?[0-9]{4,6}`)
)

// Load reads all embedded fixture files, validates them, and returns populated stores.
// It fails closed: any validation error aborts load and returns a descriptive error.
// It uses no filesystem access beyond fixtureFS; no time.Now().
func Load() (CaseStore, RuleStore, error) {
	return loadFrom(fixtureFS)
}

// loadFrom is package-internal; it loads fixtures from any fs.FS.
// Production code delegates to it via Load(); tests inject testing/fstest.MapFS.
func loadFrom(fsys fs.FS) (CaseStore, RuleStore, error) {
	ruleStore, err := loadRulesFrom(fsys)
	if err != nil {
		return nil, nil, err
	}
	caseStore, err := loadCasesFrom(fsys, ruleStore)
	if err != nil {
		return nil, nil, err
	}
	if err := validateNoOrphanRules(caseStore, ruleStore); err != nil {
		return nil, nil, err
	}
	return caseStore, ruleStore, nil
}

func loadRulesFrom(fsys fs.FS) (RuleStore, error) {
	entries, err := fs.Glob(fsys, "fixtures/rules/*.json")
	if err != nil {
		return nil, fmt.Errorf("glob rules: %w", err)
	}
	store := make(RuleStore)
	for _, path := range entries {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("read rule file %s: %w", path, err)
		}
		var rule SyntheticRule
		if err := json.Unmarshal(data, &rule); err != nil {
			return nil, fmt.Errorf("parse rule file %s: %w", path, err)
		}
		if err := validateRule(rule); err != nil {
			return nil, fmt.Errorf("invalid rule in %s: %w", path, err)
		}
		store[rule.Code] = rule
	}
	return store, nil
}

func loadCasesFrom(fsys fs.FS, ruleStore RuleStore) (CaseStore, error) {
	entries, err := fs.Glob(fsys, "fixtures/cases/*.json")
	if err != nil {
		return nil, fmt.Errorf("glob cases: %w", err)
	}
	store := make(CaseStore)
	for _, path := range entries {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("read case file %s: %w", path, err)
		}
		var c SyntheticCase
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("parse case file %s: %w", path, err)
		}
		if err := validateCase(c, ruleStore); err != nil {
			return nil, fmt.Errorf("invalid case in %s: %w", path, err)
		}
		store[c.CaseID] = c
	}
	return store, nil
}

func validateRule(r SyntheticRule) error {
	if r.Code == "" {
		return fmt.Errorf("rule.code is empty")
	}
	if r.Title == "" {
		return fmt.Errorf("rule %q: title is empty", r.Code)
	}
	if r.Severity == "" {
		return fmt.Errorf("rule %q: severity is empty", r.Code)
	}
	return nil
}

func validateCase(c SyntheticCase, ruleStore RuleStore) error {
	if c.CaseID == "" {
		return fmt.Errorf("case_id is empty")
	}
	if c.TenantID == "" {
		return fmt.Errorf("case %q: tenant_id is empty", c.CaseID)
	}
	if c.Channel == "" {
		return fmt.Errorf("case %q: channel is empty", c.CaseID)
	}
	if c.OccurredAt == "" {
		return fmt.Errorf("case %q: occurred_at is empty", c.CaseID)
	}
	if c.DebtorTimezone == "" {
		return fmt.Errorf("case %q: debtor_timezone is empty", c.CaseID)
	}
	if len(c.Transcript) == 0 {
		return fmt.Errorf("case %q: transcript is empty", c.CaseID)
	}
	for i, u := range c.Transcript {
		if u.Speaker == "" {
			return fmt.Errorf("case %q: transcript[%d].speaker is empty", c.CaseID, i)
		}
		if u.Text == "" {
			return fmt.Errorf("case %q: transcript[%d].text is empty", c.CaseID, i)
		}
	}
	// No-PII shape validation on debtor.label
	if emailRegex.MatchString(c.Debtor.Label) {
		return fmt.Errorf("case %q: debtor.label matches email pattern (PII risk): %q", c.CaseID, c.Debtor.Label)
	}
	if phoneRegex.MatchString(c.Debtor.Label) {
		return fmt.Errorf("case %q: debtor.label matches phone pattern (PII risk): %q", c.CaseID, c.Debtor.Label)
	}
	// Rule-reference integrity: every applicable_rule_id must resolve
	for _, ruleCode := range c.ApplicableRuleIDs {
		if _, ok := ruleStore[ruleCode]; !ok {
			return fmt.Errorf("case %q: applicable_rule_id %q not found in rule store", c.CaseID, ruleCode)
		}
	}
	// Rule-reference integrity: every detector_result.rule_code must resolve
	for _, dr := range c.DetectorResults {
		if _, ok := ruleStore[dr.RuleCode]; !ok {
			return fmt.Errorf("case %q: detector_result rule_code %q not found in rule store", c.CaseID, dr.RuleCode)
		}
	}
	return nil
}

// validateNoOrphanRules ensures every loaded rule is referenced by at least one case.
func validateNoOrphanRules(caseStore CaseStore, ruleStore RuleStore) error {
	caseRuleCodes := make(map[string]bool)
	for _, c := range caseStore {
		for _, code := range c.ApplicableRuleIDs {
			caseRuleCodes[code] = true
		}
	}
	for code := range ruleStore {
		if !caseRuleCodes[code] {
			return fmt.Errorf("orphan rule %q: not referenced by any case's applicable_rule_ids", code)
		}
	}
	return nil
}
