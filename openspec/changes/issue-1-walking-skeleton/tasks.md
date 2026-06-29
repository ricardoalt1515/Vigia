# Issue #1 Walking Skeleton — Implementation Tasks

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Go seed bucket (cmd/seed devdata.go + devdata_test.go + main.go dispatch) | ~250–320 lines |
| Go worker + migration bucket (noop.go + noop_test.go + main.go + integration test + migration SQL) | ~350–500 lines |
| Next.js console scaffold bucket (all apps/console files) | ~400–550 lines |
| Total estimate | ~1000–1370 lines |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (Go seed) → PR 2 (Go worker + migration) → PR 3 (Next.js console + Makefile) |
| Delivery strategy | ask-on-risk |
| Chain strategy | not yet set — surface split decision to user before apply |

Decision needed before apply: Yes
Chained PRs recommended: Yes
400-line budget risk: High

---

## Blockers / stop conditions

- [ ] Confirm issue #14 endpoint shape for the JSON envelope (`{interactions:[...]}` vs bare array `[...]`) before apply; `api.ts` already tolerates both, but verify against `internal/httpapi` to record the canonical shape.
- [ ] Resolve the River module version (latest stable v0.x tag) at apply start; use the same tag for `river`, `riverpgxv5`, and `migrate-get`; record it in apply-progress.
- [ ] Do not widen scope: no detail page, no filters, no pagination controls, no DTO changes, no CORS, no real River jobs, no Harness/Judge integration.
- [ ] Stop if `internal/auth`, `internal/tenantdb`, `internal/httpapi`, `internal/postgres`, `internal/db`, or `internal/harness` require modification; this change must consume those packages as-is.

---

## PR 1 — Go seed dev-data subcommand

All tasks in this work unit are strictly sequential. Tests use a fake `SeedQuerier` — no real DB required for unit steps.

- [x] **T1.1** [test-first · RED] Write unit test case `TestSeedDevData/fresh_run_creates_all_entities` in `cmd/seed/devdata_test.go` (table-driven). Use a fake `SeedQuerier` and fake `KeyIssuer`. Assert that a fresh DB (all list ops return empty) results in exactly one `CreateTenant`, one `CreateDebtor`, three `CreateInteractionEvent` calls (matching the three stable `transcript_ref` fixtures), and one key issuance. Record call order in the fake to assert FK ordering: tenant → debtor → interaction_events → key. Expect test to fail (no `SeedDevData` yet).
  - Verification: `go test ./cmd/seed -run TestSeedDevData/fresh_run -count=1`

- [x] **T1.2** [GREEN] Create `cmd/seed/devdata.go`. Define `SeedQuerier` interface (six methods: `GetTenantBySlug`, `CreateTenant`, `ListDebtorsByTenant`, `CreateDebtor`, `ListInteractionEventsByTenant`, `CreateInteractionEvent`). Define `DevDataParams`, `DevDataResult`, `DevDataCounts`. Implement `SeedDevData(ctx, SeedQuerier, KeyIssuer, DevDataParams) (DevDataResult, error)` happy path: insert tenant → debtor → three interactions (using `core.InteractionChannel*` and `core.InteractionDirection*` constants and the three stable `transcript_ref`s) → issue key. Return plaintext key in result. No ad hoc SQL.
  - Verification: `go test ./cmd/seed -run TestSeedDevData/fresh_run -count=1`

- [x] **T1.3** [test-first · RED] Add unit test case `TestSeedDevData/idempotent_rerun` to the table. `GetTenantBySlug` returns an existing tenant row; `ListDebtorsByTenant` returns the existing debtor (matching `external_ref`); `ListInteractionEventsByTenant` returns all three interactions (matching stable `transcript_ref`s). Assert zero `CreateTenant`, zero `CreateDebtor`, zero `CreateInteractionEvent` calls; one fresh key issuance (API key is not idempotent by design). Expect test to fail.
  - Verification: `go test ./cmd/seed -run TestSeedDevData/idempotent_rerun -count=1`

