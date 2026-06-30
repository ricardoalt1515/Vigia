# Apply Progress — Issue #1 Walking Skeleton

## Slice 1: Go seed dev-data subcommand (PR 1)

**Status:** COMPLETE
**Branch:** issue-1-seed-dev-data
**Completed:** 2026-06-29

### Tasks completed

- [x] T1.1 RED: `TestSeedDevData/fresh_run_creates_all_entities` written and failing
- [x] T1.2 GREEN: `cmd/seed/devdata.go` implemented (`SeedQuerier` port, `SeedDevData`, fixtures)
- [x] T1.3 RED: `TestSeedDevData/idempotent_rerun` written and failing
- [x] T1.4 GREEN: `GetTenantBySlug` / list-and-match guards added; idempotency confirmed
- [x] T1.5 TRIANGULATE: `TestSeedDevData/partial_state_missing_interactions` written and passing
- [x] T1.6 RED: `TestSeedDispatch` written (routing test via `routeArgs`) and failing
- [x] T1.7 GREEN: `routeArgs` + dispatch in `cmd/seed/main.go`; `defaultKeyIssuer` adapter wired
- [x] T1.8 REFACTOR: confirmed `devdata.go` imports only `internal/db`, `internal/core`, `pgx/v5`; no raw SQL; no `pgxpool` reference in `devdata.go`
- [x] T1.9 Integration test: `cmd/seed/devdata_integration_test.go` added; skips on `-short` or missing `DATABASE_URL`

### Files created / modified

| File | Action |
|------|--------|
| `cmd/seed/devdata.go` | Created — `SeedQuerier`, `KeyIssuer`, `DevDataParams`, `DevDataResult`, `DevDataCounts`, `SeedDevData`, `devDataFixtures`, `isNotFound`, `uuidToString` |
| `cmd/seed/devdata_test.go` | Created — table-driven unit tests: `TestSeedDevData/{fresh_run,idempotent_rerun,partial_state}`, `TestSeedDispatch/{dev-data,no_subcommand,empty_args}` |
| `cmd/seed/devdata_integration_test.go` | Created — `TestSeedDevDataIntegration` (skippable; mirrors `rls_isolation_test.go` skip pattern) |
| `cmd/seed/main.go` | Modified — added `defaultKeyIssuer`, `routeArgs`, `run` dispatch, `runDevData`, `runKeyIssuance` (backward compatible) |

### Verification output

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
ok  github.com/ricardoalt1515/vigia/cmd/seed 0.336s

