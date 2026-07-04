# Verify Report: Issue #6 Versioned, Immutable PolicyBundle with Reproducible Evaluations

Branch: `feat/issue-6-policy-bundle`. Verdict: **PASS**.

SDD verify gate after apply + judgment-day fix commit `e909044`, before archive.
Design was ratified across 3 judgment-day rounds, code across 2 rounds — this
verify is contract-conformance against spec/tasks, not a fourth adversarial
review.

## Test execution evidence

- `go test ./... -count=1` with `DATABASE_URL`/`APP_DATABASE_URL` pointed at
  local docker-compose Postgres (`vigia-postgres-1`, already migrated to
  version 7 via `make migrate-up`): 22/22 packages `ok`, 0 failures, 0 skips
  — integration tests ran for real, not short-mode-skipped.
- `go test ./internal/postgres/... -run PolicyBundle|CreateBundleVersion|EvaluationStamping -v`:
  all 9 subtests under `TestPolicyBundlesGuardBlocksDirectMutation` +
  `TestPolicyBundlesGuardBlocksTruncate` +
  `TestCreateBundleVersionSupersedesAndAppends` +
  `TestCreateBundleVersionConcurrencySerializesToOneActive` +
  `TestEvaluationStampingWithActiveBundleAndRLSIsolation` — PASS.
- `go test ./internal/evaluation/... ./internal/httpapi/... -v`:
  `TestServiceStampsResolvedBundleVersion` (5 subtests incl. resolver-error
  vs not-found log distinction), `TestServiceReEvaluateInteractionStampsHistoricalVersion`,
  `TestServiceReEvaluateInteractionUnknownBundleFails`,
  `TestServiceReEvaluateInteractionUnknownInteractionFails`,
  `TestServiceReEvaluateInteractionCrossTenantNeverRunsPipeline`,
  `TestReEvaluateInteraction` (4 subtests incl. cross-tenant 404) — PASS.
- `go test ./internal/ledger/... -v`: `TestHashGoldenValueUnchangedWithEmptyPolicyBundleVersion`
  (pinned hash untouched with `version=""`) + `TestHashChangesWithNonEmptyPolicyBundleVersion`
  (stamping is hashed when present) — PASS.
- `go test ./internal/db/... -v`: `TestMigration00007PolicyBundleVersioningCatalog`
  (6 subtests: CHECK constraint, partial unique index, all 4 `ENABLE ALWAYS`
  triggers), extended `TestTenantScopedTablesHaveTenantIDAndRLSEnabled` and
  `TestRestrictedAppRoleIsLeastPrivilege` for `policy_bundles`/`policy_bundle_rules` — PASS.
- `go test ./cmd/seed/... -v`: `TestSeedDevDataIntegration` extended to assert
  every seeded evaluation keeps the pre-#6 sentinel (`policy_bundle_version=""`,
  `policy_bundle_id=NULL`) since seed configures no `BundleResolver` — PASS.
- `apps/console`: `npx tsc --noEmit` clean, zero errors.

## Spec scenario → implementation/test mapping

### Requirement: Policy Bundles and Rule Snapshots Are Append-Only

| Scenario | Evidence |
|---|---|
| Direct UPDATE against `policy_bundle_rules` fails `[integration]` | `TestPolicyBundlesGuardBlocksDirectMutation/any_UPDATE_or_DELETE_on_policy_bundle_rules_fails` — `internal/postgres/policy_bundle_integration_test.go` |
| Direct TRUNCATE against either bundle table fails `[integration]` | `TestPolicyBundlesGuardBlocksTruncate` — asserts both tables reject `TRUNCATE`, rows intact |
| Editing a rule produces a new bundle version `[integration]` | `TestCreateBundleVersionSupersedesAndAppends` — two bundle rows, prior `superseded`, both rule snapshots intact |
| Allowed status transition succeeds `[integration]` | `TestPolicyBundlesGuardBlocksDirectMutation/draft_to_active_...` + `.../active_to_superseded_...` (and `.../active_to_draft_...fails` proves the carve-out is narrow) |

Migration: `db/migrations/00007_policy_bundle_versioning.sql` — 4 `ENABLE
ALWAYS` guard triggers (`policy_bundles_guard_mutation` row-level +
statement-level TRUNCATE per table, `policy_bundle_rules` block-all
row-level + its own TRUNCATE trigger), verified structurally by
`TestMigration00007PolicyBundleVersioningCatalog`.

### Requirement: Rule Snapshots Record Effective Date and Legal Basis

