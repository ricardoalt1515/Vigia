# Technical Design: Issue #13 Foundation Bootstrap

Issue #13 establishes the minimum local-development and persistence foundation needed before Vigía can implement tenant auth, the walking skeleton, River runtime proof, Harness behavior, MCP, or Bedrock. The design intentionally stops at bootstrap/schema/config/sqlc/core scaffolding and does not claim runtime tenant isolation or evidence/WORM behavior.

## Overview

The foundation slice should make a fresh checkout useful for local development:

1. Start PostgreSQL and MinIO through the existing local stack.
2. Apply an initial PostgreSQL schema through Goose migrations.
3. Mark tenant-owned tables with `tenant_id` and enable schema-level RLS.
4. Generate type-safe SQL access through sqlc and pgx.
5. Load and validate required bootstrap configuration through `internal/config`.
6. Add pure `internal/core` type scaffolding for the canonical data model.
7. Preserve required directory paths for later Harness work without implementing Harness behavior.

The implementation should preserve clean/hexagonal boundaries: domain/core types stay framework-free, generated database code stays in `internal/db`, migrations and queries stay under `db/`, and local infrastructure remains outside domain packages.

## Current State Read

Required planning inputs were read before this design:

- `openspec/config.yaml`
- `openspec/changes/issue-13-foundation-bootstrap/proposal.md`
- `openspec/changes/issue-13-foundation-bootstrap/specs/foundation-bootstrap/spec.md`
- Context handoff artifact `9a629dfe_context-builder_0_output.md`
- Current scaffold files: `docker-compose.yml`, `Makefile`, `sqlc.yaml`, `go.mod`, and placeholder paths under `db/` and `internal/`

Important current observations:

- `docker-compose.yml` already defines PostgreSQL 17 and MinIO services.
- `Makefile` already has `dev`, `down`, `logs`, `migrate-up`, `migrate-down`, `sqlc`, and `test` targets, but `goose` and `sqlc` are assumed to be globally available.
- `sqlc.yaml` already targets PostgreSQL migrations in `db/migrations`, queries in `db/queries`, and generated Go output in `internal/db` using `pgx/v5`.
- `go.mod` currently has no required dependencies beyond the module declaration.
- `.env.example` could not be read by the available file-read tool because it was blocked by the safety policy. The apply phase must inspect it through an approved path before changing config examples.

## Architectural Decisions

| Area | Decision |
|---|---|
| Local stack | Keep Docker Compose as the local dependency runner for PostgreSQL and MinIO. `make dev` remains the entry point. |
| MinIO/WORM | Treat MinIO as local S3-compatible readiness only. Do not implement Object Lock enforcement, bucket lifecycle guarantees, audio evidence storage, or evidence ledger semantics in #13. |
| Migrations | Use Goose SQL migrations under `db/migrations`. The initial migration owns schema shape and RLS enablement. |
| Persistence | Keep SQL-first persistence through sqlc + pgx. Do not introduce an ORM. |
| RLS scope | Enable RLS at the schema level for every tenant-scoped table. Runtime API-key auth, tenant session variables, and isolation proof belong to #14. |
| Core types | Add pure Go types in `internal/core` only. They must not import db/sqlc, pgx, HTTP, River, Harness, MCP, or Bedrock packages. |
| Config | Add `internal/config` fail-fast loading and validation for #13 bootstrap variables. Bedrock variables may exist as optional scaffold only. |
| Harness paths | Preserve `internal/harness`, `data/synthetic/cases`, and `data/synthetic/harness-runs` as directories only. No Harness runtime behavior. |
| Tooling | Make Goose and sqlc reproducible from the repo instead of relying on a developer's global PATH. |

## Files and Areas to Change Later in Apply

Expected #13 implementation areas:

- `docker-compose.yml`
  - Verify PostgreSQL and MinIO are sufficient for local readiness.
  - Adjust comments if they overclaim WORM/Object Lock behavior.
- `Makefile`
  - Keep `make dev`, `make down`, `make logs`.
  - Make `make migrate-up`, `make migrate-down`, and `make sqlc` use reproducible local tooling.
  - Add a tool bootstrap target if needed.
- `.env.example`
  - Document required local bootstrap variables and optional future provider variables.
  - Must be inspected before editing because direct read was blocked during design.
- `go.mod` / `go.sum`
  - Add only #13 dependencies and/or pinned tool references needed for config, pgx/sqlc runtime, UUID handling, and reproducible tools.
