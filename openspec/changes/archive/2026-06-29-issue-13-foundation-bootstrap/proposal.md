# Proposal: Issue #13 Foundation Bootstrap

## Problem / motivation

Vigía is currently a pre-build scaffold: local service definitions, a Go module, `sqlc.yaml`, and placeholder directories exist, but there are no migrations, queries, generated database code, configuration loader, or core Go types. This blocks every downstream slice because later work needs a reproducible local environment, a tenant-aware schema foundation, and stable project structure before behavior can be implemented.

Issue #13 should create the foundation for development without absorbing later product behavior. The first slice must make local infrastructure and schema readiness real while preserving the agreed issue order and architecture boundaries.

## Intent

Establish the minimum reviewable foundation required for subsequent implementation phases:

- local Postgres and MinIO development services;
- initial database schema with tenant/RLS foundations;
- SQL-first database generation using sqlc;
- fail-fast application configuration loading;
- core project/domain scaffolding; and
- placeholder Harness Lab directories only, with no harness behavior.

## Goals

- `make dev` boots the local development dependencies for #13: PostgreSQL and MinIO.
- `make migrate-up` applies the initial schema.
- Every tenant-scoped table has `tenant_id` and RLS enabled at the schema level.
- `make sqlc` generates compiling Go query code from `db/queries`.
- `internal/config` loads required environment variables and fails fast when required configuration is missing.
- Preserve the foundation directory scaffold so a fresh clone has the expected structure.
- Scaffold only these Harness Lab paths for later issues:
  - `internal/harness`
  - `data/synthetic/cases`
  - `data/synthetic/harness-runs`

## Non-goals

- Do not implement WORM behavior, audio evidence storage, evidence ledger behavior, or bucket lifecycle guarantees.
- Do not require #13 to create or verify a MinIO bucket with Object Lock unless it is already trivial and non-invasive in the existing bootstrap.
- Do not implement #14 auth middleware, API-key validation flow, or request/session tenant context.
- Do not implement #1 API/console walking skeleton or River runtime proof.
- Do not implement #16/#18-#22 Harness runtime behavior, model providers, domain agents, demo CLI, event logs, validation, budgets, or Bedrock adapter.
- Do not implement #17 Remote MCP server or make MCP the internal Harness runtime.
- Do not merge Judge and Harness model ports.
- Do not make Bedrock a default path for tests or demos; later Harness tests/demo should use a Fake Model Provider by default, with Bedrock opt-in only in #22.
- Do not add detectors, Judge behavior, STT, UI, production observability, or real debtor/PII data.

## Scope boundaries

### In scope for #13

- Local development bootstrap for PostgreSQL and MinIO via existing project commands.
- Environment examples and config validation needed by the bootstrap.
- Goose-compatible initial SQL migrations.
- Tenant-aware schema foundations, including RLS enabled on tenant-scoped tables.
- Minimal sqlc queries sufficient to prove generation and compile-time integration.
- Generated `internal/db` code if the repository convention commits generated sqlc output.
- Pure core/domain type scaffolding needed to represent the canonical data model.
- Directory preservation for expected package/data paths.
- Harness Lab scaffolding paths only, without behavior.

### Out of scope for #13

- Runtime tenant isolation proof through HTTP middleware or per-request transaction session variables; that belongs to #14.
- River worker boot/job processing proof; that remains in #1 unless explicitly re-scoped later.
- Agent Harness Lab behavior; #16 remains the parent epic for #18 -> #19 -> #20 -> #21 -> #22.
- Remote MCP, external tool contracts, or AI-client integration; #17 remains after #16 and #14.

## Affected areas

- `docker-compose.yml` and local service environment for PostgreSQL and MinIO.
- `Makefile` targets for bootstrap, migrations, sqlc generation, and tests.
- `.env.example` and configuration defaults/documentation.
- `db/migrations/` for initial schema and RLS enablement.
- `db/queries/` for minimal sqlc-backed queries.
- `internal/db/` for generated query code.
- `internal/config/` for required environment loading and validation.
- `internal/core/` for pure data/domain scaffolding.
- `internal/harness/`, `data/synthetic/cases/`, and `data/synthetic/harness-runs/` for placeholder structure only.

## Acceptance criteria aligned to #13

- `make dev` starts PostgreSQL and MinIO for local development.
- `make migrate-up` applies the initial schema successfully against the configured database.
- RLS is enabled on every tenant-scoped table introduced by the initial schema.
- Tenant-scoped tables include `tenant_id` and remain distinguishable from global/reference tables.
- `make sqlc` generates Go query code from `db/queries` without errors.
- `go test ./...` or `make test` passes once the package scaffold exists.
- `internal/config` validates required environment variables at startup and fails fast with a useful error when required values are missing.
- The committed/preserved directory scaffold survives a fresh clone, including `internal/harness`, `data/synthetic/cases`, and `data/synthetic/harness-runs`.
- MinIO/Object Lock scope is limited to local infrastructure/config readiness and WORM intent; bucket lifecycle guarantees and WORM behavior are not required in #13.

## Architecture / ADR alignment

