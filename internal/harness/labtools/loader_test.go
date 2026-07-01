package labtools

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// validCaseJSON is a minimal but complete case fixture for test variants.
const validCaseJSON = `{
	"case_id": "TEST-CASE-001",
	"tenant_id": "SYN-TENANT-001",
	"debtor": {"label": "Debtor-Synthetic-001"},
	"collector": {"despacho_id": "DESPACHO-SYN-01", "label": "Collector-Synthetic-001"},
	"transcript": [
		{"speaker": "collector", "text": "This is a courtesy call."},
		{"speaker": "debtor", "text": "Please call back during hours."}
	],
	"channel": "voice",
	"occurred_at": "2024-03-15T23:30:00-06:00",
	"debtor_timezone": "America/Mexico_City",
	"detector_results": [
		{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"},
		{"rule_code": "MX-REDECO-05", "detector_kind": "llm_judge", "outcome": "hard_block", "hitl_required": true}
	],
	"applicable_rule_ids": ["MX-REDECO-04", "MX-REDECO-05"],
	"evidence_metadata": {"status": "pending", "record_id": null}
}`

const validRule04JSON = `{
	"code": "MX-REDECO-04",
	"title": "Contact Hours Restriction",
	"description": "Contact permitted only on business days 08:00-21:00 debtor timezone.",
	"severity": "hard_block"
}`

const validRule05JSON = `{
	"code": "MX-REDECO-05",
	"title": "Threatening Tone Prohibition",
	"description": "Threats and intimidation are prohibited; violations require HITL.",
	"severity": "hard_block"
}`

func validTestFS() fstest.MapFS {
	return fstest.MapFS{
		"fixtures/cases/case.json":   {Data: []byte(validCaseJSON)},
		"fixtures/rules/rule04.json": {Data: []byte(validRule04JSON)},
		"fixtures/rules/rule05.json": {Data: []byte(validRule05JSON)},
	}
}

func loadCaseJSONForTest(caseJSON string) error {
	_, _, err := loadFrom(fstest.MapFS{
		"fixtures/cases/case.json":   {Data: []byte(caseJSON)},
		"fixtures/rules/rule04.json": {Data: []byte(validRule04JSON)},
		"fixtures/rules/rule05.json": {Data: []byte(validRule05JSON)},
	})
	return err
}

func TestLoad_ValidEmbedded(t *testing.T) {
	cases, rules, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if _, ok := cases["CASE-SYN-001"]; !ok {
		t.Error("CaseStore missing CASE-SYN-001")
	}
	if _, ok := rules["MX-REDECO-04"]; !ok {
		t.Error("RuleStore missing MX-REDECO-04")
	}
	if _, ok := rules["MX-REDECO-05"]; !ok {
		t.Error("RuleStore missing MX-REDECO-05")
	}
}

