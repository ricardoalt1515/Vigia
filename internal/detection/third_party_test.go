package detection_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
)

func TestThirdPartyContactDetector(t *testing.T) {
	detector := detection.ThirdPartyContactDetector{}
	occurredAt := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name          string
		relationship  string
		wantOutcome   detection.Outcome
		rationaleWant string
	}{
		{
			name:         "contacted party is the debtor passes",
			relationship: "debtor",
			wantOutcome:  detection.OutcomePass,
		},
		{
			name:         "contacted party is an authorized third party passes",
			relationship: "authorized",
			wantOutcome:  detection.OutcomePass,
		},
		{
			name:          "contacted party is an unauthorized third party blocks",
			relationship:  "third_party",
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "third_party",
		},
		{
			name:          "missing relationship data fails closed",
			relationship:  "",
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Evaluate(detection.Interaction{
				OccurredAt:               occurredAt,
				ContactPartyRelationship: tt.relationship,
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

// TestThirdPartyContactDetectorNoIO documents and asserts the pure-function
// contract at the signature level, per spec.md "Each new detector performs
// no I/O".
func TestThirdPartyContactDetectorNoIO(t *testing.T) {
	var d detection.Detector = detection.ThirdPartyContactDetector{}
	in := detection.Interaction{
		OccurredAt:               time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		ContactPartyRelationship: "debtor",
	}
	r1 := d.Evaluate(in)
	r2 := d.Evaluate(in)
	if r1 != r2 {
		t.Fatalf("Evaluate is not deterministic for identical input: %+v vs %+v", r1, r2)
	}
}
