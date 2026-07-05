# Deterministic Detectors Specification

## Purpose

Define the five new pure, fail-closed REDECO detectors (third-party contact,
protected population, authorized channel, payment routing, disclosure
presence), their minimal input fields, standardized rule-code registration,
and their seeding into the active versioned policy bundle. Four detectors are
hard-block; the disclosure-presence detector (`MX-REDECO-03`) is warn-level,
matching its catalog action (`docs/regulatory-ruleset.md:33`: WARN, not HARD
BLOCK). Mirrors the `ContactHoursDetector` shape from issue #2.

## Testing mode note

Strict TDD applies. `[unit]` requirements run with no external dependencies
(table-driven detector tests, `TestXNoIO` purity proofs). `[integration]`
requirements require Postgres and MUST be skippable with `testing.Short()`.

---

## Requirement: Five New Detectors Are Pure, Fail-Closed Functions

The `internal/detection` package MUST expose five new `Detector`
implementations, each a pure function of `detection.Interaction`-shaped
input with no I/O: third-party contact (`MX-REDECO-06`, hard-block),
protected population — minor-always-protected, elderly-unless-debtor
(`MX-REDECO-07`, hard-block), authorized channel (`MX-REDECO-11`,
hard-block), payment routing — creditor-only-recipient (`MX-REDECO-10`,
hard-block), and disclosure presence (`MX-REDECO-03`, warn-level: the
deterministic disclosure rule requiring the UNE / complaints-unit contact
information and that a complaint can be filed in REDECO — distinct from the
LLM-judge disclosure rule `MX-REDECO-02`, which covers the fuzzier
first-contact disclosure of despacho/creditor/debt terms). The four
hard-block detectors MUST fail closed to `BLOCK` with an explicit rationale
when their required input field is absent, rather than passing or
defaulting. The disclosure-presence detector MUST fail closed to `WARN` (not
`BLOCK`) when its required input field is absent, since MX-REDECO-03's
catalog action is WARN-level (`docs/regulatory-ruleset.md:33`); fail-closed
behavior means the detector never silently passes on missing data, not that
every detector fails closed to the same outcome value.

#### Scenario: Third-party contact without debtor relationship blocks `[unit]`

- GIVEN an interaction's contact-party relationship field indicates the
  contacted party is not the debtor and not an authorized third party
- WHEN the third-party-contact detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale naming the unauthorized
  contact-party relationship.

#### Scenario: Third-party contact with the debtor passes `[unit]`

- GIVEN an interaction's contact-party relationship field indicates the
  contacted party is the debtor
- WHEN the third-party-contact detector evaluates the interaction
- THEN the outcome MUST be `PASS`.

#### Scenario: Third-party contact fails closed on missing relationship data `[unit]`

- GIVEN an interaction has no contact-party relationship value set
- WHEN the third-party-contact detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale stating the relationship
  is unknown and cannot be proven authorized.

#### Scenario: Third-party contact with an authorized third party passes `[unit]`

- GIVEN an interaction's contact-party relationship field is `authorized`
- WHEN the third-party-contact detector evaluates the interaction
- THEN the outcome MUST be `PASS`.

#### Scenario: Protected population contact of a minor blocks regardless of relationship `[unit]`

- GIVEN an interaction's contacted party has an age below the legal majority
  threshold (18) as of `OccurredAt`, for any value of contact-party
  relationship including `debtor`
- WHEN the protected-population detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale citing the protected age
- AND this MUST hold even when the relationship is `debtor`: the debtor
  exemption applies only to the elderly case, never to minors
  (`docs/regulatory-ruleset.md:38`).

#### Scenario: Protected population contact of an elderly debtor passes `[unit]`

- GIVEN an interaction's contact-party relationship is `debtor` and the
  contacted party's age is at or above the elderly threshold (60) as of
  `OccurredAt`
- WHEN the protected-population detector evaluates the interaction
- THEN the outcome MUST be `PASS`, since the elderly-debtor exemption
  applies.

#### Scenario: Protected population contact of a non-debtor elderly person blocks `[unit]`

- GIVEN an interaction's contact-party relationship is not `debtor` and the
  contacted party's age is at or above the elderly threshold (60) as of
  `OccurredAt`
- WHEN the protected-population detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale citing the protected
  elderly age.

#### Scenario: Protected population contact of a non-debtor adult passes `[unit]`

- GIVEN an interaction's contact-party relationship is not `debtor`, the
  contacted party's date of birth is present, and the computed age as of
  `OccurredAt` is at or above the legal majority threshold (18) and below the
  elderly threshold (60)
