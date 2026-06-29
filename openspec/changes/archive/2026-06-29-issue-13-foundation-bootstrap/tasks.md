# Tasks: Issue #13 Foundation Bootstrap

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 700-1,200 if generated sqlc output is committed; 350-650 without generated sqlc output |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 tooling/local stack/config -> PR 2 schema/RLS -> PR 3 sqlc queries/generated code -> PR 4 core types/scaffold preservation |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

Generated sqlc output may push the diff over the 400-line review budget by itself. If generated `internal/db` code is committed, treat it as its own review unit or pause for an explicit size exception before apply.

## Scope Guardrails

- [x] Keep issue #13 limited to dev env/bootstrap, goose migrations, schema-level tenant/RLS foundations, sqlc generation, `internal/config` validation, pure `internal/core` type scaffolding, and scaffold path preservation.
- [x] Do not commit, push, create a PR, or edit unrelated planning documents.
- [x] Do not implement WORM behavior, bucket lifecycle guarantees, audio evidence storage, evidence ledger behavior, #14 auth/session tenant context, #1 River runtime proof, Harness behavior, MCP runtime/integration, Judge/Harness model ports, or Bedrock defaults.
- [x] Stop and ask before continuing if implementation requires a workaround, crosses the #13 boundary, depends on runtime tenant context, or cannot make goose/sqlc tooling reproducible without global PATH assumptions.

## Work Unit 0: Dependency-Ready Preflight

- [x] Inspect `git status --short` and preserve existing user/repo changes before editing.
- [x] Inspect `.env.example` through an approved local path before changing it; do not infer its current contents from planning artifacts.
- [x] Confirm current bootstrap files and targets before editing: `docker-compose.yml`, `Makefile`, `sqlc.yaml`, `go.mod`, `db/migrations/`, `db/queries/`, `internal/config/`, `internal/core/`, `internal/db/`.
- [x] Record baseline verification:
  - `go env GOVERSION GOMOD`
  - `go test ./...` (currently may report no packages before implementation)
  - `command -v docker`
  - `command -v goose || true`
  - `command -v sqlc || true`

## Work Unit 1: Tooling and Local Dependency Bootstrap

- [x] Update `Makefile` so `make dev`, `make down`, and `make logs` remain the local dependency entry points for PostgreSQL and MinIO only.
- [x] Add reproducible repo-local tooling for goose and sqlc using concrete files such as `tools.go`, `go.mod`, `go.sum`, `Makefile`, and a repo-local `bin/` workflow; do not commit downloaded binaries.
- [x] Ensure `make migrate-up`, `make migrate-down`, and `make sqlc` use pinned/reproducible tooling rather than assuming global `goose` or `sqlc` are already installed.
- [x] Adjust `docker-compose.yml` only if needed for #13 local readiness; do not claim MinIO WORM/Object Lock enforcement or bucket lifecycle guarantees.
- [x] Focused validation:
  - `make tools`
  - `make dev`
  - `docker compose ps`
  - `make down`
- [x] Acceptance mapping: Local Development Dependencies; MinIO readiness-only scenario; downstream runtime boundary scenarios.
- [x] Rollback boundary: revert `Makefile`, tool-pinning files, and local compose changes; run `make down`.

## Work Unit 2: Fail-Fast Configuration Loading (RED -> GREEN -> REFACTOR)

- [x] RED: Add behavior-focused tests in `internal/config/config_test.go` before implementation:
  - valid #13 environment returns a validated config;
  - missing required keys fail fast and name the missing/invalid keys;
  - absent `AWS_REGION` and `BEDROCK_MODEL_ID` do not fail #13 defaults.
- [x] Run `go test ./internal/config` and confirm the new tests fail for the right reason before production code exists.
- [x] GREEN: Implement `internal/config` with explicit load/validate boundaries and no package-level global mutation unless unavoidable.
- [x] Update `.env.example` with required local bootstrap variables for database and MinIO/S3-compatible readiness; keep Bedrock/AWS variables optional if present.
- [x] REFACTOR: Keep config code small and framework-free; do not introduce Harness, MCP, River, Bedrock SDK, or HTTP dependencies.
- [x] Focused validation:
  - `go test ./internal/config`
  - `go test ./...`
- [x] Acceptance mapping: Fail-Fast Configuration Loading; Preserve later-provider opt-in boundaries.
- [x] Rollback boundary: revert `internal/config/*` and `.env.example` changes.

## Work Unit 3: Initial Goose Migration and Schema-Level RLS (RED -> GREEN -> TRIANGULATE)

- [x] RED: Add a meaningful migration/RLS verification test or script target before the final migration, using a concrete path such as `internal/db/migration_test.go` or a `Makefile` validation target. It must be skippable in `testing.Short()` or when `DATABASE_URL` is unavailable.
- [x] Add `db/migrations/00001_initial_foundation.sql` as a Goose SQL migration with reversible `Up`/`Down` sections.
- [x] Keep the schema minimal for #13 foundations. Include tenant/global foundations and only tenant-scoped tables needed for #13, such as `tenant_api_keys`, `debtors`, `interaction_events`, `detector_result_rows`, and policy bundle/rule join structures if represented.
- [x] For every tenant-scoped table introduced, add `tenant_id uuid not null`, a tenant foreign key, and `ALTER TABLE ... ENABLE ROW LEVEL SECURITY`.
- [x] Add tenant-preserving composite uniqueness and foreign keys for tenant-scoped child relationships, including `interaction_events -> debtors`, `detector_result_rows -> interaction_events`, and `policy_bundle_rules -> policy_bundles`.
- [x] Distinguish global/reference tables such as `tenants` from tenant-scoped tables in the migration and verification expectations.
- [x] Do not add evidence ledger, complaint workflow, STT, River job, Harness event log, MCP, API/auth middleware, or storage-semantics tables in #13.
- [x] TRIANGULATE: Verify RLS through PostgreSQL catalog metadata, not runtime request isolation.
- [x] Focused validation:
  - `make dev`
  - `make migrate-up`
  - `go test ./internal/db -run 'Test.*Migration|Test.*RLS'`
  - `psql "$DATABASE_URL" -c "select c.relname, c.relrowsecurity from pg_class c join pg_namespace n on n.oid = c.relnamespace where n.nspname = 'public' and c.relkind = 'r';"`
