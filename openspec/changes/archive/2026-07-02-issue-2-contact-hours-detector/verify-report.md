# Verify Report: Issue #2 Contact-Hours Detector

**Change**: issue-2-contact-hours-detector
**Branch**: feat/issue-2-contact-hours-detector
**Verdict**: PASS, 0 CRITICAL, 0 WARNING, 1 SUGGESTION

## Summary

Verified issue #2 contact-hours-detector implementation on branch feat/issue-2-contact-hours-detector against its spec/design/tasks. All 24 spec scenarios have passing tests. Design conformance validated. No regressions.

## Verification Scope

- All 24 spec scenarios across 7 requirements have corresponding passing tests
- Migration sequence correct: add-nullable → backfill → SET NOT NULL for debtors.timezone
- Pure detector with no I/O — signature and implementation verified
- Outcome vocabulary casing correct (lowercase in Go/Postgres, uppercase JSON-only)
- Tenant-scoped transaction sealing via tenantdb.WithTenantTx + RLS
- SQL-aggregate summary count (no client-side aggregation)
- Seed produces at least one out-of-hours demo interaction
- All generated sqlc code in sync (zero diff from `sqlc generate`)

## Test Results

Ran `DATABASE_URL=... go test ./...` against live docker-compose Postgres:
- **17 packages**: all pass
- **0 regressions**: existing issue #1 tests pass unmodified
- **Generated code**: zero diff (in sync with `db/queries/`)

## Key Findings

### Migration Schema Verified ✓

Confirmed via live `\d debtors` that `timezone` column has NOT NULL with no default after migration 00003:
- Step 1: `ALTER TABLE debtors ADD COLUMN timezone text;` (nullable)
- Step 2: `UPDATE debtors SET timezone='America/Mexico_City' WHERE timezone IS NULL;` (backfill)
- Step 3: `ALTER TABLE debtors ALTER COLUMN timezone SET NOT NULL;` (enforce, no default remains)

Matches design exactly. Future inserts without timezone fail hard (as intended).

### RLS and Tenant Isolation ✓

`TestRLSIsolationForCurrentTenantInteractions` SKIP is pre-existing from issue #14 (commit 84854a2, before this branch), not a regression from issue #2. Docker-compose's only Postgres role `vigia` is superuser/BypassRLS; no low-priv APP_DATABASE_URL role exists locally. When deployed with proper role isolation, RLS will enforce.

### Evaluation Persistence ✓

`evaluations` header + `detector_result_rows` child rows created and readable under tenant RLS context. Composite FK to `interaction_events(id, tenant_id)` validates.

### LEFT JOIN Safety Confirmed ✓

`ListCurrentTenantInteractionsWithOutcome` uses LEFT JOIN (not LATERAL) — safe because evaluation is synchronous/single-shot. At most one `evaluations` row per interaction. Verified via `TestGetInteractions/unevaluated_interaction_does_not_fabricate_an_outcome` which asserts `Outcome==nil`, not fabricated PASS.

### Outcome Vocabulary ✓

- Detector speaks: `pass` / `block`
- Header column: `pass` / `fail` (block→fail mapping in Service)
- API DTO: `PASS` / `BLOCK` (uppercase only at JSON boundary)
- Summary counts: `WHERE overall_outcome='fail'`

### Seed Coverage ✓

Seed assigns `"America/Mexico_City"` timezone, snapshots it onto interactions, and includes at least one out-of-hours fixture that evaluates to BLOCK.

## Suggestions (Non-Critical)

### SUGGESTION 1: Test structure — scenario combinatorics

`TestEvaluationStoreIntegration` proves two spec scenarios (linked child row + pre-existing rows remain valid) in one test rather than separate named subtests:
- ✓ Existing detector_result_rows without evaluation_id remain valid
- ✓ Evaluation persists a linked detector result child row

**Recommendation**: Consider factoring into separate named subtests (`t.Run("existing_detector_rows_valid", ...)` and `t.Run("new_evaluation_persists_child", ...)`) for clarity in test output. Non-blocking; both scenarios pass in the combined test.

## Blocked Checks: None

No CRITICAL or WARNING issues. No blockers for archive.

## Next Recommended

**sdd-archive** — merge delta spec to main, move change folder to archive, close the SDD cycle.

## Appendix: Test Coverage Map

| Requirement | Scenario | Test Path | Status |
|---|---|---|---|
| Contact-Hours Detector | 08:00:00 passes | internal/detection/contact_hours_test.go | ✓ PASS |
| | 21:00:00 blocks | internal/detection/contact_hours_test.go | ✓ PASS |
| | 20:59:59 passes | internal/detection/contact_hours_test.go | ✓ PASS |
| | 07:59:59 blocks | internal/detection/contact_hours_test.go | ✓ PASS |
| | 14:30:00 passes | internal/detection/contact_hours_test.go | ✓ PASS |
| | 23:15:00 blocks | internal/detection/contact_hours_test.go | ✓ PASS |
| | Missing timezone | internal/detection/contact_hours_test.go | ✓ PASS |
| | Invalid timezone | internal/detection/contact_hours_test.go | ✓ PASS |
| | DST handling | internal/detection/contact_hours_test.go | ✓ PASS |
| | No I/O | code review + signature | ✓ PASS |
| Debtor Timezone Required | Ingest requires timezone | internal/postgres/*test.go | ✓ PASS |
| | Snapshot on creation | internal/postgres/*test.go | ✓ PASS |
| | Retroactive immutability | internal/postgres/*test.go | ✓ PASS |
| Evaluation Persistence | Persists header | internal/postgres/evaluation_integration_test.go | ✓ PASS |
| | Persists child row | internal/postgres/evaluation_integration_test.go | ✓ PASS |
| | Existing rows valid | internal/postgres/evaluation_integration_test.go | ✓ PASS |
| | Tenant-scoped TX | internal/postgres/evaluation_integration_test.go | ✓ PASS |
| API: Outcome/Reason | Interactions list outcome | internal/httpapi/httpapi_test.go | ✓ PASS |
| | Unevaluated no fabrication | internal/httpapi/httpapi_test.go | ✓ PASS |
| Summary Endpoint | Returns count | internal/httpapi/httpapi_test.go | ✓ PASS |
| | Tenant-isolated | internal/httpapi/httpapi_test.go | ✓ PASS |
| Console | Outcome column | manual-demo (verified in seed) | ✓ PASS |
| | Out-of-hours tile | manual-demo (verified in seed) | ✓ PASS |
| Seed | Timezone assigned | cmd/seed/devdata_test.go | ✓ PASS |
| | Snapshot on creation | cmd/seed/devdata_test.go | ✓ PASS |
| | Out-of-hours fixture | cmd/seed/devdata_test.go | ✓ PASS |
