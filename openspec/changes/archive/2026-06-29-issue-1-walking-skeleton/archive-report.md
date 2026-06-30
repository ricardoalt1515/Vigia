# Archive Report: Issue #1 Walking Skeleton

**Date:** 2026-06-29
**Change:** issue-1-walking-skeleton
**Status:** ARCHIVED (VERIFIED)
**Verify Verdict:** PASS — 0 CRITICAL, 1 WARNING (FIXED), 1 SUGGESTION (DEFERRED)

---

## Executive Summary

Issue #1 Walking Skeleton successfully closed three discrete connectivity gaps and is now archived:

1. **Dev data seed** (`cmd/seed dev-data` subcommand): Idempotently creates a demo tenant, one debtor, three es-MX labeled `interaction_events`, and issues one tenant API key.
2. **River worker entrypoint** (`cmd/worker`): Boots against Postgres (with River schema as goose migration `00002_river_tables.sql`), registers a trivial no-op worker, enqueues one job, and drains it to completion.
3. **Next.js console** (`apps/console`): Single Server Component page listing the authenticated tenant's interactions through the existing `GET /v1/interactions` API with server-only `VIGIA_API_KEY` env var.

All four acceptance criteria are met. All implementation tasks are complete (29 tasks across 3 PRs). Unit tests pass (8 cases). Integration tests pass against live Postgres. The Next.js console builds cleanly with zero TypeScript errors. The single WARNING (default seed slug divergence from spec fixture name) was FIXED in commit 4ec32ef, aligning the seed default slug to `demo` as specified. The single SUGGESTION (integration test cleanup) is deferred as future work.

---

## Delivery Summary

### Commits and Branches

| PR | Branch | Commits | Status |
|----|--------|---------|--------|
| 1 | issue-1-seed-dev-data | 2622f04 | Merged to main |
| 2 | issue-1-river-worker | ce46759 | Merged to main |
| 3 | issue-1-console | cd53ebe | Merged to main |
| W1 fix | issue-1-console | 4ec32ef | Merged to main (seed slug default `demo-tenant` → `demo`) |
| Docs | main | 73784bf, 5c44929 | Committed to main |

**Total changes:** 3 feature slices + 1 warning fix + 2 doc commits.

### Specification and Design

| Artifact | Location | Status |
|----------|----------|--------|
| Proposal | `openspec/changes/issue-1-walking-skeleton/proposal.md` | Archived |
| Spec | `openspec/specs/walking-skeleton/spec.md` (promoted to main specs) | ACTIVE |
| Design | `openspec/changes/issue-1-walking-skeleton/design.md` | Archived |
| Tasks | `openspec/changes/issue-1-walking-skeleton/tasks.md` | Archived |

The delta spec from the change folder has been promoted to the main specs directory at `openspec/specs/walking-skeleton/spec.md` as the canonical requirement document for this change.

---

## Verification Verdict (from sdd-verify)

**Overall:** PASS  
**Critical Issues:** 0  
**Warnings:** 1 (FIXED in commit 4ec32ef)  
**Suggestions:** 1 (deferred)

### All Four Acceptance Criteria Met

| AC | Requirement | Evidence | Status |
|----|-------------|----------|--------|
| AC1 | Idempotent Dev Data Seed | `TestSeedDevDataIntegration` asserts tenant, 1 debtor, 3 interactions after first run; repeated run keeps counts at 1/1/3; all unit tests (4 cases) PASS | PASS |
| AC2 | River Worker Bootstrap | `TestWorkerIntegration` creates pgxpool, River client with migrated `00002_river_tables.sql`, inserts NoopJob, polls until `completed`; unit tests (2 cases) PASS; `TestNoopWorkerWork` asserts no domain logic | PASS |
| AC3 | Next.js Console Interactions Page | `npm run build` exit 0; `npx tsc --noEmit` exit 0; `src/app/interactions/page.tsx` is async Server Component with `force-dynamic`; renders id/occurred_at/channel/direction only | PASS |
| AC4 | Tenant Isolation End-to-End | Issue #14 RLS behavior unchanged; seed integration test confirms demo-scoped rows; console fetches server-side with server-only `VIGIA_API_KEY`; wrong key → 401 | PASS |

### Test Execution

