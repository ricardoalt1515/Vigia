# Delta for Contact-Hours Detector

## MODIFIED Requirements

### Requirement: Contact-Hours Detector Is a Pure, Fail-Closed Function

The `internal/detection` package MUST expose a `Detector` interface and a
single `ContactHoursDetector` implementation that is a pure function of an
interaction's occurrence instant and its resolved debtor-local timezone,
returning `(outcome, rationale)` with no I/O. The evaluation window MUST be
the half-open interval `[08:00:00, 21:00:00)` in debtor-local wall-clock
time, derived from the IANA zone snapshotted on the interaction. Missing or
invalid timezone data MUST fail closed to `BLOCK` with an explicit
rationale rather than passing or defaulting. Its `NamedDetector.Code`
registration string MUST be `"MX-REDECO-04"`, replacing the prior
`"contact-hours"` wiring string.
(Previously: `Code` was the ad hoc string `"contact-hours"`; this delta
standardizes it to the REDECO rule code with no change to detection logic.)

#### Scenario: Interaction at exactly 08:00:00 local time passes `[unit]`

- GIVEN an interaction's `occurred_at`, converted to debtor-local wall-clock
  time via its snapshot IANA zone, is exactly `08:00:00`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `PASS`
- AND the rationale MUST state that the local time falls within the
  permitted `08:00:00–21:00:00` window.

#### Scenario: Interaction at exactly 21:00:00 local time blocks `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is exactly `21:00:00`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`
- AND the rationale MUST state that `21:00:00` is the first prohibited
  instant of the contact window (half-open interval, Decision 1).

#### Scenario: Interaction at 20:59:59 local time passes `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is `20:59:59`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `PASS`.

#### Scenario: Interaction at 07:59:59 local time blocks `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is `07:59:59`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`
- AND the rationale MUST state that the local time falls before the
  permitted window opens at `08:00:00`.

#### Scenario: Interaction well inside the window passes `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is `14:30:00`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `PASS`.

#### Scenario: Interaction well outside the window blocks `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is `23:15:00`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`.

#### Scenario: Missing debtor timezone fails closed `[unit]`

- GIVEN an interaction has no resolvable debtor timezone (empty or absent
  `DebtorTimezone`)
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`
- AND the rationale MUST explicitly state the timezone is missing and that
  the detector cannot prove the interaction occurred inside the window
- AND the detector MUST NOT default to UTC or any other timezone.

#### Scenario: Invalid IANA timezone fails closed `[unit]`

- GIVEN an interaction carries a `DebtorTimezone` string that does not
  resolve via `time.LoadLocation` (e.g. a malformed or unknown zone name)
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`
- AND the rationale MUST explicitly state the timezone is invalid and
  unresolvable.

#### Scenario: IANA zone resolution is correct across a DST-observing Mexican border zone `[unit]`

- GIVEN two interactions carry the same UTC instant but one has
  `DebtorTimezone = "America/Tijuana"` (a Mexican border zone that
  continues to observe DST) during a period when DST is in effect
- WHEN `ContactHoursDetector` evaluates both interactions
- THEN the local wall-clock time used for the window check MUST reflect the
  DST-adjusted offset for `America/Tijuana` at that instant, as resolved by
  `time.LoadLocation`
- AND the outcome MUST match what the correctly DST-adjusted local time
  implies, not the outcome implied by a non-DST-adjusted offset.

#### Scenario: Detector performs no I/O `[unit]`

- GIVEN the `ContactHoursDetector` implementation is reviewed
- WHEN its method signature and body are inspected
- THEN it MUST accept only interaction-shaped input, resolved timezone, and
  window bounds as arguments
- AND it MUST NOT perform database queries, network calls, clock reads via
  `time.Now()`, or any other side effect.

#### Scenario: Registration code uses the REDECO rule code `[unit]`

- GIVEN the `Detectors` slice is constructed in `cmd/api/main.go` and
  `cmd/seed/main.go`
- WHEN `ContactHoursDetector`'s `NamedDetector.Code` is inspected
- THEN it MUST equal `"MX-REDECO-04"`
- AND the rename scope is: production wiring (`cmd/api/main.go`,
  `cmd/seed/main.go`) plus ANY test that wires or asserts on the real
  `ContactHoursDetector`'s registration code — explicitly including
  `cmd/seed/devdata_integration_test.go`, which wires the real
  `ContactHoursDetector` with code `"contact-hours"` and persists rows to
  Postgres — all of which MUST NOT still use the string `"contact-hours"`
- AND unrelated tests that use the literal string `"contact-hours"` only as
  an arbitrary fixture label for a fake/stub detector (not the real
  `ContactHoursDetector`) are explicitly OUT OF SCOPE for this rename and MAY
  keep that label unchanged.
