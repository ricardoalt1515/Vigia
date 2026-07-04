// Package goldeneval evaluates deterministic golden-set agreement for CI.
package goldeneval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
	"github.com/ricardoalt1515/vigia/internal/judge"
)

const (
	DefaultThreshold       = 1.0
	contactRuleCode        = "MX-REDECO-04"
	judgeRuleCode          = "MX-REDECO-05"
	contactWindowStartHour = 8
	contactWindowEndHour   = 21
)

// Options controls the golden-set gate.
type Options struct {
	Threshold             float64
	ExpectedJudgeModelID  string
	ExpectedRubricVersion string
}

// Result summarizes golden-set agreement.
type Result struct {
	Total                     int
	Matched                   int
	Agreement                 float64
	ByRule                    map[string]RuleResult
	Mismatches                []Mismatch
	JudgeModelID              string
	RubricVersion             string
	DriftReevaluationRequired bool
}

// RuleResult summarizes per-rule raw and chance-corrected agreement.
type RuleResult struct {
	RuleCode                 string
	Total                    int
	Matched                  int
	Agreement                float64
	ChanceCorrectedAgreement float64
	HasPass                  bool
	HasHardBlock             bool
	ExpectedCounts           map[string]int
	ActualCounts             map[string]int
}

// SortedRuleResults returns per-rule results sorted by rule code for stable output.
func (r Result) SortedRuleResults() []RuleResult {
	rules := make([]RuleResult, 0, len(r.ByRule))
	for _, rule := range r.ByRule {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].RuleCode < rules[j].RuleCode })
	return rules
}

// Mismatch describes one expected-vs-actual disagreement.
type Mismatch struct {
	CaseID   string
	RuleCode string
	Expected string
	Actual   string
}

// ErrBelowThreshold marks a gate failure caused by insufficient agreement.
var ErrBelowThreshold = errors.New("golden eval agreement below threshold")

// ErrDriftReevaluationRequired marks a gate failure caused by judge/rubric drift.
var ErrDriftReevaluationRequired = errors.New("golden eval drift re-evaluation required")

// Run loads embedded synthetic golden fixtures and evaluates them.
func Run(ctx context.Context, opts Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	cases, _, err := labtools.Load()
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return Evaluate(ctx, cases, opts)
}

// Evaluate compares deterministic local predictions against golden fixture outcomes.
func Evaluate(ctx context.Context, cases labtools.CaseStore, opts Options) (Result, error) {
	threshold := opts.Threshold
	if threshold < -1 || threshold > 1 {
		return Result{}, fmt.Errorf("threshold must be between -1 and 1: %.4f", threshold)
	}

	caseIDs := make([]string, 0, len(cases))
	for id := range cases {
		caseIDs = append(caseIDs, id)
	}
	sort.Strings(caseIDs)

	result := Result{
		ByRule:        make(map[string]RuleResult),
		RubricVersion: judge.RubricVersion,
	}
	for _, id := range caseIDs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		c := cases[id]
		actual, judgeModelID, err := actualOutcomes(ctx, c)
		if err != nil {
			return result, err
		}
		if result.JudgeModelID == "" {
			result.JudgeModelID = judgeModelID
		}
		for _, expected := range c.DetectorResults {
			got := actual[expected.RuleCode]
			matched := got == expected.Outcome
			result.Total++
			if matched {
				result.Matched++
			} else {
				result.Mismatches = append(result.Mismatches, Mismatch{
					CaseID:   c.CaseID,
					RuleCode: expected.RuleCode,
					Expected: expected.Outcome,
					Actual:   got,
				})
			}
			rule := result.ByRule[expected.RuleCode]
			rule.RuleCode = expected.RuleCode
			rule.Total++
			if matched {
				rule.Matched++
			}
			if expected.Outcome == "pass" {
				rule.HasPass = true
			}
			if expected.Outcome == "hard_block" {
				rule.HasHardBlock = true
			}
			if rule.ExpectedCounts == nil {
				rule.ExpectedCounts = make(map[string]int)
			}
			if rule.ActualCounts == nil {
				rule.ActualCounts = make(map[string]int)
			}
			rule.ExpectedCounts[expected.Outcome]++
			rule.ActualCounts[got]++
			result.ByRule[expected.RuleCode] = rule
		}
	}
	if result.Total > 0 {
		result.Agreement = float64(result.Matched) / float64(result.Total)
	}
	for ruleCode, rule := range result.ByRule {
		if rule.Total > 0 {
			rule.Agreement = float64(rule.Matched) / float64(rule.Total)
		}
		rule.ChanceCorrectedAgreement = chanceCorrectedAgreement(rule)
		result.ByRule[ruleCode] = rule
	}
	if opts.ExpectedJudgeModelID != "" && opts.ExpectedJudgeModelID != result.JudgeModelID {
		result.DriftReevaluationRequired = true
	}
	if opts.ExpectedRubricVersion != "" && opts.ExpectedRubricVersion != result.RubricVersion {
		result.DriftReevaluationRequired = true
	}
	if result.DriftReevaluationRequired {
		return result, fmt.Errorf("%w: expected model=%q rubric=%q, actual model=%q rubric=%q", ErrDriftReevaluationRequired, opts.ExpectedJudgeModelID, opts.ExpectedRubricVersion, result.JudgeModelID, result.RubricVersion)
	}
	for _, rule := range result.ByRule {
		if rule.ChanceCorrectedAgreement < threshold {
			return result, fmt.Errorf("%w: rule %s chance-corrected %.4f < %.4f", ErrBelowThreshold, rule.RuleCode, rule.ChanceCorrectedAgreement, threshold)
		}
	}
	return result, nil
}

