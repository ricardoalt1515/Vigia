package detection

import (
	"fmt"
	"time"
)

// ContactHoursDetector evaluates whether an interaction occurred within a
// permitted debtor-local wall-clock window. It is the only Detector
// implementation shipped by issue #2; channel/third-party/frequency
// detectors are out of scope (issue #7).
type ContactHoursDetector struct {
	Window Window
}

// Evaluate resolves the interaction's debtor-local wall-clock time using the
// snapshotted IANA timezone and checks it against the half-open contact
// window [Window.StartHour, Window.EndHour). Missing or unresolvable
// timezone data fails closed to BLOCK — the detector never defaults to UTC
// or any other zone.
func (d ContactHoursDetector) Evaluate(in Interaction) Result {
	if in.DebtorTimezone == "" {
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: "debtor timezone is missing; cannot prove the interaction occurred inside the contact window",
		}
	}

	loc, err := time.LoadLocation(in.DebtorTimezone)
	if err != nil {
		return Result{
			Outcome:   OutcomeBlock,
			Rationale: fmt.Sprintf("debtor timezone is invalid and unresolvable via time.LoadLocation: %q (%v)", in.DebtorTimezone, err),
		}
	}

	local := in.OccurredAt.In(loc)
	startOfDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	windowStart := startOfDay.Add(time.Duration(d.Window.StartHour) * time.Hour)
	windowEnd := startOfDay.Add(time.Duration(d.Window.EndHour) * time.Hour)

	// Half-open interval [windowStart, windowEnd): windowStart is the first
	// permitted instant, windowEnd is the first prohibited instant.
	if local.Before(windowStart) {
		return Result{
			Outcome: OutcomeBlock,
			Rationale: fmt.Sprintf(
				"local time %s falls before the permitted window opens at %02d:00:00",
				local.Format("15:04:05"), d.Window.StartHour,
			),
		}
	}
	if !local.Before(windowEnd) {
		return Result{
			Outcome: OutcomeBlock,
			Rationale: fmt.Sprintf(
				"local time %s is at or after %02d:00:00, the first prohibited instant of the contact window (half-open interval)",
				local.Format("15:04:05"), d.Window.EndHour,
			),
		}
	}

	return Result{
		Outcome: OutcomePass,
		Rationale: fmt.Sprintf(
			"local time %s falls within the permitted %02d:00:00–%02d:00:00 window",
			local.Format("15:04:05"), d.Window.StartHour, d.Window.EndHour,
		),
	}
}

var _ Detector = ContactHoursDetector{}
