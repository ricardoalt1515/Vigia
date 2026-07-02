# Proposal: Issue #2 Contact-Hours Detector (deterministic REDECO 08:00–21:00 window)

## Problem / motivation

Vigía's regulatory promise is that every debtor interaction can be checked
against Mexican collection rules (REDECO / CONDUSEF) and that the check is
*explainable and reproducible*. Issue #1 delivered the walking skeleton
(seed → store → API → console) but it only lists interactions — there is no
evaluation of any kind. Nothing today tells a tenant whether a call or message
happened inside the legally permitted contact window.

REDECO restricts debtor contact to daytime hours (08:00–21:00 in the debtor's
local time). This is the single most concrete, most testable, most defensible
rule in the whole product, and it is deterministic — it needs no LLM and no
judgment call (ADR-03: deterministic-first). Shipping it first proves the
end-to-end evaluation spine (evaluate → persist evidence → surface outcome)
that every later detector (#7), policy bundle (#6), and evidence ledger (#3)
will reuse.

Without it there is no executable proof that Vigía can catch an out-of-hours
contact, persist *why* it was flagged, and show that verdict to an operator.

## Intent

Deliver the first real, tested policy evaluation end to end: for each
interaction, decide deterministically whether it fell inside the REDECO contact
window in the debtor's own timezone, persist an `Evaluation` (with the per-rule
result and a human-readable rationale), show the per-interaction outcome in the
console, and show a per-tenant "out-of-hours" count tile.

The change is "done" when:

1. Every interaction can be evaluated by a pure, table-tested contact-hours
   detector that resolves the debtor's timezone and returns pass/block plus a
   rationale.
2. Evaluating an interaction persists an `Evaluation` header row referencing that
   interaction, with a `detector_result_row` child carrying the per-rule verdict.
3. The console interactions list shows each interaction's outcome and reason.
4. The console shows a tenant-scoped out-of-hours count tile sourced from the
   server.
5. Timezone edge cases (boundaries, before/after window, missing timezone, IANA
   zones) are covered by table-driven tests.

This is deterministic-first: no LLM, no agent loop, no Harness. It is a workflow
step (ADR-09), not a judge.

## Current behavior

- `internal/core/types.go` defines `Debtor` and `InteractionEvent` with **no
  timezone field**, and `DetectorResultRow` (a per-rule result table that already
  exists) but **no `Evaluation` header**.
- `db/migrations/00001_initial_foundation.sql` created all tenant-scoped tables
  (RLS on `app.tenant_id`, composite `(id, tenant_id)` FKs) including
  `detector_result_rows`, but nothing writes to it.
- `GET /v1/interactions` returns a minimal DTO (`id`, `occurred_at`, `channel`,
  `direction`) with no outcome field.
- The console (`apps/console`) renders a plain table of that DTO — no outcome
  column, no summary tile.
- There is no `internal/detection` package and no detector code at all.
- Nothing sets or requires a debtor timezone anywhere.

## Desired behavior

- `Debtor` carries a `Timezone` (IANA zone name, e.g. `America/Mexico_City`) as
  the durable source of truth; `InteractionEvent` carries a snapshot
  `DebtorTimezone` captured at ingest, so past evaluations stay reproducible even
  if a debtor's timezone is later corrected.
- A deep `internal/detection` module exposes a `Detector` seam with a single
  contact-hours implementation: a pure function of `(interaction, resolvedTZ,
  window)` → `(outcome, rationale)`. No I/O inside the detector.
- Evaluating an interaction writes an `evaluations` header row (tenant-scoped,
  RLS, composite FK to the interaction) plus a `detector_result_rows` child
  linked to the evaluation, recording outcome + rationale.
- `GET /v1/interactions` gains per-interaction `outcome` and `reason` fields; the
  console table renders them.
- A tenant-scoped server endpoint returns the out-of-hours count; the console
  renders it as a tile.

## Scope

### In scope

