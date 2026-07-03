# Tenant Auth RLS Context Specification

## Purpose

Define the issue #14 runtime proof that a tenant API key can authenticate a request, establish transaction-scoped tenant context for PostgreSQL row-level security, and protect one product-data endpoint without expanding into the broader walking skeleton.

## Requirements

### Requirement: Tenant API Key Authentication

The system MUST authenticate protected API requests with `Authorization: Bearer <tenant_api_key>` and MUST resolve a valid presented key to exactly one active tenant API key record.

#### Scenario: Accept a valid tenant API key

- GIVEN a request includes `Authorization: Bearer <tenant_api_key>`
- AND the presented key corresponds to exactly one active tenant API key record
- WHEN the protected endpoint is invoked
- THEN the request MUST be associated with that key's tenant
- AND protected request processing MUST continue.

#### Scenario: Reject unauthorized credentials

- GIVEN a request to a protected endpoint has missing, malformed, invalid, expired, or revoked credentials
- WHEN the request is evaluated for authentication
- THEN the system MUST respond with `401 Unauthorized`
- AND the system MUST NOT run protected tenant-scoped queries for that request.

### Requirement: API Key Secret Handling

The system MUST store and look up tenant API keys by hash and MUST expose plaintext key material only at issuance time.

#### Scenario: Lookup uses hashed key material

- GIVEN a protected request presents a tenant API key
- WHEN the system validates that credential
- THEN the system MUST hash the presented key before lookup or comparison
- AND the system MUST NOT require plaintext key material to be stored in persistent storage.

#### Scenario: Plaintext key is not retained after issuance

- GIVEN a tenant API key is issued for local seed or developer setup
- WHEN issuance completes
- THEN the plaintext key MUST be returned at most once at issuance time
- AND subsequent storage, logs, errors, fixtures, and snapshots MUST NOT contain the plaintext key.

### Requirement: Transaction-Scoped Tenant RLS Context

The system MUST execute protected tenant data reads inside a database transaction that sets `SET LOCAL app.tenant_id` before protected queries run.

#### Scenario: Tenant context is established inside the request transaction

- GIVEN a request has been authenticated to a tenant
- WHEN the system begins protected database work for that request
- THEN it MUST open a database transaction for that protected work
- AND it MUST set `SET LOCAL app.tenant_id` to the authenticated tenant identifier inside that same transaction before protected queries execute.

#### Scenario: Tenant context does not become a pooled-session default

- GIVEN the protected request completes
- WHEN the transaction is committed or rolled back
- THEN the tenant context established with `SET LOCAL app.tenant_id` MUST end with that transaction
- AND the change MUST NOT require tenant session state to persist beyond the protected request transaction.

### Requirement: Protected Interactions List Endpoint

The system MUST provide `GET /v1/interactions` as the first protected endpoint for issue #14 and MUST keep its scope limited to proving tenant-authenticated RLS over product data.

#### Scenario: Authenticated tenant reads its own interactions

- GIVEN seeded interaction data exists for the authenticated tenant
- WHEN the tenant calls `GET /v1/interactions` with a valid tenant API key
- THEN the response MUST contain that tenant's interaction rows only
- AND the endpoint MUST remain limited to the minimal list behavior needed to prove the auth-to-RLS boundary.

#### Scenario: Walking skeleton scope is not widened

- GIVEN issue #14 is reviewed for completion
- WHEN the protected endpoint behavior is inspected
- THEN issue #14 MUST NOT require frontend implementation, interaction detail behavior, dashboards, evidence expansion, detector results, Harness behavior, MCP tools, or River worker behavior.

### Requirement: RLS Isolation Proof Over Product Data

The system MUST prove tenant isolation through PostgreSQL row-level security on tenant product data, including a protected read path whose SQL does not rely on an explicit tenant filter.

#### Scenario: RLS prevents cross-tenant reads without explicit tenant predicate

- GIVEN seeded interaction data exists for at least two tenants
- AND a protected interactions read path omits an explicit `tenant_id` predicate
- WHEN tenant A invokes `GET /v1/interactions` with a valid tenant A API key
- THEN the response MUST exclude tenant B rows
- AND tenant isolation MUST be enforced by PostgreSQL row-level security using the authenticated tenant context.

#### Scenario: Runtime validation uses the intended application role

- GIVEN issue #14 runtime isolation is validated
- WHEN reviewers inspect the proof of cross-tenant isolation
- THEN the protected read validation MUST use the intended application runtime role or connection behavior
- AND the proof MUST NOT depend on superuser or migration-owner access that would bypass RLS.

### Requirement: Dependency and Scope Boundary Preservation

The system MUST keep issue #14 as planning and implementation scope for tenant auth and RLS context only, and implementation MUST remain blocked until issue #13 is stable.

#### Scenario: Issue 13 remains the prerequisite

- GIVEN issue #13 provides the schema, RLS foundations, and generated database layer required by issue #14
- WHEN issue #14 implementation readiness is assessed
- THEN issue #14 implementation MUST remain blocked until issue #13 is stable
- AND this specification MAY be completed before that implementation dependency is cleared.

#### Scenario: Remote MCP and console remain out of scope

- GIVEN issue #14 establishes the tenant auth and RLS proof seam
- WHEN downstream work is planned
- THEN issue #14 MUST NOT implement the #1 console or walking skeleton beyond the minimal API proof seam
- AND issue #14 MUST NOT implement issue #17 Remote MCP behavior.
