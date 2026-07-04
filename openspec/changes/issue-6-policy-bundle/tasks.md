# Tasks: Issue #6 Versioned, Immutable PolicyBundle with Reproducible Evaluations

Delivery: single-pr (user decision, `size:exception` pre-approved). No PR
chaining. Strict TDD: `make test` must pass after every task marked
`[unit]`/`[integration]`. Tasks are grouped into work units per
`work-unit-commits`; each work unit ends at a green `make test` state and
keeps its tests (and docs, where user-visible) in the same commit.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~950–1150 |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Suggested split | Single PR (work-unit commits internally) |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

The `size:exception` was pre-approved by the user for this change; `sdd-apply`
should proceed directly with the work-unit commit sequence below rather than
asking again.

### Suggested Work Units

| Unit | Goal | Notes |
|------|------|-------|
| 1 | Migration 00007 + sqlc regen + catalog test | Foundation; must land first |
| 2 | Core types + BundleResolver seam + stamping | Unit-only, no DB |
| 3 | CreateBundleVersion + trigger/TRUNCATE/concurrency/RLS integration tests | Depends on Unit 1 |
| 4 | ReEvaluateInteraction + `POST /reevaluate` | Depends on Units 2–3 |
| 5 | Ledger golden-hash regression guard | Depends on Unit 2 |
| 6 | Console DTO/type/column | Depends on Unit 3 (query change) |
| 7 | `cmd/seed` compatibility check | Depends on Units 1–3 |

---

## Work Unit 1 — Migration 00007 + sqlc regeneration + catalog test

Satisfies: *Policy Bundles and Rule Snapshots Are Append-Only*, *Rule
Snapshots Record Effective Date and Legal Basis*, *At Most One Active
Bundle Per Tenant and Name* (schema half).

