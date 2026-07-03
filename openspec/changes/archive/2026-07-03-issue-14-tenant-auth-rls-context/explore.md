# Exploration: Issue #14 Tenant Auth -> RLS Tenant Context

## Status

Complete.

## Source

GitHub issue #14: Tenant auth: API key -> RLS tenant context.

## Executive summary

This change builds runtime tenant authentication on top of the issue #13 foundation. Issue #13 provides the `tenant_api_keys` table, hashed key storage shape, RLS policies gated on the `app.tenant_id` session variable, core Go types, and pgx/sqlc generation. Issue #14 should introduce request-time tenant authentication and tenant-scoped database execution.

Expected implementation scope:

- Add a minimal HTTP server entry point if none exists yet.
- Add API-key authentication middleware that resolves `Authorization: Bearer <key>` to a tenant.
- Hash presented keys before lookup; never store or log plaintext keys.
- Ensure `SET LOCAL app.tenant_id` is applied inside the request transaction before tenant-scoped queries run.
- Add a seed/CLI path for issuing a tenant API key.
- Add behavior-focused integration tests proving invalid/revoked keys fail and cross-tenant access is impossible even when queries omit explicit tenant filters.

## Dependencies

- Issue #13 must be landed or otherwise stable before implementing #14.
- Current repo state contains in-progress #13 changes from another agent; #14 planning can proceed, but implementation should wait until the foundation is verified and no conflicting worktree changes remain.

## Likely seams for strict TDD

- Key hashing and lookup behavior in an `internal/auth` package.
- Middleware behavior for missing, invalid, revoked, and valid bearer tokens.
- Transaction-scoped tenant context helper that sets `app.tenant_id` with `SET LOCAL`.
- RLS integration test proving tenant isolation through the actual database role and policies.
- CLI/key issuance path proving plaintext key is returned once and only the hash is persisted.

## Risks

| Risk | Severity | Note |
| --- | --- | --- |
| #13 must land before #14 implementation can start | High | Schema, sqlc, and RLS foundations are prerequisites. |
| No HTTP server exists | Medium | #14 may need a minimal `cmd/api` bootstrap. Keep it thin. |
| `SET LOCAL` var leakage or ineffective scoping | High | Must be inside the request transaction; avoid setting session state on pooled connections outside transaction scope. |
| RLS bypass if app connects as owner/superuser | Medium | Tests must use the intended runtime role, not a migration/owner role. |
| Router choice affects future middleware patterns | Low | Prefer standard library unless existing repo conventions require another router. |

## Next recommended phase

`propose`

Before proposal, confirm the product/API decisions that shape the PRD: API-key presentation, key issuance UX, revocation semantics, first protected endpoint, and tenant-isolation proof boundary.
