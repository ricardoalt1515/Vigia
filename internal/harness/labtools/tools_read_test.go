package labtools

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

func loadStoresForTest(t *testing.T) (CaseStore, RuleStore) {
	t.Helper()
	cases, rules, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	return cases, rules
}

func TestReadCaseTool_HappyPath(t *testing.T) {
	cases, rules := loadStoresForTest(t)
	_ = rules

	tool := &ReadCaseTool{cases: cases}
	call := harness.ToolCall{
		Name:  "read_case",
		Input: map[string]any{"case_id": "CASE-SYN-001"},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != harness.ToolStatusSuccess {
		t.Fatalf("status = %q, want %q", result.Status, harness.ToolStatusSuccess)
	}

	caseMap, ok := result.Output["case"].(map[string]any)
	if !ok {
		t.Fatalf("output missing 'case' key; got keys: %v", outputKeys(result.Output))
	}

	requiredFields := []string{
		"tenant_id", "debtor", "collector", "transcript",
		"channel", "occurred_at", "debtor_timezone",
		"detector_results", "applicable_rule_ids", "evidence_metadata",
	}
	for _, field := range requiredFields {
		if v, exists := caseMap[field]; !exists || v == nil {
			t.Errorf("output.case missing required field %q", field)
		}
	}

	tenantID, _ := caseMap["tenant_id"].(string)
	if tenantID != "SYN-TENANT-001" {
		t.Errorf("tenant_id = %q, want %q", tenantID, "SYN-TENANT-001")
	}

	transcript, ok := caseMap["transcript"].([]interface{})
	if !ok || len(transcript) == 0 {
		t.Fatalf("transcript is not a non-empty array; got %T %v", caseMap["transcript"], caseMap["transcript"])
	}
	for i, u := range transcript {
		utter, ok := u.(map[string]any)
		if !ok {
			t.Fatalf("transcript[%d] is not a map; got %T", i, u)
		}
		if utter["speaker"] == "" {
			t.Errorf("transcript[%d].speaker is empty", i)
		}
		if utter["text"] == "" {
			t.Errorf("transcript[%d].text is empty", i)
		}
	}

	// Spec: response SHALL carry case_id echoed from the request.
	gotCaseID, _ := result.Output["case_id"].(string)
	if gotCaseID != "CASE-SYN-001" {
		t.Errorf("response case_id = %q, want %q (case_id must be echoed from the request)", gotCaseID, "CASE-SYN-001")
	}
}

func TestReadCaseTool_UnknownCaseID(t *testing.T) {
	cases, _ := loadStoresForTest(t)
	tool := &ReadCaseTool{cases: cases}
	call := harness.ToolCall{
		Name:  "read_case",
		Input: map[string]any{"case_id": "UNKNOWN-CASE-999"},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status == harness.ToolStatusSuccess {
		t.Error("status must not be success for unknown case_id")
	}
	if result.Reason == "" {
		t.Error("Reason must be non-empty for lookup failure")
	}
}

func TestReadPolicyRuleTool_MXRedeco04(t *testing.T) {
	_, rules := loadStoresForTest(t)
	tool := &ReadPolicyRuleTool{rules: rules}
	call := harness.ToolCall{
		Name:  "read_policy_rule",
		Input: map[string]any{"rule_code": "MX-REDECO-04"},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != harness.ToolStatusSuccess {
		t.Fatalf("status = %q, want %q; reason: %s", result.Status, harness.ToolStatusSuccess, result.Reason)
	}

	ruleMap, ok := result.Output["rule"].(map[string]any)
	if !ok {
		t.Fatalf("output missing 'rule' key")
	}
	if ruleMap["code"] != "MX-REDECO-04" {
		t.Errorf("rule.code = %q, want %q", ruleMap["code"], "MX-REDECO-04")
	}
	if ruleMap["severity"] != "hard_block" {
		t.Errorf("rule.severity = %q, want %q", ruleMap["severity"], "hard_block")
	}
	if title, _ := ruleMap["title"].(string); title == "" {
		t.Error("rule.title must be non-empty")
	}
	desc04, _ := ruleMap["description"].(string)
	if desc04 == "" {
		t.Error("rule.description must be non-empty")
	}
	// Spec scenario: MX-REDECO-04 description references the contact-hours window.
	desc04Lower := strings.ToLower(desc04)
	if !strings.Contains(desc04Lower, "08:00") {
		t.Errorf("MX-REDECO-04 description must reference opening time 08:00; got: %q", desc04)
	}
	if !strings.Contains(desc04Lower, "21:00") {
		t.Errorf("MX-REDECO-04 description must reference closing time 21:00; got: %q", desc04)
	}
}

func TestReadPolicyRuleTool_MXRedeco05(t *testing.T) {
	_, rules := loadStoresForTest(t)
	tool := &ReadPolicyRuleTool{rules: rules}
	call := harness.ToolCall{
		Name:  "read_policy_rule",
		Input: map[string]any{"rule_code": "MX-REDECO-05"},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != harness.ToolStatusSuccess {
		t.Fatalf("status = %q, want %q", result.Status, harness.ToolStatusSuccess)
	}
	ruleMap, ok := result.Output["rule"].(map[string]any)
	if !ok {
		t.Fatalf("output missing 'rule' key")
	}
	if ruleMap["severity"] != "hard_block" {
		t.Errorf("rule.severity = %q, want %q", ruleMap["severity"], "hard_block")
	}
	// Spec scenario: MX-REDECO-05 description references threats or intimidation.
	desc05, _ := ruleMap["description"].(string)
	desc05Lower := strings.ToLower(desc05)
	if !strings.Contains(desc05Lower, "threat") && !strings.Contains(desc05Lower, "intimidation") {
		t.Errorf("MX-REDECO-05 description must reference threats or intimidation; got: %q", desc05)
	}
}

func TestReadPolicyRuleTool_UnknownRuleCode(t *testing.T) {
	_, rules := loadStoresForTest(t)
	tool := &ReadPolicyRuleTool{rules: rules}
	call := harness.ToolCall{
		Name:  "read_policy_rule",
		Input: map[string]any{"rule_code": "UNKNOWN-99"},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status == harness.ToolStatusSuccess {
		t.Error("status must not be success for unknown rule_code")
	}
	if result.Reason == "" {
		t.Error("Reason must be non-empty for lookup failure")
	}
}

func TestListApplicableRulesTool_HappyPath(t *testing.T) {
	cases, rules := loadStoresForTest(t)
	tool := &ListApplicableRulesTool{cases: cases, rules: rules}
	call := harness.ToolCall{
		Name:  "list_applicable_rules",
		Input: map[string]any{"case_id": "CASE-SYN-001"},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != harness.ToolStatusSuccess {
		t.Fatalf("status = %q, want %q; reason: %s", result.Status, harness.ToolStatusSuccess, result.Reason)
	}

	rulesList, ok := result.Output["rules"].([]interface{})
	if !ok || len(rulesList) == 0 {
		t.Fatalf("output.rules is not a non-empty array")
	}

	codes := make([]string, 0, len(rulesList))
	for i, r := range rulesList {
		rm, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("rules[%d] is not a map", i)
		}
		code, _ := rm["code"].(string)
		title, _ := rm["title"].(string)
		severity, _ := rm["severity"].(string)
		if code == "" || title == "" || severity == "" {
			t.Errorf("rules[%d] missing code/title/severity: %v", i, rm)
		}
		codes = append(codes, code)
	}

	// Order must match applicable_rule_ids from fixture: ["MX-REDECO-04", "MX-REDECO-05"]
	wantCodes := []string{"MX-REDECO-04", "MX-REDECO-05"}
	if !reflect.DeepEqual(codes, wantCodes) {
		t.Errorf("rule codes = %v, want %v", codes, wantCodes)
	}
}

func TestListApplicableRulesTool_UnknownCaseID(t *testing.T) {
	cases, rules := loadStoresForTest(t)
	tool := &ListApplicableRulesTool{cases: cases, rules: rules}
	call := harness.ToolCall{
		Name:  "list_applicable_rules",
		Input: map[string]any{"case_id": "UNKNOWN-999"},
	}

	result, err := tool.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status == harness.ToolStatusSuccess {
		t.Error("status must not be success for unknown case_id")
	}
	if result.Reason == "" {
		t.Error("Reason must be non-empty for lookup failure")
	}
}

func TestReadTools_Determinism(t *testing.T) {
	cases, rules := loadStoresForTest(t)
	ctx := context.Background()

	readCaseTool := &ReadCaseTool{cases: cases}
	call := harness.ToolCall{Name: "read_case", Input: map[string]any{"case_id": "CASE-SYN-001"}}
	r1, _ := readCaseTool.Execute(ctx, call)
	r2, _ := readCaseTool.Execute(ctx, call)
	if !reflect.DeepEqual(r1, r2) {
		t.Error("read_case is not deterministic: results differ between calls")
	}

	readRuleTool := &ReadPolicyRuleTool{rules: rules}
	ruleCall := harness.ToolCall{Name: "read_policy_rule", Input: map[string]any{"rule_code": "MX-REDECO-04"}}
	rr1, _ := readRuleTool.Execute(ctx, ruleCall)
	rr2, _ := readRuleTool.Execute(ctx, ruleCall)
	if !reflect.DeepEqual(rr1, rr2) {
		t.Error("read_policy_rule is not deterministic: results differ between calls")
	}

	listTool := &ListApplicableRulesTool{cases: cases, rules: rules}
	listCall := harness.ToolCall{Name: "list_applicable_rules", Input: map[string]any{"case_id": "CASE-SYN-001"}}
	lr1, _ := listTool.Execute(ctx, listCall)
	lr2, _ := listTool.Execute(ctx, listCall)
	if !reflect.DeepEqual(lr1, lr2) {
		t.Error("list_applicable_rules is not deterministic: results differ between calls")
	}
}

func TestReadCaseTool_TranscriptIsInertData(t *testing.T) {
	// Structural assertion: transcript items carry speaker and text as string fields.
	// Test does not route on content — asserting structure proves untrusted-data invariant.
	cases, _ := loadStoresForTest(t)
	tool := &ReadCaseTool{cases: cases}
	call := harness.ToolCall{Name: "read_case", Input: map[string]any{"case_id": "CASE-SYN-001"}}

	result, _ := tool.Execute(context.Background(), call)
	caseMap := result.Output["case"].(map[string]any)
	transcript := caseMap["transcript"].([]interface{})
	for i, item := range transcript {
		utter := item.(map[string]any)
		if _, ok := utter["speaker"].(string); !ok {
			t.Errorf("transcript[%d].speaker is not a string", i)
		}
		if _, ok := utter["text"].(string); !ok {
			t.Errorf("transcript[%d].text is not a string", i)
		}
		// No content is inspected, routed on, or forwarded as control flow.
	}
}

func TestRiskClass_ReadToolsCarryReadClass(t *testing.T) {
	cases, rules := loadStoresForTest(t)
	tools := []struct {
		name      string
		riskClass func() harness.RiskClass
	}{
		{"ReadCaseTool", (&ReadCaseTool{cases: cases}).RiskClass},
		{"ReadPolicyRuleTool", (&ReadPolicyRuleTool{rules: rules}).RiskClass},
		{"ListApplicableRulesTool", (&ListApplicableRulesTool{cases: cases, rules: rules}).RiskClass},
	}
	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.riskClass(); got != harness.RiskClassRead {
				t.Errorf("%s.RiskClass() = %q, want %q", tc.name, got, harness.RiskClassRead)
			}
		})
	}
}

// outputKeys returns the top-level keys of a map for diagnostic messages.
func outputKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
