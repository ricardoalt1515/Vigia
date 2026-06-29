# Proposal: Issue #14 Tenant Auth to RLS Tenant Context

## Problem / motivation

Issue #13 creates the tenant-aware PostgreSQL foundation: `tenant_api_keys`, tenant-scoped tables, RLS policies based on `app.tenant_id`, and sqlc/pgx access. That foundation still does not prove runtime tenant isolation. Without request-time tenant authentication, RLS has no authenticated tenant context and a future API or MCP surface could accidentally read across tenants or rely only on application-level `tenant_id` filters.

Issue #14 should make the tenant boundary real at the request layer: a presented API key resolves to exactly one tenant, the request runs tenant-scoped database work inside a transaction, and `SET LOCAL app.tenant_id` is applied before protected queries execute.

## Intent

Implement the thinnest runtime authentication slice that proves Vigía can safely serve tenant-scoped API reads before the #1 walking skeleton and before #17 Remote MCP.

The recommended first protected endpoint is:

```text
GET /v1/interactions
Authorization: Bearer <tenant_api_key>
```

This endpoint should return a minimal list of the authenticated tenant's seeded `interaction_events` and nothing else. It is the best first endpoint because docs already define the #1 frontend path as `Interactions list → Interaction detail`, the M0 demo calls for seeded interactions for two tenants, and `interaction_events` is already a tenant-scoped RLS table. It proves API-key authentication, transaction-scoped tenant context, and RLS-enforced read isolation without introducing interaction detail, filters, dashboards, evidence, detector results, HITL, Harness, or MCP behavior.

## Goals

- Resolve `Authorization: Bearer <key>` to one active tenant API key record.
- Reject missing, malformed, invalid, expired, or revoked credentials with `401 Unauthorized`.
- Hash presented keys before lookup; never store, return, log, or compare plaintext keys after issuance.
- Apply `SET LOCAL app.tenant_id = <tenant_id>` inside the same request transaction that runs protected queries.
- Serve the first protected endpoint, `GET /v1/interactions`, from RLS-scoped database reads.
- Prove cross-tenant access is impossible even when the protected query omits an explicit `tenant_id` predicate.
- Provide a minimal local key-issuance path for seeds/dev setup that returns plaintext once and persists only the hash.

## Non-goals

- Do not implement the #1 walking skeleton UI, interaction detail page, River worker proof, charts, filters, dashboards, or frontend data loading.
- Do not implement #17 Remote MCP, MCP tools, Synthetic Case Brief index access, or audit events.
- Do not implement #16 Harness behavior, Domain Agents, Case Brief generation, or synthetic Harness runtime integration.
- Do not add OIDC, SSO, user accounts, sessions, browser login, refresh tokens, organizations, or role-based authorization.
- Do not implement evidence ledger, detector results, Judge behavior, HITL workflow, monthly reporting, or WORM behavior.
- Do not use plaintext API keys as database identifiers or persist them in fixtures, logs, errors, snapshots, or test output.

## Scope boundaries

### In scope for #14

- Minimal Go API entrypoint or protected-route wiring if one does not yet exist.
- `internal/auth` behavior for bearer-token parsing, secure key hashing, tenant API-key lookup, and active/revoked validation.
- A request transaction helper that sets `app.tenant_id` with `SET LOCAL` before tenant-scoped queries.
- A minimal `GET /v1/interactions` handler returning only fields needed to prove tenant-scoped reads from `interaction_events`.
- A sqlc query or database adapter path for the protected endpoint that intentionally relies on RLS tenant context rather than an explicit tenant parameter.
- Local seed/CLI support for issuing tenant API keys for demo/test tenants.
- Behavior-focused tests for valid, invalid, revoked, and cross-tenant request behavior.

### Out of scope for #14

- General API framework decisions beyond the minimal server/middleware needed for the protected endpoint.
- Broader application authorization beyond tenant API-key authentication.
- Frontend implementation and interaction detail behavior owned by #1.
- Remote MCP tenant-scoped synthetic artifact access owned by #17 after #14.

## Affected areas

- `cmd/api` or equivalent API entrypoint for minimal HTTP server/protected route wiring.
- `internal/auth` for API-key parsing, hashing, lookup, and middleware/use-case boundary.
- `internal/db` or a small persistence adapter for transaction-scoped tenant context and RLS-backed reads.
- `db/queries` for current-tenant interaction reads if sqlc needs a query without an explicit `tenant_id` predicate.
- `cmd/seed` or local seed support for generating tenant API keys and two-tenant demo data.
- Integration tests around PostgreSQL RLS, runtime database role behavior, and request-level authentication.
- Documentation or `.env.example` only if needed to explain the local API/seed path.

## Acceptance criteria aligned to issue #14

- A request with a valid tenant API key resolves exactly one active tenant.
- Requests with missing, malformed, invalid, expired, or revoked API keys return `401 Unauthorized` and do not run protected tenant queries.
- API keys are stored and looked up by hash; plaintext keys are only shown once at issuance and are never stored or logged.
- Protected request database work runs in a transaction that sets `app.tenant_id` with `SET LOCAL` before querying tenant-scoped tables.
- `GET /v1/interactions` returns only rows for the authenticated tenant when seeded data exists for at least two tenants.
- A cross-tenant isolation test proves tenant A cannot read tenant B rows even when the SQL used for the protected read omits an explicit tenant filter.
- The implementation does not depend on the frontend, Harness, MCP, evidence ledger, detector results, or River worker behavior.