- `tools.go` and/or `bin/` workflow
  - Pin Goose and sqlc versions without committing downloaded binaries.
- `db/migrations/`
  - Add initial Goose SQL migration.
- `db/queries/`
  - Add minimal queries that prove sqlc generation and future repository integration.
- `internal/db/`
  - Generated sqlc output if the project chooses to commit generated code.
- `internal/config/`
  - Config structs, environment loader, validation, and behavior-focused tests.
- `internal/core/`
  - Pure type scaffolding for foundational domain/data concepts.
- `internal/harness/`, `data/synthetic/cases/`, `data/synthetic/harness-runs/`
  - Preserve with `.gitkeep` or equivalent placeholder files only.

## Schema Design

### Table Categories

Use two explicit table categories so reviewers can verify tenant scope without guessing.

| Category | Intended tables | Tenant/RLS rule |
|---|---|---|
| Global/reference tables | `tenants`, global policy/rule catalogs if introduced | No `tenant_id` required unless rows are tenant-owned. RLS is not required by #13 acceptance for truly global tables. |
| Tenant-scoped tables | `tenant_api_keys`, `debtors`, `interaction_events`, `detector_result_rows`, tenant-owned policy bundles or joins if introduced | Must include `tenant_id uuid not null` and `ALTER TABLE ... ENABLE ROW LEVEL SECURITY`. |

The exact migration should stay minimal enough to satisfy #13 and downstream foundations. A reasonable initial shape is:

- `tenants`
  - Global root table for tenant identity.
  - Suggested fields: `id`, `slug`, `name`, `status`, `created_at`, `updated_at`.
- `tenant_api_keys`
  - Tenant-scoped credential metadata for #14 to use later.
  - Suggested fields: `id`, `tenant_id`, `key_hash`, `label`, `status`, `created_at`, `expires_at`, `last_used_at`.
  - #13 creates schema only; #14 owns key validation flow and request/session tenant context.
- `debtors`
  - Tenant-scoped debtor records needed for collection interactions.
  - Suggested fields: `id`, `tenant_id`, external reference, display/name fields only as needed, `created_at`, `updated_at`.
- `interaction_events`
  - Tenant-scoped interaction metadata.
  - Suggested fields: `id`, `tenant_id`, `debtor_id`, channel/type/status fields, `occurred_at`, optional transcript/object references, `created_at`.
  - Do not implement evidence storage semantics in #13.
- `policy_bundles`, `policy_rules`, `policy_bundle_rules`
  - Versioned compliance policy scaffolding.
  - If policy bundles are tenant-specific, include `tenant_id` and enable RLS. If they are global reference data, keep them tenant-free and document them as global/reference.
- `detector_result_rows`
  - Tenant-scoped normalized detector result scaffolding.
  - Suggested fields: `id`, `tenant_id`, `interaction_event_id`, detector identifier, severity/outcome fields, structured payload, `created_at`.
  - Do not implement detector behavior in #13.

Avoid adding evidence ledger, Merkle checkpoint, complaint workflow, STT, River job, Harness event log, or MCP tables unless a later approved artifact explicitly moves them into #13.

### RLS Policy Approach

Issue #13 should prove schema readiness, not runtime isolation.

Implementation guidance:

1. Every tenant-scoped table gets `tenant_id uuid not null` and a foreign key to `tenants(id)`.
2. Every tenant-scoped table executes `ALTER TABLE <table> ENABLE ROW LEVEL SECURITY` in the initial migration.
3. Policies may define the future application contract using a session variable such as `app.tenant_id`, for example a tenant filter equivalent to `tenant_id = current_setting('app.tenant_id', true)::uuid`, with safe handling when the setting is absent.
4. Do not claim runtime isolation from those policies in #13. #14 must set tenant context after API-key auth and prove cross-tenant request behavior.
5. Do not add HTTP middleware, API-key verification, request transactions, or per-request `SET LOCAL app.tenant_id` behavior in #13.
6. Do not use `FORCE ROW LEVEL SECURITY` unless the implementation also defines how local migration/sqlc verification avoids table-owner bypass surprises. The acceptance criterion is schema-level RLS enablement.

Verification should query PostgreSQL catalogs after migration and assert that the intended tenant-scoped tables have RLS enabled. That catalog check is evidence of schema-level readiness only.

