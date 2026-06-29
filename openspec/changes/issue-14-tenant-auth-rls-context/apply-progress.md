# Apply Progress: Issue #14 Tenant Auth -> RLS Tenant Context

## Status consumed

- Change: `issue-14-tenant-auth-rls-context`
- Artifact store: `openspec`
- Action context: repo-local, workspace root `/Users/ricardoaltamirano/Developer/vigia`
- Delivery path: chained PRs approved by user, chain strategy `stacked-to-main`
- Strict TDD: active, runner `go test ./...`

## Workload / PR boundary

Implemented slices:

- PR 1: `internal/auth` API-key parsing, hashing, and active tenant resolution.
- PR 1: `internal/tenantdb` transaction helper that sets `app.tenant_id`
  with `SET LOCAL` semantics through `set_config(..., true)` inside the transaction.
- PR 2: minimal protected `GET /v1/interactions` endpoint and RLS-backed
  current-tenant read path.

Not implemented in these slices:

- PR 3 local key issuance / seed support.
- Frontend, Harness, MCP, detectors, evidence ledger, dashboards, or River behavior.

## Completed tasks and persisted checkbox updates

The following PR 1 task checkboxes are marked complete in `tasks.md`:

- RED: table-driven `internal/auth` tests for missing, malformed, invalid,
  expired, revoked, and valid bearer credentials, including hash-only lookup seam.
- GREEN: `internal/auth` bearer parsing, SHA-256 presented-key hashing, and
  active-key tenant resolution through a store port.
- REFACTOR: auth remains separate from HTTP and persistence details; plaintext
  key material is not used for store lookup.
- RED: `internal/tenantdb` tests proving tenant context is set before protected
  work and transactions roll back on failure.
- GREEN: smallest reusable transaction helper in `internal/tenantdb`.
- REFACTOR: helper accepts a pgx-compatible `Begin(ctx)` boundary and avoids a
  generic repository layer.

The following PR 2 task checkboxes are marked complete in `tasks.md`:

- RED: `GET /v1/interactions` handler tests in `internal/httpapi` for `401`
  on unauthorized credentials and `200` for a valid tenant key with seeded rows.
- GREEN: minimal protected route in `cmd/api` and `internal/httpapi` returns the
  authenticated tenant's minimal list shape.
- REFACTOR: route remains thin; no pagination, filters, detail views,
  dashboard logic, or frontend coupling.
- RED: RLS integration test proves tenant A cannot read tenant B rows when the
  protected query omits an explicit `tenant_id` predicate.
- GREEN: `db/queries` current-tenant interaction read and Postgres adapter rely
  on transaction context, not app-layer tenant filters.
- REFACTOR: RLS proof is an integration test using the runtime connection when
  `DATABASE_URL` is provided; it skips safely when no database is available.

## TDD Cycle Evidence

| Cycle | RED evidence | GREEN evidence | TRIANGULATE / REFACTOR evidence |
|---|---|---|---|
| Auth | `go test ./internal/auth -run TestTenantAPIKeyAuth -count=1` failed with undefined `HashAPIKey`, `TenantAPIKey`, `StatusActive`, and auth errors. | Added `internal/auth/auth.go`; focused auth test passed. | Re-ran focused auth package and broader package tests; auth has no HTTP or generated DB imports. |
| Tenant transaction context | `go test ./internal/tenantdb -run TestSetLocalTenantContext -count=1` failed with undefined `WithTenantTx`, `Tx`, and `setLocalTenantSQL`. | Added `internal/tenantdb/tenantdb.go`; focused tenantdb test passed. | Refactored beginner interface to `Begin(ctx)` so pgx pool/tx-style boundaries can adapt without repository indirection; re-ran focused and full suite. |
| HTTP interactions endpoint | `go test ./internal/httpapi -run TestGetInteractions -count=1` failed with undefined `Interaction`, `NewServer`, and response types. | Added `internal/httpapi` server and tests; focused endpoint test passed. | Added `cmd/api` wiring with stdlib `net/http`; route remains a single minimal endpoint. |
| RLS current-tenant read | `go test ./... -run TestRLSIsolation -count=1` failed with undefined `ListCurrentTenantInteractions` after the integration test was added. | Added no-explicit-tenant sqlc query and regenerated `internal/db`; RLS test compiles and skips safely without database URLs. | Added Postgres adapters for key-hash lookup and transaction-scoped interaction reads; auth lookup now uses a transaction-local `app.api_key_hash` setting instead of depending on tenant RLS before authentication. |

