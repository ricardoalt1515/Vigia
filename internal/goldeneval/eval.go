// Package goldeneval evaluates deterministic golden-set agreement for CI.
package goldeneval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
	"github.com/ricardoalt1515/vigia/internal/harness/labtools"
)

const (
	DefaultThreshold       = 1.0
	contactRuleCode        = "MX-REDECO-04"
	judgeRuleCode          = "MX-REDECO-05"
	contactWindowStartHour = 8
	contactWindowEndHour   = 21
)

var threatMarkers = []string{
	"serious consequences",
	"vamos a tu casa",
	"te vamos a",
	"amenaza",
	"vamos a la fuerza",
}

// Options controls the golden-set gate.
type Options struct {
	Threshold float64
}

// Result summarizes golden-set agreement.
type Result struct {
	Total      int
	Matched    int
	Agreement  float64
	Mismatches []Mismatch
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
	return Evaluate(cases, opts)
}

// Evaluate compares deterministic local predictions against golden fixture outcomes.
func Evaluate(cases labtools.CaseStore, opts Options) (Result, error) {
	threshold := opts.Threshold
	if threshold < 0 || threshold > 1 {
		return Result{}, fmt.Errorf("threshold must be between 0 and 1: %.4f", threshold)
	}

	caseIDs := make([]string, 0, len(cases))
	for id := range cases {
		caseIDs = append(caseIDs, id)
	}
	sort.Strings(caseIDs)

	result := Result{}
	for _, id := range caseIDs {
		c := cases[id]
		actual, err := actualOutcomes(c)
		if err != nil {
			return result, err
		}
		for _, expected := range c.DetectorResults {
			result.Total++
			got := actual[expected.RuleCode]
			if got == expected.Outcome {
				result.Matched++
				continue
			}
			result.Mismatches = append(result.Mismatches, Mismatch{
				CaseID:   c.CaseID,
				RuleCode: expected.RuleCode,
				Expected: expected.Outcome,
				Actual:   got,
			})
		}
	}
	if result.Total > 0 {
		result.Agreement = float64(result.Matched) / float64(result.Total)
	}
	if result.Agreement < threshold {
		return result, fmt.Errorf("%w: %.4f < %.4f", ErrBelowThreshold, result.Agreement, threshold)
	}
	return result, nil
}

func actualOutcomes(c labtools.SyntheticCase) (map[string]string, error) {
	occurredAt, err := time.Parse(time.RFC3339, c.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("case %q occurred_at: %w", c.CaseID, err)
	}

	contact := detection.ContactHoursDetector{Window: detection.Window{StartHour: contactWindowStartHour, EndHour: contactWindowEndHour}}
	contactResult := contact.Evaluate(detection.Interaction{OccurredAt: occurredAt, DebtorTimezone: c.DebtorTimezone})

	return map[string]string{
		contactRuleCode: outcomeFromBlock(contactResult.Outcome == detection.OutcomeBlock),
		judgeRuleCode:   outcomeFromBlock(transcriptContainsThreat(c.Transcript)),
	}, nil
}

func outcomeFromBlock(block bool) string {
	if block {
		return "hard_block"
	}
	return "pass"
}

func transcriptContainsThreat(transcript []labtools.Utterance) bool {
	for _, u := range transcript {
		text := strings.ToLower(u.Text)
		for _, marker := range threatMarkers {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}
	return false
}
