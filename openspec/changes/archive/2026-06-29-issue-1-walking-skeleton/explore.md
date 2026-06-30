# Exploration: Issue #1 Walking Skeleton

## Status

Complete.

## Executive Summary

Issues #13 and #14 delivered the full Go backend for the vertical slice: the database schema, RLS isolation, tenant auth middleware, and the protected `GET /v1/interactions` endpoint are working and tested. Issue #18 added the harness runtime skeleton. What remains for issue #1 is three discrete gaps: (1) a dev data seed that creates a tenant + debtor + es-MX interaction events — the current `cmd/seed` only issues API keys for pre-existing tenants; (2) a River worker entrypoint in `cmd/worker` with `riverqueue/river` added to go.mod and a River schema migration; and (3) a Next.js console scaffold in `apps/console` with a single interactions list page.

## What #13 and #14 Already Delivered

### Database foundation (#13, archived)

`db/migrations/00001_initial_foundation.sql` creates: `tenants`, `tenant_api_keys`, `debtors`, `interaction_events`, `policy_rules`, `policy_bundles`, `policy_bundle_rules`, `detector_result_rows`. RLS is enabled on all tenant-scoped tables with policies keyed on `current_setting('app.tenant_id', true)`. sqlc-generated types are in `internal/db/`; the `Querier` interface exposes `CreateTenant`, `CreateDebtor`, `CreateInteractionEvent`, `ListCurrentTenantInteractions`, `GetTenantAPIKeyByHash`, etc.

### Tenant auth + interactions endpoint (#14, active/complete)

- `internal/auth/auth.go` — bearer parsing, SHA-256 key hashing, active-key resolution through `TenantAPIKeyStore` port.
- `internal/tenantdb/tenantdb.go` — `WithTenantTx` / `WithAPIKeyHashTx` helpers set `app.tenant_id` / `app.api_key_hash` via `set_config(..., true)` (transaction-local, equivalent to `SET LOCAL`).
- `internal/httpapi/httpapi.go` — `GET /v1/interactions` route; auth gate before any DB read; returns `[]Interaction{id, occurred_at, channel, direction}`.
- `internal/postgres/adapters.go` — concrete `TenantAPIKeyStore` and `InteractionReader` adapters backed by `tenantdb` helpers and sqlc queries.
- `cmd/api/main.go` — fully wired: pgxpool → adapters → auth → HTTP server.
- `cmd/seed/main.go` — issues API keys; requires a pre-existing tenant UUID via `--tenant-id`.

### Harness runtime skeleton (#18, archived)

`internal/harness/` — `Runtime`, `ModelProvider`, `ToolRegistry`, `PermissionGate`, `Budget`, `Validator`, `Event` types. Fake Model Provider for deterministic tests. No DB persistence, no River, no frontend coupling.

## Current State per Acceptance Criterion

| AC | Status | Evidence |
|----|--------|----------|
| Seeded interaction for tenant A returned only when authenticated as tenant A | PARTIAL — API + RLS exist and are tested; no dev seed that creates tenant + debtor + interactions | `internal/httpapi`, `internal/postgres`, `internal/db/rls_isolation_test.go` |
| Console lists that tenant's interactions | MISSING — `apps/console/` has only `.gitkeep` | `apps/console/.gitkeep` |
| River worker boots and processes a trivial enqueued job | MISSING — `cmd/worker/` empty, `riverqueue/river` absent from go.mod, no River migration | `go.mod`, `cmd/worker/` (empty) |

## Data Model Reality vs Canonical Spec

The actual `interaction_events` schema is a deliberate subset of `InteractionEvent` in `docs/technical-design.md`.

**Actual columns:** `id`, `tenant_id`, `debtor_id`, `channel`, `direction`, `status`, `occurred_at`, `transcript_ref`, `created_at`.

**Not yet in schema:** `source`, `despacho_id`, `debtor_timezone`, `audio_uri`, `agent_identity`, `authorized_channel_source`, `raw_metadata`.

This subset is sufficient for the walking skeleton read path. The `httpapi.Interaction` DTO exposes only `id`, `occurred_at`, `channel`, `direction`.

`internal/core/types.go` defines typed constants: `InteractionChannel` (`call`, `message`, `email`) and `InteractionDirection` (`inbound`, `outbound`) — these should be used in the dev seed for realistic labeled es-MX data.

## RLS Isolation Pattern

`interaction_events_tenant_isolation` policy: `USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)`.

`ListCurrentTenantInteractions` has no explicit `WHERE tenant_id = $1` — it relies entirely on RLS. The `tenantdb.WithTenantTx` helper sets `app.tenant_id` before any query runs, inside the transaction. This is the proven pattern for all tenant-scoped reads. A separate `tenant_api_keys_hash_lookup` policy uses `app.api_key_hash` so the key can be found before `app.tenant_id` is established (auth bootstrap).

## What River Needs

`riverqueue/river` is not in `go.mod`. To satisfy AC3:

1. `go get github.com/riverqueue/river@latest github.com/riverqueue/river/riverdriver/riverpgxv5@latest`.
2. A new goose migration (`00002_river_tables.sql`) that embeds River's schema SQL (River ships it; it can be extracted and wrapped as a goose-managed file for consistency with `make migrate-up`).
3. A trivial `NoopJob` type in `cmd/worker` to prove the worker boots, picks up a job, and completes without error.
4. `cmd/worker/main.go` that loads config, opens pgxpool, creates a River client, registers the noop worker, and calls `client.Start(ctx)`.

## What the Console Needs

`apps/console/` has only `.gitkeep`. The full stack per `docs/frontend-design.md` is Next.js App Router + TypeScript + Tailwind CSS v4 + shadcn/ui. For the walking skeleton, only the scaffold and a single Server Component page calling `GET /v1/interactions` are needed. The page should use `VIGIA_API_KEY` from a server-side env var so no CORS middleware is required.

## Approach Options

### Dev seed command

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A — Extend `cmd/seed` | Add `dev-data` subcommand; idempotently creates demo tenant + debtor + 3 interactions + issues key using `GetTenantBySlug` for upsert safety | Reuses existing infrastructure; single command | Mixes key issuance and data seeding concerns |
| B — New `cmd/seed-dev` binary | Separate binary for dev-only data creation | Clean separation | More files to maintain |
| C — SQL migration | `00002_dev_seed.sql` with `INSERT ... ON CONFLICT DO NOTHING` | Zero Go code | Dev data permanently in migration history; can't print API key plaintext |

**Recommendation:** Option A — extend `cmd/seed` with a `dev-data` subcommand. `CreateTenant`, `CreateDebtor`, `CreateInteractionEvent` are all in the `Querier` interface today.

### River migration strategy

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A — goose migration file | River SQL wrapped in `00002_river_tables.sql` | Consistent with `make migrate-up` | Must match exact River version |
| B — programmatic at startup | `rivermigrate.New(...).Migrate(ctx, ...)` in `cmd/worker` | Self-contained | Requires River dep before migration tooling runs |

**Recommendation:** Option A — goose migration file, matching the existing `make migrate-up` workflow.

### Next.js auth for the skeleton

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A — Server Component + env var | Read `VIGIA_API_KEY` server-side; fetch from Next.js server | No CORS needed; simple | Demo key is in env; fine for skeleton |
| B — CORS middleware + browser fetch | Add CORS headers to Go API | More complete integration | Adds Go middleware complexity for a skeleton |

**Recommendation:** Option A (server-side env key). CORS middleware can be added in a later issue when interactive auth flows are introduced.

## Key File Paths

| Path | State | Relevance |
|------|-------|-----------|
| `db/migrations/00001_initial_foundation.sql` | Done | Full schema + RLS |
| `internal/auth/auth.go` | Done | Bearer auth, key hashing |
| `internal/tenantdb/tenantdb.go` | Done | Transaction-local tenant context |
| `internal/httpapi/httpapi.go` | Done | `GET /v1/interactions` endpoint |
| `internal/postgres/adapters.go` | Done | Postgres adapters |
| `cmd/api/main.go` | Done | HTTP server wired |
| `cmd/seed/main.go` | Done; needs `dev-data` extension | Issues API keys only |
| `cmd/worker/` | Empty — no `.go` files | River worker needed |
| `apps/console/` | Empty (`.gitkeep`) | Next.js scaffold needed |
| `data/synthetic/` | Empty (`.gitkeep`) | es-MX seed data needed |
| `go.mod` | Done; missing `riverqueue/river` | River not present |
| `internal/db/querier.go` | Done | `CreateTenant`, `CreateDebtor`, `CreateInteractionEvent` available |
| `internal/core/types.go` | Done | Channel/direction constants for seed |

## Open Questions

1. **Seed command shape** — Extend `cmd/seed` with `dev-data` (Option A) or new `cmd/seed-dev` binary (Option B)? Choice affects README/HANDOFF dev setup documentation.
2. **River migration strategy** — Goose migration file or programmatic `rivermigrate` at startup?
3. **Next.js API key auth for skeleton** — Server-side env var (no CORS) vs. explicit CORS middleware in Go API?
4. **Interaction response shape** — Current DTO exposes `id`, `occurred_at`, `channel`, `direction`. Should `transcript_ref` or `status` be added for more useful console columns?
5. **Debtor display name** — Should the console list show debtor info (requires a JOIN + new sqlc query), or keep the DTO minimal for the walking skeleton?

## Risks

| Risk | Severity | Note |
|------|----------|------|
| `riverqueue/river` not in go.mod — module cache cold | Medium | Straightforward `go get` but may hit proxy delays |
| River schema SQL version mismatch vs River module version | Medium | Must use River's embedded migration SQL matching the exact version added |
| Next.js scaffold requires Node/npm tooling not in current Makefile | Low | Standard but needs `make console-install` target or documented manual step |
| `interaction_events` schema subset differs from canonical `InteractionEvent` in technical-design.md | Low | Intentional for the skeleton; any future migration must be additive |
| CORS omission means the console cannot call the Go API directly from the browser | Low | Mitigated by Server Component server-side fetch for the skeleton |
| `cmd/seed dev-data` must create records in the correct FK order: tenant → debtor → interactions → API key | Low | Sequencing is straightforward but must be explicit |

## Next Recommended

`sdd-propose`
