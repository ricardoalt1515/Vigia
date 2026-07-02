# Contact-Hours Detector Specification

## Purpose

Define the testable requirements for issue #2 — the first deterministic REDECO
policy evaluation delivered end to end: a pure contact-hours detector, its
persisted `Evaluation` evidence, and the API/console surfaces that show the
outcome to an operator. This closes the "no evaluation of any kind" gap left by
issue #1 and proves the evaluate → persist → surface spine that later
detectors (#7), policy bundles (#6), and the evidence ledger (#3) will reuse.

## Testing mode note

Strict TDD applies to all Go components. Requirements marked `[unit]` MUST run
with no external dependencies (pure detector logic, table-driven boundary and
timezone tests). Requirements marked `[integration]` require a real Postgres
instance and MUST be skippable with `testing.Short()`. Requirements marked
`[manual-demo]` are validated by a human running the local dev environment.

---

## Requirement: Contact-Hours Detector Is a Pure, Fail-Closed Function

The `internal/detection` package MUST expose a `Detector` interface and a
single `ContactHoursDetector` implementation that is a pure function of an
interaction's occurrence instant and its resolved debtor-local timezone,
returning `(outcome, rationale)` with no I/O. The evaluation window MUST be the
half-open interval `[08:00:00, 21:00:00)` in debtor-local wall-clock time,
derived from the IANA zone snapshotted on the interaction. Missing or invalid
timezone data MUST fail closed to `BLOCK` with an explicit rationale rather
than passing or defaulting.

### Scenario: Interaction at exactly 08:00:00 local time passes `[unit]`

- GIVEN an interaction's `occurred_at`, converted to debtor-local wall-clock
  time via its snapshot IANA zone, is exactly `08:00:00`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `PASS`
- AND the rationale MUST state that the local time falls within the permitted
  `08:00:00–21:00:00` window.

### Scenario: Interaction at exactly 21:00:00 local time blocks `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is exactly `21:00:00`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`
- AND the rationale MUST state that `21:00:00` is the first prohibited instant
  of the contact window (half-open interval, Decision 1).

### Scenario: Interaction at 20:59:59 local time passes `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is `20:59:59`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `PASS`.

### Scenario: Interaction at 07:59:59 local time blocks `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is `07:59:59`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`
- AND the rationale MUST state that the local time falls before the permitted
  window opens at `08:00:00`.

### Scenario: Interaction well inside the window passes `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is `14:30:00`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `PASS`.

### Scenario: Interaction well outside the window blocks `[unit]`

- GIVEN an interaction's debtor-local wall-clock time is `23:15:00`
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`.

### Scenario: Missing debtor timezone fails closed `[unit]`

- GIVEN an interaction has no resolvable debtor timezone (empty or absent
  `DebtorTimezone`)
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`
- AND the rationale MUST explicitly state the timezone is missing and that the
  detector cannot prove the interaction occurred inside the window
- AND the detector MUST NOT default to UTC or any other timezone.

### Scenario: Invalid IANA timezone fails closed `[unit]`

- GIVEN an interaction carries a `DebtorTimezone` string that does not resolve
  via `time.LoadLocation` (e.g. a malformed or unknown zone name)
- WHEN `ContactHoursDetector` evaluates the interaction
- THEN the outcome MUST be `BLOCK`
- AND the rationale MUST explicitly state the timezone is invalid and
  unresolvable.

### Scenario: IANA zone resolution is correct across a DST-observing Mexican border zone `[unit]`

- GIVEN two interactions carry the same UTC instant but one has
  `DebtorTimezone = "America/Tijuana"` (a Mexican border zone that continues
  to observe DST) during a period when DST is in effect
- WHEN `ContactHoursDetector` evaluates both interactions
- THEN the local wall-clock time used for the window check MUST reflect the
  DST-adjusted offset for `America/Tijuana` at that instant, as resolved by
  `time.LoadLocation`
- AND the outcome MUST match what the correctly DST-adjusted local time
  implies, not the outcome implied by a non-DST-adjusted offset.

### Scenario: Detector performs no I/O `[unit]`

- GIVEN the `ContactHoursDetector` implementation is reviewed
- WHEN its method signature and body are inspected
- THEN it MUST accept only interaction-shaped input, resolved timezone, and
  window bounds as arguments
- AND it MUST NOT perform database queries, network calls, clock reads via
  `time.Now()`, or any other side effect.

---

## Requirement: Debtor Timezone Is Required and Snapshotted, Never Silently Defaulted

`Debtor.Timezone` MUST be a required IANA zone name and the durable source of
truth. `InteractionEvent.DebtorTimezone` MUST be a snapshot of the debtor's
timezone captured at ingest time, so past evaluations remain reproducible even
if a debtor's timezone is later corrected. Ingest MUST NOT silently default a
missing timezone to UTC.

### Scenario: Ingest requires a debtor timezone `[integration]`

- GIVEN an interaction is being created for a debtor
- WHEN the debtor record has no `Timezone` set
- THEN ingest MUST NOT silently default `DebtorTimezone` to `UTC` or any other
  zone
- AND the missing-timezone condition MUST be surfaced (rejected at ingest or
  carried through as an explicitly empty snapshot) rather than masked.

### Scenario: Interaction snapshots the debtor's timezone at creation time `[integration]`

- GIVEN a debtor has `Timezone = "America/Mexico_City"`
- WHEN an interaction is created for that debtor
- THEN the created `InteractionEvent.DebtorTimezone` MUST equal
  `"America/Mexico_City"`.

### Scenario: Later timezone correction does not retroactively change past evaluations `[integration]`

- GIVEN an interaction was created with `DebtorTimezone = "America/Mexico_City"`
  and has already been evaluated
- WHEN the debtor's `Timezone` is subsequently changed to a different IANA zone
- THEN the previously created interaction's `DebtorTimezone` snapshot MUST
  remain `"America/Mexico_City"`
- AND re-inspecting the existing `Evaluation` for that interaction MUST reflect
  the original snapshot timezone, not the corrected one.

---

## Requirement: Evaluation Persistence via Header + Child Rows

Evaluating an interaction MUST write an `evaluations` header row (tenant-scoped,
protected by RLS, with a composite foreign key to `interaction_events`) and MUST
write a `detector_result_rows` child row linked to that evaluation via a
nullable `evaluation_id` column, recording the detector's outcome and
rationale. All evaluation writes MUST occur through the established
`tenantdb.WithTenantTx` + `internal/postgres` adapter pattern. The added
`evaluation_id` column MUST be additive so issue #1's existing
`detector_result_rows` rows and tests remain valid.

### Scenario: Evaluating an interaction persists an evaluation header `[integration]`

- GIVEN a seeded interaction exists for a tenant
- WHEN the evaluation path runs the contact-hours detector for that interaction
- THEN an `evaluations` row MUST be created with a composite FK referencing
  that `interaction_events` row and the correct `tenant_id`
- AND the row MUST be readable only under that tenant's RLS context.

### Scenario: Evaluation persists a linked detector result child row `[integration]`

- GIVEN an `evaluations` header row has been created for an interaction
- WHEN the contact-hours detector result is persisted
- THEN a `detector_result_rows` row MUST be created with `evaluation_id` set to
  that evaluation's id
- AND the row MUST carry the detector's outcome and rationale.

### Scenario: Existing detector_result_rows without evaluation_id remain valid `[integration]`

- GIVEN issue #1 created `detector_result_rows` with no `evaluation_id`
  populated
- WHEN migration `00003_contact_hours.sql` adds the nullable `evaluation_id`
  column
- THEN pre-existing `detector_result_rows` rows MUST remain queryable and valid
- AND issue #1's existing tests MUST continue to pass unmodified.

### Scenario: Evaluation writes go through tenant-scoped transaction `[integration]`

- GIVEN the evaluation orchestration path is reviewed
- WHEN it writes an `evaluations` header and `detector_result_rows` child
- THEN both writes MUST occur inside a single `tenantdb.WithTenantTx` call
- AND no write MUST bypass RLS via a superuser or migration-owner connection.

---

## Requirement: Interactions API Exposes Outcome and Reason

`GET /v1/interactions` MUST include per-interaction `outcome` and `reason`
fields in its response DTO, sourced from that interaction's persisted
evaluation.

### Scenario: Interactions list includes outcome and reason `[integration]`

- GIVEN a seeded interaction has been evaluated by the contact-hours detector
- WHEN `GET /v1/interactions` is called with a valid tenant API key
- THEN each returned interaction MUST include an `outcome` field (`PASS` or
  `BLOCK`)
- AND each returned interaction MUST include a non-empty `reason` field
  matching the detector's rationale.

### Scenario: Unevaluated interaction does not fabricate an outcome `[integration]`

- GIVEN a seeded interaction has not yet been evaluated
- WHEN `GET /v1/interactions` is called
- THEN the response MUST NOT report a fabricated `PASS`/`BLOCK` outcome for
  that interaction
- AND the absence of an evaluation MUST be represented explicitly (e.g. a null
  or empty outcome/reason), not silently defaulted to `PASS`.

---

## Requirement: Tenant-Scoped Out-of-Hours Summary Endpoint

The system MUST provide a tenant-scoped summary endpoint that returns a
server-computed count of out-of-hours (`BLOCK`) evaluations for the
authenticated tenant, computed via a SQL aggregate inside the same
`tenantdb.WithTenantTx` + RLS seam that owns tenant isolation. The console MUST
NOT aggregate this count client-side.

### Scenario: Summary endpoint returns the tenant's out-of-hours count `[integration]`

- GIVEN a tenant has evaluated interactions, some `PASS` and some `BLOCK`
- WHEN the tenant calls the summary endpoint with a valid tenant API key
- THEN the response MUST return the exact count of that tenant's `BLOCK`
  evaluations
- AND the count MUST be computed by a SQL aggregate, not by fetching and
  counting the interactions list in application code.

### Scenario: Summary count is tenant-isolated `[integration]`

- GIVEN two tenants each have evaluated interactions with different
  out-of-hours counts
- WHEN tenant A calls the summary endpoint
- THEN the returned count MUST reflect only tenant A's evaluations
- AND it MUST NOT include tenant B's evaluations, enforced by RLS.

---

## Requirement: Console Shows Outcome and Out-of-Hours Tile

The `apps/console` interactions table MUST render an outcome/reason column per
interaction, and the console MUST render an out-of-hours count tile fed by the
summary endpoint.

### Scenario: Console table shows outcome and reason per row `[manual-demo]`

- GIVEN the demo tenant has seeded, evaluated interactions including at least
  one out-of-hours interaction
- WHEN a developer opens the console interactions page
- THEN each row MUST display that interaction's outcome (`PASS`/`BLOCK`)
- AND each row MUST display or expose the associated reason text.

### Scenario: Console renders the out-of-hours tile from the server endpoint `[manual-demo]`

- GIVEN the demo tenant's data is seeded and evaluated
- WHEN a developer opens the console interactions page
- THEN a tile MUST display the out-of-hours count
- AND that count MUST be fetched from the summary endpoint, not computed by
  summing rows rendered in the browser.

---

## Requirement: Seed Provides Timezone and an Out-of-Hours Demo Interaction

`cmd/seed dev-data` MUST assign the demo debtor an IANA timezone, snapshot it
onto seeded interactions, and include at least one interaction whose
debtor-local time falls outside `[08:00:00, 21:00:00)` so the out-of-hours
outcome and tile render with dev data.

### Scenario: Seeded demo debtor has a timezone `[integration]`

- GIVEN `cmd/seed dev-data` is executed against a fresh database
- WHEN the demo debtor row is inspected
- THEN it MUST have a non-empty IANA `Timezone` value.

### Scenario: Seeded interactions snapshot the debtor's timezone `[integration]`

- GIVEN the demo debtor has a timezone assigned
- WHEN `cmd/seed dev-data` creates the demo interactions
- THEN each created `interaction_events` row MUST have `debtor_timezone`
  matching the debtor's `timezone` at seed time.

### Scenario: Seed includes at least one out-of-hours interaction `[integration]`

- GIVEN `cmd/seed dev-data` completes
- WHEN the seeded interactions are evaluated by the contact-hours detector
- THEN at least one seeded interaction MUST evaluate to `BLOCK`
- AND that interaction's debtor-local wall-clock time MUST fall outside
  `[08:00:00, 21:00:00)`.

---

## Non-goals (hardened by this spec)

The following behaviors are explicitly out of scope and MUST NOT be introduced
as part of this change. Any pull request that introduces them MUST be
rejected.

- Versioned `PolicyBundle` or bundle resolution (issue #6). Any
  `policy_bundle_version`-shaped column stays inert.
- Any detector other than contact-hours (channel, third-party, frequency, etc.
  — issue #7). The `Detector` seam MUST ship with exactly one implementation.
- Evidence-ledger hash chaining or immutability proofs (issue #3).
- LLM-judge behavior, HITL routing, `RequiresHITL` behavior, Harness
  invocation, or MCP tools (issues #4/#16+).
- Asynchronous, River-job-driven evaluation. Evaluation MAY run synchronously
  in the read/ingest path for this change.
- Date-range filtering on the summary endpoint. It returns a single current
  out-of-hours count for the authenticated tenant.

---

## Dependency alignment

This spec depends on the following prior issues being stable and unmodified:

- **Issue #1**: seed, worker, console list, `detector_result_rows` table.
- **Issue #13**: schema, RLS foundations, generated `internal/db` layer.
- **Issue #14**: tenant API key auth, `tenantdb.WithTenantTx`,
  `GET /v1/interactions` endpoint.

No requirement in this spec modifies those boundaries beyond the additive
`evaluation_id` column and the DTO/endpoint additions described above.
