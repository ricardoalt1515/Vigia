package caseflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ricardoalt1515/vigia/internal/harness"
)

// validatorFunc adapts a plain function to the harness.Validator interface.
// Used in agents.go to assign per-handoff validators to AgentDefinition.Validator.
type validatorFunc func(harness.ModelOutput) error

func (f validatorFunc) Validate(out harness.ModelOutput) error { return f(out) }

// forbiddenTokens is the exact enumerated set of authority-claim tokens from the spec.
// No additions or removals without a spec update.
var forbiddenTokens = []string{
	"approved", "approval_granted", "block_campaign",
	"campaign_blocked", "override_to_compliant", "ledger_committed",
}

// scanDenylist checks whether s contains any forbidden token as a case-insensitive substring.
// Returns the matched token and true, or "", false if none matched.
func scanDenylist(s string) (token string, matched bool) {
	lower := strings.ToLower(s)
	for _, tok := range forbiddenTokens {
		if strings.Contains(lower, tok) {
			return tok, true
		}
	}
	return "", false
}

// checkStringFields checks each field in the slice against the denylist.
// Returns an error on the first match.
func checkStringFields(fields []string) error {
	for _, f := range fields {
		if tok, matched := scanDenylist(f); matched {
			return fmt.Errorf("forbidden authority claim: %q", tok)
		}
	}
	return nil
}

// ValidatePolicyExplanation validates that a ModelOutput carries a well-formed PolicyExplanation.
func ValidatePolicyExplanation(out harness.ModelOutput) error {
	if out.ToolCall != nil {
		return nil
	}
	if out.FinalOutput == "" {
		return errors.New("no final output or tool call")
	}
	var p PolicyExplanation
	if err := json.Unmarshal([]byte(out.FinalOutput), &p); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	if p.CaseID == "" {
		return errors.New("missing required field: case_id")
	}
	if len(p.Rules) == 0 {
		return errors.New("missing required field: rules must not be empty")
	}
	plains := make([]string, 0, len(p.Rules))
	for _, rule := range p.Rules {
		plains = append(plains, rule.PlainLanguage)
	}
	return checkStringFields(plains)
}

// ValidateCaseInvestigation validates that a ModelOutput carries a well-formed CaseInvestigation.
func ValidateCaseInvestigation(out harness.ModelOutput) error {
	if out.ToolCall != nil {
		return nil
	}
	if out.FinalOutput == "" {
		return errors.New("no final output or tool call")
	}
	var ci CaseInvestigation
	if err := json.Unmarshal([]byte(out.FinalOutput), &ci); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	if ci.CaseID == "" {
		return errors.New("missing required field: case_id")
	}
	if len(ci.Findings) == 0 {
		return errors.New("missing required field: findings must not be empty")
	}
	fields := make([]string, 0, len(ci.Findings)*2)
	for _, f := range ci.Findings {
		fields = append(fields, f.Evidence, f.Analysis)
	}
	return checkStringFields(fields)
}

// ValidateEvidenceManifestDraft validates that a ModelOutput carries a well-formed EvidenceManifestDraft.
func ValidateEvidenceManifestDraft(out harness.ModelOutput) error {
	if out.ToolCall != nil {
		return nil
	}
	if out.FinalOutput == "" {
		return errors.New("no final output or tool call")
	}
	var d EvidenceManifestDraft
	if err := json.Unmarshal([]byte(out.FinalOutput), &d); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	if d.CaseID == "" {
		return errors.New("missing required field: case_id")
	}
	if d.Findings == "" {
		return errors.New("missing required field: findings must not be empty")
	}
	if d.Authoritative {
		return errors.New("forbidden: authoritative field is true")
	}
	if d.Persisted {
		return errors.New("forbidden: persisted field is true")
	}
	return checkStringFields([]string{d.Findings})
}

// ValidateSupervisorNoteDraft validates that a ModelOutput carries a well-formed SupervisorNoteDraft.
func ValidateSupervisorNoteDraft(out harness.ModelOutput) error {
	if out.ToolCall != nil {
		return nil
	}
	if out.FinalOutput == "" {
		return errors.New("no final output or tool call")
	}
	var d SupervisorNoteDraft
	if err := json.Unmarshal([]byte(out.FinalOutput), &d); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	if d.CaseID == "" {
		return errors.New("missing required field: case_id")
	}
	if d.NoteBody == "" {
		return errors.New("missing required field: note_body must not be empty")
	}
	if d.Authoritative {
		return errors.New("forbidden: authoritative field is true")
	}
	if d.Persisted {
		return errors.New("forbidden: persisted field is true")
	}
	return checkStringFields([]string{d.NoteBody})
}