go build ./... → exit 0
go test ./... -short -count=1 → all packages PASS
```

### Design decisions applied

- **Owner-role RLS bypass**: seed inserts through `DATABASE_URL` (migration/owner role) with no `WithTenantTx`, matching the proven `rls_isolation_test.go` seeding pattern.
- **`SeedQuerier` minimal port**: 6-method interface (subset of `db.Querier`) so unit tests use an in-memory fake — no Docker required for the unit suite.
- **`KeyIssuer` interface**: wraps the existing `IssueTenantAPIKey` free function via `defaultKeyIssuer` adapter; key issuance runs on every call (plaintext not recoverable from hash).
- **Idempotency guards**: `GetTenantBySlug` → `isNotFound(pgx.ErrNoRows)` → create; debtor matched by `external_ref` in list; interactions matched by `transcript_ref` in list.
- **FK order enforced**: tenant → debtor → interaction_events → API key (asserted by call-order check in test).
- **Backward compatibility**: existing key-issuance path (`run` with `--tenant-id`) preserved verbatim in `runKeyIssuance`.

---

## Slice 2: River worker + goose migration (PR 2)

**Status:** COMPLETE (T2.3 migration round-trip pending manual execution — Postgres not running in apply environment)
**Branch:** issue-1-river-worker
**Completed:** 2026-06-29
**River version pinned:** v0.39.0

### Tasks completed

- [x] T2.1 SETUP: `go get github.com/riverqueue/river@v0.39.0` + `go get github.com/riverqueue/river/riverdriver/riverpgxv5@v0.39.0`; `go mod tidy`; `go build ./...` → exit 0
- [x] T2.2 MIGRATION: `db/migrations/00002_river_tables.sql` created; SQL generated via `go run github.com/riverqueue/river/cmd/river@v0.39.0 migrate-get`; wrapped verbatim in `-- +goose Up / StatementBegin/End / -- +goose Down / StatementBegin/End`
- [ ] T2.3 MIGRATION ROUND-TRIP: **PENDING** — requires `make dev` (Postgres not available in apply environment). User must run `make migrate-up && make migrate-down` to verify.
- [x] T2.4 RED: `TestNoopJobKind` written and failing (NoopJob not defined yet)
- [x] T2.5 GREEN: `cmd/worker/noop.go` created with `NoopJob` + `Kind() == "noop"`
- [x] T2.6 RED: `TestNoopWorkerWork` written and failing (NoopWorker not defined yet)
- [x] T2.7 GREEN: `NoopWorker` added to `cmd/worker/noop.go`; unit tests PASS
- [x] T2.8 WIRING: `cmd/worker/main.go` created; imports only `internal/config` + River; `go build ./cmd/worker` → exit 0; no forbidden imports (`internal/httpapi`, `internal/auth`, `internal/harness` absent from dep graph)
- [x] T2.9 INTEGRATION TEST: `cmd/worker/worker_integration_test.go` added; skips on `-short` or missing `DATABASE_URL`

### Files created / modified

| File | Action |
|------|--------|
| `cmd/worker/noop.go` | Created — `NoopJob` (river.JobArgs), `NoopWorker` (river.WorkerDefaults[NoopJob]) |
| `cmd/worker/noop_test.go` | Created — `TestNoopJobKind`, `TestNoopWorkerWork` |
| `cmd/worker/main.go` | Created — `run(ctx)`: config → pool → River client → insert NoopJob → start → graceful shutdown |
| `cmd/worker/worker_integration_test.go` | Created — `TestWorkerIntegration` (skippable; polls river_job until completed) |
| `db/migrations/00002_river_tables.sql` | Created — River v0.39.0 schema wrapped in goose markers |
| `go.mod` | Modified — added river v0.39.0, riverpgxv5 v0.39.0, and transitive deps |
| `go.sum` | Modified — updated for River v0.39.0 and pgx v5.9.2 upgrade |

### Verification output

```
=== RUN   TestNoopJobKind
--- PASS: TestNoopJobKind (0.00s)
=== RUN   TestNoopWorkerWork
--- PASS: TestNoopWorkerWork (0.00s)
=== RUN   TestWorkerIntegration
    worker_integration_test.go:24: DATABASE_URL is required for the River integration test
--- SKIP: TestWorkerIntegration (0.00s)
PASS
ok  github.com/ricardoalt1515/vigia/cmd/worker 0.321s

