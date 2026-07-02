# Design: Issue #2 Contact-Hours Detector

## Technical Approach

Introduce a deep `internal/detection` module behind a trivial `Detector` seam
(one pure method), plus a thin `internal/evaluation` orchestrator that resolves
the snapshot timezone, runs detectors, and persists an `evaluations` header +
`detector_result_rows` child in one `tenantdb.WithTenantTx`. Evaluation runs
synchronously at seed/ingest time. `GET /v1/interactions` gains `outcome` +
`reason` via a LEFT JOIN to the latest evaluation; a new `GET /v1/summary`
returns the tenant out-of-hours count from a SQL aggregate. Console renders both.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| Detector vocabulary | `detection.Outcome` = `pass`/`block` at the seam; persistence maps `block → core.DetectorOutcomeFail` (severity `high`), `pass → DetectorOutcomePass` | Reuse `core.DetectorOutcome` (pass/review/fail) inside the detector | REDECO speaks "in/out of window"; `block` is faithful and table-testable. The existing enum stays the persisted vocabulary, so no migration churn on `detector_result_rows.outcome`. |
| Detector input | Minimal `detection.Interaction{OccurredAt time.Time; DebtorTimezone string}` — not `core.InteractionEvent` | Pass full `core.InteractionEvent` | Keeps the seam narrow and the function pure; #7 detectors add fields without dragging DB types into `internal/detection`. |
| TZ resolution site | Detector receives the raw IANA string and calls `time.LoadLocation` itself; empty/invalid → `block` with rationale | Resolve `*time.Location` in the orchestrator | Fail-closed logic (Decision 2) is the detector's core behaviour and must be table-tested at the seam, not in an I/O layer. |
| Orchestration site | New `internal/evaluation.Service.EvaluateInteraction` loops `[]Detector`, builds `Evaluation`, persists via a new `internal/postgres.EvaluationStore` | Inline in the API handler or the detector | Detector stays pure (Decision 4); persistence stays in the adapter pattern. |
| Interactions outcome join | `ListCurrentTenantInteractions` LEFT JOINs the most-recent evaluation per interaction | Second round-trip per row | One query, RLS-scoped, matches the existing deep-reader shape. |

### Outcome vocabulary and casing (single source of truth)

Three layers each have their own outcome vocabulary; the mapping between them is fixed and
one-directional (detector → persistence → API). There is exactly one place per hop that
translates:

| Layer | Column / type | Stored / emitted values | Set by / mapped where |
|---|---|---|---|
| Detector (seam) | `detection.Outcome` | `"pass"` \| `"block"` | Pure detector output. `block` = out-of-window or unresolvable timezone. |
| Persistence — header | `evaluations.overall_outcome` (text) | `"pass"` \| `"fail"` | `evaluation.Service` maps `block → "fail"`, `pass → "pass"` when building the header. `"fail"` is the value the summary aggregate counts (`WHERE overall_outcome = 'fail'`). |
| Persistence — child | `detector_result_rows.outcome` (`core.DetectorOutcome`) | `"pass"` \| `"fail"` (lowercase; `"review"` unused in #2) | `evaluation.Service` maps `block → core.DetectorOutcomeFail` (severity `high`), `pass → core.DetectorOutcomePass`. Reuses the existing enum — no migration churn on the child column. |
| API — DTO | `outcome` JSON field | `"PASS"` \| `"BLOCK"` \| null | `internal/httpapi` maps the persisted `overall_outcome` to uppercase at serialization: `"pass" → "PASS"`, `"fail" → "BLOCK"`. A missing (unevaluated) row serializes as `null` — never a fabricated `PASS` (spec §Unevaluated interaction). |

Casing rule: **lowercase everywhere inside Go and Postgres; uppercase only at the JSON
boundary.** The detector never emits `"PASS"`/`"BLOCK"`; the API layer is the only place
that upper-cases, and it maps from the persisted `overall_outcome`, not from the raw
`detection.Outcome`. Note the deliberate lexical shift `block ↔ fail`: the detector speaks
window vocabulary (`block`) while persistence reuses the existing `core.DetectorOutcome`
enum (`fail`); the summary query and API mapping both key off the persisted `fail`.

## Data Flow

    seed/ingest ─→ evaluation.Service ─→ [Detector.Evaluate] (pure)
                        │                      │ pass/block + rationale
                        └── WithTenantTx ──→ INSERT evaluations (header)
                                          └→ INSERT detector_result_rows (evaluation_id)

    GET /v1/interactions ─→ InteractionReader ─→ interactions LEFT JOIN evaluations
    GET /v1/summary       ─→ SummaryReader     ─→ COUNT(*) evaluations WHERE overall_outcome = 'fail'

## Schema — `db/migrations/00003_contact_hours.sql`

`debtors.timezone` is a **required** column with **no lingering default** — Decision 2
forbids any silent timezone fallback at debtor creation. The additive migration over
#1 rows uses the add-nullable → backfill → SET NOT NULL sequence so existing rows stay
valid without leaving a default that would mask a missing timezone on future inserts:

```sql
-- Up
ALTER TABLE debtors ADD COLUMN timezone text;                 -- 1. add nullable
UPDATE debtors SET timezone = 'America/Mexico_City'           -- 2. one-time backfill of #1 rows
  WHERE timezone IS NULL;
ALTER TABLE debtors ALTER COLUMN timezone SET NOT NULL;       -- 3. enforce; NO DEFAULT remains
```

After step 3 there is no column default: every future `INSERT INTO debtors` MUST supply
`timezone` explicitly or the write fails. This is the DB-level half of Decision 2; the
app-level guard below is the other half.

- `ALTER TABLE interaction_events ADD COLUMN debtor_timezone text NOT NULL DEFAULT ''` (snapshot; empty means unresolved → detector fails closed. The `''` default is intentional and safe: an empty snapshot is *loud* — the detector BLOCKs on it — unlike a wrong timezone which would silently pass).
- `CREATE TABLE evaluations (id uuid PK default gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants ON DELETE CASCADE, interaction_event_id uuid NOT NULL, overall_outcome text NOT NULL, policy_bundle_version text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), UNIQUE (id, tenant_id), FOREIGN KEY (interaction_event_id, tenant_id) REFERENCES interaction_events(id, tenant_id) ON DELETE CASCADE)` — RLS enabled + `evaluations_tenant_isolation` policy mirroring #1.
- `ALTER TABLE detector_result_rows ADD COLUMN evaluation_id uuid` (nullable, additive) + `FOREIGN KEY (evaluation_id, tenant_id) REFERENCES evaluations(id, tenant_id) ON DELETE CASCADE`.
- Down: drop `detector_result_rows.evaluation_id`, drop `evaluations`, drop `interaction_events.debtor_timezone`, drop `debtors.timezone`.

**App-level guard (Decision 2).** Because the column no longer has a default, `CreateDebtor`
in `db/queries/debtors.sql` takes `timezone` as a required parameter (see File Changes and
Interfaces). Before the write, debtor creation MUST reject an empty or invalid timezone by
validating it with `time.LoadLocation`; only a value that resolves is passed to the query.
A missing/invalid timezone is a loud error at ingest, never a silent default.

## Interfaces / Contracts

```go
// internal/detection
type Interaction struct { OccurredAt time.Time; DebtorTimezone string }
type Window struct { StartHour, EndHour int } // [08:00, 21:00)
type Result struct { Outcome Outcome; Rationale string } // Outcome: "pass"|"block"
type Detector interface { Evaluate(in Interaction) Result }
type ContactHoursDetector struct { Window Window }
```

`EvaluationStore.CreateEvaluation(ctx, tenantID, ...)` and
`CountOutOfHours(ctx, tenantID)` wrap `WithTenantTx` + sqlc. New/changed sqlc queries:
`CreateEvaluation :one`, `CreateDetectorResultRow` gains `evaluation_id`,
`ListCurrentTenantInteractionsWithOutcome :many` (LEFT JOIN), `CountOutOfHoursEvaluations :one`
(`WHERE overall_outcome = 'fail'`), and `CreateDebtor` gains a **required** `timezone`
parameter (regenerated `CreateDebtorParams.Timezone string`) — no DB default backs it, so
every caller (seed, future ingest) supplies a `time.LoadLocation`-validated zone or the
insert fails closed.

## File Changes

| File | Action |
|---|---|
| `db/migrations/00003_contact_hours.sql` | Create |
| `internal/detection/{detector.go,contact_hours.go,*_test.go}` | Create |
| `internal/evaluation/{service.go,*_test.go}` | Create |
| `internal/postgres/adapters.go` | Modify (EvaluationStore, SummaryReader, join mapping) |
| `db/queries/{evaluations.sql,detector_result_rows.sql,interaction_events.sql}` | Modify/Create |
| `db/queries/debtors.sql` | Modify (`CreateDebtor` gains required `timezone` param; `INSERT INTO debtors (tenant_id, external_ref, display_name, timezone) VALUES ($1,$2,$3,$4)` and add `timezone` to every `RETURNING`/`SELECT` column list) |
| `internal/core/types.go` | Modify (`Debtor.Timezone`, `InteractionEvent.DebtorTimezone`) |
| `internal/httpapi/httpapi.go` | Modify (`Outcome`/`Reason` fields, `overall_outcome`→`PASS`/`BLOCK` casing map, `/v1/summary` route) |
| `cmd/api/main.go` | Modify (wire summary reader) |
| `cmd/seed/devdata.go` | Modify (`CreateDebtor` now passes `Timezone: "America/Mexico_City"` explicitly — no default to lean on; snapshot the timezone onto each seeded `interaction_events` row; add an out-of-hours fixture interaction; run the evaluator) |
| `apps/console/src/lib/api.ts`, `.../interactions/page.tsx` | Modify (types, column, tile) |

## Testing Strategy

| Layer | What | How |
|---|---|---|
| Unit | Detector | Table-driven: 07:59:59 block, 08:00:00 pass, 20:59:59 pass, 21:00:00 block, before/after, empty tz block, invalid tz block, `America/Mexico_City` vs DST-observing `America/Tijuana` (the border zone the spec's DST scenario names, spec.md:98) |
| Unit | Evaluation service | Fake detector + fake store; asserts header+child built, outcome mapping |
| Integration | Persistence + summary | `*_integration_test.go`, `testing.Short()` skip, `DATABASE_URL`; assert row linkage + count |
| Handler | API | httptest: DTO carries outcome/reason; `/v1/summary` returns count |

## Assumptions — confirmed

- **Synchronous evaluation**: CONFIRMED. Evaluate at seed/ingest; no River job.
- **Single count, no date range**: CONFIRMED. `/v1/summary` returns one integer.
- **Minimal `evaluations` header**: CONFIRMED, with an inert `policy_bundle_version text DEFAULT ''` for #6.

## Migration / Rollout

Additive, nullable/defaulted columns keep #1 rows valid. Roll back via
`make migrate-down`. Strict TDD: `make test` → `go test ./...`.

## Open Questions

- None blocking.