## Architecture / ADR alignment

- **Multi-tenancy:** Follow ADR-04: shared schema, `tenant_id` on tenant-scoped tables, PostgreSQL RLS as the engine-level isolation boundary, and app-layer guardrails as defense in depth.
- **Tenant auth:** Follow ADR-12: per-tenant API keys in `tenant_api_keys`, request header `Authorization: Bearer <key>`, middleware/use-case resolution to tenant context, then RLS session variable before queries.
- **SQL-first persistence:** Continue using PostgreSQL + sqlc + pgx. Do not introduce an ORM or bypass RLS through ad hoc database access.
- **Clean boundaries:** Keep auth/key-resolution behavior separate from HTTP formatting and database implementation details. Domain/core types should not depend on HTTP, pgx, or generated sqlc code.
- **Frontend boundary:** Go owns tenant auth and RLS enforcement; Next.js later consumes the API but does not own tenant isolation.
- **Future MCP boundary:** #17 can reuse the tenant boundary established here, but #14 does not expose MCP tools or synthetic artifact reads.

## Recommended first protected endpoint

Use `GET /v1/interactions` as the first protected endpoint.

| Candidate | Decision | Reason |
|---|---|---|
| `GET /v1/interactions` | Recommended | Aligns with the documented #1 first frontend path, uses an existing tenant-scoped table, supports the M0 two-tenant seeded-data demo, and proves RLS through a simple read. |
| `GET /v1/tenant` | Rejected for first proof | Proves key resolution but not tenant-scoped RLS over product data. |
| `GET /v1/debtors` | Rejected for first proof | Thin, but less aligned with the documented walking-skeleton path and less directly useful for the next user-visible slice. |
| Interaction detail or evidence endpoint | Rejected for first proof | Pulls in joins, detector/evidence semantics, and UI expectations that belong to later issues. |

The endpoint should remain intentionally small: no filters, no pagination beyond a safe default limit if needed, no joins unless already trivial, no evidence, no detector result expansion, and no interaction detail response.

## Risks and mitigations

| Risk | Severity | Mitigation |
|---|---:|---|
| `SET LOCAL` is applied outside the request transaction or leaks across pooled connections | High | Require a transaction wrapper that sets `app.tenant_id` inside the same transaction as protected queries and always commits/rolls back before returning the connection. |
| Tests pass because explicit `tenant_id` filters hide an RLS failure | High | Include at least one protected read/isolation test whose SQL intentionally omits an explicit tenant predicate and relies on RLS. |
| Runtime database role bypasses RLS as owner/superuser | High | Validate using the intended application role/connection; do not run isolation tests as migration owner or superuser. |
| Plaintext API keys leak through logs, fixtures, snapshots, or errors | High | Hash before lookup, redact credential values in errors/logs, and keep plaintext available only at issuance. |
| Revocation semantics drift from the #13 schema | Medium | Treat non-`active` status as revoked/inactive; if `expires_at` exists, expired keys are unauthorized. Avoid schema expansion unless #13 final shape requires it. |
| Endpoint scope creeps into #1 walking skeleton | Medium | Limit #14 to API auth/RLS proof and a minimal list response; leave frontend, detail views, filters, and River proof to #1. |
| Auth middleware becomes coupled to one handler | Medium | Keep the protected-route path thin but reusable enough that #17 and #1 can share the same tenant context boundary later. |

## Rollback

Rollback is straightforward before production rollout:

- remove or disable the protected API route and auth middleware wiring;
- revert `internal/auth`, transaction helper, seed/key-issuance, and query additions from the #14 slice;
- delete local/demo API keys and seeded interaction rows if they were created during validation;
- keep #13 schema/RLS foundations intact unless the rollback explicitly targets the dependency branch;
- no production data migration rollback is expected for this planning slice.

## Success criteria

- A developer can generate or obtain a local tenant API key, call `GET /v1/interactions`, and see only that tenant's seeded interactions.
- Invalid or revoked credentials reliably fail with `401 Unauthorized` before tenant-scoped query execution.
- The RLS session variable is set only within the protected request transaction.
- Cross-tenant reads fail through PostgreSQL RLS even when application SQL omits a tenant filter.
- Reviewers can verify #14 without approving frontend, Harness, MCP, evidence, detector, or River behavior.
- #1 can build the interaction-list walking skeleton on top of the tenant-authenticated API without redesigning the auth boundary.
- #17 can later reuse the authenticated tenant context for tenant-scoped Remote MCP without bypassing this boundary.

## Proposal question round

No interactive question round was run because the user explicitly requested using existing docs/issues and not asking unless blocked. The proposal assumes:

- API keys are presented with `Authorization: Bearer <key>`.
- Revoked means `tenant_api_keys.status != 'active'`; expired means `expires_at <= now()` when the column is present.
- The first protected endpoint should be `GET /v1/interactions`, not a tenant metadata endpoint, because it proves RLS on product data and directly prepares #1.
- The #14 endpoint remains an auth/RLS proof, not the full walking skeleton.

## Next recommended phase

Spec.
