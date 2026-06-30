# Walking Skeleton Specification

## Purpose

Define the testable requirements for issue #1 — the walking skeleton that closes
three discrete connectivity gaps: a dev data seed, a River worker entrypoint, and
a Next.js console page. Together they prove that a seeded interaction flows from
the database, through RLS-scoped auth, out of the API, and onto a rendered page
for exactly one tenant, without introducing any business logic beyond list/read.

## Testing mode note

Strict TDD applies to all Go components (seed subcommand, worker entrypoint,
migration smoke-test). Requirements marked `[integration]` require a real Postgres
instance and MUST be skippable with `testing.Short()`. Requirements marked `[unit]`
must run with no external dependencies. Requirements marked `[manual-demo]` are
validated by a human running the local dev environment; they cannot be expressed
as automated Go or Node tests in this skeleton phase.

---

## Requirement: Idempotent Dev Data Seed

The `cmd/seed dev-data` subcommand MUST idempotently create, in correct FK order,
a demo tenant, one debtor belonging to that tenant, three `interaction_events`
labeled with es-MX content, and one issued tenant API key. The plaintext key MUST
be printed exactly once to stdout.

### Scenario: First-run seed creates all required rows `[integration]`

- GIVEN the database contains no tenant with slug `demo`
- WHEN `cmd/seed dev-data` is executed
- THEN a tenant row with slug `demo` MUST exist in `app.tenants`
- AND exactly one debtor row belonging to that tenant MUST exist in `app.debtors`
- AND exactly three `app.interaction_events` rows belonging to that debtor MUST
  exist, each with `channel` drawn from `{call, message, email}`, `direction`
  drawn from `{inbound, outbound}`, and non-empty `transcript_ref` or label
  field consistent with es-MX locale
- AND exactly one `app.tenant_api_keys` row linked to the demo tenant MUST exist
- AND the plaintext API key MUST be printed to stdout.

### Scenario: Repeated runs do not duplicate rows `[integration]`

- GIVEN `cmd/seed dev-data` has already been run at least once
- WHEN `cmd/seed dev-data` is run a second time
- THEN the count of `app.tenants` rows with slug `demo` MUST remain exactly 1
- AND the count of `app.debtors` rows owned by the demo tenant MUST remain
  exactly 1
- AND the count of `app.interaction_events` rows owned by that debtor MUST
  remain exactly 3
- AND the command MUST exit with status 0.

### Scenario: FK insertion order is respected `[integration]`

- GIVEN the database is freshly migrated (no demo data)
- WHEN `cmd/seed dev-data` is executed
- THEN no foreign-key constraint violation error MUST occur
- AND rows are created in the order: tenant → debtor → interaction_events →
  tenant_api_keys.

### Scenario: Plaintext key is exposed at most once `[integration]`