- WHEN the protected-population detector evaluates the interaction
- THEN the outcome MUST be `PASS`.

#### Scenario: Protected population debtor contact with missing DOB passes `[unit]`

- GIVEN an interaction's contact-party relationship is `debtor` and no
  debtor date-of-birth value is set
- WHEN the protected-population detector evaluates the interaction
- THEN the outcome MUST be `PASS`, since a minor-debtor with no DOB on file
  is an accepted, documented residual risk rather than a reason to block
  ordinary debtor contact.

#### Scenario: Protected population fails closed on missing DOB/age data for non-debtor contacts `[unit]`

- GIVEN an interaction has a contact-party relationship that is not
  `debtor` (`authorized`, `third_party`, or unset/`NULL`), and no debtor
  date-of-birth or age value set
- WHEN the protected-population detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale stating age cannot be
  verified, since the relationship is not `debtor`.

#### Scenario: Protected population age is computed relative to `OccurredAt`, not evaluation time `[unit]`

- GIVEN an interaction's contacted party has a date of birth placing them
  below the legal majority threshold as of the interaction's `OccurredAt`
  timestamp, but at or above the majority threshold as of the current wall-clock
  time
- WHEN the protected-population detector evaluates the interaction, including
  on a subsequent `ReEvaluateInteraction` re-evaluation performed after the
  party has since reached majority in wall-clock time
- THEN the outcome MUST be `BLOCK` with a rationale citing the protected age
  as of `OccurredAt`
- AND the outcome MUST be identical across the original evaluation and any
  later re-evaluation, since age is computed from `OccurredAt`, never
  `time.Now()`
- AND the detector's table-driven tests MUST include a fixture case pinning
  this OccurredAt-relative computation.

#### Scenario: Authorized channel contact via an unlisted channel blocks `[unit]`

- GIVEN an interaction's existing `Channel` value is not present in the
  interaction's authorized-channel list
- WHEN the authorized-channel detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale naming the unauthorized
  channel.

#### Scenario: Authorized channel contact via a listed channel passes `[unit]`

- GIVEN an interaction's existing `Channel` value is present in its
  authorized-channel list
- WHEN the authorized-channel detector evaluates the interaction
- THEN the outcome MUST be `PASS`.

#### Scenario: Authorized channel fails closed on missing channel data `[unit]`

- GIVEN an interaction has no authorized-channel list set (`Channel` itself
  is always populated on every interaction, so only the list can be absent)
- WHEN the authorized-channel detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale stating the channel
  cannot be verified as authorized.

#### Scenario: Payment routing to a non-creditor recipient blocks `[unit]`

- GIVEN an interaction's payment-recipient designation field does not
  identify the creditor as recipient
- WHEN the payment-routing detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale naming the disallowed
  recipient designation.

#### Scenario: Payment routing to the creditor passes `[unit]`

- GIVEN an interaction's payment-recipient designation field identifies the
  creditor as recipient
- WHEN the payment-routing detector evaluates the interaction
- THEN the outcome MUST be `PASS`.

#### Scenario: Payment routing fails closed on missing recipient data `[unit]`

- GIVEN an interaction has no payment-recipient designation set
- WHEN the payment-routing detector evaluates the interaction
- THEN the outcome MUST be `BLOCK` with a rationale stating the recipient
  cannot be verified.

#### Scenario: Disclosure presence with the required disclosure not stated emits WARN `[unit]`

- GIVEN an interaction's disclosure field indicates the required UNE /
  complaints-unit disclosure was not stated
- WHEN the disclosure-presence detector evaluates the interaction
- THEN the outcome MUST be `WARN`, not `BLOCK`, with a rationale naming the
  missing disclosure, since MX-REDECO-03's catalog action is WARN-level
  (`docs/regulatory-ruleset.md:33`), not HARD BLOCK.

#### Scenario: Disclosure presence with the required disclosure stated passes `[unit]`

- GIVEN an interaction's disclosure field indicates the required disclosure
  was stated
- WHEN the disclosure-presence detector evaluates the interaction
- THEN the outcome MUST be `PASS`.

#### Scenario: Disclosure presence fails closed to WARN when it cannot be verified `[unit]`

- GIVEN an interaction has no disclosure value set
- WHEN the disclosure-presence detector evaluates the interaction
- THEN the outcome MUST be `WARN`, not `BLOCK`, with a rationale stating
  disclosure presence cannot be verified, since MX-REDECO-03's catalog action
  is WARN-level and fail-closed behavior for this rule cannot escalate to a
  hard block.

