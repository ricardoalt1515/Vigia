# Design: Issue #14 Tenant Auth -> RLS Tenant Context

## Status

Planned. Implementation must wait until issue #13 is stable because #14 depends on its schema, RLS policies, sqlc output, and local database workflow.

## Objective

Establish the first runtime tenant boundary for Vigía: a tenant API key authenticates a request, the request runs tenant-scoped database work in a transaction, and PostgreSQL RLS receives `app.tenant_id` through `SET LOCAL` before protected product-data queries execute.

The first protected endpoint is intentionally thin:

```text
GET /v1/interactions
Authorization: Bearer <tenant_api_key>
```

It exists to prove API-key authentication and RLS tenant isolation over product data. It does not implement the full #1 console walking skeleton.

## Non-goals

- No frontend, interaction detail, filters, dashboards, charts, River worker, detector, evidence-ledger, Harness, or MCP behavior.
- No user accounts, browser sessions, OAuth/OIDC, RBAC, refresh tokens, or organization administration.
- No ORM or generic repository framework.
- No plaintext API-key persistence in DB rows, fixtures, logs, errors, or snapshots.
- No app-layer tenant filtering as the only isolation proof.

## Architecture

Use a small clean/hexagonal boundary, not a framework-heavy web stack.

```text
cmd/api
  wires config, db pool, HTTP mux, auth middleware, handlers

internal/auth
  parses Bearer credentials
  hashes presented keys
  validates active key records through a store port
  exposes authenticated tenant context

internal/httpapi
  HTTP handlers and response/error formatting
  depends on application/service interfaces, not generated DB details

internal/tenantdb
  transaction helper:
    BeginTx
    SET LOCAL app.tenant_id = $tenantID
    run protected query using transaction-bound sqlc Querier
    commit/rollback

internal/db + db/queries
  sqlc generated code and SQL queries
```

Dependencies point inward: HTTP and DB adapters depend on small auth/application contracts; core auth behavior does not import `net/http`, `pgx`, or generated sqlc types unless the final #13 shape makes a narrower adapter impractical.

## Request flow

1. `cmd/api` receives `GET /v1/interactions`.
2. Auth middleware reads `Authorization`.
3. Missing, malformed, invalid, expired, or revoked credentials return `401 Unauthorized` before protected tenant queries run.
4. The presented key is hashed immediately with the project-approved hash scheme from #13.
5. The key hash is looked up in `tenant_api_keys` through a store/adapter.
6. A valid active key resolves exactly one tenant ID.
7. The handler calls a transaction-scoped tenant database helper.
8. The helper begins a transaction, executes `SET LOCAL app.tenant_id = <tenant_id>` inside that transaction, then runs the protected interactions query through that same transaction.
9. The transaction commits on success or rolls back on error.
10. The response returns only minimal interaction list fields for the authenticated tenant.

## Transaction and RLS boundary

The core safety invariant is:

> Tenant-scoped product queries must execute in the same transaction where `SET LOCAL app.tenant_id` was set.

Implementation guidance:

- Use `SET LOCAL app.tenant_id = $1` or the safest pgx-compatible equivalent inside the transaction.
- Do not call plain `SET app.tenant_id` on pooled connections.
- Do not set tenant context globally on the pool or outside the request transaction.
- Ensure rollback happens on any error after `Begin`.
- Ensure tests validate that tenant context ends with the transaction.
- Use the intended runtime DB role/connection for isolation tests; do not validate RLS as migration owner or superuser.

The protected interactions query should intentionally omit an explicit `tenant_id = $1` predicate for the RLS proof path. App-layer tenant filters may be added later as defense in depth, but #14 must prove PostgreSQL RLS is doing the isolation work.

## API key handling

Expected behavior:

- API keys are presented as `Authorization: Bearer <key>`.
- The plaintext key is returned only once during issuance for local seed/developer setup.
- Only a hash is stored in `tenant_api_keys`.
- Key lookup compares hash material, not plaintext.
- Non-active status is unauthorized.
- If #13 includes expiry fields, expired keys are unauthorized.
- Errors and logs must never include the presented key.

If #13 has already chosen the hash column and algorithm, use that exact shape. If not, prefer a deterministic secret hash suitable for lookup, e.g. SHA-256 over a high-entropy generated key, encoded consistently. Do not introduce password-style slow hashing if the lookup model requires direct equality by hash and keys are already random high-entropy secrets.

## First protected endpoint

`GET /v1/interactions` should return a minimal stable JSON shape needed to prove tenant isolation. Suggested fields, subject to the #13 schema:

```json
{
  "interactions": [
    {
      "id": "...",
      "occurred_at": "...",
      "channel": "...",
      "direction": "..."
    }
  ]
}
```