| Scenario | Evidence |
|---|---|
| New rule snapshot carries `effective_date`/`legal_basis` `[integration]` | `TestCreateBundleVersionSupersedesAndAppends` asserts non-null values on new rows; migration 00007 adds columns via add-nullable → backfill → `SET NOT NULL`, before guard triggers are created (ordering confirmed in `db/migrations/00007_policy_bundle_versioning.sql`) |

### Requirement: At Most One Active Bundle Per Tenant and Name

| Scenario | Evidence |
|---|---|
| Concurrent bundle creation cannot yield two active bundles `[integration]` | `TestCreateBundleVersionConcurrencySerializesToOneActive` — two goroutines, `FOR UPDATE` lock serializes them, exactly one active row, no duplicate version. Schema-level enforcement (`policy_bundles_one_active_per_tenant_name` partial unique index) confirmed by `TestMigration00007PolicyBundleVersioningCatalog/policy_bundles_one_active_per_tenant_name_partial_unique_index_exists` |

### Requirement: Evaluations Are Stamped With the Resolved Bundle Version

| Scenario | Evidence |
|---|---|
| New evaluation stamps the real bundle version and FK `[integration]` | `TestEvaluationStampingWithActiveBundleAndRLSIsolation` — `evaluations.policy_bundle_version`/`policy_bundle_id` set, evidence hash incorporates real version; tenant isolation on `BundleResolver` also asserted here |
| No active bundle leaves the field empty as before `[integration]` | `TestServiceStampsResolvedBundleVersion/not_found_resolver_keeps_empty_sentinel` + `.../nil_resolver_keeps_empty_sentinel_(pre-#6_behavior)` + `TestSeedDevDataIntegration` (real no-resolver-configured path) |
| Ledger hash stability (design's load-bearing hazard) | `TestHashGoldenValueUnchangedWithEmptyPolicyBundleVersion` (pinned hex unchanged) + `TestHashChangesWithNonEmptyPolicyBundleVersion` (stamping is hashed when present) |

### Requirement: Reproducible Re-Evaluation Against a Specific Bundle Version

| Scenario | Evidence |
|---|---|
| Re-evaluation stamps the requested historical version `[integration]` | `TestServiceReEvaluateInteractionStampsHistoricalVersion` — same interaction+bundle rerun twice ⇒ identical outcome + `inputs_digest` + hash; `TestReEvaluateInteraction/valid_historical_bundle_id_returns_200...` at the HTTP layer |
| Re-evaluation against an unknown bundle id fails `[integration]` | `TestServiceReEvaluateInteractionUnknownBundleFails`, `TestServiceReEvaluateInteractionUnknownInteractionFails`, `TestReEvaluateInteraction/unknown_bundle_id_returns_a_defined_error_status_and_creates_no_evaluation` |

### Requirement: Console Surfaces the Judging Bundle Version

| Scenario | Evidence |
|---|---|
| Interactions list includes the bundle version column `[integration]` | `TestListInteractionsDistinguishesPolicyBundleVersionNullFromEmpty` (query layer) + `internal/httpapi/httpapi.go` DTO field `PolicyBundleVersion *string` + `apps/console/src/app/interactions/InteractionsTable.tsx` `PolicyBundleVersionCell` renders a visible "Bundle Version" column |
| Unevaluated interaction shows null, not an empty string `[integration]` | Same test, `null` case — `CASE WHEN e.id IS NULL THEN NULL ELSE ... END` convention in `db/queries/interaction_events.sql` |
| Evaluated interaction with no active bundle shows an empty string `[integration]` | Same test, `""` sentinel case, distinct from `null`; console cell distinguishes via `(none)` vs `—` (see diff, `PolicyBundleVersionCell`) |

## Judgment-day round-1 fixes (commit `e909044`) — present and tested

1. Tenant check now precedes `ReEvaluateInteraction` pipeline execution
   (foreign-tenant transcripts no longer reach the judge before the tenant
   check) — `TestServiceReEvaluateInteractionCrossTenantNeverRunsPipeline`
   asserts the detector is never invoked cross-tenant;
   `TestReEvaluateInteraction/cross-tenant_result_never_leaks...` confirms
   404 at the HTTP layer.
2. `resolveActiveBundle` now logs resolver errors distinctly from the
   not-found case (`slog.Error`, `evaluation.resolve_active_bundle_failed`)
   via a nil-safe `Service.Logger` — `TestServiceStampsResolvedBundleVersion/resolver_error_is_logged_distinctly_from_the_not-found_case`.

Both fixes: fail-open `""`/`nil` sentinel behavior unchanged, ledger golden
hashes untouched, no migration files touched by the fix commit.

## GitHub issue #6 acceptance criteria

| # | Criterion | Status |
|---|---|---|
| 1 | Rules are compiled into a versioned, immutable bundle via a PolicyBundleRule snapshot | Met — `CreateBundleVersion` (`internal/postgres/adapters.go`), append-only triggers, `TestCreateBundleVersionSupersedesAndAppends` |
| 2 | Each Evaluation records the bundle version it used | Met — `evaluations.policy_bundle_version`/`policy_bundle_id`, `TestEvaluationStampingWithActiveBundleAndRLSIsolation` |
| 3 | Editing a rule produces a new bundle version; prior versions remain intact | Met — supersede-before-insert in one tx, `TestCreateBundleVersionSupersedesAndAppends` |
| 4 | Re-evaluating an interaction against a specific version is reproducible | Met — `ReEvaluateInteraction` + `POST /v1/interactions/{id}/reevaluate`, determinism proven by `TestServiceReEvaluateInteractionStampsHistoricalVersion` |
| 5 | The console shows which bundle version judged an interaction | Met — `InteractionsTable.tsx` "Bundle Version" column, `apps/console/src/lib/api.ts` `policy_bundle_version: string | null` |

All 5 acceptance criteria are satisfied by implementation + passing tests.

## Tasks.md truthfulness

32/32 checkboxes marked `[x]` across all 7 work units, 0 unchecked. Every
task's referenced file/behavior exists in the diff and is exercised by a
passing test:

- Work Unit 1 (migration + sqlc + catalog test): `db/migrations/00007_policy_bundle_versioning.sql`,
  `internal/db/migration_test.go` — confirmed.
- Work Unit 2 (core types + resolver seam + stamping): `internal/evaluation/service_bundle_test.go`,
  `internal/core/types.go` `Evaluation.PolicyBundleID`,
  `PolicyBundleRule.{EffectiveDate,LegalBasis}` — confirmed.
- Work Unit 3 (CreateBundleVersion + integration tests): `internal/postgres/policy_bundle_integration_test.go`,
  `internal/postgres/adapters.go` `CreateBundleVersion` — confirmed.
- Work Unit 4 (ReEvaluateInteraction + endpoint): `internal/evaluation/reevaluate_test.go`,
  `internal/httpapi/httpapi.go` `POST /v1/interactions/{id}/reevaluate` — confirmed.
- Work Unit 5 (ledger golden-hash guard): `internal/ledger/ledger_test.go` extensions — confirmed, no production code change (as expected).
- Work Unit 6 (console): `apps/console/src/lib/api.ts`, `apps/console/src/app/interactions/InteractionsTable.tsx`,
  `internal/httpapi/httpapi_test.go` — confirmed.
- Work Unit 7 (`cmd/seed` compatibility): `cmd/seed/devdata_integration_test.go` extension — confirmed.

## Design conformance

- Ledger hash stability (load-bearing hazard) verified byte-identical for the
  empty sentinel, and demonstrably different for a real version — both
  golden tests pass.
- Resolver seam is nil-safe (`Service.Resolver BundleResolver`, nil ⇒ no
  stamping) — pre-#6 unit tests remain green with no resolver configured.
- `CreateBundleVersion` follows Decision 6's mandatory
  supersede-before-insert ordering under `SELECT ... FOR UPDATE` (this was
  itself a round-2 judgment-day design fix, already re-verified working via
  `TestCreateBundleVersionSupersedesAndAppends` and the concurrency test).
- Re-evaluation is explicitly non-persisting (`core.Evaluation` returned,
  no `CreateEvaluation` call) per Decision 5 — confirmed by inspection of
  `internal/evaluation/service.go`'s `ReEvaluateInteraction`.
- No `internal/ledger` field-order or Body-schema change — diff touches
  only `internal/ledger/ledger_test.go` (test-only), confirming the
  non-goal was respected.

## Non-goals confirmed honored

- No data-driven rule interpretation: `Service` still hardcodes detector/judge
  wiring; `ReEvaluateInteraction` reruns the same wired pipeline regardless of
  bundle rule content.
- No backfill of historical evaluations' `policy_bundle_version`/`policy_bundle_id`.
- No new console detail page — AC5 met via the existing list page only.
- No policy authoring UI or seeded real rule content beyond what tests need.

## Verdict: PASS

0 CRITICAL, 0 WARNING, 0 SUGGESTION. Full `make test`-equivalent suite
(22/22 packages) green against local Postgres with genuine integration-test
execution (no short-mode skip). All 32 tasks verified against the diff. All
5 GitHub issue #6 acceptance criteria met. Both judgment-day round-1 fix
commits present and covered by dedicated tests.

**Next**: sdd-archive.
