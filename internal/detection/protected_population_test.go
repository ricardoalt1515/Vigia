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

// dobAt derives a *time.Time birth date exactly yearsBefore years before
// occurredAt, shifted by dayOffset days, so age-boundary fixtures are pinned
// to occurredAt (never a hardcoded calendar date): dayOffset=0 means the
// contacted party turns yearsBefore years old exactly at occurredAt (age ==
// yearsBefore); dayOffset=1 means the birthday is one day AFTER occurredAt
// — i.e. the contacted party is still yearsBefore-1 years old, one day shy
// of turning yearsBefore (age == yearsBefore-1). A negative yearsBefore
// derives a DOB in the future relative to occurredAt (bad/corrupted data).
func dobAt(occurredAt time.Time, yearsBefore, dayOffset int) *time.Time {
	dob := occurredAt.AddDate(-yearsBefore, 0, dayOffset)
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

		// Exact age-boundary fixtures (derived from occurredAt, never a
		// hardcoded calendar date), pinning the < / >= comparisons in the
		// decision table at the exact legalMajorityAge (18) and elderlyAge
		// (60) thresholds.
		{
			name:          "17 years 364 days old (one day before the 18th birthday) blocks even for the debtor",
			relationship:  "debtor",
			dob:           dobAt(occurredAt, 18, 1),
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "protected age",
		},
		{
			name:         "exactly 18 years old (18th birthday) passes as an adult",
			relationship: "authorized",
			dob:          dobAt(occurredAt, 18, 0),
			wantOutcome:  detection.OutcomePass,
		},
		{
			name:         "59 years 364 days old (one day before the 60th birthday) passes as an adult",
			relationship: "authorized",
			dob:          dobAt(occurredAt, 60, 1),
			wantOutcome:  detection.OutcomePass,
		},
		{
			name:          "exactly 60 years old (60th birthday), non-debtor, blocks as elderly",
			relationship:  "authorized",
			dob:           dobAt(occurredAt, 60, 0),
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "elderly",
		},
		{
			name:         "exactly 60 years old (60th birthday), debtor, passes (elderly-debtor exemption boundary)",
			relationship: "debtor",
			dob:          dobAt(occurredAt, 60, 0),
			wantOutcome:  detection.OutcomePass,
		},
		{
			// Bad/corrupted data path: a future date of birth (relative to
			// OccurredAt) yields a negative computed age. The detector MUST
			// fail closed to BLOCK rather than silently treat a negative age
			// as "not yet a minor" or otherwise pass — even for a relationship
			// of "debtor", since a negative age is never a valid elderly-
			// debtor exemption case.
			name:          "future date of birth (negative computed age) fails closed to block",
			relationship:  "debtor",
			dob:           dobAt(occurredAt, -5, 0),
			wantOutcome:   detection.OutcomeBlock,
			rationaleWant: "protected age",
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
