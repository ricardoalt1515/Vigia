package labtools

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"
)

var (
	emailRegex   = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRegex   = regexp.MustCompile(`[\+]?[(]?[0-9]{3}[)]?[-\s\.]?[0-9]{3}[-\s\.]?[0-9]{4,6}`)
	curpRegex    = regexp.MustCompile(`\b[A-Z][AEIOUX][A-Z]{2}\d{6}[HM][A-Z]{5}[A-Z0-9]\d\b`)
	rfcRegex     = regexp.MustCompile(`\b[A-Z&Ñ]{3,4}\d{6}[A-Z0-9]{3}\b`)
	accountRegex = regexp.MustCompile(`\b\d{12,20}\b`)
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
	if len(entries) == 0 {
		return nil, fmt.Errorf("no rule fixtures found")
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
		if _, exists := store[rule.Code]; exists {
			return nil, fmt.Errorf("invalid rule in %s: duplicate rule code %q", path, rule.Code)
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
	if len(entries) == 0 {
		return nil, fmt.Errorf("no case fixtures found")
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
		if _, exists := store[c.CaseID]; exists {
			return nil, fmt.Errorf("invalid case in %s: duplicate case_id %q", path, c.CaseID)
		}
		store[c.CaseID] = c
	}
	return store, nil
}

func validateRule(r SyntheticRule) error {
	if blank(r.Code) {
		return fmt.Errorf("rule.code is empty")
	}
	if blank(r.Title) {
		return fmt.Errorf("rule %q: title is empty", r.Code)
	}
	if blank(r.Severity) {
		return fmt.Errorf("rule %q: severity is empty", r.Code)
	}
	return nil
}

func validateCase(c SyntheticCase, ruleStore RuleStore) error {
	if blank(c.CaseID) {
		return fmt.Errorf("case_id is empty")
	}
	if blank(c.TenantID) {
		return fmt.Errorf("case %q: tenant_id is empty", c.CaseID)
	}
	if blank(c.Debtor.Label) {
		return fmt.Errorf("case %q: debtor.label is empty", c.CaseID)
	}
	if blank(c.Collector.DespachoID) {
		return fmt.Errorf("case %q: collector.despacho_id is empty", c.CaseID)
	}
	if blank(c.Collector.Label) {
		return fmt.Errorf("case %q: collector.label is empty", c.CaseID)
	}
	if blank(c.Channel) {
		return fmt.Errorf("case %q: channel is empty", c.CaseID)
	}
	if blank(c.OccurredAt) {
		return fmt.Errorf("case %q: occurred_at is empty", c.CaseID)
	}
	if _, err := time.Parse(time.RFC3339, c.OccurredAt); err != nil {
		return fmt.Errorf("case %q: occurred_at must be RFC3339: %w", c.CaseID, err)
	}
	if blank(c.DebtorTimezone) {
		return fmt.Errorf("case %q: debtor_timezone is empty", c.CaseID)
	}
	if _, err := time.LoadLocation(c.DebtorTimezone); err != nil {
		return fmt.Errorf("case %q: debtor_timezone must be valid IANA timezone: %w", c.CaseID, err)
	}
	if len(c.Transcript) == 0 {
		return fmt.Errorf("case %q: transcript is empty", c.CaseID)
	}
	for i, u := range c.Transcript {
		if blank(u.Speaker) {
			return fmt.Errorf("case %q: transcript[%d].speaker is empty", c.CaseID, i)
		}
		if blank(u.Text) {
			return fmt.Errorf("case %q: transcript[%d].text is empty", c.CaseID, i)
		}
	}
	if len(c.DetectorResults) == 0 {
		return fmt.Errorf("case %q: detector_results is empty", c.CaseID)
	}
	for i, dr := range c.DetectorResults {
		if blank(dr.RuleCode) {
			return fmt.Errorf("case %q: detector_results[%d].rule_code is empty", c.CaseID, i)
		}
		if blank(dr.DetectorKind) {
			return fmt.Errorf("case %q: detector_results[%d].detector_kind is empty", c.CaseID, i)
		}
		if blank(dr.Outcome) {
			return fmt.Errorf("case %q: detector_results[%d].outcome is empty", c.CaseID, i)
		}
	}
	if len(c.ApplicableRuleIDs) == 0 {
		return fmt.Errorf("case %q: applicable_rule_ids is empty", c.CaseID)
	}
	for i, ruleCode := range c.ApplicableRuleIDs {
		if blank(ruleCode) {
			return fmt.Errorf("case %q: applicable_rule_ids[%d] is empty", c.CaseID, i)
		}
		if _, ok := ruleStore[ruleCode]; !ok {
			return fmt.Errorf("case %q: applicable_rule_id %q not found in rule store", c.CaseID, ruleCode)
		}
	}
	for _, dr := range c.DetectorResults {
		if _, ok := ruleStore[dr.RuleCode]; !ok {
			return fmt.Errorf("case %q: detector_result rule_code %q not found in rule store", c.CaseID, dr.RuleCode)
		}
	}
	if c.EvidenceMetadata == nil {
		return fmt.Errorf("case %q: evidence_metadata is required and must be an object", c.CaseID)
	}
	if err := validateNoPIIStrings(c); err != nil {
		return fmt.Errorf("case %q: %w", c.CaseID, err)
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

func blank(s string) bool {
	return strings.TrimSpace(s) == ""
}

func validateNoPIIStrings(c SyntheticCase) error {
	checks := []struct {
		path  string
		value string
	}{
		{"case_id", c.CaseID},
		{"tenant_id", c.TenantID},
		{"debtor.label", c.Debtor.Label},
		{"collector.despacho_id", c.Collector.DespachoID},
		{"collector.label", c.Collector.Label},
		{"channel", c.Channel},
		{"occurred_at", c.OccurredAt},
		{"debtor_timezone", c.DebtorTimezone},
	}
	for i, u := range c.Transcript {
		checks = append(checks,
			struct {
				path  string
				value string
			}{fmt.Sprintf("transcript[%d].speaker", i), u.Speaker},
			struct {
				path  string
				value string
			}{fmt.Sprintf("transcript[%d].text", i), u.Text},
		)
	}
	for i, dr := range c.DetectorResults {
		checks = append(checks,
			struct {
				path  string
				value string
			}{fmt.Sprintf("detector_results[%d].rule_code", i), dr.RuleCode},
			struct {
				path  string
				value string
			}{fmt.Sprintf("detector_results[%d].detector_kind", i), dr.DetectorKind},
			struct {
				path  string
				value string
			}{fmt.Sprintf("detector_results[%d].outcome", i), dr.Outcome},
		)
	}
	for i, ruleID := range c.ApplicableRuleIDs {
		checks = append(checks, struct {
			path  string
			value string
		}{fmt.Sprintf("applicable_rule_ids[%d]", i), ruleID})
	}
	for _, check := range checks {
		if err := validateNoPIIString(check.path, check.value); err != nil {
			return err
		}
	}
	return validateNoPIIValue("evidence_metadata", c.EvidenceMetadata)
}

func validateNoPIIValue(path string, value any) error {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return validateNoPIIString(path, v)
	case []any:
		for i, item := range v {
			if err := validateNoPIIValue(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range v {
			keyPath := fmt.Sprintf("%s.%s", path, key)
			if err := validateNoPIIString(keyPath+" key", key); err != nil {
				return err
			}
			if err := validateNoPIIValue(keyPath, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNoPIIString(path, value string) error {
	upper := strings.ToUpper(value)
	switch {
	case emailRegex.MatchString(value):
		return fmt.Errorf("%s matches email pattern (PII risk): %q", path, value)
	case accountRegex.MatchString(value):
		return fmt.Errorf("%s matches account-number pattern (PII risk): %q", path, value)
	case phoneRegex.MatchString(value):
		return fmt.Errorf("%s matches phone pattern (PII risk): %q", path, value)
	case curpRegex.MatchString(upper):
		return fmt.Errorf("%s matches CURP pattern (PII risk): %q", path, value)
	case rfcRegex.MatchString(upper):
		return fmt.Errorf("%s matches RFC pattern (PII risk): %q", path, value)
	default:
		return nil
	}
}
