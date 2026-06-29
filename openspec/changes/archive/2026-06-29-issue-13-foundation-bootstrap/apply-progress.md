# Apply Progress: Issue #13 Foundation Bootstrap

## Status consumed

- Change: `issue-13-foundation-bootstrap`
- Artifact store: `openspec`
- Structured status: authoritative, `applyState: ready`, `nextRecommended: sdd-apply`
- Action context: `repo-local`, workspace `/Users/ricardoaltamirano/Developer/vigia`, allowed edit root `/Users/ricardoaltamirano/Developer/vigia`
- Strict TDD: active; runner `go test ./...` / `make test`
- Delivery path: user-approved size exception for one complete #13 diff despite high 400-line risk

## Workload / PR boundary

Implemented as one size-exception work unit as explicitly approved. Generated sqlc code and Go tool-management dependencies make this a large review unit; changed-line evidence is recorded below.

## Completed tasks and persisted checkbox updates

Persisted task checkboxes updated in `openspec/changes/issue-13-foundation-bootstrap/tasks.md`:

- Completed: 61 / 61 tasks.
- Remaining unchecked: 0 / 61 tasks.

Completed implementation coverage:

- Dependency-ready preflight completed, including git status, `.env.example` inspection through local shell, bootstrap file confirmation, and baseline command capture.
- Makefile now keeps `dev`, `down`, and `logs` as PostgreSQL + MinIO dependency entry points only.
- Repo-local pinned tooling added through Go tool directives, `go.sum`, and `bin/` install workflow; `make migrate-up`, `make migrate-down`, and `make sqlc` use repo-local binaries.
- Docker Compose retains PostgreSQL and MinIO only and describes MinIO as readiness-only, with no WORM/Object Lock lifecycle guarantees.
- `internal/config` added with behavior tests first, explicit env lookup/load/validate boundaries, required bootstrap variables, optional AWS/Bedrock fields, and no global config mutation.
- `.env.example` updated for #13 bootstrap variables and optional future AWS/Bedrock scaffold only.
- Goose initial migration added with reversible Up/Down sections, tenant/global distinction, tenant-scoped tables, `tenant_id uuid not null`, tenant foreign keys, and schema-level RLS enablement.
- Migration/RLS catalog test added before the final migration; it skips in `testing.Short()` or without `DATABASE_URL`.
- Minimal sqlc queries added; tenant-scoped reads include explicit `tenant_id` predicates.
- `make sqlc` generated compiling `internal/db` code; generated files were not hand-edited.
- Pure `internal/core` type scaffolding added with standard-library-only dependencies.
- Scaffold paths preserved with inert placeholders only: `internal/harness/.gitkeep`, `data/synthetic/cases/.gitkeep`, `data/synthetic/harness-runs/.gitkeep`.
- Forbidden downstream scope inspection completed; grep hits were only negative scope comments in `.env.example` / `docker-compose.yml`.
- Docker/PostgreSQL live validation completed: `make dev`, `docker compose ps`, `make migrate-up`, live RLS catalog query, live `DATABASE_URL` integration test, `make sqlc`, `make test`, and final `make down` all passed.

## Remaining unchecked tasks

```text
None.
```

Reason: Docker/PostgreSQL live validation is now complete, and the remaining task checkboxes were updated in `tasks.md`.

## Files changed by this apply

Tracked files changed:

- `.env.example`
- `Makefile`
- `docker-compose.yml`
- `go.mod`

New files/directories:

- `go.sum`
- `db/migrations/00001_initial_foundation.sql`
- `db/queries/tenants.sql`
- `db/queries/tenant_api_keys.sql`
- `db/queries/debtors.sql`
- `db/queries/interaction_events.sql`
- `db/queries/policies.sql`
- `db/queries/detector_result_rows.sql`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/core/types.go`
- `internal/db/migration_test.go`
- `internal/db/db.go`
- `internal/db/models.go`
- `internal/db/querier.go`
- `internal/db/tenants.sql.go`
- `internal/db/tenant_api_keys.sql.go`
- `internal/db/debtors.sql.go`
- `internal/db/interaction_events.sql.go`
- `internal/db/policies.sql.go`
- `internal/db/detector_result_rows.sql.go`
- `data/synthetic/cases/.gitkeep`
- `data/synthetic/harness-runs/.gitkeep`
- `openspec/changes/issue-13-foundation-bootstrap/apply-progress.md`

Pre-existing user/repo changes were observed and not intentionally edited: `HANDOFF.md`, `docs/architecture.md`, `docs/build-plan.md`, `docs/technical-design.md`, `CONTEXT.md`, `docs/adr/`, `docs/frontend-design.md`, `judgment/`, `research/`, `sdd/`, `.pi/`, and `openspec/` planning files other than this change's tasks/apply-progress.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| Config loader | `internal/config/config_test.go` | Unit | Baseline `go test ./...` failed pre-existing/no packages | `go test ./internal/config` failed on undefined `Load`, `FromMap`, `MissingKeysError` | `go test ./internal/config` passed | 3 behavior cases: valid env, missing required keys, optional AWS/Bedrock absent | Config kept framework-free, explicit lookup boundary, no package-level mutation |
| Migration/RLS catalog check | `internal/db/migration_test.go` | Skippable integration | Baseline no package existed | `go test ./internal/db -run TestTenantScopedTablesHaveTenantIDAndRLSEnabled` failed before dependency/migration existed | Live Docker/PostgreSQL run passed with `DATABASE_URL=postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable` | Table-driven subtests cover each tenant-scoped table and verify both `tenant_id` metadata and `relrowsecurity` | Test uses PostgreSQL catalog metadata only; no runtime tenant isolation proof |
| Core scaffolding | N/A | Structural | N/A | Triangulation skipped: pure structural type scaffolding; field-restating tests intentionally not added | `go test ./internal/core` passed with no test files | Import boundary validated with `go list -deps ./internal/core` | Core uses only `time` from the standard library |
| sqlc generation | Generated compile proof | Compile | `make tools` installed pinned tools | Queries written before generation | `make sqlc`, `go test ./internal/db`, and `go test ./...` passed | Tenant-scoped query set includes explicit tenant predicates on reads | Generated code was not hand-edited |

## Test Summary

- Total behavior/integration tests written: 4 top-level tests plus table-driven RLS subtests.
- Layers used: Unit (`internal/config`), skippable integration (`internal/db` catalog metadata), compile validation (`internal/db` generated sqlc package).
- Approval tests: none; no behavioral refactor of existing Go code existed.
- Pure functions/boundaries created: explicit `Load`, `LoadFromEnv`, `FromMap`, validation helpers.

## Commands run with evidence

| Command | Exit | Evidence |
|---|---:|---|
| `git status --short` | 0 | Pre-existing modified/untracked planning/docs state observed before editing. |
| `sed -n '1,220p' .env.example` | 0 | Existing `.env.example` inspected through local shell before editing. |
| `go env GOVERSION GOMOD` | 0 | `go1.26.4`, `/Users/ricardoaltamirano/Developer/vigia/go.mod`. |
| `go test ./...` baseline | 1 | Pre-existing expected failure: `./... matched no packages`. |
| `command -v docker` | 0 | `/usr/local/bin/docker`. |
| `command -v goose || true` | 0 | No global goose reported. |
| `command -v sqlc || true` | 0 | No global sqlc reported. |
| `go test ./internal/config` RED | 1 | Failed on undefined `Load`, `FromMap`, `MissingKeysError`. |
| `go test ./internal/db -run TestTenantScopedTablesHaveTenantIDAndRLSEnabled` RED | 1 | Failed before pgx dependency/migration setup. |
| `go get -tool github.com/pressly/goose/v3/cmd/goose@v3.25.0` | 0 | Added pinned Go tool directive/dependencies. |
| `go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0` | 0 | Added pinned Go tool directive/dependencies. |
| `go get github.com/jackc/pgx/v5@v5.7.5` | 0 | Added pgx runtime/test dependency. |
| `go test ./internal/config` | 0 | Config behavior tests passed. |
| `go test ./internal/db -run TestTenantScopedTablesHaveTenantIDAndRLSEnabled` without `DATABASE_URL` | 0 | Skipped/passed because live DB URL unavailable. |
| `go test ./internal/core` | 0 | Package compiled; no field-restating tests added. |
| `make tools` first attempt | 2 | Failed due missing transitive `go.sum` entry; fixed with `go mod tidy`. |
| `go mod tidy` | 0 | Completed and added missing sums. |
| `make tools` | 0 | Installed repo-local `bin/goose` and `bin/sqlc`; binaries are ignored. |
| `make sqlc` | 0 | Generated `internal/db` code. |
| `go test ./...` | 0 | `internal/config`, `internal/core`, and `internal/db` passed/compiled. |
| `make dev && docker compose ps && make migrate-up && RLS catalog check` | 1 | Historical first attempt blocked by Docker daemon unavailability; superseded by the successful live validation rows below. |
| `make test` | 0 | `go test ./...` passed. |
| `go list -deps ./internal/core` | 0 | Deps limited to stdlib/time and package itself. |
| `find internal/harness data/synthetic/cases data/synthetic/harness-runs -maxdepth 1 -type f -print` | 0 | Only `.gitkeep` placeholders. |
| Forbidden-scope grep | 0 | Only negative/disclaimer comments found, no implementation paths. |
| `make down` | 2 | Historical first attempt blocked by Docker daemon unavailability; superseded by successful final `make down`. |
| Static RLS declaration check via `awk` | 0 | All tenant-scoped tables declared with RLS in migration. |
| `go test ./internal/db` | 0 | Generated db package and migration test compiled/skipped as expected. |
| Final `make sqlc && go test ./...` | 0 | sqlc generation and full Go suite passed. |

## RLS live catalog evidence

Docker-backed live PostgreSQL catalog checks now show all final tenant-scoped tables have RLS enabled:

```text
       relname        | relrowsecurity
