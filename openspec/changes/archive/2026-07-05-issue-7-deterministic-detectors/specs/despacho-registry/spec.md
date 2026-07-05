# Despacho Registry Specification

## Purpose

Establish `despachos` as a tenant-scoped attribution/scoping dimension: one
tenant (creditor) contracts N despachos (collection firms). This is the
minimal identity schema — not the full aspirational lifecycle/contract model
in `docs/technical-design.md` — needed to attribute interactions to a
despacho and feed the by-despacho dashboard.

## Testing mode note

`[integration]` requirements require Postgres and MUST be skippable with
`testing.Short()`.

---

## Requirement: Despachos Table Is Tenant-Scoped With RLS

A `despachos` table MUST exist with a required `tenant_id` foreign key and
MUST be protected by row-level security consistent with the pattern
established in migrations `00004`/`00007`, so a despacho row is only
visible under its owning tenant's RLS context.

#### Scenario: Despacho row is visible only to its owning tenant `[integration]`

- GIVEN two tenants each have at least one `despachos` row
- WHEN tenant A's RLS context queries `despachos`
- THEN the result MUST include only tenant A's rows
- AND MUST exclude tenant B's rows, enforced by PostgreSQL RLS.

#### Scenario: Despacho row cannot be created without a tenant `[integration]`

- GIVEN an insert into `despachos` omits `tenant_id`
- WHEN the insert is attempted
- THEN the database MUST reject the insert with a not-null constraint
  violation.

---

## Requirement: One Tenant Contracts N Despachos

The `despachos` table MUST support a 1-tenant-to-N-despachos cardinality:
multiple `despachos` rows MAY reference the same `tenant_id`, and no unique
constraint MUST limit a tenant to a single despacho.

#### Scenario: A tenant can have multiple despachos `[integration]`

- GIVEN a tenant has no existing despachos
- WHEN two `despachos` rows are created for that same tenant
- THEN both inserts MUST succeed
- AND both rows MUST be queryable under that tenant's RLS context.

---

## Requirement: Interaction Events Carry an Optional Despacho FK

`interaction_events` MUST gain a nullable despacho foreign key referencing
`despachos(id)`, scoped to the same tenant as the interaction (composite FK
or equivalent tenant-consistency constraint), so despacho attribution is
optional per interaction and does not require backfilling existing rows.

#### Scenario: Existing interactions remain valid after the FK is added `[integration]`

- GIVEN `interaction_events` rows exist prior to this migration
- WHEN the nullable despacho FK column is added
- THEN pre-existing rows MUST remain valid with the new column `NULL`
- AND existing interaction-creation tests MUST continue to pass unmodified.

#### Scenario: An interaction can be attributed to a despacho of the same tenant `[integration]`

- GIVEN a tenant has a `despachos` row
- WHEN an `interaction_events` row is created for that tenant referencing
  that despacho's id
- THEN the insert MUST succeed
- AND the interaction MUST be readable with its despacho attribution
  intact.

#### Scenario: An interaction cannot reference a despacho from a different tenant `[integration]`

- GIVEN tenant A has a despacho and tenant B has an interaction
- WHEN tenant B's interaction attempts to reference tenant A's despacho id
- THEN the insert MUST fail due to the tenant-consistency constraint.

---

## Requirement: Despacho Go Type Mirrors the Minimal Schema

`internal/core/types.go` MUST expose a `Despacho` type carrying at minimum
`ID`, `TenantID`, `ExternalRef`, and a display name, matching the minimal
schema — no status-lifecycle or contract-URI fields from the aspirational
design. `ExternalRef` is required by the `UNIQUE (tenant_id, external_ref)`
constraint on the `despachos` table.

#### Scenario: Despacho type round-trips through the generated data layer `[unit]`

- GIVEN a `Despacho` value with `ID`, `TenantID`, `ExternalRef`, and a name
  set
- WHEN it is passed through the sqlc-generated create/read path
- THEN the returned value MUST match the input's `ID`, `TenantID`,
  `ExternalRef`, and name.

---

## Non-goals (hardened by this spec)

- Despacho status lifecycle, contract URIs, or RFC fields from
  `docs/technical-design.md`.
- Any UI for creating/editing despachos (seed-only for this change).
- Making despacho attribution mandatory on interactions.

## Dependency alignment

Depends on the tenant RLS foundation established by the
`tenant-auth-rls-context` spec (issue #14) and reuses its RLS/grant
pattern.
