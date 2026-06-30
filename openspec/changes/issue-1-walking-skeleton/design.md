# Design: Issue #1 Walking Skeleton (seed -> store -> API -> console + River worker)

## Status

Planned. Depends on the stable #13 schema/RLS, #14 auth + `GET /v1/interactions`, and #18
harness skeleton. This change adds three connectivity gaps only; it modifies none of the
proven `internal/auth`, `internal/tenantdb`, `internal/httpapi`, `internal/postgres`,
`internal/db`, or `internal/harness` code.

## Objective

Prove every layer connects for a single tenant with the thinnest possible code:

1. an idempotent `cmd/seed dev-data` subcommand that creates a demo tenant + debtor + three
   labeled es-MX `interaction_events` and issues one tenant API key (plaintext printed once);
2. a `cmd/worker` River process that boots against the same Postgres, registers a no-op job,
   enqueues one, and drains it to completion, with River's schema carried as goose migration
   `00002_river_tables.sql`;
3. a Next.js App Router console under `apps/console` with one Server Component page that lists
   the authenticated tenant's interactions through the existing API.

No business logic beyond list/read is introduced.

## Architecture overview

Three independent process boundaries share only the database; none embeds another's state.

```text
cmd/seed dev-data ─┐ writes via Querier port (privileged migration role, bypasses RLS)
                   ▼
            Postgres (RLS)
                   ▲ reads via tenantdb.WithTenantTx + RLS (non-owner runtime role)
cmd/api  ──────────┘ GET /v1/interactions  (UNCHANGED, #14)
                   ▲ server-side fetch, Authorization: Bearer ${VIGIA_API_KEY}
apps/console ──────┘ single Server Component page

cmd/worker ─── River client on the same pool; river_* tables only; no API, no domain job.
```

Dependency rule (clean/hexagonal): the seed depends on the existing `internal/db.Querier`
port, not ad hoc SQL. The console depends on the HTTP contract, never on the database. The
worker depends on River + the pool, never on `internal/httpapi` or `internal/auth`.

---

## Gap 1 — Dev data seed (`cmd/seed dev-data`)

### The RLS decision (highest-risk — resolved with evidence)

**Decision: the seed inserts as the privileged migration/owner role and does NOT use
`tenantdb.WithTenantTx`. It does not set `app.tenant_id` at all.**

Evidence from the live codebase:

- The migration enables RLS on `tenant_api_keys`, `debtors`, `interaction_events`, etc., but
  never issues `ALTER TABLE ... FORCE ROW LEVEL SECURITY`
  (`db/migrations/00001_initial_foundation.sql`). In PostgreSQL the **table owner bypasses RLS
  entirely** unless `FORCE ROW LEVEL SECURITY` is set.
- `DATABASE_URL` in `docker-compose.yml` and the `Makefile` is the `vigia` role that runs the
  goose migrations, i.e. the table owner.
- The existing `cmd/seed` already inserts `tenant_api_keys` through `CreateTenantAPIKey`
  **without** setting `app.tenant_id`, and it works — because the owner bypasses RLS.
- `internal/db/rls_isolation_test.go` confirms the two-role model: it seeds rows over
  `DATABASE_URL` (owner, no `set_config`) and then proves isolation only over a separate
  non-owner `APP_DATABASE_URL` connection that must `set_config('app.tenant_id', ...)`.

Therefore the seed reuses the same connection model as the existing key issuer: a plain
`pgxpool` over `cfg.DatabaseURL`, inserting through the `Querier`. Wrapping seed inserts in
`WithTenantTx` would be wrong twice over: it is unnecessary (owner bypass), and the per-table
policies are `USING`-only with no `WITH CHECK`, so `WITH CHECK` defaults to the `USING`
predicate and an INSERT under a non-owner role with `app.tenant_id` set to the new tenant
would only *coincidentally* pass for `debtors`/`interaction_events` and would still **fail**
for `tenant_api_keys` (whose two policies check `tenant_id` OR `key_hash`). The owner-bypass
path is the correct, proven one. This is dev-only tooling; production ingestion will use the
runtime role and is out of scope.

The seed must run after `make migrate-up` as the same `vigia` owner role. The design documents
this prerequisite; it does not introduce a new role.

### Subcommand interface

Extend the existing `cmd/seed` binary with a subcommand dispatch on `args[0]`:

```text
seed dev-data [--slug demo-tenant] [--name "Demo Tenant"] [--debtor-ref debtor-001] [--label local-dev]
seed [--tenant-id <uuid>] [--label local-dev]     # existing key-issuance path, unchanged
```

