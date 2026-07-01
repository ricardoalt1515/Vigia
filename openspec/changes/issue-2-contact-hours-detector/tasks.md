# Tasks: Issue #2 Contact-Hours Detector

Delivery: single-pr (user decision, size:exception pre-approved). No PR
chaining. Strict TDD: `make test` (`go test ./...`) must pass after every
task marked `[unit]`/`[integration]`. Tasks are grouped into work units per
`work-unit-commits`; each work unit keeps its tests (and docs, where
user-visible) in the same commit.

Spec scenario references use the spec's own scenario titles
(`spec.md` §Requirement headers). `[unit]`/`[integration]`/`[manual-demo]`
tags mirror the spec's testing-mode annotations.

---

## Work Unit 1 — Schema migration + sqlc regeneration

Satisfies: *Debtor Timezone Is Required and Snapshotted*, *Evaluation
Persistence via Header + Child Rows* (schema half).

- [ ] 1.1 Write `db/migrations/00003_contact_hours.sql` (Up + Down) exactly
      per design.md §Schema:
      - `debtors.timezone` via add-nullable → backfill
        (`'America/Mexico_City'`) → `SET NOT NULL` (no default remains)
      - `interaction_events.debtor_timezone text NOT NULL DEFAULT ''`
      - `CREATE TABLE evaluations (...)` with RLS enabled +
        `evaluations_tenant_isolation` policy mirroring issue #1's pattern,
        composite FK `(interaction_event_id, tenant_id)` to
        `interaction_events(id, tenant_id)`
      - `detector_result_rows.evaluation_id uuid` (nullable, additive) +
        composite FK to `evaluations(id, tenant_id)`
      - Down: drop `detector_result_rows.evaluation_id`, drop `evaluations`,
        drop `interaction_events.debtor_timezone`, drop `debtors.timezone`
- [ ] 1.2 Run `make migrate-up` against local Postgres; verify no errors and
      that issue #1 seed/tests still pass against the migrated schema.
- [ ] 1.3 Update `db/queries/debtors.sql`: `CreateDebtor` gains required
      `timezone` param (`INSERT INTO debtors (tenant_id, external_ref,
      display_name, timezone) VALUES ($1,$2,$3,$4)`); add `timezone` to
      `RETURNING`/`SELECT` column lists in `CreateDebtor`, `GetDebtorByTenant`,
      `ListDebtorsByTenant`.
- [ ] 1.4 Create `db/queries/evaluations.sql`: `CreateEvaluation :one`,
      `CountOutOfHoursEvaluations :one` (`WHERE overall_outcome = 'fail'`),
      tenant-scoped.
- [ ] 1.5 Update `db/queries/detector_result_rows.sql`:
      `CreateDetectorResultRow` gains `evaluation_id` param/column.
- [ ] 1.6 Update `db/queries/interaction_events.sql`: add
      `ListCurrentTenantInteractionsWithOutcome :many` (LEFT JOIN latest
      evaluation per interaction); snapshot `debtor_timezone` on the existing
      create-interaction query.
