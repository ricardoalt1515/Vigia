package detection_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ricardoalt1515/vigia/internal/detection"
)

// mustLoadLocation loads an IANA zone or fails the test immediately; it keeps
// test-case construction readable while still surfacing a broken test
// environment loudly.
func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %q: %v", name, err)
	}
	return loc
}

func TestContactHoursDetector(t *testing.T) {
	detector := detection.ContactHoursDetector{
		Window: detection.Window{StartHour: 8, EndHour: 21},
	}

	mxLoc := "America/Mexico_City"

	tests := []struct {
		name           string
		occurredAt     time.Time
		debtorTimezone string
		wantOutcome    detection.Outcome
		rationaleWant  string // substring the rationale must contain
	}{
		{
			name:           "exactly 08:00:00 local passes",
			occurredAt:     time.Date(2026, 6, 15, 8, 0, 0, 0, mustLoadLocation(t, mxLoc)),
			debtorTimezone: mxLoc,
			wantOutcome:    detection.OutcomePass,
			rationaleWant:  "within the permitted",
		},
		{
			name:           "exactly 21:00:00 local blocks (half-open boundary)",
			occurredAt:     time.Date(2026, 6, 15, 21, 0, 0, 0, mustLoadLocation(t, mxLoc)),
			debtorTimezone: mxLoc,
			wantOutcome:    detection.OutcomeBlock,
			rationaleWant:  "21:00:00",
		},
		{
			name:           "20:59:59 local passes",
			occurredAt:     time.Date(2026, 6, 15, 20, 59, 59, 0, mustLoadLocation(t, mxLoc)),
			debtorTimezone: mxLoc,
			wantOutcome:    detection.OutcomePass,
		},
		{
			name:           "07:59:59 local blocks",
			occurredAt:     time.Date(2026, 6, 15, 7, 59, 59, 0, mustLoadLocation(t, mxLoc)),
			debtorTimezone: mxLoc,
			wantOutcome:    detection.OutcomeBlock,
			rationaleWant:  "before the permitted window opens",
		},
		{
			name:           "14:30:00 local well inside window passes",
			occurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, mustLoadLocation(t, mxLoc)),
			debtorTimezone: mxLoc,
			wantOutcome:    detection.OutcomePass,
		},
		{
			name:           "23:15:00 local well outside window blocks",
			occurredAt:     time.Date(2026, 6, 15, 23, 15, 0, 0, mustLoadLocation(t, mxLoc)),
			debtorTimezone: mxLoc,
			wantOutcome:    detection.OutcomeBlock,
		},
		{
			name:           "empty debtor timezone fails closed",
			occurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
			debtorTimezone: "",
			wantOutcome:    detection.OutcomeBlock,
			rationaleWant:  "timezone is missing",
		},
		{
			name:           "invalid IANA timezone fails closed",
			occurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
			debtorTimezone: "Not/A_Real_Zone",
			wantOutcome:    detection.OutcomeBlock,
			rationaleWant:  "timezone is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Evaluate(detection.Interaction{
				OccurredAt:     tt.occurredAt,
				DebtorTimezone: tt.debtorTimezone,
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

// TestContactHoursDetectorDSTBorderZone proves the detector resolves the
// debtor-local wall-clock time via time.LoadLocation (DST-adjusted), not a
// naive fixed UTC offset, for America/Tijuana — a Mexican border zone that
// continues to observe DST.
func TestContactHoursDetectorDSTBorderZone(t *testing.T) {
	detector := detection.ContactHoursDetector{
		Window: detection.Window{StartHour: 8, EndHour: 21},
	}
	tijuana := mustLoadLocation(t, "America/Tijuana")

	// Same UTC instant, interpreted in America/Tijuana at a time of year when
	// DST is in effect (July). At 03:30 UTC, Tijuana is UTC-7 during DST
	// (PDT), giving 20:30 local -> PASS. If the detector naively used a fixed
	// non-DST offset (UTC-8, PST), the local hour would be 19:30, which is
	// still PASS at this instant, so we pick an instant where DST vs
	// non-DST produce different outcomes: 04:30 UTC.
	//   DST offset (UTC-7):    04:30 UTC -> 21:30 local -> BLOCK
	//   Non-DST offset (UTC-8): 04:30 UTC -> 20:30 local -> PASS
	dstInstant := time.Date(2026, 7, 15, 4, 30, 0, 0, time.UTC)

	result := detector.Evaluate(detection.Interaction{
		OccurredAt:     dstInstant.In(tijuana),
		DebtorTimezone: "America/Tijuana",
	})

	localTime := dstInstant.In(tijuana)
	if localTime.Hour() != 21 || localTime.Minute() != 30 {
		t.Fatalf("test setup invariant broken: local time = %s, want 21:30 (DST-adjusted)", localTime.Format("15:04:05"))
	}
	if result.Outcome != detection.OutcomeBlock {
		t.Fatalf("outcome = %q, want %q — detector must use the DST-adjusted local time from time.LoadLocation, not a naive fixed offset", result.Outcome, detection.OutcomeBlock)
	}
}

// TestContactHoursDetectorNoIO documents and asserts the pure-function
// contract at the signature level: Evaluate takes only interaction-shaped
// input and returns a Result, with no context/DB/clock params. This is a
// compile-time/signature proof rather than a runtime behavior test, per
// spec.md "Detector performs no I/O".
func TestContactHoursDetectorNoIO(t *testing.T) {
	var d detection.Detector = detection.ContactHoursDetector{
		Window: detection.Window{StartHour: 8, EndHour: 21},
	}
	// Evaluate(Interaction) Result is the entire contract; if this compiles,
	// the method has no context.Context, no *sql.DB/tenantdb.Tx, and no
	// clock parameter. Calling it twice with the same input must be
	// deterministic (no time.Now() reliance).
	in := detection.Interaction{
		OccurredAt:     time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC),
		DebtorTimezone: "UTC",
	}
	r1 := d.Evaluate(in)
	r2 := d.Evaluate(in)
	if r1 != r2 {
		t.Fatalf("Evaluate is not deterministic for identical input: %+v vs %+v", r1, r2)
	}
}
