# Policy Bundle Specification

## Purpose

Define testable requirements for issue #6 — versioned, immutable
`PolicyBundle` snapshots with reproducible evaluations. Closes the wiring
gap on the existing (inert) `policy_bundles`/`policy_bundle_rules` schema:
enforces append-only immutability, records `effective_date`/`legal_basis`
per rule snapshot, stamps the real bundle version + FK onto every
evaluation, provides a rerun-and-stamp re-evaluation path, and surfaces the
bundle version in the console. Full data-driven rule interpretation
(rules selecting which detectors/judge run) is explicitly OUT of scope —
detector/judge wiring stays hardcoded; re-evaluation reruns the same wired
pipeline while stamping a requested historical version.

## Testing mode note

Strict TDD applies. `[unit]` requirements run with no external
dependencies. `[integration]` requirements require Postgres and MUST be
skippable with `testing.Short()`.

---

## Requirement: Policy Bundles and Rule Snapshots Are Append-Only

`policy_bundles` and `policy_bundle_rules` MUST reject any `UPDATE` or
`DELETE` via a database-level `BEFORE UPDATE OR DELETE` trigger (reusing
the evidence-ledger append-only trigger pattern), with `ENABLE ALWAYS` so
no role or replica-mode session can bypass it. Each table MUST also reject
`TRUNCATE` via a separate `BEFORE TRUNCATE FOR EACH STATEMENT` trigger
(mirroring `evidence_records_no_truncate`), also `ENABLE ALWAYS`, since a
row-level trigger never fires on `TRUNCATE`. The UPDATE/DELETE trigger MUST
allow exactly one carve-out: updating only the `status` column on
`policy_bundles` along an allowed lifecycle transition
(`draft -> active -> superseded`). Editing a rule MUST NOT mutate an
existing snapshot row — it MUST produce a new `policy_bundle_rules` row
under a new bundle version instead.

### Scenario: Direct UPDATE against policy_bundle_rules fails `[integration]`

- GIVEN an existing `policy_bundle_rules` row
- WHEN a direct SQL `UPDATE` targeting a non-`status` column is executed
- THEN the statement MUST fail with an exception from the trigger
- AND the row's stored values MUST remain unchanged.

### Scenario: Direct TRUNCATE against either bundle table fails `[integration]`

- GIVEN existing rows in `policy_bundles` and in `policy_bundle_rules`
- WHEN a direct SQL `TRUNCATE` is executed against either table
- THEN the statement MUST fail with an exception from the statement-level
  trigger
- AND all rows in that table MUST remain intact.

### Scenario: Editing a rule produces a new bundle version `[integration]`

- GIVEN an active bundle with one or more `policy_bundle_rules` rows
- WHEN a rule change is requested
- THEN a new `policy_bundles` row (new version) and new
  `policy_bundle_rules` rows MUST be created
- AND the prior bundle version's rows MUST remain unmutated and queryable.

### Scenario: Allowed status transition succeeds `[integration]`

- GIVEN a bundle with `status = 'draft'`
- WHEN its `status` is updated to `'active'`
- THEN the update MUST succeed
- AND no other column on that row MUST change.

---

## Requirement: Rule Snapshots Record Effective Date and Legal Basis

Each `policy_bundle_rules` row MUST carry `effective_date` and
`legal_basis` columns, populated at snapshot-creation time, so every rule
inclusion in a bundle version records when it applied and its legal
grounding.

### Scenario: New rule snapshot carries effective_date and legal_basis `[integration]`

- GIVEN a new bundle version is created with rule snapshots
- WHEN a `policy_bundle_rules` row is inserted
- THEN it MUST have non-null `effective_date` and `legal_basis` values.

---

## Requirement: At Most One Active Bundle Per Tenant and Name

At most one `policy_bundles` row per `(tenant_id, name)` MUST have
`status = 'active'` at any time. This MUST be enforced at the schema level
via a partial unique index, not only by application logic, so that
concurrent bundle-creation requests cannot leave two active bundles for
the same tenant+name.

### Scenario: Concurrent bundle creation cannot yield two active bundles `[integration]`

- GIVEN a tenant has an active bundle named `retention-policy`
- WHEN two concurrent requests each attempt to create a new active version
  of `retention-policy` for that tenant
- THEN exactly one of the resulting bundle rows MUST have
  `status = 'active'`
- AND any attempt that would violate the single-active constraint MUST
  fail rather than silently succeed.

---

## Requirement: Evaluations Are Stamped With the Resolved Bundle Version

A resolver MUST return the current active bundle (version string + id) for
a tenant. `EvaluateInteraction` MUST stamp both the version string (via
the existing `evaluations.policy_bundle_version` text field) and a new
`policy_bundle_id` composite FK (`(policy_bundle_id, tenant_id) REFERENCES
policy_bundles(id, tenant_id)`) onto every new evaluation. The ledger body
MUST continue hashing the same string field with no field-order or
hash-schema change.