- **Workflow-first authority:** #13 only prepares foundations. It does not convert the product into an autonomous agent loop and does not implement compliance authority behavior.
- **Clean/hexagonal boundaries:** core types should remain pure and framework-free. Database access belongs behind generated SQL code and later adapters, not inside domain objects.
- **SQL-first persistence:** use PostgreSQL, goose migrations, sqlc, and pgx-aligned generation. Do not introduce an ORM for #13.
- **Tenant/RLS foundation split:** #13 creates schema-level tenant and RLS foundations. #14 owns runtime API-key authentication, tenant session context, and request-level isolation proof.
- **Harness boundary:** #13 may create harness directories only. Harness behavior stays in #16/#18-#22.
- **Separate model ports:** keep Judge and Harness model ports separate in later phases; #13 should not introduce a shared model abstraction.
- **MCP boundary:** MCP remains an external integration surface for #17, not an internal Harness runtime.
- **Bedrock boundary:** Bedrock is opt-in only in #22; later harness tests/demo default to Fake Model Provider.

## Risks and mitigations

| Risk | Severity | Mitigation |
|---|---:|---|
| Scope creep into auth, River, Harness, MCP, or Bedrock behavior | High | Keep #13 limited to bootstrap/schema/config/sqlc/scaffold; document later issue ownership in spec and tasks. |
| RLS false confidence | High | Acceptance should distinguish schema-level RLS enablement from runtime tenant isolation, which belongs to #14. |
| Missing local `goose` or `sqlc` tooling blocks validation | High | Spec/tasks should require reproducible tool setup or explicit install documentation before implementation is considered complete. |
| Initial schema and core types may exceed the 400-line review budget | Medium | Split tasks by reviewable work units and keep generated code/tooling expectations explicit. If the forecast exceeds budget, pause before apply for delivery strategy. |
| MinIO Object Lock/WORM expectations could expand #13 beyond readiness | Medium | Treat #13 as local infrastructure/config readiness only; defer bucket lifecycle guarantees and WORM behavior. |
| River proof conflict between planning docs and issue ownership | Medium | Record that River runtime proof remains #1 unless explicitly re-scoped; #13 may keep only placeholders. |

## Rollback

Rollback should be straightforward because #13 is a foundation slice with no production data migration:

- revert bootstrap/config/schema/query/core scaffold changes as one work unit if the foundation direction is wrong;
- run `make down` to stop local services;
- if local migrations were applied during validation, drop/reset the local development database volume rather than preserving partial local state;
- no production rollback path is required for #13 because this proposal does not include deployment or production data changes.

## Success criteria

- A developer can start the local infrastructure with `make dev` and run the initial schema path.
- The database foundation proves tenant-scoped schema and RLS enablement without claiming #14 runtime isolation.
- sqlc generation succeeds and produced code compiles.
- Required environment configuration fails fast and is understandable.
- The repository has stable foundation paths for future slices without implementing their behavior.
- Reviewers can verify #13 without reading or approving unrelated Harness, MCP, River, UI, or auth middleware behavior.

## Out-of-scope later issues

- **#16** remains the parent epic for Agent Harness Lab and owns the chain **#18 -> #19 -> #20 -> #21 -> #22**.
- **#18** owns Harness runtime skeleton and invariant tests after #13.
- **#19** owns tool contracts and synthetic Case fixtures.
- **#20** owns deterministic Case orchestrator and Domain Agents.
- **#21** owns demo CLI and Case Brief outputs with Fake Model Provider default.
- **#22** owns explicit Bedrock Claude opt-in provider.
- **#14** owns tenant auth middleware, API-key flow, and runtime RLS tenant context.
- **#1** owns the thin walking skeleton and River worker proof after #13 and #14.
- **#17** owns Remote MCP after #16 and #14.

## Review workload note for 400-line budget

Issue #13 is likely close to or above the 400 changed-line review budget if it includes migrations, core types, config loader, tests, sqlc queries, and generated code in one diff. The next phases should forecast review workload carefully and prefer work-unit boundaries that keep schema/config/sqlc changes reviewable. If implementation forecasting exceeds the budget, pause before apply for an explicit delivery decision rather than silently producing an oversized PR.

## Proposal question round

The user has already resolved the key product/scope decision for #13: MinIO/WORM means local infrastructure/config readiness only, not bucket lifecycle guarantees or WORM behavior. The following questions are recorded for interactive review and can improve the spec if the user wants a second pass:

1. Which schema entities are mandatory in the first migration for #13 versus acceptable as later migrations if the review budget is at risk?
2. Should #13 require reproducible local tool pinning for `goose` and `sqlc`, or is documented installation acceptable for the first slice?
3. For MinIO/Object Lock intent, is documentation and environment configuration enough, or should `make dev` include a non-invasive readiness note/check if trivial?
4. Should generated sqlc code be committed in #13, or should the repository rely on `make sqlc` generation during developer setup/CI?

Current proposal assumptions: #13 remains a foundation/bootstrap issue; schema-level RLS is required but runtime tenant context is #14; River proof remains #1; Harness paths are scaffolds only; MCP and Bedrock remain later opt-in boundaries.

## Next recommended phase

Spec.
