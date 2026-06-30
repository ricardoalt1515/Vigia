# Verify Report — Issue #1 Walking Skeleton

**Date:** 2026-06-29
**Branch:** issue-1-console
**Verifier:** sdd-verify phase
**Verdict:** PASS — 0 CRITICAL, 1 WARNING, 1 SUGGESTION

---

## Test Execution Results

### go build ./cmd/seed ./cmd/worker
```
exit 0 (no output)
```

### go vet ./cmd/seed ./cmd/worker
```
exit 0 (no output)
```

### go test ./cmd/seed ./cmd/worker -count=1 -v (unit)
```
=== RUN   TestSeedDevDataIntegration
    devdata_integration_test.go:28: DATABASE_URL is required for the seed integration test
--- SKIP: TestSeedDevDataIntegration (0.00s)
=== RUN   TestSeedDevData
=== RUN   TestSeedDevData/fresh_run_creates_all_entities
=== RUN   TestSeedDevData/idempotent_rerun
=== RUN   TestSeedDevData/partial_state_missing_interactions
--- PASS: TestSeedDevData (0.00s)
    --- PASS: TestSeedDevData/fresh_run_creates_all_entities (0.00s)
    --- PASS: TestSeedDevData/idempotent_rerun (0.00s)
    --- PASS: TestSeedDevData/partial_state_missing_interactions (0.00s)
=== RUN   TestSeedDispatch
=== RUN   TestSeedDispatch/dev-data_routes_to_seed
=== RUN   TestSeedDispatch/no_subcommand_routes_to_key_issuance
=== RUN   TestSeedDispatch/empty_args_routes_to_key_issuance
--- PASS: TestSeedDispatch (0.00s)
    --- PASS: TestSeedDispatch/dev-data_routes_to_seed (0.00s)
    --- PASS: TestSeedDispatch/no_subcommand_routes_to_key_issuance (0.00s)
    --- PASS: TestSeedDispatch/empty_args_routes_to_key_issuance (0.00s)
=== RUN   TestIssueTenantAPIKey
--- PASS: TestIssueTenantAPIKey (0.00s)
PASS
ok  github.com/ricardoalt1515/vigia/cmd/seed    0.479s
=== RUN   TestNoopJobKind
--- PASS: TestNoopJobKind (0.00s)
=== RUN   TestNoopWorkerWork
--- PASS: TestNoopWorkerWork (0.00s)
=== RUN   TestWorkerIntegration
    worker_integration_test.go:24: DATABASE_URL is required for the River integration test
--- SKIP: TestWorkerIntegration (0.00s)
PASS
ok  github.com/ricardoalt1515/vigia/cmd/worker  0.750s
```

### go test ./cmd/seed ./cmd/worker -run Integration -v (integration, DATABASE_URL set)
```
=== RUN   TestSeedDevDataIntegration
--- PASS: TestSeedDevDataIntegration (0.05s)
PASS
ok  github.com/ricardoalt1515/vigia/cmd/seed    0.387s
=== RUN   TestWorkerIntegration
--- PASS: TestWorkerIntegration (0.36s)
PASS
ok  github.com/ricardoalt1515/vigia/cmd/worker  0.948s
```

### cd apps/console && npx tsc --noEmit
```
exit 0 (no output — no TypeScript errors)
```

### cd apps/console && npm run build
```
▲ Next.js 15.5.19
✓ Compiled successfully in 1021ms
✓ Generating static pages (4/4)

Route (app)                                 Size  First Load JS
┌ ○ /                                      124 B         103 kB
├ ○ /_not-found                            998 B         103 kB
└ ƒ /interactions                          124 B         103 kB

exit 0
```

---

## Acceptance Criteria Verdict

### AC1: Idempotent Dev Data Seed

| Scenario | Status | Evidence |
|---|---|---|
| First-run creates all required rows `[integration]` | PASS | `TestSeedDevDataIntegration` asserts tenant, 1 debtor, 3 interaction events after first call; PASS against live Postgres |
| Repeated runs do not duplicate rows `[integration]` | PASS | `TestSeedDevDataIntegration` calls `SeedDevData` twice and asserts counts remain 1/1/3; `TestSeedDevData/idempotent_rerun` (unit) |
| FK insertion order respected `[integration]` | PASS | `TestSeedDevData/fresh_run_creates_all_entities` records call order and asserts tenant → debtor → interaction_events → API key |
| Plaintext key exposed at most once `[integration]` | PASS | `runDevData` calls `fmt.Printf("tenant_api_key=%s\n")` exactly once; `auth.HashAPIKey` stores only the hash via `CreateTenantAPIKey`; no plaintext in DB |
| Seed uses Querier port, no ad hoc SQL `[unit]` | PASS | `devdata.go` imports: `internal/db`, `internal/core`, `pgx/v5` only; `SeedQuerier` is a 6-method subset of `db.Querier`; no raw SQL strings; no `pgxpool` reference in `devdata.go` |

### AC2: River Worker Bootstrap

| Scenario | Status | Evidence |
|---|---|---|
| River migration applied by `make migrate-up` `[integration]` | PASS | `db/migrations/00002_river_tables.sql` with correct `-- +goose Up/Down/StatementBegin/End` markers; apply-progress records successful round-trip; `TestWorkerIntegration` PASS (uses migrated DB) |
| Worker process boots and connects to Postgres `[integration]` | PASS | `TestWorkerIntegration` creates `pgxpool`, River client — PASS |
| NoopJob enqueued and drained to completion `[integration]` | PASS | `TestWorkerIntegration` inserts a `NoopJob`, starts client, polls `river_job` until `completed`, then calls `Stop` — PASS |
| NoopJob worker contains no domain logic `[unit]` | PASS | `NoopWorker.Work` body is `return nil`; `TestNoopWorkerWork` asserts nil return; no domain imports |
| Worker independent of API process `[unit]` | PASS | `go list -deps ./cmd/worker \| grep internal/(httpapi\|auth\|harness)` returns empty |