- [x] **T1.4** [GREEN] Add `GetTenantBySlug` / `ListDebtorsByTenant` / `ListInteractionEventsByTenant` guards to `SeedDevData`: reuse existing entities if found; skip `Create*` calls for already-present rows. Key issuance always runs and plaintext is always printed.
  - Verification: `go test ./cmd/seed -run TestSeedDevData/idempotent_rerun -count=1`

- [x] **T1.5** [TRIANGULATE] Add unit test case `TestSeedDevData/partial_state_missing_interactions`: tenant and debtor already exist, but `ListInteractionEventsByTenant` returns only one of the three. Assert zero `CreateTenant`, zero `CreateDebtor`, exactly two `CreateInteractionEvent` calls for the missing `transcript_ref`s. Verify the per-entity guard already handles this; adjust if not.
  - Verification: `go test ./cmd/seed -run TestSeedDevData/partial_state -count=1`

- [x] **T1.6** [test-first · RED] Add unit test `TestSeedDispatch` in `cmd/seed/devdata_test.go` (or a dedicated `main_test.go`). Assert that passing `args = ["dev-data"]` routes to the seed path and calling `run(ctx, []string{"--tenant-id", "...", "--label", "..."})` without `dev-data` still routes to the legacy key-issuance path unchanged. Expect test to fail (dispatch not wired yet).
  - Verification: `go test ./cmd/seed -run TestSeedDispatch -count=1`

- [x] **T1.7** [GREEN] Update `cmd/seed/main.go`: extend `run(ctx, args)` to inspect `args[0]`; if equal to `"dev-data"`, parse dev-data flags and call `SeedDevData`, printing `tenant_api_key=<plaintext>` to stdout. Otherwise fall through to the current key-issuance path verbatim (backward compatible). Wire `SeedQuerier` via `*db.Queries` on the owner-role `pgxpool` (`cfg.DatabaseURL`); wire `KeyIssuer` via existing `TenantAPIKeyCreator`. Defaults make `seed dev-data` runnable with zero flags.
  - Verification: `go test ./cmd/seed -run TestSeedDispatch -count=1`

- [x] **T1.8** [REFACTOR] Confirm `cmd/seed/devdata.go` imports only `internal/db`, `internal/auth` (KeyIssuer), and `internal/core`; no raw SQL strings outside generated `internal/db`; no direct `pgxpool` reference in `devdata.go`. Review error wrapping and boundary clarity.
  - Verification: `go test ./cmd/seed -count=1` (full package, all unit tests green)

- [x] **T1.9** [integration · skippable] Add `cmd/seed/devdata_integration_test.go`. Guard with `testing.Short()` and a present `DATABASE_URL` (skip if either). Call `SeedDevData` twice against a real Postgres. After both calls: assert exactly one `app.tenants` row with slug `demo`, exactly one `app.debtors` row owned by that tenant, exactly three `app.interaction_events` rows owned by that debtor, exit status 0. Mirror the skip pattern from `internal/db/rls_isolation_test.go`.
  - Verification: `go test ./cmd/seed -run TestSeedDevDataIntegration -count=1` (requires DB)

---

## PR 2 — River worker entrypoint + goose migration

Within this work unit, T2.2–T2.3 (migration) and T2.4–T2.7 (noop unit tests) are independent of each other and can run in parallel after T2.1 is complete. T2.8 (main.go wiring) requires both tracks done. T2.9 (integration) requires T2.8.

- [x] **T2.1** [setup · blocking] Resolve latest stable River v0.x tag at apply time. Run `go get github.com/riverqueue/river@<tag>` and `go get github.com/riverqueue/river/riverdriver/riverpgxv5@<tag>` using the same tag. Run `go mod tidy`. Verify `go build ./...` succeeds. Record the resolved tag in apply-progress for the migration step.
  - Verification: `go build ./...`