go build ./... → exit 0
go vet ./cmd/worker → exit 0
go test ./... -short -count=1 → all packages PASS
```

### Design decisions applied

- **River version v0.39.0**: latest stable v0.x tag; both `river` and `riverpgxv5` pinned to same tag; `riverpgxv5` added as direct dep.
- **Migration generated, not hand-authored**: `migrate-get --line main --up --all` and `--down --all` output captured verbatim; goose `StatementBegin/End` wraps the entire block (matches existing `00001_initial_foundation.sql` pattern).
- **No rivermigrate at startup**: `cmd/worker/main.go` calls no migration code; `make migrate-up` is the single migration path.
- **Process isolation enforced**: `go list -deps ./cmd/worker | grep internal/(httpapi|auth|harness)` returns nothing.
- **pgx upgraded**: River v0.39.0 requires pgx v5.9.2; `go get` upgraded it from v5.7.5 automatically.

## Slice 3: Next.js console + Makefile targets (PR 3)

**Status:** COMPLETE
**Branch:** issue-1-console
**Completed:** 2026-06-29
**Next.js version:** 15.5.19 (resolved at install time)

### Confirmed API JSON envelope shape

Canonical shape from `internal/httpapi/httpapi.go` (`interactionsResponse` struct, line 44–46):
```json
{ "interactions": [ { "id": "...", "occurred_at": "...", "channel": "...", "direction": "..." } ] }
```
`occurred_at` is a Go `time.Time`, serialized as RFC 3339 string.
`api.ts` handles the canonical `{ interactions: [...] }` envelope first; tolerates bare array for forward compatibility.

### Tasks completed

- [x] T3.1 SCAFFOLD: `.gitkeep` removed; `package.json`, `tsconfig.json`, `next.config.ts`, `postcss.config.mjs`, `.gitignore`, `.env.example` created
- [x] T3.2 APP SHELL: `src/app/globals.css`, `src/app/layout.tsx`, `src/app/page.tsx` (redirect to `/interactions`) created
- [x] T3.3 DATA LAYER: `src/lib/api.ts` with `import "server-only"`, `Interaction` type, `listInteractions()` — no `NEXT_PUBLIC_` prefix
- [x] T3.4 INTERACTIONS PAGE: `src/app/interactions/page.tsx` async Server Component with `export const dynamic = "force-dynamic"` and semantic `<table>`
- [x] T3.5 MAKEFILE: `worker`, `seed-dev`, `console-install`, `console-dev` targets added; existing Go targets untouched
- [x] T3.6 DOCS: `HANDOFF.md` updated with step-by-step end-to-end run order

### Files created / modified

| File | Action |
|------|--------|
| `apps/console/.gitignore` | Created — `.env.local`, `node_modules`, `.next` |
| `apps/console/.env.example` | Created — `VIGIA_API_KEY=`, `VIGIA_API_BASE_URL=http://localhost:8080`, end-to-end run order comment |
| `apps/console/package.json` | Created — next@^15.3.0, react@^19.0.0, react-dom@^19.0.0, server-only; tailwindcss@^4.0.0, @tailwindcss/postcss@^4.0.0, typescript@^5.0.0 |
| `apps/console/package-lock.json` | Created — lock file (47 packages) |
| `apps/console/tsconfig.json` | Created — strict, moduleResolution: bundler, `@/*` path alias; Next.js added `target: ES2017` during build |
| `apps/console/next.config.ts` | Created — empty NextConfig export |
| `apps/console/postcss.config.mjs` | Created — `@tailwindcss/postcss` plugin |
| `apps/console/next-env.d.ts` | Created by Next.js — TypeScript env types |
| `apps/console/src/app/globals.css` | Created — `@import "tailwindcss"` |
| `apps/console/src/app/layout.tsx` | Created — RootLayout, metadata, slate-50 body |
| `apps/console/src/app/page.tsx` | Created — `redirect("/interactions")` |
| `apps/console/src/app/interactions/page.tsx` | Created — async Server Component, `force-dynamic`, semantic table |
| `apps/console/src/lib/api.ts` | Created — `server-only`, `Interaction` type, `listInteractions()` |
| `Makefile` | Modified — added `worker`, `seed-dev`, `console-install`, `console-dev` targets |
| `HANDOFF.md` | Modified — added walking skeleton demo end-to-end run order |

### Verification output

```
npm install → 47 packages, exit 0

npx tsc --noEmit → exit 0 (no TypeScript errors)

npm run build:
   ▲ Next.js 15.5.19
 ✓ Compiled successfully in 2.9s
 ✓ Generating static pages (4/4)

Route (app)          Size   First Load JS
○ /                  124 B  103 kB     (static redirect)
ƒ /interactions      124 B  103 kB     (dynamic — force-dynamic)

build exit: 0
```

### Design decisions applied

- **`force-dynamic` on interactions page**: prevents Next.js from pre-rendering the page at build time, which would fail because `VIGIA_API_KEY` is not set in the build environment.
- **`server-only` import**: compile-time guard that prevents `listInteractions` from being accidentally bundled in a Client Component.
- **Canonical envelope first**: `api.ts` checks `{ interactions: [...] }` before the bare-array fallback, matching the actual API implementation.
- **No Geist font package**: layout uses Tailwind's system font stack for the walking skeleton; font customization deferred to later slices.
- **No CORS added**: console fetches from the Go API server-side only (Next.js Server Component), so CORS is not needed and was not added.
- **`tsconfig.tsbuildinfo` not committed**: build artifact; gitignored via Next.js convention.
