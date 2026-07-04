package detection_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
)

func dobYearsBefore(occurredAt time.Time, years int) *time.Time {
	dob := occurredAt.AddDate(-years, 0, 0)
	return &dob
}

func TestProtectedPopulationDetector(t *testing.T) {
	detector := detection.ProtectedPopulationDetector{}
	occurredAt := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name          string
		relationship  string
		dob           *time.Time
		wantOutcome   detection.Outcome
		rationaleWant string
	}{
		{
			name:          "minor debtor blocks regardless of relationship",
			relationship:  "debtor",
			dob:           dobYearsBefore(occurredAt, 10),
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "protected age",
		},
		{
			name:          "minor non-debtor blocks",
			relationship:  "third_party",
			dob:           dobYearsBefore(occurredAt, 10),
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "protected age",
		},
		{
			name:         "elderly debtor passes (debtor exemption)",
			relationship: "debtor",
			dob:          dobYearsBefore(occurredAt, 65),
			wantOutcome:  detection.OutcomePass,
		},
		{
			name:          "elderly non-debtor blocks",
			relationship:  "authorized",
			dob:           dobYearsBefore(occurredAt, 65),
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "elderly",
		},
		{
			name:         "adult non-debtor between thresholds passes",
			relationship: "authorized",
			dob:          dobYearsBefore(occurredAt, 30),
			wantOutcome:  detection.OutcomePass,
		},
		{
			name:         "debtor with missing DOB passes",
			relationship: "debtor",
			dob:          nil,
			wantOutcome:  detection.OutcomePass,
		},
		{
			name:          "non-debtor with missing DOB fails closed",
			relationship:  "authorized",
			dob:           nil,
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "cannot be verified",
		},
		{
			name:          "unset relationship with missing DOB fails closed",
			relationship:  "",
			dob:           nil,
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "cannot be verified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Evaluate(detection.Interaction{
				OccurredAt:               occurredAt,
				ContactPartyRelationship: tt.relationship,
				ContactedPartyDOB:        tt.dob,
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

// TestProtectedPopulationDetectorOccurredAtRelative proves age is computed
// relative to Interaction.OccurredAt, never time.Now(): a contacted party
// who is a minor as of OccurredAt but has since reached majority in
// wall-clock time MUST still block, and MUST do so identically on repeated
// evaluation (e.g. a later ReEvaluateInteraction rerun).
func TestProtectedPopulationDetectorOccurredAtRelative(t *testing.T) {
	detector := detection.ProtectedPopulationDetector{}

	// OccurredAt is in the past; the contacted party was 17 at OccurredAt
	// but would be well past majority by any "current" wall-clock time.
	occurredAt := time.Date(2010, 6, 15, 12, 0, 0, 0, time.UTC)
	dob := occurredAt.AddDate(-17, 0, 0)

	in := detection.Interaction{
		OccurredAt:               occurredAt,
		ContactPartyRelationship: "third_party",
		ContactedPartyDOB:        &dob,
	}

	r1 := detector.Evaluate(in)
	r2 := detector.Evaluate(in)

	if r1.Outcome != detection.OutcomeBlock {
		t.Fatalf("outcome = %q, want %q (age must be computed as of OccurredAt)", r1.Outcome, detection.OutcomeBlock)
	}
	if r1 != r2 {
		t.Fatalf("Evaluate is not deterministic across repeated calls: %+v vs %+v", r1, r2)
	}
}

// TestProtectedPopulationDetectorNoIO documents and asserts the
// pure-function contract at the signature level.
func TestProtectedPopulationDetectorNoIO(t *testing.T) {
	var d detection.Detector = detection.ProtectedPopulationDetector{}
	occurredAt := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	dob := occurredAt.AddDate(-30, 0, 0)
	in := detection.Interaction{
		OccurredAt:               occurredAt,
		ContactPartyRelationship: "authorized",
		ContactedPartyDOB:        &dob,
	}
	r1 := d.Evaluate(in)
	r2 := d.Evaluate(in)
	if r1 != r2 {
		t.Fatalf("Evaluate is not deterministic for identical input: %+v vs %+v", r1, r2)
	}
}