- [x] 1.1 Write `db/migrations/00007_policy_bundle_versioning.sql` Up, in
      order: (a) `ALTER TABLE policy_bundle_rules ADD effective_date date`
      nullable, `ADD legal_basis text NOT NULL DEFAULT ''`; (b) backfill
      `UPDATE policy_bundle_rules SET effective_date = created_at::date`;
      (c) `ALTER COLUMN effective_date SET NOT NULL`; (d) `ALTER TABLE
      evaluations ADD policy_bundle_id uuid` + `FOREIGN KEY
      (policy_bundle_id, tenant_id) REFERENCES policy_bundles(id,
      tenant_id)` + supporting index; (e) `ADD CONSTRAINT
      policy_bundles_status_check CHECK (status IN
      ('draft','active','superseded'))`; (f) `CREATE UNIQUE INDEX
      policy_bundles_one_active_per_tenant_name ON policy_bundles
      (tenant_id, name) WHERE status = 'active'`; (g) the four `ENABLE
      ALWAYS` guard triggers — `policy_bundles_guard_mutation()` carve-out
      function (per design.md's exact `RAISE EXCEPTION` blocks) wired as
      `BEFORE UPDATE OR DELETE`, plus its own `BEFORE TRUNCATE FOR EACH
      STATEMENT` trigger; `policy_bundle_rules` block-all `BEFORE UPDATE OR
      DELETE` trigger (reusing the evidence-ledger pattern) plus its own
      `BEFORE TRUNCATE FOR EACH STATEMENT` trigger; (h) `GRANT SELECT ON
      policy_bundles, policy_bundle_rules TO vigia_app`. The guard triggers
      MUST be created AFTER the backfill (step b), not before, or the
      backfill's own UPDATE fails against its own guard.
- [x] 1.2 Write the Down migration: revoke grants, drop the partial unique
      index, drop the CHECK constraint, drop all four triggers + both
      trigger functions, drop the evaluations FK + index + column, drop
      `effective_date`/`legal_basis` columns.
- [x] 1.3 Run `make migrate-up` locally; verify no errors and existing
      seed/tests still pass against the migrated schema.
- [x] 1.4 Modify `db/queries/policies.sql`: update `AddPolicyBundleRule` to
      accept `effective_date`/`legal_basis` params; add
      `GetActiveBundleByTenant :one` and `CountBundleVersions :one`
      (scoped to `tenant_id, name`).
- [x] 1.5 Modify `db/queries/evaluations.sql`: `CreateEvaluation` INSERT +
      RETURNING gains `policy_bundle_version`, `policy_bundle_id`.
- [x] 1.6 Modify `db/queries/interaction_events.sql`:
      `ListCurrentTenantInteractionsWithOutcome` adds
      `e.policy_bundle_version` using the same `CASE WHEN e.id IS NULL
      THEN NULL ELSE ... END` convention as `threat_flagged`.
- [x] 1.7 Run `make sqlc` to regenerate `internal/db`; verify `go build
      ./...` succeeds with the new generated types.
- [x] 1.8 [unit] Extend `internal/db/migration_test.go`'s grant-check table
      list with `policy_bundles`, `policy_bundle_rules`: assert both have
      non-null `tenant_id`, RLS enabled, `vigia_app` has SELECT and no
      write privileges, the CHECK constraint exists, the partial unique
      index exists, and all four triggers are present with `ENABLE
      ALWAYS`.

Verification: `make migrate-up` succeeds; `go build ./...` succeeds; `go
test ./internal/db/... -short` green.

---

## Work Unit 2 — Core types + BundleResolver seam + evaluation stamping

Satisfies: *Evaluations Are Stamped With the Resolved Bundle Version*
(`[unit]` half).

- [ ] 2.1 [unit] Write table-driven tests in `internal/evaluation` (extend
      or create `service_bundle_test.go`) before implementing: fake
      `BundleResolver` returning `(version, id, found=true, nil)` ⇒
      `CreateEvaluationInput.PolicyBundleVersion`/`PolicyBundleID` carry
      those values; `found=false` or nil `Resolver` ⇒ `""`/`nil`
      (nil-safe, no panic); resolver error ⇒ evaluation still proceeds
      with sentinel values (per design Decision 3, evaluation must not
      hard-fail on a missing/erroring bundle).
- [ ] 2.2 Add `BundleResolver` interface to `internal/evaluation`:
      `ActiveBundle(ctx, tenantID) (version, id string, found bool, err
      error)`. Add `Resolver BundleResolver` field to `Service`. Modify
      `internal/core/types.go`: `Evaluation.PolicyBundleID *string`,
      `PolicyBundleRule.{EffectiveDate, LegalBasis}`.
- [ ] 2.3 Wire the resolver call into `EvaluateInteraction` before
      `CreateEvaluation`: nil resolver or error or `found=false` ⇒ keep
      existing `""`/`nil` sentinel path unchanged (existing pre-#6 unit
      tests must stay green with no resolver configured).
- [ ] 2.4 Implement the `BundleResolver` Postgres adapter in
      `internal/postgres/adapters.go` backed by `GetActiveBundleByTenant`.
      Pass `PolicyBundleVersion`/`PolicyBundleID` through to
      `CreateEvaluation`'s new params (Work Unit 1.5).

Verification: `go test ./internal/evaluation/... -v` green with no
`DATABASE_URL` required.

---

## Work Unit 3 — CreateBundleVersion + trigger/TRUNCATE/concurrency/RLS integration tests

Satisfies: *Policy Bundles and Rule Snapshots Are Append-Only*
(`[integration]` half), *At Most One Active Bundle Per Tenant and Name*
(`[integration]` half), *Evaluations Are Stamped With the Resolved Bundle
Version* (`[integration]` half).

- [ ] 3.1 [integration] Write
      `internal/postgres/policy_bundle_integration_test.go`
      (`testing.Short()` skip, requires `DATABASE_URL`) before
      `CreateBundleVersion` exists, covering: owner-conn direct `UPDATE`
      of a non-`status` column on `policy_bundles` fails, row unchanged;
      owner-conn `DELETE` on `policy_bundles` fails; `draft→active` and
      `active→superseded` status-only updates succeed with no other
      column changed; `active→draft` fails; any `UPDATE`/`DELETE` on
      `policy_bundle_rules` fails; owner-conn `TRUNCATE policy_bundles`
      and `TRUNCATE policy_bundle_rules` both fail, rows intact after the
      failed attempt.
- [ ] 3.2 [integration] Extend the same file: `CreateBundleVersion` called
      twice for the same `(tenant, name)` produces two bundle rows, the
      prior marked `superseded`, both rule-snapshot sets intact and
      queryable, `effective_date`/`legal_basis` non-null on the new rows.
- [ ] 3.3 [integration] Add a concurrency case: two goroutines call
      `CreateBundleVersion` concurrently for the same `(tenant, name)`;
      assert the `FOR UPDATE` lock serializes them, exactly one resulting
      active row, no duplicate `(tenant_id, name, version)`.
- [ ] 3.4 [integration] Add a stamping + RLS case: evaluate an interaction
      for a tenant with an active bundle ⇒ `evaluations.policy_bundle_id`
      set and version non-empty, evidence hash incorporates the real
      version; tenant A cannot resolve tenant B's bundle via
      `BundleResolver`.
- [ ] 3.5 Implement `CreateBundleVersion(ctx, tenantID, name, rules)
      (core.PolicyBundle, error)` in `internal/postgres/adapters.go`: one
      tenant tx — `SELECT ... FOR UPDATE` the prior active row scoped to
      `(tenant_id, name)`, `UPDATE` it to `superseded` FIRST, THEN `INSERT`
      the new bundle (`status='active'`, `version='vN'` via
      `CountBundleVersions`+1) and its rule-snapshot rows via
      `AddPolicyBundleRule`. Supersede-before-insert ordering is mandatory
      (the partial unique index is non-deferrable).

Verification: `go test ./internal/postgres/... -run PolicyBundle -v`
green against local Postgres; clean skip under `go test ./... -short`.

---

## Work Unit 4 — ReEvaluateInteraction + reevaluate endpoint

Satisfies: *Reproducible Re-Evaluation Against a Specific Bundle Version*.

- [ ] 4.1 [unit] Write table-driven tests before implementing: same
      interaction + same historical `policyBundleID` rerun twice ⇒
      identical outcome + `inputs_digest` + computed hash (determinism);
      unknown/foreign-tenant `policyBundleID` ⇒ defined error, no
      evaluation persisted.
- [ ] 4.2 Implement `Service.ReEvaluateInteraction(ctx, interactionID,
      policyBundleID string) (core.Evaluation, error)` in
      `internal/evaluation/service.go`: reruns the same wired
      detectors/judge, stamps the caller-supplied historical version+id,
      returns an **unpersisted** `core.Evaluation` (no `CreateEvaluation`
      call).
- [ ] 4.3 [integration] Write `internal/httpapi/httpapi_test.go` cases
      before wiring: `POST /v1/interactions/{id}/reevaluate` with a valid
      historical bundle id returns 200 with the stamped version/id;
      unknown bundle id returns a defined error status and creates no
      evaluation row.
- [ ] 4.4 Wire `POST /v1/interactions/{id}/reevaluate` in
      `internal/httpapi/httpapi.go`, authenticated via existing tenant
      auth, calling `Service.ReEvaluateInteraction`.

Verification: `go test ./internal/evaluation/... ./internal/httpapi/...
-v` green.

---

## Work Unit 5 — Ledger golden-hash regression guard

Satisfies: *Evaluations Are Stamped With the Resolved Bundle Version*
(ledger-hash-stability half, referenced in design's load-bearing hazard).

- [ ] 5.1 [unit] Extend `internal/ledger` golden-hash test: assert the
      existing pinned hex hash is unchanged when `PolicyBundleVersion =
      ""` (byte-identical Body, no regression from schema/field additions
      elsewhere). Add a second case: a non-empty `PolicyBundleVersion`
      value produces a **different** hash than the empty-sentinel case,
      proving stamping is hashed when present.
- [ ] 5.2 No production code change expected in this unit — if the golden
      test fails, treat it as a blocking regression against Work Units 1–4
      and fix the regression, not the pinned literal.

Verification: `go test ./internal/ledger/... -v` green, golden literal
untouched.

---

## Work Unit 6 — Console surfaces the judging bundle version

Satisfies: *Console Surfaces the Judging Bundle Version*.

- [ ] 6.1 [integration] Extend `internal/httpapi/httpapi_test.go` (console
      interactions list handler) with cases: an evaluated interaction
      under bundle version `v2` ⇒ response `policy_bundle_version =
      "v2"`; an unevaluated interaction ⇒ `policy_bundle_version = null`
      (not `""`); an evaluated interaction with no active bundle at
      evaluation time (sentinel `""`) ⇒ `policy_bundle_version = ""`,
      distinct from `null`.
- [ ] 6.2 Add `PolicyBundleVersion *string` to the httpapi `Interaction`
      DTO in `internal/httpapi/httpapi.go`, mapped from Work Unit 1.6's
      query column with no `null`/`""` coercion.
- [ ] 6.3 Update `apps/console/src/lib/api.ts`: `Interaction` type gains
      `policy_bundle_version: string | null`.
- [ ] 6.4 Update `apps/console/src/app/interactions/InteractionsTable.tsx`:
      add a visible `policy_bundle_version` column, rendering `null` as an
      explicit empty/dash state distinct from an empty string.

Verification: `go test ./internal/httpapi/... -v` green; console
`npm run build` (or project's equivalent typecheck) succeeds.

---

## Work Unit 7 — `cmd/seed` compatibility check

Satisfies: no new scenario — confirms Work Units 1–3 do not break existing
seed/demo data paths.

- [ ] 7.1 [integration] Extend the seed integration test
      (`cmd/seed/devdata_test.go` or equivalent, `testing.Short()` skip):
      after `cmd/seed dev-data` runs against a fresh migrated database,
      seeded evaluations still succeed with no active bundle (sentinel
      `policy_bundle_version = ""`, `policy_bundle_id = NULL`), and
      existing evidence-ledger/golden-hash seed assertions remain
      unaffected.
- [ ] 7.2 If `cmd/seed` directly inserts `policy_bundle_rules` rows,
      update those call sites to supply `effective_date`/`legal_basis`
      (now `NOT NULL`) via the updated `AddPolicyBundleRule` query.

Verification: `go test ./cmd/seed/... -v` green.

---

## Sequencing summary

1. Work Unit 1 (migration + sqlc + catalog test) — no dependencies, must
   land first.
2. Work Unit 2 (core types + resolver seam) — depends on Work Unit 1's
   generated types only; no DB required, can be developed in parallel
   with Work Unit 1 up to the point of needing regenerated `internal/db`
   types.
3. Work Unit 3 (CreateBundleVersion + integration tests) — depends on
   Work Unit 1 (migrated schema, generated queries) and Work Unit 2
   (`BundleResolver` interface).
4. Work Unit 4 (ReEvaluateInteraction + endpoint) — depends on Work Units
   2–3 (stamping mechanism + a way to obtain historical bundle ids).
5. Work Unit 5 (ledger golden-hash guard) — depends on Work Unit 2
   (stamped `PolicyBundleVersion` flowing into `ledger.Body`); can run in
   parallel with Work Unit 3.
6. Work Unit 6 (console) — depends on Work Unit 1.6 (query column) and
   Work Unit 2 (stamping produces real values to display).
7. Work Unit 7 (`cmd/seed` check) — depends on Work Units 1–3; lands last
   as a full-spine sanity check.

Parallelizable: Work Unit 2 (pure Go, no DB) and Work Unit 5 (pure
`internal/ledger`) can be developed in parallel with Work Unit 3's
integration-test-heavy track by a second contributor if this were split
across people; for single-PR single-author delivery, sequence 1 → 2 → 3 →
4 → 5 → 6 → 7 to keep the failing-test-then-implementation rhythm clean
per commit.
