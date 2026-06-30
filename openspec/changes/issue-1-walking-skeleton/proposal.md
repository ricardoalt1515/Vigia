# Proposal: Issue #1 Walking Skeleton (ingest -> store -> API -> console, one tenant)

## Problem / motivation

Vigía now has a real, tested backend vertical slice. Issue #13 delivered the
tenant-aware PostgreSQL foundation (schema, RLS policies on `app.tenant_id`,
sqlc/pgx access). Issue #14 delivered runtime tenant authentication and the
protected `GET /v1/interactions` endpoint that reads through RLS. Issue #18
added the harness runtime skeleton. Each layer is proven in isolation, but the
layers have never been demonstrated connecting end to end with real seeded data
and a user-visible surface.

Without a walking skeleton, there is no executable proof that a seeded
interaction flows from the database, through RLS-scoped auth, out of the API,
and onto a rendered page — and no proof that the async worker process (River)
boots against the same database. The connectivity gaps are also a recurring
source of onboarding friction: a new developer cannot run one command and see a
tenant's interactions in a browser.

Issue #1 closes exactly three discrete connectivity gaps and nothing more. It is
a tracer bullet, not a feature.

## Intent

Prove that all layers connect for a single tenant with the thinnest possible
code: a dev data seed, a River worker that boots and drains a trivial job, and a
Next.js console page that lists one tenant's interactions through the existing
authenticated API.

The skeleton is "done" when a developer can, in one local environment:

1. seed a demo tenant + debtor + labeled es-MX interactions and obtain an API
   key,
2. start the River worker and watch it process a no-op enqueued job,
3. open the console and see exactly that tenant's interactions.

No business logic beyond list/read is introduced. The skeleton exists to prove
wiring, not to add capability.

## Goals

- Provide an idempotent dev data seed that creates a demo tenant, a debtor,
  three labeled es-MX `interaction_events`, and issues one tenant API key
  (plaintext shown once).
- Stand up a River worker entrypoint that boots against the same Postgres,
  registers a trivial no-op worker, enqueues one job, and processes it to
  completion.
- Add River's schema as a goose migration consistent with `make migrate-up`.
- Scaffold the Next.js console with a single Server Component page that lists the
  authenticated tenant's interactions from `GET /v1/interactions`.
- Keep the interaction DTO minimal (`id`, `occurred_at`, `channel`,
  `direction`) — prove the list renders, nothing else.
- Demonstrate tenant isolation end to end: tenant A's seeded interaction is
  returned and rendered only when authenticated as tenant A.

## Non-goals

- Do not add any business logic beyond list/read. No interaction detail page, no
  filters, no pagination beyond a safe default, no search, no charts, no
  dashboards.
- Do not widen the interaction DTO or add a debtor JOIN/new sqlc query for the
  console list. Keep the response minimal.
- Do not implement real River jobs (detector runs, Harness invocation, evidence
  generation). The only job is a trivial no-op that proves the worker drains the
  queue.
- Do not add CORS middleware or browser-side API-key handling. The console reads
  the API server-side with a server-only env var.
- Do not introduce MCP, Judge behavior, Harness integration, detector results,
  evidence ledger, HITL, or monthly reporting.
- Do not add multi-tenant demo data flows or interactive login/SSO. One tenant is
  sufficient for the skeleton; a second tenant is only relevant to the existing
  #14 isolation tests, not to this UI.
- Do not introduce Bedrock or any non-default model provider path.
- Do not modify the proven #13/#14/#18 auth, RLS, or endpoint behavior beyond
  what the three gaps strictly require.

## Scope boundaries

### In scope for #1

- A `dev-data` subcommand extending `cmd/seed` that idempotently seeds a demo
  tenant + debtor + 3 es-MX interactions and issues one API key, in correct FK
  order (tenant -> debtor -> interactions -> API key).
- `riverqueue/river` added to `go.mod` with the pgx v5 driver.
- A goose migration (`00002_river_tables.sql`) carrying River's schema, applied
  through the existing `make migrate-up` workflow.
- A `cmd/worker/main.go` entrypoint that loads config, opens a pgxpool, creates a
  River client, registers a trivial no-op worker, enqueues one job, and starts
  the client.
- A trivial `NoopJob` / no-op worker type proving boot + job drain.
- A Next.js App Router scaffold under `apps/console` (TypeScript + Tailwind v4
  baseline per `docs/frontend-design.md`) with a single interactions list page
  rendered as a Server Component.