#### Scenario: A warn-only interaction evaluation stays overall `pass` `[integration]`

- GIVEN an interaction whose only non-pass detector result is the
  disclosure-presence detector (`MX-REDECO-03`) emitting `WARN`, with every
  other detector and the judge passing
- WHEN the interaction is evaluated end-to-end through `Service`
- THEN the persisted evaluation's `OverallOutcome` MUST be `pass`
- AND `detector_result_rows` MUST contain the `MX-REDECO-03` row with
  `outcome = 'warn'` and `severity = 'medium'`
- AND the warn row MUST NOT flip `OverallOutcome` to `fail` by itself.

#### Scenario: A warn row coexisting with a hard-block row yields overall `fail` `[integration]`

- GIVEN an interaction where the disclosure-presence detector
  (`MX-REDECO-03`) emits `WARN` AND at least one hard-block detector (e.g.
  `MX-REDECO-06/07/10/11`) emits `BLOCK`
- WHEN the interaction is evaluated end-to-end through `Service`
- THEN the persisted evaluation's `OverallOutcome` MUST be `fail`, driven by
  the hard-block detector's `BLOCK` result, not by the coexisting `warn` row
- AND `detector_result_rows` MUST contain both the `fail`/`high` row from the
  blocking detector and the `warn`/`medium` row from `MX-REDECO-03`.

#### Scenario: A protected-population block sets `requires_hitl` `[integration]`

- GIVEN an interaction where the protected-population detector
  (`MX-REDECO-07`) emits `BLOCK`
- WHEN the interaction is evaluated end-to-end through `Service`
- THEN the persisted evaluation's `requires_hitl` MUST be `true`, per
  `docs/regulatory-ruleset.md:38`'s "HARD BLOCK + HITL" catalog action.

#### Scenario: Other detectors' blocks do not set `requires_hitl` `[integration]`

- GIVEN an interaction where a hard-block detector OTHER than
  `MX-REDECO-07` (e.g. third-party contact, authorized channel, or payment
  routing) emits `BLOCK`, and `MX-REDECO-07` itself passes or is not
  triggered
- WHEN the interaction is evaluated end-to-end through `Service`
- THEN the persisted evaluation's `requires_hitl` MUST remain `false` on
  account of the deterministic-detector fold (unless the judge loop
  independently sets it via its own failure/timeout path, which is
  unaffected by this change).

#### Scenario: Each new detector performs no I/O `[unit]`

- GIVEN each of the five new detector implementations is reviewed
- WHEN its method signature and body are inspected (`TestXNoIO` per
  detector)
- THEN it MUST accept only interaction-shaped input as arguments
- AND it MUST NOT perform database queries, network calls, `time.Now()`
  reads, or any other side effect.

---

## Requirement: MX-REDECO-07 Blocks Require Human-in-the-Loop

`docs/regulatory-ruleset.md:38` defines MX-REDECO-07's catalog action as
"HARD BLOCK + HITL". A `RequiresHITL bool` field on `NamedDetector` MUST be
`true` for the protected-population detector's wiring entry and `false` (the
zero value) for every other detector. When the protected-population detector
emits `BLOCK` for an interaction, the evaluation's `requires_hitl` MUST be
set `true`, OR'd alongside any HITL requirement already raised by the
LLM-judge loop's own failure/timeout path. Other hard-block detectors'
`BLOCK` outcomes MUST NOT, by themselves, set `requires_hitl`.

#### Scenario: MX-REDECO-07 block sets `requires_hitl` on the evaluation `[unit]`

- GIVEN the `MX-REDECO-07` `NamedDetector` wiring entry has `RequiresHITL =
  true`
- WHEN the protected-population detector evaluates an interaction and
  returns `BLOCK`
- THEN the evaluation-folding logic MUST set the evaluation's
  `requires_hitl` to `true`.

#### Scenario: Other detectors' `NamedDetector` entries have `RequiresHITL = false` `[unit]`

- GIVEN the `Detectors` slice is constructed in `cmd/api/main.go`
- WHEN each `NamedDetector` entry other than `MX-REDECO-07` is inspected
- THEN its `RequiresHITL` field MUST be `false` (the zero value).

---

## Requirement: Minimal Nullable Detector-Input Schema

