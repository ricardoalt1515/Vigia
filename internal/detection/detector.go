// Package detection contains pure, side-effect-free policy detectors.
//
// Detectors accept only interaction-shaped input and return a deterministic
// Result. No detector may perform I/O (database, network, clock reads via
// time.Now()) — every input the detector needs is passed in explicitly.
package detection

import (
	"time"

	"github.com/ricardoalt1515/vigia/internal/core"
)

// Outcome is the detector-seam vocabulary. It is intentionally narrower than
// core.DetectorOutcome: the detector speaks "in/out of window", and
// persistence maps this to the shared enum (block -> fail, pass -> pass).
type Outcome string

const (
	OutcomePass  Outcome = "pass"
	OutcomeBlock Outcome = "block"

	// OutcomeWarn is a confirmed, warn-level policy signal (e.g. MX-REDECO-03
	// disclosure presence) — distinct from both a confirmed pass and a
	// hard-block. A warn result never flips an evaluation's overall outcome
	// to "fail" by itself (see internal/evaluation/service.go's 3-way fold).
	OutcomeWarn Outcome = "warn"
)

// Interaction is the minimal input a Detector needs: the instant the
// interaction occurred, the debtor-local IANA timezone snapshotted on it,
// and the optional per-interaction fields the issue #7 detectors consume.
// It deliberately does not carry core.InteractionEvent so the detection
// package stays independent of persistence types. The optional fields below
// are additive: leaving them at their zero value MUST NOT change
// ContactHoursDetector's behavior or break its purity test.
type Interaction struct {
	OccurredAt     time.Time
	DebtorTimezone string

	// Channel is the channel actually used for the interaction, snapshotted
	// from core.InteractionEvent.Channel the same way DebtorTimezone is
	// snapshotted from the debtor's resolved timezone. It is always
	// populated on every interaction (unlike the other optional fields
	// below, which may be absent).
	Channel core.InteractionChannel

	// ContactPartyRelationship is the single source of truth for the
	// contacted party's relationship to the debtor ("debtor", "authorized",
	// "third_party", or "" when unset). It is used by both the third-party
	// contact detector and the protected-population detector's debtor
	// short-circuit — there is no separate contacted-party-is-debtor flag.
	ContactPartyRelationship string

	// ContactedPartyDOB is the contacted party's date of birth, used by the
	// protected-population detector to compute age relative to OccurredAt
	// (never time.Now()). nil means "unknown/not on file".
	ContactedPartyDOB *time.Time

	// AuthorizedChannels is the per-interaction snapshot of channels the
	// debtor authorized, compared against Channel by the authorized-channel
	// detector. Nil/empty means "no authorized-channel list on file".
	AuthorizedChannels []string

	// PaymentRecipient is the payment-recipient designation ("creditor" or
	// any other value), consumed by the payment-routing detector. Empty
	// means "unknown/not on file".
	PaymentRecipient string

	// DisclosureProvided is a tri-state flag (nil = unknown/not on file)
	// consumed by the disclosure-presence detector.
	DisclosureProvided *bool
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