func chanceCorrectedAgreement(rule RuleResult) float64 {
	if rule.Total == 0 {
		return 0
	}
	observed := float64(rule.Matched) / float64(rule.Total)
	expectedChance := 0.0
	for label, expectedCount := range rule.ExpectedCounts {
		actualCount := rule.ActualCounts[label]
		expectedChance += (float64(expectedCount) / float64(rule.Total)) * (float64(actualCount) / float64(rule.Total))
	}
	if expectedChance == 1 {
		if observed == 1 {
			return 1
		}
		return -1
	}
	return (observed - expectedChance) / (1 - expectedChance)
}

func actualOutcomes(ctx context.Context, c labtools.SyntheticCase) (map[string]string, string, error) {
	occurredAt, err := time.Parse(time.RFC3339, c.OccurredAt)
	if err != nil {
		return nil, "", fmt.Errorf("case %q occurred_at: %w", c.CaseID, err)
	}

	contact := detection.ContactHoursDetector{Window: detection.Window{StartHour: contactWindowStartHour, EndHour: contactWindowEndHour}}
	contactResult := contact.Evaluate(detection.Interaction{OccurredAt: occurredAt, DebtorTimezone: c.DebtorTimezone})

	judgeResult, err := (judge.FakeJudge{}).Evaluate(ctx, judge.JudgeInput{
		Utterances: toJudgeUtterances(c.Transcript),
		Rubric:     judge.Rubric{Version: judge.RubricVersion},
	})
	if err != nil {
		return nil, "", fmt.Errorf("case %q judge evaluation: %w", c.CaseID, err)
	}

	return map[string]string{
		contactRuleCode: outcomeFromBlock(contactResult.Outcome == detection.OutcomeBlock),
		judgeRuleCode:   outcomeFromBlock(judgeResult.Outcome == judge.OutcomeBlock),
	}, judgeResult.JudgeModelID, nil
}

func outcomeFromBlock(block bool) string {
	if block {
		return "hard_block"
	}
	return "pass"
}

func toJudgeUtterances(utterances []labtools.Utterance) []judge.Utterance {
	out := make([]judge.Utterance, 0, len(utterances))
	for _, u := range utterances {
		out = append(out, judge.Utterance{Speaker: u.Speaker, Text: u.Text})
	}
	return out
}