`run(ctx, args)` inspects `args[0]`: if it equals `dev-data`, parse the dev flags and call the
seed flow; otherwise fall through to the current key-issuance behavior verbatim (backward
compatible). Defaults make `seed dev-data` runnable with zero flags.

### Idempotency strategy and FK ordering

Order is FK-driven: tenant -> debtor -> interactions -> API key.

| Entity | Idempotency check | Action if present | Action if absent |
|---|---|---|---|
| tenant | `GetTenantBySlug(slug)` | reuse returned id | `CreateTenant(slug, name, "active")` |
| debtor | `ListDebtorsByTenant(tenantID)`, match by `external_ref` | reuse id | `CreateDebtor(tenantID, ref, displayName)` |
| 3 interactions | `ListInteractionEventsByTenant(tenantID)`, match by stable `transcript_ref` | skip that row | `CreateInteractionEvent(...)` for each missing fixture |
| API key | none possible (hash of random secret) | mint a fresh key, print plaintext once | same |

`GetTenantBySlug`, `ListDebtorsByTenant`, `ListInteractionEventsByTenant`, `CreateTenant`,
`CreateDebtor`, `CreateInteractionEvent` all already exist on the `Querier` interface
(`internal/db/querier.go`) — no new sqlc query is added. The `tenants` and `interaction_events`
tables already enforce `UNIQUE (slug)` and `UNIQUE (tenant_id, external_ref)` on debtors, so the
checks are race-safe enough for single-developer dev use.

API-key idempotency note: re-running `dev-data` mints an additional active key and prints its
plaintext. This is intentional and acceptable for dev tooling — plaintext of a prior key cannot
be recovered (only the hash is stored). The command prints exactly one fresh plaintext per run.

### Fixture data (es-MX, typed constants)

Three interactions using `internal/core` constants (channel is free `text` in the schema, but
the seed must use the typed values for realistic, valid data):

| occurred_at (relative) | channel | direction | transcript_ref (idempotency key) |
|---|---|---|---|
| now − 72h | `core.InteractionChannelCall` | `core.InteractionDirectionOutbound` | `seed/demo/call-01` |
| now − 48h | `core.InteractionChannelMessage` | `core.InteractionDirectionInbound` | `seed/demo/message-01` |
| now − 24h | `core.InteractionChannelEmail` | `core.InteractionDirectionOutbound` | `seed/demo/email-01` |

Debtor: `external_ref = debtor-001`, `display_name = "Juana Pérez (demo)"`. Status defaults to
`recorded` from the schema. Optional es-MX source fixtures may live in `data/synthetic/` but are
not required; inline fixtures keep the change smaller and are preferred for the skeleton.

### File layout

```text
cmd/seed/main.go        existing: add dev-data dispatch + plaintext print (unchanged key path)
cmd/seed/devdata.go     new: SeedDevData(ctx, Querier-port, KeyIssuer, params) + fixtures
cmd/seed/devdata_test.go new: unit tests over a fake Querier
```

### Key types / signatures

```go
// devdata.go — depends only on the small port, not on *pgxpool or *db.Queries.
type DevDataParams struct {
    Slug, Name, DebtorRef, DebtorName, Label string
}

type DevDataResult struct {
    TenantID       string
    DebtorID       string
    InteractionIDs []string
    PlaintextKey   string
    Created        DevDataCounts // tenantCreated, debtorCreated, interactionsCreated bool/int
}

// SeedQuerier is the minimal read/write port the seed needs (subset of db.Querier),
// so the unit test can supply an in-memory fake (architecture-patterns: in-memory adapter).
type SeedQuerier interface {
    GetTenantBySlug(ctx context.Context, slug string) (db.Tenant, error)
    CreateTenant(ctx context.Context, arg db.CreateTenantParams) (db.Tenant, error)
    ListDebtorsByTenant(ctx context.Context, tenantID pgtype.UUID) ([]db.Debtor, error)
    CreateDebtor(ctx context.Context, arg db.CreateDebtorParams) (db.Debtor, error)
    ListInteractionEventsByTenant(ctx context.Context, tenantID pgtype.UUID) ([]db.InteractionEvent, error)
    CreateInteractionEvent(ctx context.Context, arg db.CreateInteractionEventParams) (db.InteractionEvent, error)
}

func SeedDevData(ctx context.Context, q SeedQuerier, issue KeyIssuer, p DevDataParams) (DevDataResult, error)
```

`KeyIssuer` reuses the existing `IssueTenantAPIKey` flow (the `TenantAPIKeyCreator` boundary
already in `cmd/seed/main.go`) so key generation/hashing is not duplicated. `*db.Queries`
already satisfies `SeedQuerier` structurally via the existing `Querier` interface.