- [ ] 1.7 Run sqlc regeneration (`sqlc generate` / repo's generate target)
      to refresh `internal/db` generated code for all query changes above.
      Verify generated `CreateDebtorParams.Timezone string` and new
      generated methods compile (`go build ./...`).

Verification: `make migrate-up` succeeds; `go build ./...` succeeds;
existing issue #1 tests referencing `detector_result_rows`/`debtors` still
pass (`go test ./internal/postgres/... -short`).

---

## Work Unit 2 — `internal/core` type additions

Satisfies: *Debtor Timezone Is Required and Snapshotted* (domain types).

- [ ] 2.1 Add `Timezone string` to `Debtor` and `DebtorTimezone string` to
      `InteractionEvent` in `internal/core/types.go`. Add/extend
      `Evaluation` core type if not already covering `overall_outcome`
      (per design.md §Interfaces).

Verification: `go build ./...` succeeds; no behavior change yet (types
only), no test file required for this unit alone (covered transitively by
Work Unit 3+ tests).

---

## Work Unit 3 — Pure `internal/detection` package (test-first)

Satisfies: *Contact-Hours Detector Is a Pure, Fail-Closed Function* (all
`[unit]` scenarios).

- [ ] 3.1 [unit] Write `internal/detection/contact_hours_test.go`: table-driven
      test with cases for each spec scenario before any implementation
      exists (must fail to compile/run first):
      - 08:00:00 local → PASS
      - 21:00:00 local → BLOCK (half-open boundary)
      - 20:59:59 local → PASS
      - 07:59:59 local → BLOCK
      - 14:30:00 local → PASS
      - 23:15:00 local → BLOCK
      - empty `DebtorTimezone` → BLOCK, rationale states timezone missing
      - invalid IANA string → BLOCK, rationale states timezone invalid
      - `America/Tijuana` DST vs non-DST instant → outcome matches
        DST-adjusted local time, not naive offset
- [ ] 3.2 Implement `internal/detection/detector.go` (`Interaction`,
      `Window`, `Result`, `Outcome`, `Detector` interface) and
      `internal/detection/contact_hours.go` (`ContactHoursDetector`) per
      design.md §Interfaces, satisfying all cases from 3.1. Detector method
      accepts only `Interaction` + `Window`; no I/O, no `time.Now()`.
- [ ] 3.3 [unit] Add a reflection/code-review style test (or clear
      documentation-by-signature) proving "Detector performs no I/O": assert
      `Evaluate(in Interaction) Result` signature has no context/DB
      params; no `time.Now()` call in the implementation (grep-based test
      or code comment + manual review acceptable given it's a signature
      constraint, not a runtime behavior).

Verification: `go test ./internal/detection/... -run TestContactHours -v`
green for all 9+ table cases; `go vet ./internal/detection/...` clean.

---

## Work Unit 4 — Evaluation orchestration + persistence (test-first)

Satisfies: *Evaluation Persistence via Header + Child Rows* (all scenarios).

- [ ] 4.1 [unit] Write `internal/evaluation/service_test.go` using a fake
      `Detector` and fake `EvaluationStore`: asserts
      `Service.EvaluateInteraction` builds an evaluation header + child row
      with correct outcome mapping (`block → fail`/`DetectorOutcomeFail`
      severity `high`, `pass → pass`/`DetectorOutcomePass`) before the
      service exists.
- [ ] 4.2 Implement `internal/evaluation/service.go`
      (`Service.EvaluateInteraction`): loops `[]detection.Detector`, builds
      `Evaluation` header + `detector_result_rows` child, calls
      `EvaluationStore.CreateEvaluation`.
- [ ] 4.3 [integration] Write
      `internal/postgres/evaluation_integration_test.go`
      (`testing.Short()` skip, requires `DATABASE_URL`): seed a tenant +
      interaction, run evaluation, assert:
      - `evaluations` header row created with composite FK to
        `interaction_events` and correct `tenant_id`, readable only under
        that tenant's RLS context
      - `detector_result_rows` child row created with `evaluation_id` set,
        carrying outcome + rationale
      - both writes occur inside a single `tenantdb.WithTenantTx` call (no
        superuser/migration-owner bypass)
      - pre-existing (issue #1-style) `detector_result_rows` rows with no
        `evaluation_id` remain queryable and valid
- [ ] 4.4 Implement `internal/postgres.EvaluationStore`
      (`CreateEvaluation`, `CountOutOfHoursEvaluations`) in
      `internal/postgres/adapters.go`, using generated sqlc code from Work
      Unit 1, wrapped in `tenantdb.WithTenantTx`.

Verification: `go test ./internal/evaluation/... -v` green (unit, no
external deps); `go test ./internal/postgres/... -run Evaluation -v` green
against local Postgres (skips clean under `-short`).

---

## Work Unit 5 — API: outcome/reason on interactions list + summary endpoint (test-first)

Satisfies: *Interactions API Exposes Outcome and Reason*, *Tenant-Scoped
Out-of-Hours Summary Endpoint*.

- [ ] 5.1 [integration] Extend/add `internal/httpapi/httpapi_test.go`
      cases (httptest, before wiring): `GET /v1/interactions` DTO includes
      non-null `outcome`/`reason` for an evaluated interaction and
      null/empty (not fabricated `PASS`) for an unevaluated one; new
      `GET /v1/summary` returns the tenant's exact out-of-hours count and
      is tenant-isolated (two tenants, two counts, no cross-leak).
- [ ] 5.2 Implement DTO + mapping in `internal/httpapi/httpapi.go`:
      `Outcome`/`Reason` fields sourced from
      `ListCurrentTenantInteractionsWithOutcome`; casing map
      `overall_outcome` (`"pass"→"PASS"`, `"fail"→"BLOCK"`, missing→`null`)
      at the JSON boundary only.
- [ ] 5.3 Implement `GET /v1/summary` handler + `SummaryReader` (backed by
      `CountOutOfHoursEvaluations`, SQL aggregate — no in-app counting);
      wire route.
- [ ] 5.4 Wire the new summary reader in `cmd/api/main.go`.

Verification: `go test ./internal/httpapi/... -v` green; manual `curl` spot
check optional (not required for TDD gate).

---

## Work Unit 6 — Console: outcome column + out-of-hours tile

Satisfies: *Console Shows Outcome and Out-of-Hours Tile* (`[manual-demo]`
— no automated test required by spec; verified via seed + local dev run).

- [ ] 6.1 Update `apps/console/src/lib/api.ts`: extend interaction type
      with `outcome`/`reason`, add a summary-endpoint client call/type.
- [ ] 6.2 Update `apps/console/src/app/interactions/page.tsx`: render
      outcome/reason column per row; render out-of-hours tile fed by the
      summary endpoint response (no client-side aggregation of the list).

Verification: manual demo per spec `[manual-demo]` scenarios — run
`cmd/seed dev-data`, start console, confirm outcome column and tile render
against seeded out-of-hours interaction (see Work Unit 7).

---

## Work Unit 7 — Seed: timezone + out-of-hours fixture (test-first)

Satisfies: *Seed Provides Timezone and an Out-of-Hours Demo Interaction*.

- [ ] 7.1 [integration] Write/extend a seed integration test
      (`cmd/seed/devdata_test.go` or existing equivalent,
      `testing.Short()` skip) asserting: demo debtor has non-empty IANA
      `Timezone`; each seeded `interaction_events` row's `debtor_timezone`
      matches the debtor's timezone at seed time; at least one seeded
      interaction evaluates to `BLOCK` (its debtor-local wall-clock time
      falls outside `[08:00:00, 21:00:00)`).
- [ ] 7.2 Implement in `cmd/seed/devdata.go`: pass
      `Timezone: "America/Mexico_City"` explicitly to `CreateDebtor`;
      snapshot that timezone onto each created `interaction_events` row; add
      an out-of-hours fixture interaction (local time outside the window);
      run `internal/evaluation.Service.EvaluateInteraction` over seeded
      interactions so evaluations exist for the API/console to surface.

Verification: `go test ./cmd/seed/... -v` green; running `cmd/seed dev-data`
against a migrated local DB followed by manual console check (Work Unit 6)
completes the `[manual-demo]` scenarios.

---

## Sequencing summary

1. Work Unit 1 (migration + sqlc) — no dependencies, must land first.
2. Work Unit 2 (core types) — depends on Unit 1's schema decisions, no code
   dependency; can be folded into Unit 1's commit or its own tiny commit.
3. Work Unit 3 (pure detector) — depends only on Unit 2's types existing
   conceptually; in practice `internal/detection` types are self-contained
   and can be built in parallel with Units 1–2.
4. Work Unit 4 (evaluation service + persistence) — depends on Units 1–3
   (needs generated sqlc code + detector).
5. Work Unit 5 (API) — depends on Unit 4 (needs evaluations to read) and
   Unit 1 (needs `ListCurrentTenantInteractionsWithOutcome`).
6. Work Unit 6 (console) — depends on Unit 5 (needs the DTO/endpoint shape).
7. Work Unit 7 (seed) — depends on Units 1–4 (needs `CreateDebtor` timezone
   param + evaluation service); should land last since it exercises the
   full spine end to end and unblocks the `[manual-demo]` scenarios in
   Unit 6.

Parallelizable: Work Unit 3 (pure, no DB dependency) can be developed in
parallel with Work Unit 1 by a second contributor if this were split across
people; for a single-PR single-author delivery, sequence 1 → 2 → 3 → 4 → 5
→ 6 → 7 to keep the failing-test-then-implementation rhythm clean per
commit.

---

## Review Workload Forecast

- **Estimated changed lines (rough)**: ~750–950 lines total across:
  - migration SQL (~40 lines)
  - sqlc query files + regenerated `internal/db` code (~150–250 lines,
    much of it generated boilerplate)
  - `internal/detection` (~120 lines incl. table-driven test)
  - `internal/evaluation` (~150 lines incl. unit test)
  - `internal/postgres` adapter additions (~80 lines)
  - `internal/httpapi` DTO/route additions + tests (~120 lines)
  - `cmd/seed` changes + test (~80 lines)
  - console changes (~60 lines)
- **Chained PRs recommended**: No — user already selected `single-pr` with
  pre-approved `size:exception`. This section is informational only per
  the Review Workload Guard; no action is required from `sdd-apply`.
- **400-line budget risk**: High (total estimate exceeds 400 lines), but
  already covered by the pre-approved `size:exception`. Work-unit commits
  (per `work-unit-commits` skill) should still be used internally so the
  single PR reads as a clear story, even though it will not be split into
  multiple PRs.
- **Decision needed before apply**: No. The single-pr + size:exception
  decision was already made by the user; `sdd-apply` should proceed
  directly using the work-unit commit sequence above.