Keep it boring. No pagination unless a small default limit is already idiomatic. No joins, no detector results, no evidence expansion, no dashboard aggregates.

## SQL and sqlc implications

Likely additions after #13 stabilizes:

- A tenant-api-key lookup query by key hash and active state.
- A current-tenant interactions list query that does not accept tenant ID and relies on RLS.
- Optional seed/key issuance query if not already present.

Example intent, not final SQL:

```sql
-- name: GetActiveTenantAPIKeyByHash :one
SELECT tenant_id, id, status, expires_at
FROM tenant_api_keys
WHERE key_hash = $1
  AND status = 'active'
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListCurrentTenantInteractions :many
SELECT id, occurred_at, channel, direction
FROM interaction_events
ORDER BY occurred_at DESC
LIMIT $1;
```

The second query intentionally has no `tenant_id` predicate for the #14 RLS proof.

## Seed / CLI path

Provide the smallest local key issuance path compatible with #13's Makefile/seed conventions.

Expected behavior:

- Generate a high-entropy API key.
- Hash it before persistence.
- Insert a `tenant_api_keys` row tied to a tenant.
- Print the plaintext key once to stdout for developer use.
- Avoid committing generated plaintext keys to fixtures or `.env.example`.

If #13 already defines `make seed`, extend it. Otherwise add the smallest command path that fits the repo, such as `cmd/seed` or a seed subcommand.

## Strict TDD plan

Strict TDD is active for this project. Use behavior seams that prove the boundary without restating implementation.

### RED candidates

1. `internal/auth`: table-driven test for missing, malformed, invalid, revoked/expired, and valid credentials.
2. Key issuance: test that plaintext is returned once by the issuer boundary and persisted storage receives only hash material.
3. Transaction helper: test or integration test that protected work sets tenant context inside a transaction before query execution.
4. HTTP behavior: `GET /v1/interactions` returns `401` for unauthorized credentials and `200` for a valid key.
5. RLS integration: seed tenant A and tenant B rows, authenticate as tenant A, run the no-explicit-tenant query, assert tenant B rows are absent.

### Verification commands

Run focused tests frequently:

```bash
go test ./internal/auth
 go test ./internal/httpapi
 go test ./internal/tenantdb
```

Run broader verification before completion:

```bash
go test ./...
```

If DB integration tests require Postgres, keep them skippable in `testing.Short()` and document the required local command, likely `make dev` followed by `make test` once #13 finalizes the Makefile.

## Review workload strategy

Keep #14 as one reviewable auth/RLS slice:

- Avoid introducing a third-party router unless the repo already depends on one.
- Prefer `net/http` and a small `http.ServeMux` for the first endpoint.
- Keep `cmd/api` wiring minimal.
- Keep auth, transaction helper, and handler tests with the code they prove.
- Do not add frontend or Harness files in this change.

If implementation starts exceeding the review budget, split key issuance/seed support from the runtime protected endpoint only if both slices can still be reviewed meaningfully.

## Open decisions for implementation

Resolve against the finalized #13 artifacts before coding:

- Exact `tenant_api_keys` columns and status/expiry representation.
- Exact `interaction_events` columns available for the minimal response.
- Runtime application DB role name and migration/test role separation.
- Existing seed command shape from #13.

These are not product blockers; they are dependency-shape checks after #13 lands.

## Failure modes and handling

- Unauthorized credentials: return `401 Unauthorized` with a generic message.
- Auth store/database lookup error: return `500` unless the error is a normal not-found/unauthorized path.
- Tenant transaction setup failure: rollback and return `500`.
- Protected query failure: rollback and return `500`.
- Never reveal whether a specific key hash, tenant, or key status exists.

## Success criteria

- Valid tenant API key reaches `GET /v1/interactions` and sees only that tenant's rows.
- Invalid/revoked/expired/missing credentials fail before protected tenant queries.
- Plaintext key material is not persisted or logged.
- `SET LOCAL app.tenant_id` happens inside the protected request transaction.
- RLS isolation is proven by a query that omits an explicit tenant filter.
- #1 can later build the console list on this API without redesigning tenant auth.

## Deviation note (applied at implementation, confirmed at archive)

The implementation used `SELECT set_config('app.tenant_id', $1, true)` rather
than literal `SET LOCAL app.tenant_id = $1`, because PostgreSQL parameters
are safely supported in function calls and the third `true` argument gives
transaction-local behavior equivalent to `SET LOCAL`. This preserves the
design's core safety invariant (tenant context is scoped to the request
transaction) and is documented in `apply-progress.md`.