## Files changed

- `cmd/api/main.go`
- `db/queries/interaction_events.sql`
- `db/queries/tenant_api_keys.sql`
- `internal/db/interaction_events.sql.go`
- `internal/db/querier.go`
- `internal/db/rls_isolation_test.go`
- `internal/db/tenant_api_keys.sql.go`
- `internal/httpapi/httpapi.go`
- `internal/httpapi/httpapi_test.go`
- `internal/postgres/adapters.go`
- `internal/tenantdb/tenantdb.go`
- `internal/tenantdb/tenantdb_test.go`
- `openspec/changes/issue-14-tenant-auth-rls-context/tasks.md`
- `openspec/changes/issue-14-tenant-auth-rls-context/apply-progress.md`

## Verification commands run

| Command | Result | Summary |
|---|---|---|
| `go test ./internal/auth -run TestTenantAPIKeyAuth -count=1` | Failed as RED, then passed | Auth test failed before production code, then passed after implementation. |
| `go test ./internal/tenantdb -run TestSetLocalTenantContext -count=1` | Failed as RED, then passed | Tenant transaction test failed before production code, then passed after implementation. |
| `go test ./internal/auth ./internal/tenantdb -count=1` | Passed | Focused PR 1 packages pass together. |
| `go test ./internal/httpapi -run TestGetInteractions -count=1` | Failed as RED, then passed | Endpoint behavior test failed before server code, then passed. |
| `go test ./internal/httpapi ./cmd/api -run TestGetInteractions -count=1` | Passed | Endpoint package passed; `cmd/api` compiles with no package tests. |
| `go test ./... -run TestRLSIsolation -count=1` | Failed as RED, then passed/skipped | Failed before current-tenant sqlc query existed; after implementation the integration test compiles and skips unless both `DATABASE_URL` and `APP_DATABASE_URL` are present. |
| `go test ./...` | Passed | Full Go suite passes for current packages. |
| `git diff --check` | Passed | No whitespace errors in the current diff. |

## Deviations from design

- Used `SELECT set_config('app.tenant_id', $1, true)` rather than literal
  `SET LOCAL app.tenant_id = $1` because PostgreSQL parameters are safely
  supported in function calls and the third argument `true` gives
  transaction-local behavior equivalent to `SET LOCAL`.
- The RLS integration proof is skippable without both `DATABASE_URL` and
  `APP_DATABASE_URL`; this keeps local verification safe when Postgres or the
  intended app-role connection is unavailable while still providing a live
  proof seam for reviewers/CI.
- Added `internal/postgres` as a small adapter package so `internal/httpapi`
  remains independent from generated sqlc and pgx details.

## Remaining tasks

Exact unchecked task lines remaining:

```text
- [ ] Stop immediately if scope starts widening into frontend, #1 console walking skeleton, River, Harness, MCP, detectors, evidence ledger, or dashboards.
- [ ] Do not start apply if any #13 schema or sqlc output is still moving.
- [ ] RED: add tests in `cmd/seed` (or the smallest existing seed entrypoint) proving plaintext API keys are returned once and only the hash persists.
- [ ] GREEN: implement high-entropy tenant API-key generation, hash persistence, and one-time plaintext output for local/demo tenant setup.
- [ ] REFACTOR: keep the issuer boundary reusable and ensure plaintext never lands in logs, fixtures, errors, or snapshots.
- [ ] Run focused package tests for auth, transaction helper, HTTP, seed, and the RLS proof seam.
- [ ] Run the broader suite only after #13 is stable and the worktree is clean.
```

## Risks / notes

- PR 3 key issuance/seed support remains incomplete by design.
- The live RLS proof requires `DATABASE_URL` for setup and `APP_DATABASE_URL`
  for runtime-role validation; without both, the integration test is skipped
  rather than mutating unavailable services.
- There are pre-existing unrelated modified/untracked files in the worktree.
  This slice only modified PR 2 code and issue #14 OpenSpec apply artifacts.