## sqlc Strategy

Keep sqlc as a narrow compile-time proof, not a repository layer design for every future use case.

- Keep `sqlc.yaml` aligned with:
  - `schema: db/migrations`
  - `queries: db/queries`
  - output: `internal/db`
  - driver: `pgx/v5`
- Add minimal query files by table category:
  - tenant lookup/create/list queries only as needed to prove generation;
  - tenant-scoped queries that include `tenant_id` predicates explicitly;
  - no query that requires #14 middleware, River runtime, or Harness behavior.
- Generated code must compile with `go test ./...` after packages exist.
- If generated code is committed, expect line-count pressure and call that out in the task review forecast.
- Keep domain mapping outside generated code. Future adapters can translate `internal/db` rows into `internal/core` types.

## Configuration Design

`internal/config` should expose a small validated configuration value for local bootstrap. It should load from environment variables and fail fast before dependent runtime work begins.

### Required Configuration Categories

| Category | Examples | Required for #13? | Notes |
|---|---|---:|---|
| App/runtime | `APP_ENV` or `VIGIA_ENV`, optional `LOG_LEVEL` | Yes for environment identity; logging can have a default | Keep simple and local-development friendly. |
| Database | `DATABASE_URL` | Yes | Used by migrations and future app startup. |
| Object storage / MinIO | S3-compatible endpoint, access key, secret key, bucket name, path-style/SSL flag | Yes if config loader models object storage in #13 | Readiness only; no bucket creation, lifecycle, WORM, or evidence behavior. |
| Optional AWS/Bedrock scaffold | `AWS_REGION`, `BEDROCK_MODEL_ID` | No | Optional fields only. Missing values must not fail #13 tests or demos. Bedrock stays opt-in for #22. |

Validation rules:

- Missing required variables return a useful error naming each missing/invalid key.
- Optional Bedrock/AWS fields are parsed only when present and must never become default requirements.
- Config loading should be unit-testable without Docker or network access.
- Avoid package-level global config mutation unless the project later needs it; prefer explicit `Load`/`Validate` boundaries.

## Core Type Scaffolding

Add pure Go types under `internal/core` for the foundational data model. These are data/domain scaffolds only, not active workflow behavior.

Required types:

- `Tenant`
- `Debtor`
- `InteractionEvent`
- `TenantAPIKey`
- `DetectorResultRow`
- `PolicyBundleRule`

Design rules:

- Use standard library types where possible (`time.Time`, strings, typed IDs as needed).
- Do not import `internal/db`, pgx, sqlc, HTTP, River, Harness, MCP, Bedrock, or cloud SDK packages.
- Keep validation minimal. Do not add behavior tests that merely restate field declarations.
- Prefer small, explicit types over a generic catch-all model abstraction.
- If UUID v7 helpers are introduced, keep them infrastructure-neutral and avoid coupling core structs to database libraries.

## Scaffold Path Preservation

Issue #13 must preserve these paths in a fresh clone:

- `internal/harness`
- `data/synthetic/cases`
- `data/synthetic/harness-runs`

Use `.gitkeep` or another inert placeholder. These paths must not contain required runtime behavior, model-provider behavior, domain-agent behavior, event-log behavior, demo CLI behavior, MCP integration, or Bedrock integration.

## Tool Reproducibility

The context pass found `goose` and `sqlc` missing from PATH. #13 should not rely on globally installed tools.

Preferred approach:

1. Pin Goose and sqlc versions in the repository using a Go tool-management pattern compatible with the project's Go version.
2. Add a Make target such as `make tools` that installs pinned tools into a repo-local `bin/` directory.
3. Make `make migrate-up`, `make migrate-down`, and `make sqlc` use the repo-local binaries or invoke the pinned tool path.
4. Do not commit downloaded tool binaries.

Rejected as the primary path:

- Global installation documentation only: too fragile for reproducible onboarding.
- Docker-only wrappers for Goose/sqlc: more moving parts and slower feedback for a Go-first repository.
- Vendoring generated binaries: unnecessary repository bloat.

## Testing and Verification Plan

Strict TDD is active for implementation phases. Tests should prove behavior and state, not restate field definitions.

### Focused tests to add during apply

- `internal/config`
  - Valid env returns expected config.
  - Missing required env returns a useful error naming the missing keys.
  - Optional `AWS_REGION` / `BEDROCK_MODEL_ID` absence does not fail #13.