### Scenario: New evaluation stamps the real bundle version and FK `[integration]`

- GIVEN a tenant has an active bundle at version `v2`
- WHEN an interaction is evaluated for that tenant
- THEN the resulting `evaluations` row MUST have
  `policy_bundle_version = "v2"` and `policy_bundle_id` set to that
  bundle's id
- AND the evidence ledger hash MUST incorporate the real version string.

### Scenario: No active bundle leaves the field empty as before `[integration]`

- GIVEN a tenant has no active bundle
- WHEN an interaction is evaluated for that tenant
- THEN `policy_bundle_version` MUST remain the existing empty-string
  default and `policy_bundle_id` MUST be null
- AND evaluation MUST NOT fail solely due to a missing bundle.

---

## Requirement: Reproducible Re-Evaluation Against a Specific Bundle Version

A `ReEvaluateInteraction(ctx, interactionID, policyBundleID)` path MUST
rerun the currently-wired detectors and judge against the interaction and
stamp the caller-supplied historical `policy_bundle_id`/version onto the
resulting evaluation, proving the bundle-stamping mechanism is
reproducible. This does NOT select detectors/rubric based on bundle rule
content — that is an explicit follow-up.

### Scenario: Re-evaluation stamps the requested historical version `[integration]`

- GIVEN an interaction was originally evaluated under bundle version `v1`
  and a later bundle version `v2` now exists
- WHEN `ReEvaluateInteraction` is called with the `v1` bundle id
- THEN a new evaluation MUST be produced with `policy_bundle_version =
  "v1"` and the matching `policy_bundle_id`
- AND the same wired detectors/judge MUST run, independent of bundle rule
  content.

### Scenario: Re-evaluation against an unknown bundle id fails `[integration]`

- GIVEN an interaction exists
- WHEN `ReEvaluateInteraction` is called with a `policyBundleID` that does
  not exist for that tenant
- THEN the call MUST return a defined error
- AND MUST NOT create an evaluation row.

---

## Requirement: Console Surfaces the Judging Bundle Version

`ListCurrentTenantInteractionsWithOutcome`, the httpapi DTO, the console
API type, and the interactions table MUST expose the evaluation's
`policy_bundle_version` for each interaction. The field MUST distinguish
`null` (no evaluation row exists for the interaction) from the empty
string (an evaluation ran but no bundle was active), following the same
`CASE WHEN ... IS NULL THEN NULL ELSE ... END` convention used for
`threat_flagged` in `db/queries/interaction_events.sql`.

### Scenario: Interactions list includes the bundle version column `[integration]`

- GIVEN a seeded interaction has been evaluated under bundle version `v2`
- WHEN the console interactions list is fetched
- THEN the response MUST include `policy_bundle_version = "v2"` for that
  interaction
- AND the `InteractionsTable` MUST render it as a visible column.

### Scenario: Unevaluated interaction shows null, not an empty string `[integration]`

- GIVEN a seeded interaction has not yet been evaluated (no evaluation row
  exists, so the LEFT JOIN yields no matching row)
- WHEN the console interactions list is fetched
- THEN the `policy_bundle_version` field MUST be `null`
- AND MUST NOT be fabricated as an empty string.

### Scenario: Evaluated interaction with no active bundle shows an empty string `[integration]`

- GIVEN a seeded interaction was evaluated while the tenant had no active
  bundle (an evaluation row exists with the sentinel `policy_bundle_version
  = ''`)
- WHEN the console interactions list is fetched
- THEN the `policy_bundle_version` field MUST be the empty string `""`,
  distinct from `null`
- AND this distinction MUST follow the same convention as
  `threat_flagged` in `db/queries/interaction_events.sql` (`CASE WHEN
  e.id IS NULL THEN NULL ELSE ... END`), where a missing evaluation row
  produces `NULL` and a present-but-unset value produces its stored
  sentinel.

---

## Non-goals (hardened by this spec)

- Data-driven rule interpretation: no requirement in this spec permits
  `policy_bundle_rules` content to select detectors or judge rubric.
  Detector/judge wiring stays hardcoded in `Service`.
- Backfilling `policy_bundle_version`/`policy_bundle_id` on evaluations
  created before this migration.
- Any change to `internal/ledger` field order or new Body fields.
- A new console detail page — AC5 is met via a list column only.
- Policy authoring UI or seeding real rule content.

---

## Dependency alignment

Depends on the existing `policy_bundles`/`policy_rules`/
`policy_bundle_rules` foundation schema and the issue #3 evidence-ledger
append-only trigger pattern (reused, not modified). Does not modify the
`evidence-ledger` spec: the ledger continues hashing the existing
`policy_bundle_version` string field, now populated with a real value.
