package detection_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
)

func TestDisclosureDetector(t *testing.T) {
	detector := detection.DisclosureDetector{}
	occurredAt := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)

	trueVal := true
	falseVal := false

	tests := []struct {
		name               string
		disclosureProvided *bool
		wantOutcome        detection.Outcome
		rationaleWant      string
	}{
		{
			name:               "disclosure stated passes",
			disclosureProvided: &trueVal,
			wantOutcome:        detection.OutcomePass,
		},
		{
			name:               "disclosure not stated emits warn, not block",
			disclosureProvided: &falseVal,
			wantOutcome:        detection.OutcomeWarn,
			rationaleWant:      "not stated",
		},
		{
			name:               "missing disclosure value fails closed to warn, not block",
			disclosureProvided: nil,
			wantOutcome:        detection.OutcomeWarn,
			rationaleWant:      "cannot be verified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Evaluate(detection.Interaction{
				OccurredAt:         occurredAt,
				DisclosureProvided: tt.disclosureProvided,
			})
			if result.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %q, want %q (rationale: %s)", result.Outcome, tt.wantOutcome, result.Rationale)
			}
			if tt.rationaleWant != "" && !strings.Contains(result.Rationale, tt.rationaleWant) {
				t.Fatalf("rationale = %q, want it to contain %q", result.Rationale, tt.rationaleWant)
			}
		})
	}
}

// TestDisclosureDetectorNoIO documents and asserts the pure-function contract
// at the signature level, per spec.md "Each new detector performs no I/O".
func TestDisclosureDetectorNoIO(t *testing.T) {
	var d detection.Detector = detection.DisclosureDetector{}
	trueVal := true
	in := detection.Interaction{
		OccurredAt:         time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		DisclosureProvided: &trueVal,
	}
	r1 := d.Evaluate(in)
	r2 := d.Evaluate(in)
	if r1 != r2 {
		t.Fatalf("Evaluate is not deterministic for identical input: %+v vs %+v", r1, r2)
	}
}