- Migration/schema verification
  - Prefer an integration-style check, skippable in short mode if it requires PostgreSQL.
  - Verify the initial migration applies successfully.
  - Verify RLS is enabled for the expected tenant-scoped tables through PostgreSQL catalog metadata.
- sqlc compile verification
  - Ensure generated query package compiles through `go test ./...`.
  - Add only meaningful smoke coverage if it proves generated code integration; do not test sqlc internals.

### Commands for #13 acceptance evidence

Expected verification commands after implementation:

```sh
make tools        # if the apply phase adds repo-local tool bootstrap
make dev
make migrate-up
make sqlc
go test ./...
```

Additional useful database check after migration:

```sql
select c.relname, c.relrowsecurity
from pg_class c
join pg_namespace n on n.oid = c.relnamespace
where n.nspname = 'public'
  and c.relname in ('tenant_api_keys', 'debtors', 'interaction_events', 'detector_result_rows');
```

The exact table list must match the final migration. This check validates schema-level RLS enablement only.

## Alternatives Considered and Rejected

| Alternative | Rejected because |
|---|---|
| Implement #14 API-key middleware and tenant session context in #13 | Violates resolved boundary. #14 owns runtime auth and request-level RLS proof. |
| Treat MinIO Object Lock/WORM behavior as #13 acceptance | Overclaims scope. #13 is local infrastructure/config readiness only. |
| Add River worker/job proof now | Conflicts with resolved ownership: River runtime proof belongs to #1 unless explicitly re-scoped later. |
| Build Harness runtime behavior in scaffold paths | Violates #13 scope and the #16/#18-#22 plan. Paths only. |
| Make MCP the internal Harness runtime | Explicitly forbidden. MCP remains later external integration. |
| Merge Judge and Harness model ports | Explicitly forbidden. Later ports stay separate. |
| Require Bedrock variables for config validation | Violates Bedrock opt-in boundary. Bedrock remains #22 and optional. |
| Introduce GORM/ORM | Conflicts with SQL-first sqlc + pgx architecture. |
| Depend on global Goose/sqlc installs | Not reproducible; known local missing-tool risk. |

## Open Risks and Stop Conditions

| Risk / stop condition | Action |
|---|---|
| `.env.example` contents remain unverified because direct read was blocked in design | Apply phase must inspect it through an approved path before editing or relying on existing variable names. |
| Initial schema grows into ledger, complaint workflow, Harness, MCP, River, or auth runtime behavior | Stop and ask for explicit re-scope before continuing. |
| RLS policy implementation requires runtime tenant context to pass tests | Stop and keep #13 at schema-level enablement; move runtime proof to #14. |
| Goose/sqlc reproducibility cannot be solved cleanly with repo-local pinned tools | Stop and ask before falling back to global install assumptions. |
| Generated sqlc output pushes the diff far beyond the 400-line review budget | Pause before apply delivery decisions; consider chained review slices or an explicit size exception. |
| Tests would only restate pure struct field declarations | Do not add those tests. Focus tests on config validation, migration application, RLS catalog state, and compile integration. |

## Review Workload Forecast Implications

Issue #13 is likely medium-to-high review workload because it can touch migrations, Makefile/tooling, config, core types, sqlc queries, generated code, and tests.

Forecast signals:

- Chained PRs recommended: **Possible / likely if generated sqlc output is committed**.
- 400-line budget risk: **Medium to High**.
- Decision needed before apply: **Yes if tasks forecast generated code + schema + config + tests above budget**.

Recommended task slicing:

1. Tooling and local stack reproducibility.
2. Migration/schema/RLS foundation.
3. sqlc queries and generated code compile proof.
4. `internal/config` with behavior tests.
5. Pure `internal/core` scaffolding and path preservation.

Keep tests with the work unit they verify. Do not commit, push, or create PRs during SDD planning or apply unless explicitly instructed later.

## Rollout and Rollback

Rollout is local-development only:

1. Install pinned tools through the repo target.
2. Start local services.
3. Apply migrations to local PostgreSQL.
4. Generate sqlc output.
5. Run Go tests.

Rollback is straightforward:

- Revert #13 files as a foundation work unit.
- Run `make down` to stop local services.
- Reset local Docker volumes if a local migration state must be discarded.
- No production rollback is required because #13 does not deploy production behavior or migrate production data.

## Next Recommended Phase

Tasks.