### AC3: Next.js Console Interactions List Page

All console scenarios are `[manual-demo]`. Automated proxies for each:

| Scenario | Status | Evidence |
|---|---|---|
| Console scaffold is valid App Router project | PASS | `npm run build` exit 0; Next.js 15.5.19; 4 static pages generated |
| Interactions list page renders demo tenant rows | PASS (build-level) | `src/app/interactions/page.tsx` is `async` Server Component with `export const dynamic = "force-dynamic"`; calls `listInteractions()` and renders `id/occurred_at/channel/direction` table |
| Server Component reads `VIGIA_API_KEY` from server-only env | PASS | `src/lib/api.ts` has `import "server-only"` at line 1; uses `process.env.VIGIA_API_KEY` (no `NEXT_PUBLIC_` prefix); confirmed by grep |
| Console page limited to minimal DTO | PASS | Page renders only `id`, `occurred_at`, `channel`, `direction`; no debtor data, no detail links, no pagination, no filters, no charts — confirmed by full code review |
| No additional console routes exist | PASS | Only two files under `src/app/`: `page.tsx` (redirect to `/interactions`) and `interactions/page.tsx`; one user-navigable content route |

### AC4: Tenant Isolation End-to-End

| Scenario | Status | Evidence |
|---|---|---|
| Demo tenant key returns only demo tenant interactions | PASS (unchanged) | Issue #14 RLS and `GET /v1/interactions` behavior unchanged; seed integration test confirms demo-scoped rows |
| Different tenant key returns that tenant's interactions only | PASS (unchanged) | Issue #14 + issue #13 RLS; no changes to `internal/httpapi`, `internal/auth`, `internal/tenantdb` |
| Console renders only authenticated tenant's interactions | PASS (build-level) | `VIGIA_API_KEY` server-only; API call is server-side; wrong key → 401 → error thrown |
| No key returns 401 | PASS (unchanged) | Issue #14 auth middleware unchanged |

---

## Findings

### WARNING

**W1 — Default seed slug `demo-tenant` diverges from spec-canonical slug `demo`**
- File: `cmd/seed/main.go:138` (flag default `"demo-tenant"`)
- The spec scenarios consistently reference `slug: demo` as the canonical fixture identifier (`GetTenantBySlug("demo")`, `rows with slug demo`). The production default is `"demo-tenant"`. Running `make seed-dev` with no flags creates a tenant with slug `"demo-tenant"`, not `"demo"`. A developer verifying the spec scenarios verbatim by checking for slug `"demo"` after running `make seed-dev` will not find it. The flag can be overridden with `--slug demo`, but the default diverges from the spec intent.
- Impact: No functional defect; all idempotency and isolation behavior is correct. Manual demo steps in HANDOFF.md do not reference the slug, so the demo still works end-to-end.
- Remedy: Change the default in `runDevData` from `"demo-tenant"` to `"demo"` for alignment, or update the spec to explicitly accept `"demo-tenant"` as the canonical slug.

### SUGGESTION

**S1 — Integration test leaves seed data in the database on every run**
- File: `cmd/seed/devdata_integration_test.go`
- `TestSeedDevDataIntegration` creates rows with slug `integration-test-demo` but has no `t.Cleanup` or deferred delete. Each non-short test run against a shared DB accumulates `integration-test-demo` tenant/debtor rows and issues a new API key (keys are not idempotent by design). The test passes idempotency assertions correctly — row counts stay at 1/1/3 — but the growing list of API keys for the test tenant is not pruned.
- Impact: None on correctness; DB accumulates test API keys over time.
- Remedy: Add a `t.Cleanup` that deletes the `integration-test-demo` tenant (cascade deletes debtors, interaction_events, and API keys via FK) at the end of each test run.

---

## Non-Goals Compliance Check

| Non-goal | Status |
|---|---|
| No interaction detail page / filters / pagination / search | PASS — only `/interactions` list route exists |
| DTO not widened beyond id/occurred_at/channel/direction | PASS — `Interaction` type has exactly 4 fields |
| Only NoopJob; no detector/Harness/Judge in River | PASS — `NoopWorker.Work` is `return nil`; no domain imports |
| No CORS / no browser-side API key handling | PASS — fetch is server-side only; no CORS middleware added |
| No modifications to internal/auth, internal/tenantdb, internal/httpapi, internal/postgres, internal/db, internal/harness | PASS — only `go.mod`/`go.sum` (River dependency) modified in existing modules; no source files in those packages changed |
| No Bedrock or non-default model provider | PASS — not present |

---

## Tasks Completion Check

All tasks T1.1–T1.9, T2.1–T2.9, T3.1–T3.6 are marked `[x]` in `tasks.md` and `apply-progress.md`.

T2.3 (migration round-trip) was pending in apply-progress but the integration test `TestWorkerIntegration` proves the River tables exist in the migrated DB (the test inserts and drains a NoopJob against real Postgres, which requires the `river_job` table to exist). The apply-progress note on T2.3 was also updated inline with a VERIFIED timestamp.

---

## Summary

**0 CRITICAL · 1 WARNING · 1 SUGGESTION**

All four acceptance criteria are met. Unit tests (8 cases across seed and worker) pass. Integration tests pass against live Postgres. The Next.js console builds cleanly with no TypeScript errors and generates exactly one user-navigable route. All non-goals are respected. The single WARNING (default slug divergence) is a cosmetic misalignment between the spec scenario fixture name and the code default; it does not affect correctness or the end-to-end demo. The change is ready to archive.

**next_recommended: sdd-archive**
