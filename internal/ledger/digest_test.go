package ledger_test

import (
	"testing"

	"github.com/ricardoalt1515/vigia/internal/ledger"
)

func baselineDetectorResults() []ledger.DetectorResult {
	return []ledger.DetectorResult{
		{Code: "contact-hours", Outcome: "fail", Severity: "high", Rationale: "outside window"},
		{Code: "profanity", Outcome: "pass", Severity: "low", Rationale: "clean"},
	}
}

func TestComputeInputsDigestOrderingInvariant(t *testing.T) {
	original := baselineDetectorResults()
	shuffled := []ledger.DetectorResult{original[1], original[0]}

	got1 := ledger.ComputeInputsDigest(original)
	got2 := ledger.ComputeInputsDigest(shuffled)

	if got1 != got2 {
		t.Fatalf("ComputeInputsDigest not order-independent: %q != %q", got1, got2)
	}
}

func TestComputeInputsDigestChangesWithFieldChange(t *testing.T) {
	baseline := baselineDetectorResults()
	baselineDigest := ledger.ComputeInputsDigest(baseline)

	tests := []struct {
		name   string
		mutate func([]ledger.DetectorResult) []ledger.DetectorResult
	}{
		{
			name: "code changes",
			mutate: func(rs []ledger.DetectorResult) []ledger.DetectorResult {
				rs[0].Code = "different-code"
				return rs
			},
		},
		{
			name: "outcome changes",
			mutate: func(rs []ledger.DetectorResult) []ledger.DetectorResult {
				rs[0].Outcome = "pass"
				return rs
			},
		},
		{
			name: "severity changes",
			mutate: func(rs []ledger.DetectorResult) []ledger.DetectorResult {
				rs[0].Severity = "critical"
				return rs
			},
		},
		{
			name: "rationale changes",
			mutate: func(rs []ledger.DetectorResult) []ledger.DetectorResult {
				rs[0].Rationale = "different rationale"
				return rs
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := baselineDetectorResults()
			mutated = tt.mutate(mutated)

			gotDigest := ledger.ComputeInputsDigest(mutated)
			if gotDigest == baselineDigest {
				t.Fatalf("ComputeInputsDigest did not change after %s", tt.name)
			}
		})
	}
}

func TestComputeInputsDigestEmptyResults(t *testing.T) {
	got := ledger.ComputeInputsDigest(nil)
	if got == "" {
		t.Fatal("ComputeInputsDigest(nil) returned empty string, want a defined digest")
	}
}