- [x] **T2.2** [migration · parallel-after-T2.1] Generate River's canonical schema SQL using the pinned tag:
  ```
  go run github.com/riverqueue/river/cmd/river@<tag> migrate-get --line main --up   --all > /tmp/river_up.sql
  go run github.com/riverqueue/river/cmd/river@<tag> migrate-get --line main --down --all > /tmp/river_down.sql
  ```
  Create `db/migrations/00002_river_tables.sql` by wrapping the captured SQL verbatim in goose markers (`-- +goose Up`, `-- +goose StatementBegin`, `-- +goose StatementEnd`, `-- +goose Down`). Do not hand-author any River SQL.
  - Verification: file exists and goose markers are syntactically correct (`./bin/goose postgres $DATABASE_URL validate`)

- [ ] **T2.3** [migration verification · sequential-after-T2.2] Run `make migrate-up` against a local Postgres. Verify all River-required tables exist. Run `make migrate-down`. Verify tables are removed. Confirm round-trip is clean (exit code 0 both directions). Document the outcome in apply-progress.
  - Verification: `make migrate-up && make migrate-down` (manual; exit 0 both)
  - **BLOCKED**: Postgres not running in apply environment. Run `make dev` first, then execute this step manually.

- [x] **T2.4** [test-first · RED · parallel-after-T2.1] Write `cmd/worker/noop_test.go` with unit test `TestNoopJobKind`: assert `NoopJob{}.Kind() == "noop"`. Expect test to fail (file does not exist yet).
  - Verification: `go test ./cmd/worker -run TestNoopJobKind -count=1`

- [x] **T2.5** [GREEN] Create `cmd/worker/noop.go`. Define `NoopJob struct{}` implementing `river.JobArgs` with `Kind() string { return "noop" }`.
  - Verification: `go test ./cmd/worker -run TestNoopJobKind -count=1`

- [x] **T2.6** [test-first · RED] Add unit test `TestNoopWorkerWork` to `noop_test.go`: assert `(&NoopWorker{}).Work(ctx, &river.Job[NoopJob]{}) == nil`. Verify `Work` never calls any detector, Harness, Judge, MCP, or domain behavior (white-box assertion: only `return nil` in body). Expect test to fail (NoopWorker not defined yet).
  - Verification: `go test ./cmd/worker -run TestNoopWorkerWork -count=1`

- [x] **T2.7** [GREEN] Add `NoopWorker struct{ river.WorkerDefaults[NoopJob] }` to `cmd/worker/noop.go`. Implement `func (NoopWorker) Work(ctx context.Context, job *river.Job[NoopJob]) error { return nil }`. No domain imports.
  - Verification: `go test ./cmd/worker -run TestNoopWorkerWork -count=1`

- [x] **T2.8** [sequential-after-T2.3+T2.7] Create `cmd/worker/main.go`. Implement `run(ctx)`: load config, open `pgxpool` over `cfg.DatabaseURL`, create `river.NewWorkers()`, `river.AddWorker(workers, &NoopWorker{})`, `river.NewClient(riverpgxv5.New(pool), &river.Config{Queues: ..., Workers: workers})`, `client.Insert(ctx, NoopJob{}, nil)`, `client.Start(ctx)`, wait for `SIGINT`/`SIGTERM` via `signal.NotifyContext`, `client.Stop(shutdownCtx)`. Imports: `internal/config` + River only. Must NOT import `internal/httpapi`, `internal/auth`, or `internal/harness`.
  - Verification: `go build ./cmd/worker` (compilation proof)

- [x] **T2.9** [integration · skippable · sequential-after-T2.8] Create `cmd/worker/worker_integration_test.go`. Guard with `testing.Short()` and `DATABASE_URL` present. Build a real River client against migrated Postgres. Insert one `NoopJob`. Call `Start`. Poll `river_job` table (or River completion subscription) until the job reaches `completed` state with a timeout. Assert completion. Call `Stop`. Confirm `Stop` returns without error.
  - Verification: `go test ./cmd/worker -run TestWorkerIntegration -count=1` (requires migrated DB)

---