### Test seams (strict TDD)

- **Unit (no DB)** `devdata_test.go`, table-driven over a fake `SeedQuerier` + fake `KeyIssuer`:
  1. fresh DB -> creates tenant, debtor, 3 interactions, issues 1 key; FK order asserted by
     recording call order in the fake.
  2. idempotent re-run -> `GetTenantBySlug` hit, zero new tenant/debtor/interaction creates.
  3. partial state (tenant+debtor exist, interactions missing) -> only missing interactions created.
  4. fixtures use the `core` channel/direction constants and the three stable `transcript_ref`s.
- **Integration (skippable)**: a `//go:build`-free test guarded by `testing.Short()` and a
  present `DATABASE_URL`; runs `SeedDevData` twice against real Postgres and asserts exactly one
  tenant / one debtor / three interactions after both runs. Mirrors the existing
  `rls_isolation_test.go` skip pattern.
- **Manual demo**: `go run ./cmd/seed dev-data` prints `tenant_api_key=...` once; operator copies
  it into `apps/console/.env.local`.

### RED/GREEN order

1. RED: unit test (1) fresh seed -> fail (no `SeedDevData`). GREEN: implement happy path.
2. RED: unit test (2) idempotent re-run -> fail. GREEN: add `GetTenantBySlug` / list-and-match guards.
3. TRIANGULATE: unit test (3) partial state. GREEN: per-entity guards already cover it; adjust if not.
4. RED: dispatch test that `dev-data` routes to seed and base args still issue a key. GREEN: wire `run`.
5. Integration test added last, skipped in `-short`.

---

## Gap 2 — River worker (`cmd/worker` + migration)

### Versions and dependency strategy

Add two modules **pinned to the same River release**:

```bash
go get github.com/riverqueue/river@<vX.Y.Z>
go get github.com/riverqueue/river/riverdriver/riverpgxv5@<vX.Y.Z>
go mod tidy
```

River is pre-1.0; `riverpgxv5` is versioned in lockstep with `river`. The exact tag MUST be
resolved at apply time (latest stable v0.x) and the **same** tag used for both modules and for
the migration SQL generation below. Record the resolved version in `go.mod`/`go.sum` and in the
apply-progress note. Do not mix tags — a driver/core/migration version skew is the top risk here.

### River schema migration (deterministic SQL extraction)

Carry River's schema as goose `00002_river_tables.sql` (proposal Option A) so `make migrate-up`
stays the single migration path. Obtain the exact SQL deterministically from the pinned version
using River's own migration CLI (it prints the canonical SQL, no hand-authoring):

```bash
go run github.com/riverqueue/river/cmd/river@<vX.Y.Z> migrate-get --line main --up   --all > /tmp/river_up.sql
go run github.com/riverqueue/river/cmd/river@<vX.Y.Z> migrate-get --line main --down --all > /tmp/river_down.sql
```

Wrap the captured SQL in goose markers, preserving River's statements verbatim:

```sql
-- +goose Up
-- +goose StatementBegin
<contents of river_up.sql>
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
<contents of river_down.sql>
-- +goose StatementEnd
```

`migrate-get` is generated from the same module version, so the SQL provably matches the linked
River client (mitigates the version-mismatch risk). The worker must NOT call `rivermigrate` at
startup (Option B rejected): it would split the migration source of truth away from goose. Verify
with `make migrate-up` then `make migrate-down` round-trips cleanly before wiring the worker.

### `cmd/worker/main.go` structure

```text
cmd/worker/main.go     run(ctx): config -> pool -> river client -> register -> enqueue -> start -> wait -> stop
cmd/worker/noop.go     NoopJob (river.JobArgs) + NoopWorker (river.Worker[NoopJob])
cmd/worker/noop_test.go unit test for Kind() + Work()
```

```go
// noop.go
type NoopJob struct{}                        // empty args; trivial proof job
func (NoopJob) Kind() string { return "noop" }

type NoopWorker struct{ river.WorkerDefaults[NoopJob] }
func (NoopWorker) Work(ctx context.Context, job *river.Job[NoopJob]) error { return nil }
```

```go
// main.go run(ctx)
cfg := config.LoadFromEnv()
pool := pgxpool.New(ctx, cfg.DatabaseURL)   // same pool model as cmd/api; shares only the DB
defer pool.Close()

workers := river.NewWorkers()
river.AddWorker(workers, &NoopWorker{})

client := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
    Workers: workers,
})

client.Insert(ctx, NoopJob{}, nil)          // enqueue one proof job
client.Start(ctx)                           // begins draining
// graceful shutdown:
sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
defer stop()
<-sigCtx.Done()
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
client.Stop(shutdownCtx)
```