- Migration `00003_contact_hours.sql`: add `debtors.timezone`, add
  `interaction_events.debtor_timezone` (snapshot), create `evaluations` header
  table (RLS + composite FK to `interaction_events`), and add
  `detector_result_rows.evaluation_id` (additive, nullable FK — does not break
  issue #1's existing rows/tests).
- `internal/detection` package: `Detector` interface + `ContactHoursDetector`
  pure implementation, with table-driven tests for boundaries, before/after,
  missing/invalid timezone, and IANA zone resolution.
- A thin evaluation service/orchestration path that resolves the interaction's
  snapshot timezone, runs the registered detector(s), builds an `Evaluation`, and
  persists header + child through the established `tenantdb.WithTenantTx` +
  `internal/postgres` adapter pattern.
- sqlc queries for `evaluations` and updated `detector_result_rows`, plus an
  aggregate query for the tenant out-of-hours count.
- API changes: `outcome` + `reason` fields on the interactions DTO, and a new
  tenant-scoped summary endpoint returning the out-of-hours count.
- Console changes: outcome/reason column on the interactions table + an
  out-of-hours count tile fed by the summary endpoint.
- Seed update: give the demo debtor a timezone and snapshot it onto seeded
  interactions so the outcome and tile render with dev data (including at least
  one out-of-hours interaction).
- Require a debtor timezone at ingest (no silent UTC default).

### Out of scope

- Versioned `PolicyBundle` / bundle resolution (issue #6). We persist a plain
  `Evaluation`; no `policy_bundle_version` semantics beyond an inert column if the
  existing schema needs one.
- Any detector other than contact-hours — channel, third-party, frequency, etc.
  (issue #7). The `Detector` seam is introduced but ships with exactly one
  implementation.
- Evidence-ledger hash chaining / immutability proofs (issue #3). We snapshot the
  timezone for evidence *fidelity*, but no ledger chaining.
- LLM-judge, HITL routing, `RequiresHITL` behavior, Harness, MCP (issues #4/#16+).
- Fixing the pre-existing UUID v4-vs-v7 drift between `technical-design.md` and
  the migrations (flagged in exploration; unrelated to #2).
- Real-time / on-ingest async evaluation via River jobs is not required; #2 may
  evaluate synchronously in the read/ingest path. (A River-driven pipeline is a
  later concern.)

### Delivery

Single PR. `size:exception` is acceptable (user decision) — the migration,
detector, persistence, API, and console changes form one coherent vertical slice
and splitting them would ship half-wired states.

## Resolved decisions

### Decision 1 — Boundary inclusivity: half-open window `[08:00:00, 21:00:00)`

**Decision.** An interaction **PASSES** iff its local wall-clock time `t`
satisfies `08:00:00 ≤ t < 21:00:00`. `08:00:00.000` is the first permitted
instant; `21:00:00.000` is the first prohibited instant (so exactly 21:00:00
BLOCKS, and 20:59:59.999 passes). Comparison is on the debtor-local wall-clock
time of `occurred_at`, at second (or finer) resolution.

**Rationale.** REDECO permits contact "from 08:00 to 21:00". A half-open
interval removes all double-boundary ambiguity: every instant is
unambiguously inside or outside, and the two boundaries are defined by one rule
(`start ≤ t < end`) instead of two special cases. Choosing to BLOCK exactly
21:00:00 is the conservative, regulator-defensible reading (the permitted window
closes *at* 21:00, it does not include the 21:00 instant), and it keeps the
detector trivially table-testable. This is stated precisely so the spec's edge
tests are unambiguous. *(Flagged decision — the AC left this undefined.)*

### Decision 2 — Debtor timezone is required at ingest; never silently default to UTC

**Decision.** A debtor timezone (IANA zone name) is **required**. `Debtor.Timezone`
is the source of truth and is snapshotted onto `InteractionEvent.DebtorTimezone`
at creation time. Ingest MUST NOT silently default a missing timezone to UTC. If
an interaction has no resolvable timezone, the detector **fails closed** to
`BLOCK` with an explicit rationale (e.g. "missing debtor timezone — cannot prove
in-window") rather than passing or guessing.

**Rationale.** A wrong default silently defeats the detector's entire regulatory
purpose: UTC-defaulting a Mexico City debtor shifts the window by 6 hours and
would mislabel out-of-hours contacts as compliant. Requiring the timezone makes
the missing-data case loud, and failing closed keeps the system safe when data
is incomplete. Snapshotting preserves evidence fidelity — a later correction to a
debtor's timezone must not retroactively change how a past interaction was judged.
Storing an IANA zone name (not a fixed UTC offset) keeps DST/policy changes
correct via Go's `time.LoadLocation`. *(Adopts the exploration recommendation.)*

### Decision 3 — Out-of-hours tile is served by a tenant-scoped API endpoint (server-computed)

**Decision.** Add a tenant-scoped summary endpoint (e.g.
`GET /v1/summary` / `GET /v1/interactions/summary`) that returns a
server-computed out-of-hours count via a SQL aggregate over the tenant's
evaluations. The console reads it directly for the tile. We do **not** aggregate
client-side over the interactions list.

**Rationale.** Client-side aggregation would force the console to fetch the full
(paginated, capped) interactions list and count locally — it breaks as soon as the
list is paginated, duplicates the pass/block rule in the client, and puts a
compliance number outside the tenant-isolation authority. A server aggregate runs
inside the same `tenantdb.WithTenantTx` + RLS seam that owns tenant isolation,
returns an exact count independent of list pagination, and keeps the console a
thin renderer (consistent with the #1 architecture: Go is the authority, the
console never owns isolation). The endpoint is a small deep module — one number
behind one route. *(Flagged decision — scope choice deferred from exploration.)*

### Decision 4 — Introduce the `Detector` interface now (one implementation)

**Decision.** Define a `Detector` seam in `internal/detection` now, shipping a
single `ContactHoursDetector`. The interface is trivial — one method with a pure,
side-effect-free signature that accepts an interaction-shaped value plus the
resolved timezone/window and returns outcome + rationale, no I/O. The persistence
orchestration (loop detectors → build `Evaluation` → persist) lives *outside* the
detector, so the detector stays a pure function behind the seam.

**Rationale.** Normally "one adapter means a hypothetical seam" argues against
introducing an interface for a single implementation. Here the second adapter is
already known and imminent: issue #7 (the next issue in the dependency chain)
adds more detectors and will iterate over `[]Detector`. Defining the seam now —
kept deliberately trivial so it is not a shallow wrapper — lets #7 land without
reworking the evaluation loop, and it makes the detector the natural test surface
(callers and table tests cross the same seam). The interface earns its keep
because behaviour genuinely varies across it in the very next slice. *(Adopts the
exploration recommendation.)*

## Acceptance criteria and how they are satisfied

| Acceptance criterion | How #2 satisfies it |
|---|---|
| Each interaction is evaluated against the REDECO 08:00–21:00 window in the debtor's timezone | `ContactHoursDetector` computes the debtor-local wall-clock time of `occurred_at` via the snapshot IANA zone and applies the half-open rule `[08:00:00, 21:00:00)` (Decision 1). |
| Evaluating an interaction persists an `Evaluation` referencing that interaction, with a per-rule pass/block result and rationale | The evaluation path writes an `evaluations` header (composite FK to `interaction_events`) plus a `detector_result_rows` child (`evaluation_id`, outcome, rationale) through `tenantdb.WithTenantTx` + a new `internal/postgres` adapter. |
| The console shows each interaction's outcome | `GET /v1/interactions` DTO gains `outcome` + `reason`; the console table renders them (Go DTO and TS type updated 1:1). |
| The console shows a per-tenant out-of-hours count tile | A server-computed tenant-scoped summary endpoint returns the count; the console renders the tile (Decision 3). |
| Timezone edge cases are covered by table-driven tests | `internal/detection` table tests cover 08:00:00 / 21:00:00 boundaries, before/after window, missing/invalid timezone (fail-closed), and IANA zone resolution (incl. any Mexico DST-relevant zone verified against `time.LoadLocation`). |
| Deterministic, no LLM | The detector is a pure function; no model provider, agent loop, or Harness is touched (ADR-03, ADR-09). |

## Architecture / ADR alignment

- **Deterministic-first (ADR-03):** the detector is a pure function, no LLM.
- **Workflow-first (ADR-09):** evaluation is a workflow step, not an agent loop.
- **Clean / hexagonal boundaries + deep modules:** `internal/detection` is a deep
  module behind the trivial `Detector` seam; persistence stays in
  `internal/postgres` adapters behind the existing ports; the console consumes the
  API and never owns tenant isolation.
- **Tenant isolation:** all reads/writes (including the summary aggregate) flow
  through `tenantdb.WithTenantTx` + RLS; no query bypasses RLS.
- **SQL-first persistence:** schema changes land as goose migration
  `00003_contact_hours.sql`; sqlc regenerates access code. No ORM.
- **Evidence fidelity:** timezone is snapshotted onto the interaction at ingest so
  past evaluations are reproducible (precursor to, but not, the #3 ledger).

## Risks and mitigations

| Risk | Severity | Mitigation |
|---|---:|---|
| Boundary rule left ambiguous, causing off-by-one compliance errors | High | Decision 1 fixes the exact half-open rule `[08:00:00, 21:00:00)`; explicit boundary table tests at 08:00:00 and 21:00:00. |
| Silent UTC default mislabels out-of-hours contacts as compliant | High | Decision 2: require timezone at ingest, snapshot it, fail closed to BLOCK on missing/invalid tz. |
| DST / IANA correctness (Mexico abolished DST for most zones in 2022; northern border still observes it) | Medium | Store IANA zone names, resolve via `time.LoadLocation`; table tests include a DST-observing zone to prove correctness. |
| Adding `Evaluation` header + `evaluation_id` breaks issue #1's existing `detector_result_rows` rows/tests | Medium | Additive, nullable `evaluation_id` column (no rename); new `evaluations` table; #1's flat rows remain valid. |
| Summary tile drifts from list truth if computed client-side | Medium | Decision 3: server-computed aggregate inside RLS; console renders one number. |
| Detector seam becomes a shallow wrapper for one impl | Low | Interface kept to one pure method; orchestration stays outside; #7 immediately adds real variation. |
| Scope creep into policy bundles / other detectors / ledger / judge | Medium | Hard non-goals above; reviewers reject anything beyond the contact-hours slice. |
| Single-PR size exceeds 400-line budget | Medium | `size:exception` pre-approved by the user; slice is coherent and splitting would ship half-wired states. |

## Rollback

- Roll back migration `00003_contact_hours.sql` via `make migrate-down` (drops
  `evaluations`, the `evaluation_id` column, and the two timezone columns).
- Delete `internal/detection` and the evaluation orchestration path/adapter.
- Revert the interactions DTO/TS additions and remove the summary endpoint + tile.
- Revert the seed timezone/snapshot additions.
- Issue #1's seed, worker, console list, and #13/#14 auth/RLS remain untouched.

## Proposal question round

No interactive question round was run; the orchestrator supplied the four open
decisions with an instruction to recommend-and-resolve. The proposal resolves all
four (half-open window, required+snapshotted timezone with fail-closed, server
summary endpoint, `Detector` interface now). Assumptions the spec/design should
confirm:

- Evaluation runs synchronously in the read/ingest path for #2 (no River job
  required yet).
- The summary endpoint returns a single out-of-hours count for the current tenant
  (no date-range filtering in this slice).
- `evaluations` is a minimal header (`id`, `tenant_id`, `interaction_event_id`,
  `overall_outcome`, `created_at`); any `policy_bundle_version` column stays inert
  until #6.

If any of these should change (e.g. evaluate via a River job, or add a date range
to the summary), raise it before spec.

## Next recommended phase

Spec and Design (can run in parallel).