- Server-side fetch of `GET /v1/interactions` using a `VIGIA_API_KEY` server-only
  env var.
- Minimal dev documentation / `.env.example` and a console install/run step so
  the skeleton is reproducible.

### Out of scope for #1

- Any console route other than the single interactions list page.
- Any real worker job logic, scheduling, retries policy, or queue topology beyond
  what booting and draining one job requires.
- Any change to the interaction read contract (DTO shape, query, RLS) owned by
  #14.
- Browser-based auth, CORS, session handling, or interactive key entry.
- shadcn/ui component build-out beyond what a plain list page needs.

## Approach

### Gap 1 — Dev data seed (extend `cmd/seed`)

Add a `dev-data` subcommand to the existing `cmd/seed` binary (exploration
Option A). It reuses the `Querier` interface already in `internal/db`
(`CreateTenant`, `CreateDebtor`, `CreateInteractionEvent`) plus the existing
key-issuance path. The subcommand is idempotent: it upserts the demo tenant by
slug (via `GetTenantBySlug`) so repeated runs do not duplicate rows, then
creates the debtor and three interactions if absent, then issues one API key and
prints the plaintext exactly once.

The three interactions use the typed constants in `internal/core/types.go`
(`InteractionChannel`: `call`/`message`/`email`; `InteractionDirection`:
`inbound`/`outbound`) with realistic es-MX labeling, written in the correct FK
order. Reusing the existing binary keeps the change small and avoids a second
maintained entrypoint; the mild concern of mixing key-issuance and data-seeding
concerns is acceptable for a dev-only command.

### Gap 2 — River worker (`cmd/worker` + migration)

Add `github.com/riverqueue/river` and `riverdriver/riverpgxv5` to `go.mod`. Carry
River's schema as goose migration `00002_river_tables.sql` (exploration Option A)
so `make migrate-up` remains the single migration path; the migration SQL must
match the exact River module version added.

`cmd/worker/main.go` loads config, opens a pgxpool against the same database,
builds a River client with the pgx v5 driver, registers a trivial `NoopJob`
worker that returns success, enqueues one `NoopJob`, and calls
`client.Start(ctx)`. This proves the worker process boots, connects, and drains
the queue without introducing any domain job logic. The worker stays an
independent process; it shares only the database, not the API in-process state.

### Gap 3 — Next.js console (`apps/console`)

Scaffold a Next.js App Router project (TypeScript, Tailwind v4) under
`apps/console`, replacing the `.gitkeep` placeholder. Implement one route: an
interactions list page as a Server Component that fetches
`GET /v1/interactions` server-side with `Authorization: Bearer ${VIGIA_API_KEY}`
read from a server-only env var (exploration Option A). No CORS is needed because
the browser never calls the Go API directly.

The page renders the minimal DTO (`id`, `occurred_at`, `channel`, `direction`)
as a simple table/list. No debtor JOIN, no detail link, no client-side data
fetching. A documented `make` target or manual step covers Node install and
running the dev server.

## Affected areas

- `cmd/seed/` — add `dev-data` subcommand; idempotent tenant/debtor/interaction
  seeding plus key issuance.
- `cmd/worker/` — new `main.go`, no-op worker type, River client wiring (currently
  empty).
- `db/migrations/00002_river_tables.sql` — new River schema migration.
- `go.mod` / `go.sum` — add `riverqueue/river` + pgx v5 driver.
- `apps/console/` — Next.js App Router scaffold + single interactions list page
  (currently only `.gitkeep`).
- `data/synthetic/` — optional es-MX seed source data if the seed reads from a
  fixture (currently `.gitkeep`).
- `Makefile` — optional `console-install` / `worker` / `seed-dev` convenience
  targets.
- `.env.example` / docs — document `VIGIA_API_KEY` and the local skeleton run
  steps.

The proven #13/#14/#18 code (`internal/auth`, `internal/tenantdb`,
`internal/httpapi`, `internal/postgres`, `internal/db`, `internal/harness`) is
consumed as-is and not modified.

## Acceptance criteria and how they are satisfied

