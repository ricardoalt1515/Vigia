// Package detection contains pure, side-effect-free policy detectors.
//
// Detectors accept only interaction-shaped input and return a deterministic
// Result. No detector may perform I/O (database, network, clock reads via
// time.Now()) — every input the detector needs is passed in explicitly.
package detection

import "time"

// Outcome is the detector-seam vocabulary. It is intentionally narrower than
// core.DetectorOutcome: the detector speaks "in/out of window", and
// persistence maps this to the shared enum (block -> fail, pass -> pass).
type Outcome string

const (
	OutcomePass  Outcome = "pass"
	OutcomeBlock Outcome = "block"
)

// Interaction is the minimal input a Detector needs: the instant the
// interaction occurred and the debtor-local IANA timezone snapshotted on it.
// It deliberately does not carry core.InteractionEvent so the detection
// package stays independent of persistence types.
type Interaction struct {
	OccurredAt     time.Time
	DebtorTimezone string
}

// Window is a half-open contact-hours interval [StartHour, EndHour) in
// debtor-local wall-clock time.
type Window struct {
	StartHour int
	EndHour   int
}

// Result is a detector's pure decision plus a human-readable rationale.
type Result struct {
	Outcome   Outcome
	Rationale string
}

// Detector is the seam every policy detector implements. Evaluate MUST be a
// pure function of its input: no I/O, no time.Now(), no side effects.
type Detector interface {
	Evaluate(in Interaction) Result
}