## PR 3 — Next.js console scaffold + Makefile targets

Tasks T3.1–T3.4 are sequential (scaffold before app structure before data layer before page). T3.5 (Makefile) and T3.6 (docs) are independent of each other and of T3.1–T3.4, but are logically grouped at the end.

- [ ] **T3.1** [scaffold] Remove `apps/console/.gitkeep`. Create the minimal App Router scaffold files: `package.json` (dependencies: `next`, `react`, `react-dom`, `typescript`, `tailwindcss` v4, `@tailwindcss/postcss`; scripts: `dev`, `build`, `start`), `tsconfig.json`, `next.config.ts`, `postcss.config.mjs` (`@tailwindcss/postcss`), `.gitignore` (`.env.local`, `node_modules`, `.next`), `.env.example` (`VIGIA_API_KEY=`, `VIGIA_API_BASE_URL=http://localhost:8080`).
  - Verification: `cd apps/console && npm install && npm run build` (or at minimum `npx next build` — fails gracefully with no pages yet, which is expected)

- [ ] **T3.2** [app shell] Create `src/app/globals.css` (`@import "tailwindcss";`), `src/app/layout.tsx` (root layout with Geist font import and `globals.css`), `src/app/page.tsx` (redirects to `/interactions` or renders a shell — no business content here).
  - Verification: `npm run build` completes without TypeScript errors on the layout/root page

- [ ] **T3.3** [data layer] Create `src/lib/api.ts`. Add `import "server-only"` at top. Define the `Interaction` type (`id`, `occurred_at`, `channel`, `direction`). Implement `listInteractions()`: read `VIGIA_API_BASE_URL` and `VIGIA_API_KEY` from `process.env`; throw if key is absent; fetch `GET /v1/interactions` with `Authorization: Bearer ${key}` and `cache: "no-store"`; tolerate both `{ interactions: [...] }` and bare array `[...]` envelope shapes. No `NEXT_PUBLIC_` prefix — key stays server-side only.
  - Verification: TypeScript compilation clean (`npx tsc --noEmit`)

- [ ] **T3.4** [interactions page] Create `src/app/interactions/page.tsx` as an `async` Server Component (no `"use client"`). Call `listInteractions()`. Render a semantic HTML table with one row per interaction showing `id`, `occurred_at`, `channel`, `direction` only. No client fetch, no debtor data, no detail links, no pagination, no filters. Exactly one user-navigable route exists in the app.
  - Verification: `npm run build` succeeds; manual demo confirms page renders the three seeded rows

- [ ] **T3.5** [Makefile targets] Add the following targets to the repo root `Makefile` without touching existing Go targets:
  ```
  console-install: cd apps/console && npm install
  console-dev:     cd apps/console && npm run dev
  worker:          go run ./cmd/worker
  seed-dev:        go run ./cmd/seed dev-data
  ```
  - Verification: `make seed-dev` (with DB up) prints `tenant_api_key=...` once; `make console-install` exits 0

- [ ] **T3.6** [docs] Update `HANDOFF.md` with the end-to-end local run order for the walking skeleton demo: `make dev` → `make migrate-up` → `make seed-dev` (copy plaintext key) → start `cmd/api` → `make worker` → `make console-install` (once) → `make console-dev` → open the interactions page. Note that the worker can be `Ctrl-C`d after the no-op job completes. No permanent change to API/schema docs.
  - Verification: A developer reading the HANDOFF can reproduce the full demo without consulting the design doc.

---

## Final validation (all PRs merged)

- [ ] Run Go unit tests for seed and worker: `go test ./cmd/seed ./cmd/worker -count=1`
- [ ] Run full Go suite in short mode (no DB): `go test ./... -short -count=1`
- [ ] Confirm River migration round-trip is clean: `make migrate-up && make migrate-down`
- [ ] Manual end-to-end demo: `make seed-dev` → `cmd/api` → `make worker` → `make console-dev` → verify the three demo interactions appear on the page; verify a wrong key shows nothing (RLS proof).
