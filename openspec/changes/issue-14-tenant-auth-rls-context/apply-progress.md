# Apply Progress: Issue #14 Tenant Auth -> RLS Tenant Context

## Status consumed

- Change: `issue-14-tenant-auth-rls-context`
- Artifact store: `openspec`
- Action context: repo-local, workspace root `/Users/ricardoaltamirano/Developer/vigia`
- Delivery path: chained PRs approved by user, chain strategy `stacked-to-main`
- Assigned slice: PR 1 only — tenant auth + transaction-scoped context
- Strict TDD: active, runner `go test ./...`

## Workload / PR boundary

PR 1 implemented only:

- `internal/auth` API-key parsing, hashing, and active tenant resolution.
- `internal/tenantdb` transaction helper that sets `app.tenant_id` with `SET LOCAL` semantics through `set_config(..., true)` inside the transaction.

Not implemented in this slice:

- PR 2 endpoint/RLS proof (`GET /v1/interactions`, HTTP wiring, RLS integration proof).
- PR 3 local key issuance / seed support.
- Frontend, Harness, MCP, detectors, evidence ledger, dashboards, or River behavior.

## Completed tasks and persisted checkbox updates

The following PR 1 task checkboxes are marked complete in `tasks.md`:

- RED: table-driven `internal/auth` tests for missing, malformed, invalid, expired, revoked, and valid bearer credentials, including hash-only lookup seam.
- GREEN: `internal/auth` bearer parsing, SHA-256 presented-key hashing, and active-key tenant resolution through a store port.
- REFACTOR: auth remains separate from HTTP and persistence details; plaintext key material is not used for store lookup.
- RED: `internal/tenantdb` tests proving tenant context is set before protected work and transactions roll back on failure.
- GREEN: smallest reusable transaction helper in `internal/tenantdb`.
- REFACTOR: helper accepts a pgx-compatible `Begin(ctx)` boundary and avoids a generic repository layer.

## TDD Cycle Evidence

| Cycle | RED evidence | GREEN evidence | TRIANGULATE / REFACTOR evidence |
|---|---|---|---|
| Auth | `go test ./internal/auth -run TestTenantAPIKeyAuth -count=1` failed with undefined `HashAPIKey`, `TenantAPIKey`, `StatusActive`, and auth errors. | Added `internal/auth/auth.go`; focused auth test passed. | Re-ran focused auth package and broader package tests; auth has no HTTP or generated DB imports. |
| Tenant transaction context | `go test ./internal/tenantdb -run TestSetLocalTenantContext -count=1` failed with undefined `WithTenantTx`, `Tx`, and `setLocalTenantSQL`. | Added `internal/tenantdb/tenantdb.go`; focused tenantdb test passed. | Refactored beginner interface to `Begin(ctx)` so pgx pool/tx-style boundaries can adapt without repository indirection; re-ran focused and full suite. |

## Files changed

- `internal/auth/auth.go`
- `internal/auth/auth_test.go`
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
| `go test ./...` | Passed | Full Go suite passes for current packages. |

## Deviations from design

- Used `SELECT set_config('app.tenant_id', $1, true)` rather than literal `SET LOCAL app.tenant_id = $1` because PostgreSQL parameters are safely supported in function calls and the third argument `true` gives transaction-local behavior equivalent to `SET LOCAL`.
- Did not add a concrete sqlc-backed auth store in PR 1 because the assigned slice called for the auth store port and transaction context helper only; DB query/adapters can be wired with PR 2 endpoint work.

## Remaining tasks

Exact unchecked task lines remaining:

```text
- [ ] Wait until issue #13 is stable/verified and the other agent's changes are settled.
- [ ] Reconfirm the final #13 shape before coding: `tenant_api_keys` hash/status/expiry columns, `interaction_events` columns, runtime DB role, and seed entrypoint.
- [ ] Stop immediately if scope starts widening into frontend, #1 console walking skeleton, River, Harness, MCP, detectors, evidence ledger, or dashboards.
- [ ] Do not start apply if any #13 schema or sqlc output is still moving.
- [ ] RED: add `GET /v1/interactions` handler tests in `internal/httpapi` for `401` on unauthorized credentials and `200` for a valid tenant key with seeded rows.
- [ ] GREEN: wire the minimal protected route in `cmd/api` and `internal/httpapi` so the endpoint returns only the authenticated tenant's minimal list shape.
- [ ] REFACTOR: keep the route thin; no pagination, filters, detail view, dashboard logic, or frontend coupling.
- [ ] RED: add an integration test proving tenant A cannot read tenant B rows when the protected query omits an explicit `tenant_id` predicate.
- [ ] GREEN: implement the RLS-backed read path in `db/queries` and the adapter layer so protected reads rely on transaction context, not app-layer tenant filters.
- [ ] REFACTOR: validate the proof using the intended application runtime role, not migration-owner or superuser behavior.
- [ ] RED: add tests in `cmd/seed` (or the smallest existing seed entrypoint) proving plaintext API keys are returned once and only the hash persists.
- [ ] GREEN: implement high-entropy tenant API-key generation, hash persistence, and one-time plaintext output for local/demo tenant setup.
- [ ] REFACTOR: keep the issuer boundary reusable and ensure plaintext never lands in logs, fixtures, errors, or snapshots.
- [ ] Run focused package tests for auth, transaction helper, HTTP, seed, and the RLS proof seam.
- [ ] Run the broader suite only after #13 is stable and the worktree is clean.
```

## Risks / notes

- `internal/auth` currently defines the store port and does not yet include the sqlc/Postgres adapter. That keeps PR 1 small but means PR 2 must add or wire the actual query by `key_hash` before HTTP endpoint work can authenticate against PostgreSQL.
- There are pre-existing unrelated modified/untracked files in the worktree. This slice only modified PR 1 code and issue #14 OpenSpec apply artifacts.