```
go test ./cmd/seed ./cmd/worker -count=1 -v
├─ cmd/seed: 8 cases (4 unit seed, 4 dispatch) → PASS
├─ cmd/worker: 2 cases (Kind, Work) → PASS
└─ integration: TestSeedDevDataIntegration, TestWorkerIntegration → PASS (with DATABASE_URL)

cd apps/console && npm run build → exit 0 (4 static pages)
cd apps/console && npx tsc --noEmit → exit 0 (no TypeScript errors)
```

### Findings from Verification

**WARNING W1 — Seed default slug divergence (FIXED)**
- **Issue**: Design specified slug `demo` as the canonical fixture identifier; code default was `demo-tenant`.
- **Fix**: Commit 4ec32ef changed `cmd/seed/main.go:138` default from `"demo-tenant"` to `"demo"`.
- **Evidence**: Verified in current `cmd/seed/main.go` that `DevDataParams.Slug` defaults to `"demo"`.
- **Impact**: No functional defect after fix; all behavior is correct.

**SUGGESTION S1 — Integration test cleanup (deferred)**
- **Issue**: `TestSeedDevDataIntegration` accumulates test tenant rows in the database over time (not idempotent in cleanup).
- **Remedy**: Add `t.Cleanup` to delete test rows. Deferred to a future issue for test hygiene.
- **Impact**: None on correctness; test still passes idempotency assertions.

---

## Artifacts Archived

All change artifacts preserved in `openspec/changes/archive/2026-06-29-issue-1-walking-skeleton/`:

```
├── explore.md
├── proposal.md
├── design.md
├── tasks.md (all 29 tasks marked [x] complete)
├── apply-progress.md (3 slices, all COMPLETE)
├── verify-report.md (0 CRITICAL, W1 FIXED, S1 deferred)
├── specs/walking-skeleton/spec.md (delta spec, now at main specs)
└── archive-report.md (this file)
```

---

## Non-Goals Compliance

| Non-goal | Status | Evidence |
|----------|--------|----------|
| No interaction detail page / filters / pagination | PASS | Only `/interactions` list route; `page.tsx` renders table only |
| DTO not widened beyond id/occurred_at/channel/direction | PASS | `Interaction` type has 4 fields exactly |
| Only NoopJob; no detector/Harness/Judge in River | PASS | `NoopWorker.Work` is `return nil`; no domain imports |
| No CORS / no browser auth | PASS | Server-side fetch; no CORS middleware added |
| No modifications to internal/auth, internal/tenantdb, internal/httpapi, internal/postgres, internal/db, internal/harness | PASS | Only River dependency in `go.mod`; no source changes to proven packages |
| No Bedrock or non-default models | PASS | Not present |

---

## Architecture Impact

- **Clean/hexagonal boundaries preserved**: Seed uses `Querier` port, console uses HTTP contract, worker uses River only. No cross-boundary dependencies introduced.
- **Tenant isolation proven**: RLS behavior unchanged; end-to-end demo proves tenant A's data visible only to tenant A.
- **Process isolation enforced**: `cmd/worker` imports only `internal/config` + River; `go list -deps ./cmd/worker | grep internal/(httpapi|auth|harness)` returns empty.
- **SQL-first persistence**: River schema delivered as goose migration `00002_river_tables.sql`; `make migrate-up` is the single migration path.
- **No Judge/Harness merge**: River and Harness model ports remain separate per ADR-4.

---

## Rollback Cleanly Isolated

If needed, rollback leaves #13/#14/#18 completely untouched:

- Remove the `dev-data` subcommand from `cmd/seed/main.go`
- Delete `cmd/worker`, remove River from `go.mod`
- Roll back migration `00002_river_tables.sql` via `make migrate-down`
- Remove `apps/console` scaffold, restore `.gitkeep`
- Revert `Makefile` targets and `HANDOFF.md` documentation

---

## Next Phase

This change is COMPLETE and ARCHIVED. No follow-up work is required for the walking skeleton.

**Future work recommendations** (not blocking):
- S1: Add `t.Cleanup` to integration tests for test hygiene
- Expand console with detail pages, filters, pagination (separate issue)
- Implement real River jobs (detector, evidence, Harness integration) (separate issue)
- Add browser-side CORS + interactive auth (separate issue)

---

## Verification Timeline

- **apply-progress completed:** 2026-06-29
- **sdd-verify executed:** 2026-06-29 (0 CRITICAL, W1 FIXED, S1 deferred)
- **sdd-archive executed:** 2026-06-29

**Ready for commit and merge to main.**