- GIVEN `cmd/seed dev-data` completes successfully
- WHEN the issued API key is inspected
- THEN the plaintext key MUST appear in stdout output exactly once
- AND the plaintext key MUST NOT be stored in the `app.tenant_api_keys` table
  (only its hash is stored, consistent with the #14 key-issuance behavior).

### Scenario: Seed uses existing Querier port, not ad hoc SQL `[unit]`

- GIVEN the seed implementation is reviewed
- WHEN the database operations are inspected
- THEN the subcommand MUST use the `internal/db.Querier` interface methods
  (`CreateTenant`, `CreateDebtor`, `CreateInteractionEvent`, and the key-issuance
  path from `internal/auth`) exclusively
- AND the subcommand MUST NOT contain raw SQL strings outside the existing
  generated `internal/db` layer.

---

## Requirement: River Worker Bootstrap

The `cmd/worker` entrypoint MUST boot against the configured Postgres database,
register a trivial no-op worker, enqueue one `NoopJob`, and drain it to
completion. This proves the River client connects to the migrated schema and
processes jobs without introducing domain job logic.

### Scenario: River migration is applied by `make migrate-up` `[integration]`

- GIVEN `db/migrations/00002_river_tables.sql` exists
- AND the file carries the exact River schema for the River module version added
  to `go.mod`
- WHEN `make migrate-up` is executed against a fresh database
- THEN the goose migration MUST succeed with exit code 0
- AND all River-required tables MUST exist in the database.

### Scenario: Worker process boots and connects to Postgres `[integration]`

- GIVEN the database has been migrated with `make migrate-up` (including
  `00002_river_tables.sql`)
- AND valid `DATABASE_URL` / connection config is present in the environment
- WHEN `cmd/worker` is started
- THEN the process MUST open a `pgxpool` connection to Postgres without error
- AND the River client MUST be created successfully using the pgx v5 driver.

### Scenario: NoopJob is enqueued and drained to completion `[integration]`

- GIVEN the worker process has booted with a connected River client
- AND a `NoopJob` worker is registered on the client
- WHEN the worker process enqueues one `NoopJob` and starts the client
- THEN the River client MUST process the enqueued job
- AND the job MUST reach a terminal `completed` state in the River job table
- AND the worker MUST NOT exit with a non-zero status as a result of job
  processing.

### Scenario: NoopJob worker contains no domain logic `[unit]`

- GIVEN the `NoopJob` worker implementation is reviewed
- WHEN the `Work` method is inspected
- THEN it MUST return `nil` (success) unconditionally
- AND it MUST NOT invoke any detector, evidence, Harness, Judge, or MCP behavior.

### Scenario: Worker process is independent of the API process `[unit]`

- GIVEN the `cmd/worker` entrypoint is reviewed
- WHEN its imports and initialization are inspected
- THEN it MUST NOT import or start the HTTP API server
- AND it MUST NOT share in-process state with the API beyond the database
  connection pool.

---

## Requirement: Next.js Console Interactions List Page

The `apps/console` Next.js App Router scaffold MUST expose exactly one route: an
interactions list page rendered as a Server Component. It MUST fetch
`GET /v1/interactions` server-side using the `VIGIA_API_KEY` server-only
environment variable and render the minimal interaction DTO
(`id`, `occurred_at`, `channel`, `direction`).

### Scenario: Console scaffold exists as a valid Next.js App Router project `[manual-demo]`

- GIVEN `apps/console` is checked out and `VIGIA_API_KEY` is set to the demo
  tenant's key in `.env.local`
- WHEN `npm install && npm run dev` is executed inside `apps/console`
- THEN the Next.js dev server MUST start without errors
- AND navigating to the root route MUST render the interactions list page.

### Scenario: Interactions list page renders the demo tenant's rows `[manual-demo]`

- GIVEN the API server is running and the demo data has been seeded
- AND `VIGIA_API_KEY` in `.env.local` is set to the demo tenant's plaintext key
- WHEN a developer opens the console root route in a browser
- THEN the page MUST display the three seeded interaction rows for the demo tenant
- AND each row MUST show at minimum `id`, `occurred_at`, `channel`, and
  `direction`
- AND the page MUST NOT perform any client-side fetch of the Go API directly.

### Scenario: Server Component reads VIGIA_API_KEY from server-only env `[manual-demo]`

- GIVEN the interactions list page is inspected
- WHEN the data-fetching implementation is reviewed
- THEN the `Authorization: Bearer` header sent to `GET /v1/interactions` MUST be
  built from `process.env.VIGIA_API_KEY` inside a Server Component or equivalent
  server-side boundary
- AND `VIGIA_API_KEY` MUST NOT be exposed to the browser bundle or client
  components.

### Scenario: Console page is limited to the minimal DTO `[manual-demo]`

- GIVEN the console page renders interaction rows
- WHEN the rendered fields are inspected
- THEN the page MUST render only `id`, `occurred_at`, `channel`, and `direction`
- AND it MUST NOT render debtor names, debtor identifiers, detail links,
  pagination controls, filter inputs, search inputs, charts, or dashboard
  widgets.

### Scenario: No additional console routes exist `[manual-demo]`

- GIVEN the `apps/console` project is reviewed
- WHEN the route structure is inspected
- THEN there MUST be exactly one user-navigable route (the interactions list)
- AND there MUST NOT be any detail page, settings page, login page, or other
  console route.

---

## Requirement: Tenant Isolation End-to-End

A seeded interaction for tenant A MUST be returned and rendered only when the
request is authenticated as tenant A. Using any other key — or no key — MUST
return no rows, enforced by RLS. This is the end-to-end isolation proof that
closes gap #1.

### Scenario: Demo tenant's key returns only demo tenant interactions `[integration]`

- GIVEN the demo tenant and its three interactions have been seeded by
  `cmd/seed dev-data`
- AND a second tenant exists with its own `interaction_events` rows (the
  isolation baseline established by issue #14)
- WHEN `GET /v1/interactions` is called with the demo tenant's bearer token
- THEN the response MUST contain exactly the three demo tenant interaction rows
- AND the response MUST NOT contain any rows belonging to the second tenant.

### Scenario: A different tenant's key returns that tenant's interactions only `[integration]`

- GIVEN both the demo tenant and a second tenant have seeded interaction rows
- WHEN `GET /v1/interactions` is called with the second tenant's bearer token
- THEN the response MUST contain only that tenant's rows
- AND the response MUST NOT contain the demo tenant's rows.

### Scenario: Console renders only the authenticated tenant's interactions `[manual-demo]`

- GIVEN the demo tenant data is seeded
- AND the console is running with `VIGIA_API_KEY` set to the demo tenant's key
- WHEN the interactions list page is loaded
- THEN the page MUST display exactly the three demo tenant interactions
- AND it MUST NOT display interactions from any other tenant.

### Scenario: No key returns empty interactions list `[integration]`

- GIVEN the API server is running
- WHEN `GET /v1/interactions` is called without an `Authorization` header
- THEN the response MUST be `401 Unauthorized`
- AND no interaction rows MUST be returned (consistent with issue #14 behavior,
  unchanged by this change).

---

## Non-goals (hardened by this spec)

The following behaviors are explicitly out of scope and MUST NOT be introduced
as part of this change. Any pull request that introduces them MUST be rejected.

- Interaction detail page, filters, pagination controls, search, charts, or
  dashboards in the console.
- Widening the interaction DTO beyond `id`, `occurred_at`, `channel`,
  `direction` (no debtor JOIN, no `status`, no `transcript_ref` in the response
  body).
- Real River jobs: no detector runs, no Harness invocation, no evidence
  generation. The only job type is the trivial `NoopJob`.
- CORS middleware or browser-side API key handling.
- MCP tools, Judge behavior, Harness integration, evidence ledger, HITL, or
  monthly reporting.
- Multi-tenant demo data flows or interactive login/SSO.
- Bedrock or any non-default model provider path.
- Any modification to the `internal/auth`, `internal/tenantdb`,
  `internal/httpapi`, `internal/postgres`, `internal/db`, or `internal/harness`
  packages beyond what the three gaps strictly require.

---

## Dependency alignment

This spec depends on the following prior issues being stable and unmodified:

- **Issue #13**: schema, RLS foundations, and generated `internal/db` layer.
- **Issue #14**: tenant API key auth, `tenantdb.WithTenantTx`, and
  `GET /v1/interactions` endpoint.
- **Issue #18**: harness runtime skeleton (consumed as-is).

No requirement in this spec modifies those boundaries.