- [x] Acceptance mapping: Initial Schema Migration; Tenant-Scoped Tables and Schema-Level RLS; Runtime tenant isolation remains issue #14.
- [x] Rollback boundary: run `make migrate-down` where safe, or reset local dev database volumes after `make down`; revert `db/migrations/*` and related migration tests.

## Work Unit 4: sqlc Query Generation and Compile Proof

- [x] Add minimal SQL queries under `db/queries/` that prove sqlc generation against the #13 schema without designing every future repository method.
- [x] Ensure tenant-scoped queries include explicit `tenant_id` predicates where applicable.
- [x] Run `make sqlc` and commit generated `internal/db` code only if that is the repository convention for #13; do not hand-edit generated files.
- [x] If generated output causes the work unit to exceed the 400-line review budget, stop before apply continuation and request chained-PR or size-exception direction.
- [x] Add only meaningful compile/integration coverage if needed; do not test sqlc internals or generated field declarations.
- [x] Focused validation:
  - `make sqlc`
  - `go test ./internal/db`
  - `go test ./...`
- [x] Acceptance mapping: SQLC Query Generation; SQL-first persistence boundary.
- [x] Rollback boundary: revert `db/queries/*` and generated `internal/db/*` output from this work unit.

## Work Unit 5: Pure Core Types and Scaffold Path Preservation

- [x] Add pure Go type scaffolding under `internal/core/` for #13 foundation entities represented by the schema, including `Tenant`, `Debtor`, `InteractionEvent`, `TenantAPIKey`, `DetectorResultRow`, and `PolicyBundleRule`.
- [x] Keep `internal/core` free of imports from `internal/db`, pgx/sqlc, HTTP frameworks, River, Harness runtime packages, MCP packages, Bedrock/cloud SDKs, or object-storage SDKs.
- [x] Do not add tests that merely restate struct fields. Add tests only if they prove behavior, parsing, validation, or import-boundary constraints.
- [x] Preserve scaffold paths with inert placeholder files only:
  - `internal/harness/.gitkeep` or equivalent;
  - `data/synthetic/cases/.gitkeep` or equivalent;
  - `data/synthetic/harness-runs/.gitkeep` or equivalent.
- [x] Ensure scaffold directories contain no Harness runtime behavior, model-provider behavior, domain-agent behavior, event-log behavior, demo CLI behavior, MCP integration, or Bedrock integration.
- [x] Focused validation:
  - `go test ./internal/core`
  - `go list -deps ./internal/core`
  - `find internal/harness data/synthetic/cases data/synthetic/harness-runs -maxdepth 1 -type f -print`
  - `go test ./...`
- [x] Acceptance mapping: Core Foundation Types; Preserved Scaffold Paths; Harness behavior remains out of scope.
- [x] Rollback boundary: revert `internal/core/*` and inert scaffold placeholder changes.

## Work Unit 6: Final #13 Acceptance Verification

- [x] Run the full focused acceptance command set after all selected work units are complete:
  - `make tools`
  - `make dev`
  - `make migrate-up`
  - `make sqlc`
  - `make test` or `go test ./...`
- [x] Run the RLS catalog check against the final tenant-scoped table list and record which tables have `relrowsecurity = true`.
- [x] Verify no forbidden scope landed by inspecting changed paths and dependencies for auth middleware/session tenant context, River jobs, Harness runtime, MCP, merged model ports, Bedrock defaults, evidence ledger behavior, or WORM semantics.
- [x] Run `git diff --stat` and compare additions + deletions against the 400-line budget before reporting ready for review.
- [x] Run `make down` after local-service validation unless the parent explicitly wants services left running.

## Acceptance Mapping Summary

| Spec requirement | Covered by tasks |
|---|---|
| Local Development Dependencies | Work Units 1 and 6 |
| Initial Schema Migration | Work Units 3 and 6 |
| Tenant-Scoped Tables and Schema-Level RLS | Work Units 3 and 6 |
| SQLC Query Generation | Work Units 4 and 6 |
| Fail-Fast Configuration Loading | Work Unit 2 |
| Core Foundation Types | Work Unit 5 |
| Preserved Scaffold Paths | Work Unit 5 |
| Downstream Runtime Boundaries | Scope Guardrails, Work Units 1-6 |

## Stop Conditions

- [x] Stop if #13 starts requiring #14 API-key middleware, tenant session variables, request-level RLS proof, or runtime auth tests.
- [x] Stop if River worker boot, queue processing, or trivial job proof becomes necessary; that remains #1 unless explicitly re-scoped later.
- [x] Stop if MinIO work turns into WORM enforcement, bucket lifecycle guarantees, audio evidence storage, or evidence ledger semantics.
- [x] Stop if Harness runtime, model providers, domain agents, event logs, demo CLI, MCP integration, or Bedrock SDK/default behavior appears in the implementation path.
- [x] Stop if goose/sqlc reproducibility requires non-idiomatic workarounds or global tool assumptions.
- [x] Stop before apply continuation if generated sqlc output or combined schema/config/core changes exceed the 400-line review budget without a chained-PR or size-exception decision.
