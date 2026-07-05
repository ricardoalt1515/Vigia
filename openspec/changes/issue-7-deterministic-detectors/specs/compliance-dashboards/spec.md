# Compliance Dashboards Specification

## Purpose

Give compliance owners two tenant-scoped views built on the new deterministic
detectors: a by-despacho violation-rate ranking and a by-REDECO-cause
breakdown, both computed as SQL aggregates following the `CountOutOfHours`
convention (aggregation in SQL, never client-side).

## Testing mode note

`[integration]` requirements require Postgres and MUST be skippable with
`testing.Short()`. `[manual-demo]` requirements are validated by a human
running the local dev environment.

---

## Requirement: By-Despacho Violation-Rate Aggregate Endpoint

The system MUST provide a tenant-scoped endpoint that returns, per despacho
attributed to that tenant, a server-computed violation rate (violating
interactions / total evaluated interactions — explicitly NOT
`outcome != 'pass'`, since `review` rows are judge uncertainty and `warn`
rows (MX-REDECO-03) are a confirmed warn-level signal, neither of which is a
confirmed hard-block violation), computed via a SQL aggregate inside the same
`tenantdb.WithTenantTx` + RLS seam used by `CountOutOfHours`.

Both `total` and `violations` MUST be counted at interaction grain, never
`detector_result_rows` row grain: a single evaluated interaction produces up
to 7 detector-result rows (one per rule), so a row-grain division would let
the rate exceed 100% or dilute a real violation. `total` MUST be
`COUNT(DISTINCT interaction_events.id)` of evaluated interactions per
despacho; `violations` MUST be `COUNT(DISTINCT interaction_events.id)` of
interactions with at least one `detector_result_rows.outcome = 'fail'` row
(e.g. an `EXISTS` correlated subquery or `bool_or(outcome = 'fail')` per
interaction), never `COUNT(detector_result_rows.*)`. `violation_rate` MUST
guard the zero-total case with `NULLIF(total, 0)`. Despachos tied on
`violation_rate` MUST be tie-broken by `despacho_name` ascending for a
deterministic ranking.

#### Scenario: Endpoint ranks despachos by violation rate `[integration]`

- GIVEN a tenant has interactions attributed to two or more despachos with
  differing `BLOCK`/total ratios
- WHEN the tenant calls the by-despacho endpoint with a valid tenant API
  key
- THEN the response MUST return each despacho's violation rate
- AND despachos MUST be ordered by descending violation rate.

#### Scenario: Interactions with no despacho attribution are reported under an explicit unattributed bucket `[integration]`

- GIVEN a tenant has some interactions with no despacho FK set
- WHEN the by-despacho endpoint is called
- THEN unattributed interactions MUST NOT be silently folded into any
  named despacho's rate
- AND the response MUST include an explicit bucket shaped
  `{despacho_id: null, despacho_name: "unattributed", total, violations,
  violation_rate}` covering those interactions, so compliance sees
  attribution-coverage gaps rather than a silently shrunk denominator.

#### Scenario: By-despacho aggregate is tenant-isolated `[integration]`

- GIVEN two tenants each have despachos and evaluated interactions
- WHEN tenant A calls the by-despacho endpoint
- THEN the response MUST include only tenant A's despachos and rates,
  enforced by RLS.

---

## Requirement: By-REDECO-Cause Breakdown Aggregate Endpoint

The system MUST provide a tenant-scoped endpoint that returns, per REDECO
rule code, a server-computed count of violations (rows where
`detector_result_rows.outcome = 'fail'`, explicitly NOT `outcome != 'pass'`)
for that tenant, computed via a SQL aggregate grouped by rule code. The
endpoint MUST additionally return a separate `warnings` count per rule code
(rows where `outcome = 'warn'`), so MX-REDECO-03 warn-level activity is
visible without inflating `violations`.

#### Scenario: Endpoint breaks down violations by rule code `[integration]`

- GIVEN a tenant has `outcome = 'fail'` evaluations across at least three
  different REDECO rule codes
- WHEN the tenant calls the by-cause endpoint with a valid tenant API key
- THEN the response MUST return a count per rule code present in that
  tenant's `outcome = 'fail'` evaluations
- AND the counts MUST be computed by a SQL `GROUP BY`, not fetched and
  counted in application code
- AND `review` outcome rows MUST NOT be counted as violations.

#### Scenario: Endpoint reports MX-REDECO-03 warn activity separately from violations `[integration]`

- GIVEN a tenant has `outcome = 'warn'` evaluations for `MX-REDECO-03`
- WHEN the tenant calls the by-cause endpoint with a valid tenant API key
- THEN the response MUST include a `warnings` count for `MX-REDECO-03`
  reflecting those `outcome = 'warn'` rows
- AND those `warn` rows MUST NOT be included in the `violations` count for
  `MX-REDECO-03` or any other rule code.

#### Scenario: By-cause aggregate is tenant-isolated `[integration]`

- GIVEN two tenants each have `outcome = 'fail'` evaluations across REDECO
  rule codes
- WHEN tenant A calls the by-cause endpoint
- THEN the response MUST reflect only tenant A's evaluations, enforced by
  RLS.

---

## Requirement: Console Dashboards Render Both Aggregates

`apps/console` MUST provide two greenfield dashboard pages: one rendering
the by-despacho violation-rate ranking, one rendering the by-REDECO-cause
breakdown, each fed by its respective aggregate endpoint and not by
client-side aggregation.

#### Scenario: By-despacho dashboard renders the ranking `[manual-demo]`

- GIVEN the demo tenant has seeded interactions attributed to multiple
  despachos with evaluated outcomes
- WHEN a developer opens the by-despacho dashboard page
- THEN despachos MUST be listed ordered by violation rate
- AND the rate values MUST come from the by-despacho endpoint response.

#### Scenario: By-cause dashboard renders the breakdown `[manual-demo]`

- GIVEN the demo tenant has seeded, evaluated interactions across multiple
  REDECO rule codes
- WHEN a developer opens the by-cause dashboard page
- THEN the page MUST display a count per rule code
- AND the values MUST come from the by-cause endpoint response, not be
  summed client-side.

---

## Non-goals (hardened by this spec)

- Detail/drill-down UX beyond the two ranking/breakdown views.
- Date-range filtering on either aggregate endpoint.
- Cross-tenant or platform-wide rankings — both aggregates are
  tenant-scoped only.

## Dependency alignment

Depends on the `deterministic-detectors` spec (rule-code-tagged
evaluations) and the `despacho-registry` spec (despacho attribution). Must
ship after both, per the proposal's chained delivery order.