The worker imports `internal/config` only. It does **not** import `internal/httpapi`,
`internal/auth`, or `internal/harness` — process isolation is enforced by the import graph. It
shares the database, nothing else. (Harness model port and River are unrelated; no coupling, per
the CLAUDE.md boundary.)

### Test seams (strict TDD)

- **Unit (no DB)** `noop_test.go`:
  1. `NoopJob{}.Kind() == "noop"` (stable kind constant).
  2. `NoopWorker{}.Work(ctx, &river.Job[NoopJob]{}) == nil` (the job is a no-op).
- **Integration (skippable)** `worker_integration_test.go`, `testing.Short()` + `DATABASE_URL`
  guarded: build a real client against migrated Postgres, `Insert` one `NoopJob`, `Start`, then
  poll the `river_job` table (or River's completion subscription) until the job reaches
  `completed`, with a timeout; assert completion and `Stop`. Skipped in `-short` and when no DB.
- **Manual demo**: `go run ./cmd/worker` logs a booted client and a completed no-op job, then
  Ctrl-C shuts down gracefully.

### RED/GREEN order

1. RED: `Kind()` test -> fail. GREEN: add `NoopJob`.
2. RED: `Work()` returns nil test -> fail. GREEN: add `NoopWorker`.
3. Add migration; verify `make migrate-up`/`down` manually (not a Go test).
4. Wire `main.go run`; add the skippable integration test last (it is the boot+drain proof).

---

## Gap 3 — Next.js console (`apps/console`)

### Scaffold layout (App Router, TypeScript, Tailwind v4)

Replace `apps/console/.gitkeep` with a minimal App Router project. Only what a single read-only
list page needs — no shadcn build-out, TanStack Table, Recharts, or Zod required for the skeleton
(those are deferred per `docs/frontend-design.md` "Not needed for the first walking skeleton").

```text
apps/console/
  package.json                next, react, react-dom, typescript, tailwindcss v4, @tailwindcss/postcss
  tsconfig.json
  next.config.ts
  postcss.config.mjs          @tailwindcss/postcss
  .env.example                VIGIA_API_KEY=, VIGIA_API_BASE_URL=http://localhost:8080
  .gitignore                  .env.local, node_modules, .next
  src/
    app/
      layout.tsx              root layout, Geist font, imports globals.css
      globals.css             @import "tailwindcss";
      page.tsx                redirect or render the interactions list
      interactions/page.tsx   the single Server Component list page
    lib/
      api.ts                  server-only fetch helper
```

### Server Component data flow (no CORS, no client fetch)

`src/lib/api.ts` (server-only — uses `VIGIA_API_KEY`, never shipped to the browser):

```ts
import "server-only";

export type Interaction = {
  id: string;
  occurred_at: string;
  channel: string;
  direction: string;
};

export async function listInteractions(): Promise<Interaction[]> {
  const base = process.env.VIGIA_API_BASE_URL ?? "http://localhost:8080";
  const key = process.env.VIGIA_API_KEY;
  if (!key) throw new Error("VIGIA_API_KEY is not set");
  const res = await fetch(`${base}/v1/interactions`, {
    headers: { Authorization: `Bearer ${key}` },
    cache: "no-store",            // always read live tenant data
  });
  if (!res.ok) throw new Error(`API ${res.status}`);
  const body = await res.json();
  return body.interactions ?? body;  // tolerate {interactions:[...]} or [...]
}
```

`src/app/interactions/page.tsx` is an `async` Server Component that calls `listInteractions()`
and renders a plain semantic table of `id`, `occurred_at`, `channel`, `direction`. No
`"use client"`, no client data fetching, no debtor JOIN, no detail link. The browser never sees
the API key and never calls the Go API directly, so no CORS middleware is needed (proposal
Option A confirmed).

The exact JSON envelope must be confirmed against `internal/httpapi` during apply: the helper
tolerates both an `{ "interactions": [...] }` object and a bare array so the skeleton renders
regardless of the wrapper shape #14 settled on.

### Env handling

`VIGIA_API_KEY` and `VIGIA_API_BASE_URL` are server-only env vars (no `NEXT_PUBLIC_` prefix, so
Next.js keeps them out of the client bundle). `.env.example` is committed with empty values;
`.env.local` (gitignored) holds the real key the operator pasted from `seed dev-data`. No
plaintext key is committed.

### Make/dev step

Add convenience targets to the `Makefile`, keeping Go workflows untouched:

```make
console-install:           # one-time Node deps
	cd apps/console && npm install
console-dev:               # run the console dev server
	cd apps/console && npm run dev
worker:                    # run the River worker
	go run ./cmd/worker
seed-dev:                  # seed demo tenant + interactions, print API key
	go run ./cmd/seed dev-data
```

Document the end-to-end run order in `.env.example`/HANDOFF: `make dev` -> `make migrate-up` ->
`make seed-dev` (copy key) -> run `cmd/api` -> `make worker` -> `make console-install` once ->
`make console-dev` -> open the interactions page.

### Test seams

- The console has **no unit tests** in this skeleton (Playwright smoke tests are explicitly
  deferred in `docs/frontend-design.md`). Verification is the manual demo: with the API running
  and the seeded key in `.env.local`, the page lists exactly the demo tenant's three
  interactions; with an absent/wrong key the page shows nothing/an error, demonstrating RLS
  isolation end to end.
- Strict TDD applies to Go only here; the console is proven by the integration demo, consistent
  with the project's frontend test posture.

---

## Cross-cutting decisions (ADR-style)

### ADR-1: Seed inserts via the owner role and bypasses RLS (no WithTenantTx)

- **Context**: seed must write tenant-scoped rows that have RLS enabled.
- **Decision**: insert through the `vigia` owner role over `DATABASE_URL`, relying on
  PostgreSQL owner RLS bypass (no `FORCE ROW LEVEL SECURITY`); do not use `WithTenantTx`.
- **Rationale**: matches the proven existing key-issuer and the `rls_isolation_test.go` seeding
  path; `WithTenantTx` is unnecessary and would fail for `tenant_api_keys` whose policies don't
  check `app.tenant_id` alone.
- **Rejected**: running the seed under the runtime non-owner role with `WithTenantTx`
  (`tenant_api_keys` INSERT would be denied; needless ceremony for dev tooling).
- **Consequence**: the seed is a documented dev/owner-only path; production ingestion via the
  runtime role is out of scope and unaffected.

### ADR-2: River schema as a goose migration generated by `river migrate-get`

- **Decision**: extract canonical SQL from the pinned River version's `migrate-get` and wrap it
  in `00002_river_tables.sql`; never run `rivermigrate` at worker startup.
- **Rationale**: one migration source of truth (`make migrate-up`); version-matched SQL.
- **Rejected**: programmatic `rivermigrate.Migrate` at boot (splits migration ownership, needs
  the River dep before the migration toolchain runs).

### ADR-3: Console reads server-side with a server-only env key (no CORS)

- **Decision**: a Server Component fetches the API with `Authorization: Bearer ${VIGIA_API_KEY}`
  from a non-public env var.
- **Rationale**: the browser never holds the key or calls Go directly, so no CORS middleware is
  added to the proven API.
- **Rejected**: CORS middleware + browser fetch (adds Go surface for a skeleton; deferred to a
  later interactive-auth issue).

### ADR-4: Process isolation by import graph

- **Decision**: `cmd/worker` imports only `internal/config` + River; `apps/console` consumes only
  HTTP. No shared in-process state across API/worker/console.
- **Rationale**: enforces the clean process boundaries the architecture requires; keeps Judge,
  Harness, and River ports unmerged.

---

## Out of scope (reaffirmed)

No interaction detail page, filters, pagination beyond the existing default limit, search, debtor
JOIN, new sqlc query, DTO change, real River jobs, CORS, browser auth, MCP, Judge, Harness
integration, detector results, evidence ledger, Bedrock, or multi-tenant demo flows.

## Verification

```bash
go test ./cmd/seed ./cmd/worker        # unit (fast)
go test ./...                          # full suite (-short keeps integration skipped)
make migrate-up && make migrate-down   # River migration round-trip
# manual demo: make seed-dev -> run cmd/api -> make worker -> make console-dev
```

## Risks carried into tasks/apply

| Risk | Severity | Mitigation owned by apply |
|---|---|---:|
| River core/driver/migration version skew | Medium | Pin one tag for both modules + `migrate-get`; record it; round-trip the migration. |
| API JSON envelope shape (`{interactions:[]}` vs `[]`) | Low | `api.ts` tolerates both; confirm against `internal/httpapi` at apply. |
| Owner-role assumption misread in CI/another env | Low-Med | Design documents the owner-bypass prerequisite; seed runs as the migration role only. |
| Cold module cache / proxy delay on `go get river` | Low | Pin versions; commit `go.sum`; verify `go build ./...`. |
| Node toolchain absent for console | Low | `make console-install` + documented manual step; Go workflows unchanged. |