func TestLoad_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name     string
		caseJSON string
		ruleJSON string
	}{
		{
			name:     "empty case_id",
			caseJSON: `{"case_id": "", "tenant_id": "SYN-T", "debtor": {"label": "Debtor-Synthetic-001"}, "collector": {"despacho_id": "D", "label": "L"}, "transcript": [{"speaker": "s", "text": "t"}], "channel": "voice", "occurred_at": "2024-01-01T00:00:00Z", "debtor_timezone": "America/Mexico_City", "detector_results": [{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"}], "applicable_rule_ids": ["MX-REDECO-04"], "evidence_metadata": {}}`,
			ruleJSON: validRule04JSON,
		},
		{
			name:     "empty tenant_id",
			caseJSON: `{"case_id": "C1", "tenant_id": "", "debtor": {"label": "Debtor-Synthetic-001"}, "collector": {"despacho_id": "D", "label": "L"}, "transcript": [{"speaker": "s", "text": "t"}], "channel": "voice", "occurred_at": "2024-01-01T00:00:00Z", "debtor_timezone": "America/Mexico_City", "detector_results": [{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"}], "applicable_rule_ids": ["MX-REDECO-04"], "evidence_metadata": {}}`,
			ruleJSON: validRule04JSON,
		},
		{
			name:     "empty transcript",
			caseJSON: `{"case_id": "C1", "tenant_id": "SYN-T", "debtor": {"label": "Debtor-Synthetic-001"}, "collector": {"despacho_id": "D", "label": "L"}, "transcript": [], "channel": "voice", "occurred_at": "2024-01-01T00:00:00Z", "debtor_timezone": "America/Mexico_City", "detector_results": [{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"}], "applicable_rule_ids": ["MX-REDECO-04"], "evidence_metadata": {}}`,
			ruleJSON: validRule04JSON,
		},
		{
			name:     "utterance with empty speaker",
			caseJSON: `{"case_id": "C1", "tenant_id": "SYN-T", "debtor": {"label": "Debtor-Synthetic-001"}, "collector": {"despacho_id": "D", "label": "L"}, "transcript": [{"speaker": "", "text": "hello"}], "channel": "voice", "occurred_at": "2024-01-01T00:00:00Z", "debtor_timezone": "America/Mexico_City", "detector_results": [{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"}], "applicable_rule_ids": ["MX-REDECO-04"], "evidence_metadata": {}}`,
			ruleJSON: validRule04JSON,
		},
		{
			name:     "empty channel",
			caseJSON: `{"case_id": "C1", "tenant_id": "SYN-T", "debtor": {"label": "Debtor-Synthetic-001"}, "collector": {"despacho_id": "D", "label": "L"}, "transcript": [{"speaker": "s", "text": "t"}], "channel": "", "occurred_at": "2024-01-01T00:00:00Z", "debtor_timezone": "America/Mexico_City", "detector_results": [{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"}], "applicable_rule_ids": ["MX-REDECO-04"], "evidence_metadata": {}}`,
			ruleJSON: validRule04JSON,
		},
		{
			name:     "rule missing code",
			caseJSON: validCaseJSON,
			ruleJSON: `{"code": "", "title": "T", "description": "D", "severity": "hard_block"}`,
		},
		{
			name:     "rule missing severity",
			caseJSON: validCaseJSON,
			ruleJSON: `{"code": "MX-REDECO-04", "title": "T", "description": "D", "severity": ""}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"fixtures/cases/case.json": {Data: []byte(tc.caseJSON)},
				"fixtures/rules/rule.json": {Data: []byte(tc.ruleJSON)},
			}
			_, _, err := loadFrom(fsys)
			if err == nil {
				t.Errorf("loadFrom() expected error for %q, got nil", tc.name)
			}
		})
	}
}

func TestLoad_DuplicateFixtureIdentifiers(t *testing.T) {
	t.Run("duplicate case_id", func(t *testing.T) {
		fsys := fstest.MapFS{
			"fixtures/cases/case-a.json": {Data: []byte(validCaseJSON)},
			"fixtures/cases/case-b.json": {Data: []byte(validCaseJSON)},
			"fixtures/rules/rule04.json": {Data: []byte(validRule04JSON)},
			"fixtures/rules/rule05.json": {Data: []byte(validRule05JSON)},
		}
		_, _, err := loadFrom(fsys)
		if err == nil || !strings.Contains(err.Error(), "duplicate case_id") {
			t.Fatalf("loadFrom() error = %v, want duplicate case_id", err)
		}
	})

	t.Run("duplicate rule code", func(t *testing.T) {
		fsys := fstest.MapFS{
			"fixtures/cases/case.json":   {Data: []byte(validCaseJSON)},
			"fixtures/rules/rule-a.json": {Data: []byte(validRule04JSON)},
			"fixtures/rules/rule-b.json": {Data: []byte(validRule04JSON)},
			"fixtures/rules/rule05.json": {Data: []byte(validRule05JSON)},
		}
		_, _, err := loadFrom(fsys)
		if err == nil || !strings.Contains(err.Error(), "duplicate rule code") {
			t.Fatalf("loadFrom() error = %v, want duplicate rule code", err)
		}
	})
}

func TestLoad_RejectsMalformedCaseShapes(t *testing.T) {
	tests := []struct {
		name       string
		caseJSON   string
		wantReason string
	}{
		{
			name:       "empty debtor label",
			caseJSON:   strings.Replace(validCaseJSON, `"label": "Debtor-Synthetic-001"`, `"label": ""`, 1),
			wantReason: "debtor.label is empty",
		},
		{
			name:       "empty collector label",
			caseJSON:   strings.Replace(validCaseJSON, `"label": "Collector-Synthetic-001"`, `"label": ""`, 1),
			wantReason: "collector.label is empty",
		},
		{
			name:       "invalid occurred_at",
			caseJSON:   strings.Replace(validCaseJSON, `"occurred_at": "2024-03-15T23:30:00-06:00"`, `"occurred_at": "2024-03-15 23:30:00"`, 1),
			wantReason: "occurred_at must be RFC3339",
		},
		{
			name:       "invalid debtor timezone",
			caseJSON:   strings.Replace(validCaseJSON, `"debtor_timezone": "America/Mexico_City"`, `"debtor_timezone": "Mexico City"`, 1),
			wantReason: "debtor_timezone must be valid IANA timezone",
		},
		{
			name:       "empty detector results",
			caseJSON:   strings.Replace(validCaseJSON, `"detector_results": [`, `"detector_results": [], "unused_detector_results": [`, 1),
			wantReason: "detector_results is empty",
		},
		{
			name:       "detector missing rule_code",
			caseJSON:   strings.Replace(validCaseJSON, `"rule_code": "MX-REDECO-04"`, `"rule_code": ""`, 1),
			wantReason: "detector_results[0].rule_code is empty",
		},
		{
			name:       "detector missing detector_kind",
			caseJSON:   strings.Replace(validCaseJSON, `"detector_kind": "deterministic"`, `"detector_kind": ""`, 1),
			wantReason: "detector_results[0].detector_kind is empty",
		},
		{
			name:       "detector missing outcome",
			caseJSON:   strings.Replace(validCaseJSON, `"outcome": "hard_block"`, `"outcome": ""`, 1),
			wantReason: "detector_results[0].outcome is empty",
		},
		{
			name:       "empty applicable_rule_ids",
			caseJSON:   strings.Replace(validCaseJSON, `"applicable_rule_ids": ["MX-REDECO-04", "MX-REDECO-05"]`, `"applicable_rule_ids": []`, 1),
			wantReason: "applicable_rule_ids is empty",
		},
		{
			name: "missing evidence metadata",
			caseJSON: strings.Replace(validCaseJSON, `,
	"evidence_metadata": {"status": "pending", "record_id": null}`, ``, 1),
			wantReason: "evidence_metadata is required",
		},
		{
			name:       "null evidence metadata",
			caseJSON:   strings.Replace(validCaseJSON, `"evidence_metadata": {"status": "pending", "record_id": null}`, `"evidence_metadata": null`, 1),
			wantReason: "evidence_metadata is required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := loadCaseJSONForTest(tc.caseJSON)
			if err == nil || !strings.Contains(err.Error(), tc.wantReason) {
				t.Fatalf("loadFrom() error = %v, want to contain %q", err, tc.wantReason)
			}
		})
	}
}

func TestLoad_RejectsPIIShapedCaseStrings(t *testing.T) {
	tests := []struct {
		name       string
		caseJSON   string
		wantReason string
	}{
		{
			name:       "transcript email",
			caseJSON:   strings.Replace(validCaseJSON, "Please call back during hours.", "Email me at debtor@example.com.", 1),
			wantReason: "transcript[1].text matches email pattern",
		},
		{
			name:       "collector phone",
			caseJSON:   strings.Replace(validCaseJSON, "Collector-Synthetic-001", "555-123-4567", 1),
			wantReason: "collector.label matches phone pattern",
		},
		{
			name:       "metadata RFC",
			caseJSON:   strings.Replace(validCaseJSON, `"record_id": null`, `"reviewer_rfc": "GODE561231GR8"`, 1),
			wantReason: "evidence_metadata.reviewer_rfc matches RFC pattern",
		},
		{
			name:       "metadata account number",
			caseJSON:   strings.Replace(validCaseJSON, `"record_id": null`, `"account": "123456789012"`, 1),
			wantReason: "evidence_metadata.account matches account-number pattern",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := loadCaseJSONForTest(tc.caseJSON)
			if err == nil || !strings.Contains(err.Error(), tc.wantReason) {
				t.Fatalf("loadFrom() error = %v, want to contain %q", err, tc.wantReason)
			}
		})
	}
}

func TestLoad_DanglingRuleReference(t *testing.T) {
	// Case references MX-REDECO-99 but it is not in the rule store.
	caseJSON := `{
		"case_id": "TEST-CASE-001",
		"tenant_id": "SYN-TENANT-001",
		"debtor": {"label": "Debtor-Synthetic-001"},
		"collector": {"despacho_id": "DESPACHO-SYN-01", "label": "Collector-Synthetic-001"},
		"transcript": [
			{"speaker": "collector", "text": "Call about balance."},
			{"speaker": "debtor", "text": "Call back later."}
		],
		"channel": "voice",
		"occurred_at": "2024-03-15T23:30:00-06:00",
		"debtor_timezone": "America/Mexico_City",
		"detector_results": [
			{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"}
		],
		"applicable_rule_ids": ["MX-REDECO-04", "MX-REDECO-99"],
		"evidence_metadata": {}
	}`
	fsys := fstest.MapFS{
		"fixtures/cases/case.json":   {Data: []byte(caseJSON)},
		"fixtures/rules/rule04.json": {Data: []byte(validRule04JSON)},
	}
	_, _, err := loadFrom(fsys)
	if err == nil {
		t.Error("loadFrom() expected error for dangling rule reference, got nil")
	}
}

func TestLoad_PIIShapedDebtorLabel(t *testing.T) {
	cases := []struct {
		name  string
		label string
	}{
		{"email address", "john.doe@example.com"},
		{"phone number", "555-123-4567"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caseJSON := `{
				"case_id": "TEST-CASE-001",
				"tenant_id": "SYN-TENANT-001",
				"debtor": {"label": "` + tc.label + `"},
				"collector": {"despacho_id": "DESPACHO-SYN-01", "label": "Collector-Synthetic-001"},
				"transcript": [
					{"speaker": "collector", "text": "Call about balance."},
					{"speaker": "debtor", "text": "Call back later."}
				],
				"channel": "voice",
				"occurred_at": "2024-03-15T23:30:00-06:00",
				"debtor_timezone": "America/Mexico_City",
				"detector_results": [
					{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"}
				],
				"applicable_rule_ids": ["MX-REDECO-04"],
				"evidence_metadata": {}
			}`
			fsys := fstest.MapFS{
				"fixtures/cases/case.json":   {Data: []byte(caseJSON)},
				"fixtures/rules/rule04.json": {Data: []byte(validRule04JSON)},
			}
			_, _, err := loadFrom(fsys)
			if err == nil {
				t.Errorf("loadFrom() expected error for PII-shaped label %q, got nil", tc.label)
			}
		})
	}
}

func TestLoad_RuleReferenceIntegrity(t *testing.T) {
	cases, rules, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	c := cases["CASE-SYN-001"]
	for _, ruleCode := range c.ApplicableRuleIDs {
		if r, ok := rules[ruleCode]; !ok || r.Code == "" {
			t.Errorf("applicable_rule_id %q does not resolve to a non-nil SyntheticRule", ruleCode)
		}
	}
}

func TestLoad_NoOrphanRules(t *testing.T) {
	// A rule fixture that is NOT referenced in any case should cause load to fail.
	caseJSON := `{
		"case_id": "TEST-CASE-001",
		"tenant_id": "SYN-TENANT-001",
		"debtor": {"label": "Debtor-Synthetic-001"},
		"collector": {"despacho_id": "DESPACHO-SYN-01", "label": "Collector-Synthetic-001"},
		"transcript": [
			{"speaker": "collector", "text": "Call about balance."},
			{"speaker": "debtor", "text": "Call back later."}
		],
		"channel": "voice",
		"occurred_at": "2024-03-15T23:30:00-06:00",
		"debtor_timezone": "America/Mexico_City",
		"detector_results": [
			{"rule_code": "MX-REDECO-04", "detector_kind": "deterministic", "outcome": "hard_block"}
		],
		"applicable_rule_ids": ["MX-REDECO-04"],
		"evidence_metadata": {}
	}`
	orphanRuleJSON := `{
		"code": "MX-REDECO-99",
		"title": "Orphan Rule",
		"description": "This rule is not referenced by any case.",
		"severity": "hard_block"
	}`
	fsys := fstest.MapFS{
		"fixtures/cases/case.json":        {Data: []byte(caseJSON)},
		"fixtures/rules/rule04.json":      {Data: []byte(validRule04JSON)},
		"fixtures/rules/rule-orphan.json": {Data: []byte(orphanRuleJSON)},
	}
	_, _, err := loadFrom(fsys)
	if err == nil {
		t.Error("loadFrom() expected error for orphan rule, got nil")
	}
}

func TestLoad_Determinism(t *testing.T) {
	cases1, rules1, err1 := Load()
	if err1 != nil {
		t.Fatalf("first Load() error: %v", err1)
	}
	cases2, rules2, err2 := Load()
	if err2 != nil {
		t.Fatalf("second Load() error: %v", err2)
	}
	if !reflect.DeepEqual(cases1, cases2) {
		t.Error("Load() is not deterministic: CaseStore results differ")
	}
	if !reflect.DeepEqual(rules1, rules2) {
		t.Error("Load() is not deterministic: RuleStore results differ")
	}
}

func TestLoad_NoExternalServicesRequired(t *testing.T) {
	// Structural test: Load() must succeed with only the embedded FS.
	// If it reaches this point without error, no external service was needed.
	_, _, err := Load()
	if err != nil {
		t.Fatalf("Load() requires no external services but returned error: %v", err)
	}
}

// TestLoad_DetectorResultsContent asserts the content of the CASE-SYN-001 detector_results
// entries match the expected detector_kind, outcome, and hitl_required for each rule code.
// Satisfies CRITICAL-1 (MX-REDECO-04) and CRITICAL-2 (MX-REDECO-05).
func TestLoad_DetectorResultsContent(t *testing.T) {
	cases, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	c, ok := cases["CASE-SYN-001"]
	if !ok {
		t.Fatal("CaseStore missing CASE-SYN-001")
	}

	type wantEntry struct {
		detectorKind string
		outcome      string
		hitlRequired bool
	}
	wants := map[string]wantEntry{
		"MX-REDECO-04": {detectorKind: "deterministic", outcome: "hard_block", hitlRequired: false},
		"MX-REDECO-05": {detectorKind: "llm_judge", outcome: "hard_block", hitlRequired: true},
	}

	// Index by rule_code for O(1) lookup; fail clearly if a rule_code is missing.
	found := make(map[string]DetectorResult, len(c.DetectorResults))
	for _, dr := range c.DetectorResults {
		found[dr.RuleCode] = dr
	}

	for ruleCode, want := range wants {
		t.Run(ruleCode, func(t *testing.T) {
			dr, present := found[ruleCode]
			if !present {
				t.Fatalf("DetectorResults missing entry for rule_code %q", ruleCode)
			}
			if dr.DetectorKind != want.detectorKind {
				t.Errorf("DetectorResult[%q].DetectorKind = %q, want %q", ruleCode, dr.DetectorKind, want.detectorKind)
			}
			if dr.Outcome != want.outcome {
				t.Errorf("DetectorResult[%q].Outcome = %q, want %q", ruleCode, dr.Outcome, want.outcome)
			}
			if dr.HITLRequired != want.hitlRequired {
				t.Errorf("DetectorResult[%q].HITLRequired = %v, want %v", ruleCode, dr.HITLRequired, want.hitlRequired)
			}
		})
	}
}

// TestLoad_OutOfHoursContact asserts that the CASE-SYN-001 occurred_at timestamp, when
// converted to the case's debtor_timezone, places the contact OUTSIDE the 08:00–21:00
// permitted window. This proves the fixture models an out-of-hours contact per the spec.
// Satisfies CRITICAL-3.
func TestLoad_OutOfHoursContact(t *testing.T) {
	cases, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	c, ok := cases["CASE-SYN-001"]
	if !ok {
		t.Fatal("CaseStore missing CASE-SYN-001")
	}

	ts, err := time.Parse(time.RFC3339, c.OccurredAt)
	if err != nil {
		t.Fatalf("Parse OccurredAt %q as RFC 3339: %v", c.OccurredAt, err)
	}

	loc, err := time.LoadLocation(c.DebtorTimezone)
	if err != nil {
		t.Fatalf("LoadLocation %q: %v", c.DebtorTimezone, err)
	}

	local := ts.In(loc)
	hour := local.Hour()
	// Permitted window: 08:00 <= hour < 21:00. Contact outside means hour < 8 or hour >= 21.
	if hour >= 8 && hour < 21 {
		t.Errorf(
			"OccurredAt %q converts to local hour %d in %s, which is INSIDE 08:00–21:00; "+
				"fixture must model an out-of-hours contact",
			c.OccurredAt, hour, c.DebtorTimezone,
		)
	}
}