`detection.Interaction` MUST also gain a `Channel` field (snapshotted from
`InteractionEvent.Channel`), since the authorized-channel detector needs the
channel actually used and `detection.Interaction` deliberately does not
carry `core.InteractionEvent` (`internal/detection/detector.go`, mirroring
the `OccurredAt`/`DebtorTimezone` pattern already established for
`ContactHoursDetector`). `Interaction`, `InteractionEvent`, and `Debtor` MUST
otherwise gain only the minimal nullable fields each new detector requires:
contact-party relationship (single source of truth for both the third-party
and protected-population debtor short-circuit — no separate
`IsDebtor`/`contacted_party_is_debtor`
flag), debtor date-of-birth, an authorized-channel list (compared against
the interaction's existing `Channel` field, not a redundant channel-used
column), a payment-recipient designation, and disclosure markers. These
additions MUST be additive/optional and MUST NOT change
`ContactHoursDetector` behavior or break its purity test.

#### Scenario: New fields are additive and optional `[unit]`

- GIVEN the updated `detection.Interaction` and `core` types are inspected
- WHEN existing `ContactHoursDetector` tests run unmodified
- THEN all existing contact-hours tests MUST continue to pass
- AND `TestContactHoursDetectorNoIO` MUST remain valid without changes to
  that detector's signature.

#### Scenario: Absent optional fields do not panic `[unit]`

- GIVEN an `Interaction` value with all new optional fields left at their
  zero value
- WHEN any of the five new detectors evaluate it
- THEN the four hard-block detectors (third-party contact, protected
  population, authorized channel, payment routing) MUST return `BLOCK` with
  a data-missing rationale
- AND the disclosure-presence detector MUST return `WARN` with a
  data-missing rationale, per its warn-level fail-closed behavior
- AND none of the five MUST panic or return an ambiguous zero-value `PASS`.

---

## Requirement: Rule-Code Registration Is Standardized

`NamedDetector.Code` MUST use REDECO rule codes for all detectors, including
the five new ones (`MX-REDECO-06`, `MX-REDECO-07`, `MX-REDECO-11`,
`MX-REDECO-10`, and `MX-REDECO-03` for deterministic disclosure), wired into
`Detectors []NamedDetector` in both `cmd/api/main.go` and `cmd/seed/main.go`.

#### Scenario: New detectors register with their REDECO code `[unit]`

- GIVEN the `Detectors` slice is constructed in `cmd/api/main.go`
- WHEN each new detector's `NamedDetector.Code` is inspected
- THEN each MUST match its assigned REDECO rule code exactly, with no
  duplicate codes across the slice.

---

## Requirement: All Seven Rules Are Seeded Into the Active Bundle

`cmd/seed` MUST establish a single seeding path that creates `policy_rules`
catalog rows and an active `policy_bundle_rules` snapshot (with
`LegalBasis` and `EffectiveDate`) for all seven rules: the two existing
(`MX-REDECO-04`, `MX-REDECO-05`) plus the five new ones (`MX-REDECO-06`, `MX-REDECO-07`, `MX-REDECO-10`,
`MX-REDECO-11`, `MX-REDECO-03`), via `CreatePolicyRule` and
`CreateBundleVersion`.

#### Scenario: Seed creates catalog rows for all seven rules `[integration]`

- GIVEN `cmd/seed` is executed against a fresh database
- WHEN the `policy_rules` table is inspected
- THEN it MUST contain exactly one row per rule code among the seven REDECO
  codes, each with a non-null code, title, description, and severity
- AND `MX-REDECO-03`'s severity MUST reflect its warn-level catalog action
  (e.g. `medium`), distinct from the `high` severity seeded for the four
  hard-block rules (`MX-REDECO-06/07/10/11`).

#### Scenario: Seed produces one active bundle snapshotting all seven rules `[integration]`

- GIVEN `cmd/seed` completes
- WHEN the tenant's active `policy_bundles` row is resolved
- THEN its `policy_bundle_rules` snapshot MUST include all seven rule codes
- AND each snapshotted row MUST have non-null `LegalBasis` and non-null
  `EffectiveDate`.

---

## Non-goals (hardened by this spec)

- Transcript ingestion, STT, or NLP extraction. Disclosure and channel
  detectors consume structured fields only.
- Data-driven rule interpretation — detector wiring stays hardcoded in
  `Service`.
- Golden-eval fixtures for the new detectors.

## Dependency alignment

Depends on the `policy-bundle` spec's append-only bundle/versioning
mechanism (issue #6, merged) and the `contact-hours-detector` spec's pure
`Detector` seam (issue #2).
