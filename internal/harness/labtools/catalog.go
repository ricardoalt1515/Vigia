package labtools

import "github.com/ricardoalt1515/vigia/internal/harness"

// toolRiskClasses maps every known tool name to its static RiskClass.
// Read and draft tools are registered implementations; authority tools are
// classified here for the gate but are NEVER registered as callable implementations.
var toolRiskClasses = map[string]harness.RiskClass{
	// Read tools — allowed by the lab permission gate
	"read_case":             harness.RiskClassRead,
	"read_policy_rule":      harness.RiskClassRead,
	"list_applicable_rules": harness.RiskClassRead,

	// Draft tools — allowed by the lab permission gate
	"draft_evidence_manifest": harness.RiskClassDraft,
	"draft_supervisor_note":   harness.RiskClassDraft,

	// Authority tools — denied by the lab permission gate, never registered
	"append_evidence":   harness.RiskClassAuthority,
	"update_case_state": harness.RiskClassAuthority,
	"submit_report":     harness.RiskClassAuthority,
	"block_campaign":    harness.RiskClassAuthority,
}

// riskClassFor returns the RiskClass for a named tool.
// Returns ("", false) when the name is not found.
func riskClassFor(name string) (harness.RiskClass, bool) {
	rc, ok := toolRiskClasses[name]
	return rc, ok
}
