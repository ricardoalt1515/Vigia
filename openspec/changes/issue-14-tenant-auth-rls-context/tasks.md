# Issue #14 Implementation Tasks

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 500-750 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

## Blockers / stop conditions

- [x] Wait until issue #13 is stable/verified and the other agent's changes are settled.
- [x] Reconfirm the final #13 shape before coding: `tenant_api_keys` hash/status/expiry columns, `interaction_events` columns, runtime DB role, and seed entrypoint.
- [x] Stop immediately if scope starts widening into frontend, #1 console walking skeleton, River, Harness, MCP, detectors, evidence ledger, or dashboards.
- [x] Do not start apply if any #13 schema or sqlc output is still moving.

## PR 1 — tenant auth + transaction-scoped context

- [x] RED: add table-driven tests in `internal/auth` for missing, malformed, invalid, expired, revoked, and valid `Authorization: Bearer <key>` inputs, including the hash-only lookup seam.
  - Verification: `go test ./internal/auth -run TestTenantAPIKeyAuth -count=1`
- [x] GREEN: implement `internal/auth` bearer parsing, presented-key hashing, and active-key tenant resolution against the final #13 store port.
  - Verification: `go test ./internal/auth -run TestTenantAPIKeyAuth -count=1`
- [x] REFACTOR: keep auth separate from HTTP formatting and persistence details; no plaintext key material in logs, errors, fixtures, or snapshots.
  - Verification: `go test ./internal/auth ./... -run TestTenantAPIKeyAuth -count=1`
- [x] RED: add transaction-scoped tenant context tests in `internal/tenantdb` proving `SET LOCAL app.tenant_id` happens inside `BeginTx` and clears on commit/rollback.
  - Verification: `go test ./internal/tenantdb -run TestSetLocalTenantContext -count=1`
- [x] GREEN: implement the smallest transaction helper in `internal/tenantdb` (or the final #13 adapter seam) and keep it independent from HTTP.
  - Verification: `go test ./internal/tenantdb -run TestSetLocalTenantContext -count=1`
- [x] REFACTOR: make the transaction helper reusable for later tenant-scoped reads without introducing a generic repository layer.
  - Verification: `go test ./internal/tenantdb ./... -run TestSetLocalTenantContext -count=1`

## PR 2 — minimal protected endpoint + RLS proof

- [x] RED: add `GET /v1/interactions` handler tests in `internal/httpapi` for `401` on unauthorized credentials and `200` for a valid tenant key with seeded rows.
  - Verification: `go test ./internal/httpapi -run TestGetInteractions -count=1`
- [x] GREEN: wire the minimal protected route in `cmd/api` and `internal/httpapi` so the endpoint returns only the authenticated tenant's minimal list shape.
  - Verification: `go test ./internal/httpapi ./cmd/api -run TestGetInteractions -count=1`
- [x] REFACTOR: keep the route thin; no pagination, filters, detail view, dashboard logic, or frontend coupling.
  - Verification: `go test ./internal/httpapi ./cmd/api -run TestGetInteractions -count=1`
- [x] RED: add an integration test proving tenant A cannot read tenant B rows when the protected query omits an explicit `tenant_id` predicate.
  - Verification: `go test ./... -run TestRLSIsolation -count=1`
- [x] GREEN: implement the RLS-backed read path in `db/queries` and the adapter layer so protected reads rely on transaction context, not app-layer tenant filters.
  - Verification: `go test ./... -run TestRLSIsolation -count=1`
- [x] REFACTOR: validate the proof using the intended application runtime role, not migration-owner or superuser behavior.
  - Verification: `go test ./... -run TestRLSIsolation -count=1`

## PR 3 — local key issuance / seed support

- [x] RED: add tests in `cmd/seed` (or the smallest existing seed entrypoint) proving plaintext API keys are returned once and only the hash persists.
  - Verification: `go test ./cmd/seed -run TestIssueTenantAPIKey -count=1`
- [x] GREEN: implement high-entropy tenant API-key generation, hash persistence, and one-time plaintext output for local/demo tenant setup.
  - Verification: `go test ./cmd/seed ./... -run TestIssueTenantAPIKey -count=1`
- [x] REFACTOR: keep the issuer boundary reusable and ensure plaintext never lands in logs, fixtures, errors, or snapshots.
  - Verification: `go test ./cmd/seed ./... -run TestIssueTenantAPIKey -count=1`

## Final validation

- [x] Run focused package tests for auth, transaction helper, HTTP, seed, and the RLS proof seam.
  - Verification: `go test ./internal/auth ./internal/tenantdb ./internal/httpapi ./cmd/seed -count=1`
- [x] Run the broader suite only after #13 is stable and the worktree is clean.
  - Verification: `go test ./...`