----------------------+----------------
 debtors              | t
 detector_result_rows | t
 interaction_events   | t
 policy_bundle_rules  | t
 policy_bundles       | t
 tenant_api_keys      | t
(6 rows)
```

## Review workload actuals

- `git diff --stat -- .env.example Makefile docker-compose.yml go.mod`: 4 tracked files, 140 insertions, 44 deletions.
- New-file line count from `wc -l`: 1,753 lines, including 443 lines of `go.sum` and generated sqlc output.
- Approximate #13 apply workload visible to review: 1,937 added/deleted/new lines before counting required OpenSpec progress/task artifact edits.
- This exceeds the 400-line review budget; proceeding is covered by the user-approved size exception.

## Deviations from design

- Used Go 1.26 `go.mod` tool directives plus repo-local `bin/` installation rather than a `tools.go` import file. This avoids importing command packages and still pins reproducible goose/sqlc tooling in `go.mod`, `go.sum`, and `Makefile`.
- Earlier live Docker/PostgreSQL validation was blocked by Docker daemon unavailability; this continuation completed the Docker-backed validation once the environment permitted it.

## Blockers / stop conditions

- No active blockers remain for #13 validation.
- Docker/PostgreSQL validation completed and local services were stopped with `make down`.
- No scope-expansion stop condition was hit. The implementation did not add auth middleware/session tenant context, River jobs/workers, Harness runtime behavior, MCP, merged model ports, Bedrock defaults, evidence ledger behavior, WORM semantics, bucket lifecycle, or audio evidence storage.

## Post-apply review blocker fix

Fixed the confirmed tenant-integrity blocker found after apply review:

- Added tenant-preserving composite unique keys on tenant-scoped parents used by child references: `debtors(id, tenant_id)`, `interaction_events(id, tenant_id)`, and `policy_bundles(id, tenant_id)`.
- Replaced child single-column parent references with composite tenant-preserving foreign keys for:
  - `interaction_events(debtor_id, tenant_id) -> debtors(id, tenant_id)`;
  - `detector_result_rows(interaction_event_id, tenant_id) -> interaction_events(id, tenant_id)`;
  - `policy_bundle_rules(policy_bundle_id, tenant_id) -> policy_bundles(id, tenant_id)`.
- Added `TestMigrationPreservesTenantScopedParentChildIntegrity`, a Docker-independent static migration contract test that preserves this schema guarantee when the Docker daemon is unavailable.
- Regenerated sqlc with `make sqlc`; no generated Go API changes were produced by the constraint-only schema update.
- Updated `tasks.md` with the completed composite tenant-integrity task; Docker-live validation tasks were completed in the later validation continuation.

Additional commands run for this blocker fix:

| Command | Exit | Evidence |
|---|---:|---|
| `gofmt -w internal/db/migration_test.go && go test ./internal/db -run TestMigrationPreservesTenantScopedParentChildIntegrity` | 0 | Static tenant-integrity migration test passed. |
| `make sqlc` | 0 | sqlc generation completed after migration constraint changes. |
| `go test ./...` | 0 | Full Go suite passed: `internal/config`, `internal/core`, and `internal/db`. |
| `docker info` | 1 | Historical environment check showed Docker daemon unavailable at that time; superseded by successful live Docker/PostgreSQL validation. |
| `make test` | 0 | Makefile test target passed through `go test ./...`. |
| `go test ./internal/db -run 'Test.*Migration|Test.*RLS' -v` | 0 | Static migration tenant-integrity test passed; live RLS catalog test skipped because `DATABASE_URL` is unset. |
| `make tools` | 0 | Repo-local goose/sqlc tools already installed; make reported nothing to do. |
| `make dev` | 0 | Docker Compose started PostgreSQL and MinIO successfully. |
| `docker compose ps` | 0 | PostgreSQL and MinIO reported `Up` and `healthy`. |
| `make migrate-up` | 0 | Goose applied `00001_initial_foundation.sql` to version 1; rerun reported no migrations to run at current version 1. |
| Live RLS catalog check via `docker compose exec -T postgres psql ...` | 0 | All final tenant-scoped tables returned `relrowsecurity = t`: `debtors`, `detector_result_rows`, `interaction_events`, `policy_bundle_rules`, `policy_bundles`, `tenant_api_keys`. |
| `DATABASE_URL='postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable' go test ./internal/db -run TestTenantScopedTablesHaveTenantIDAndRLSEnabled` | 0 | Live PostgreSQL catalog test passed for tenant IDs and RLS metadata. |
| `make sqlc` | 0 | sqlc generation completed with no hand edits. |
| `make test` | 0 | Full Go suite passed: `internal/config`, `internal/core`, and `internal/db`. |
| `DATABASE_URL='postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable' go test ./internal/db -run 'Test.*Migration|Test.*RLS' -v` after first `make down` | 1 | Expected environment/order failure after services had already been stopped; corrected by restarting local services and rerunning with PostgreSQL up. |
| `make dev && docker compose ps && make migrate-up && DATABASE_URL='postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable' go test ./internal/db -run 'Test.*Migration|Test.*RLS' -v` | 0 | Corrective focused validation passed with static tenant-integrity test and live RLS catalog test. |
| Final `docker compose ps` | 0 | PostgreSQL and MinIO reported `Up` and `healthy` before final shutdown. |
| Final live RLS catalog check via `docker compose exec -T postgres psql ...` | 0 | All six tenant-scoped tables still returned `relrowsecurity = t`. |
| Final `make down` | 0 | PostgreSQL and MinIO containers and project network stopped/removed successfully. |

## Next recommended

Ready for fresh review of the #13 diff. Do not widen scope into #14 auth runtime, River proof, Harness behavior, MCP, Bedrock behavior, WORM/evidence behavior, or PR publication without explicit user direction.