| Acceptance criterion | How #1 satisfies it |
|---|---|
| A seeded interaction for tenant A is returned by the API only when authenticated as tenant A | `cmd/seed dev-data` creates tenant A + debtor + interactions and issues tenant A's key; the existing #14 `GET /v1/interactions` returns those rows only under tenant A's bearer token, enforced by RLS (no explicit `tenant_id` predicate). |
| The console lists that tenant's interactions | The `apps/console` Server Component page fetches `GET /v1/interactions` server-side with tenant A's `VIGIA_API_KEY` and renders the minimal DTO list. |
| A River worker boots and processes a trivial enqueued job | `cmd/worker/main.go` boots a River client against Postgres (schema from `00002_river_tables.sql`), registers a no-op worker, enqueues one `NoopJob`, and drains it to completion. |
| No business logic beyond list/read | The seed only inserts fixture rows; the worker job is a no-op; the console only reads and renders. No detectors, evidence, Harness, Judge, or MCP behavior is added. |

## Architecture / ADR alignment

- **Clean / hexagonal boundaries:** The console consumes the API over HTTP and
  never owns tenant isolation; Go remains the authority for auth + RLS. The worker
  is a separate process sharing only the database. The seed uses the existing
  `Querier` port, not ad hoc SQL.
- **Tenant isolation:** Reads continue to flow through the proven
  `tenantdb.WithTenantTx` + RLS pattern. #1 adds no new query that bypasses RLS.
- **SQL-first persistence:** River schema lands as a goose migration so
  `make migrate-up` stays the single source of truth; no ORM is introduced.
- **Process boundaries:** API, worker, and console are distinct entrypoints; the
  worker does not embed API logic and the console does not embed tenant secrets in
  the browser.
- **Workflow-first / untrusted data:** No agent loop, no LLM, no transcript
  interpretation is added; seeded transcripts are inert data.

## Risks and mitigations

| Risk | Severity | Mitigation |
|---|---:|---|
| `riverqueue/river` not in `go.mod`; cold module cache / proxy delays | Medium | Pin exact versions via `go get`; commit `go.sum`; verify `go build ./...`. |
| River schema SQL version mismatch vs the River module version | Medium | Generate `00002_river_tables.sql` from the exact River version added and verify the worker boots against the migrated DB. |
| Next.js scaffold needs Node/npm tooling absent from the Makefile | Low | Add a documented `console-install` target or manual step; keep Go workflows unchanged. |
| CORS omission blocks future browser-side calls | Low | Acceptable for the skeleton; server-side fetch covers the read path. CORS deferred to a later interactive-auth issue. |
| Seed FK ordering errors (tenant -> debtor -> interactions -> key) | Low | Make ordering explicit and idempotent; upsert tenant by slug. |
| `interaction_events` schema subset differs from canonical spec | Low | Intentional; any future column addition must be additive. The skeleton uses only existing columns. |
| Scope creep into detail views, filters, or real jobs | Medium | Hard non-goals above; reviewers reject anything beyond list/read + no-op job. |

## Rollback

Rollback is clean and isolated to the three gaps:

- remove the `dev-data` subcommand and delete any demo tenant/debtor/interaction
  rows and demo API key created during validation;
- delete `cmd/worker` and revert the `riverqueue/river` additions in
  `go.mod`/`go.sum`;
- roll back the River migration via `make migrate-down`
  (`00002_river_tables.sql`);
- remove the `apps/console` scaffold, restoring the `.gitkeep` placeholder;
- the #13/#14/#18 schema, RLS, auth, and endpoint code remain untouched.

## Success criteria

- A developer runs the dev seed, starts the worker, and opens the console to see
  exactly tenant A's three es-MX interactions — proving ingest(seed) -> store ->
  API -> console end to end for one tenant.
- The River worker logs a booted client and a completed no-op job against the
  migrated database.
- The console shows nothing for a different/absent tenant key, consistent with
  RLS isolation.
- Reviewers can approve #1 without approving any business logic, detail view,
  detector, evidence, Harness, Judge, or MCP behavior.
- The skeleton leaves a reproducible local setup that later issues build on
  without redesigning the seed, worker, or console boundary.

## Proposal question round

No interactive question round was run; the orchestrator supplied locked decisions
from exploration. The proposal assumes:

- Dev seed is a `dev-data` subcommand on `cmd/seed` (Option A), idempotent, one
  tenant + debtor + 3 es-MX interactions + one issued key.
- River schema ships as goose migration `00002_river_tables.sql` (Option A).
- Console auth is a Server Component reading a server-only `VIGIA_API_KEY`
  (Option A); no CORS.
- The interaction DTO stays minimal (`id`, `occurred_at`, `channel`,
  `direction`); no debtor JOIN for the skeleton.

If any of these defaults should change (for example, exposing `status`/
`transcript_ref` columns or showing debtor display info), raise it before spec.

## Next recommended phase

Spec and Design (can run in parallel).
