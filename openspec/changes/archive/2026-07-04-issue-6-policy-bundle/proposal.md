# Proposal: Issue #6 Versioned, Immutable PolicyBundle with Reproducible Evaluations

## Problem / motivation

Vigía's compliance promise is that every verdict can be traced to *the exact
rules that were in force when it was decided*. The foundation migration
(`00001_initial_foundation.sql`) already created `policy_rules`,
`policy_bundles`, and the `policy_bundle_rules` join — but the schema is
**inert**: nothing enforces immutability, the join carries no `effective_date`
or `legal_basis`, and evaluations never record which bundle judged them. Today
`evaluations.policy_bundle_version` is a `text` column populated only by its DB
default (`''`), so every evaluation claims an empty policy version. The evidence
ledger (issue #3) already hashes that string into its chain, so the field exists
and is load-bearing — it is simply always empty.

Without this change Vigía has bundles but no *point-in-time* rule snapshot, no
proof of which rules applied, and no way to re-run an interaction against a
specific historical bundle. This closes the wiring + immutability +
effective-date/legal-basis + console-surfacing gaps on an existing schema.

## Intent

Deliver reproducible, versioned, immutable policy bundles on top of the existing
scaffold:

1. Make `policy_bundles` and `policy_bundle_rules` **append-only** (reuse the
   evidence-ledger mutation-block trigger pattern), so editing a rule produces a
   **new** bundle version with prior versions intact.
2. Add `effective_date` and `legal_basis` to `policy_bundle_rules` so each
   snapshot row records when the rule applied and its legal grounding.
3. Resolve the **current bundle** for a tenant and stamp its real version string
   (+ a new FK id) onto every `Evaluation` at evaluation time.
4. Provide a **re-evaluation** path that reruns the currently-wired detectors/
   judge against an interaction while stamping the requested historical bundle
   version, proving the reproducibility mechanism.
5. Surface the bundle version that judged each interaction in the console.

## Current behavior

- `policy_bundles` / `policy_bundle_rules` exist (RLS tenant-isolated, composite
  PK on the join) but have **no** immutability trigger and no
  `effective_date`/`legal_basis` columns.
- `evaluations.policy_bundle_version text NOT NULL DEFAULT ''` exists and is
  hashed by `internal/ledger`, but is never populated: `CreateEvaluationInput`
  has no bundle field and `EvaluationStore.CreateEvaluation` never passes one.
- Detectors (contact-hours) and the LLM judge are hardcoded via
  `NamedDetector`/`NamedJudge` wiring in `Service`, **not** driven by
  `policy_bundle_rules` content.
- `ListCurrentTenantInteractionsWithOutcome` and the console `InteractionsTable`
  expose no bundle-version field.

## Desired behavior

- A migration adds `effective_date`/`legal_basis` to `policy_bundle_rules`,
  append-only triggers on both bundle tables (with a carve-out so lifecycle
  `status` transitions remain possible), and a `policy_bundle_id` composite FK
  (`(policy_bundle_id, tenant_id) REFERENCES policy_bundles(id, tenant_id)`) on
  `evaluations` alongside the retained `policy_bundle_version` text.
- A resolver returns the current active bundle (version + id) for a tenant;
  `EvaluateInteraction` stamps both onto the evaluation and the ledger body
  carries the **real** version string (no ledger schema/hash-field change).
- A re-evaluation entry point reruns detectors/judge and stamps a caller-supplied
  historical bundle version/id, demonstrating reproducibility.
- The interaction list query, httpapi DTO, console `Interaction` type, and table
  gain a bundle-version column.

## Scope

### In scope

- Migration `0000X_policy_bundle_versioning.sql`: `effective_date` +
  `legal_basis` on `policy_bundle_rules`; append-only + TRUNCATE-blocking
  triggers on `policy_bundles` and `policy_bundle_rules` (reuse the
  `evidence_records_no_update_delete` / `evidence_records_no_truncate`
  pattern, with an explicit allowed `status` transition path); a `status`
  CHECK constraint and a partial unique index enforcing at most one active
  bundle per `(tenant_id, name)`; `policy_bundle_id` composite FK on
  `evaluations`; `GRANT SELECT` for `vigia_app` on both bundle tables.
- Bundle resolution: a query + method returning the current bundle (version, id)
  for a tenant.
- Thread the bundle through evaluation: extend `CreateEvaluationInput`,
  `EvaluationStore.CreateEvaluation`, and the `CreateEvaluation` sqlc
  query/params to persist the real version string + FK id.
- A re-evaluation path (`ReEvaluateInteraction(ctx, interactionID,
  policyBundleID)`) that reruns the wired detectors/judge and stamps the
  requested version.
- Console surfacing: add `policy_bundle_version` to
  `ListCurrentTenantInteractionsWithOutcome`, the httpapi DTO, the console API
  type, and the interactions table column (AC5).
- sqlc regeneration and table-driven + integration tests (immutability trigger,
  new-version-on-edit, stamped evaluation, reproducible re-evaluation).

### Out of scope

- **Data-driven rule interpretation** — rules selecting which detectors/judge
  rubric run per bundle version. Detector/judge wiring stays hardcoded in
  `Service`; re-evaluation reruns the *same* wired pipeline while stamping the
  requested version. Full rule-interpreter is an explicit follow-up.
- Changing `internal/ledger` field order or adding ledger Body fields (would
  break golden hashes). The existing `policy_bundle_version` string is reused
  as-is — only its value changes from `''` to the resolved version.
- Backfilling historical evaluations with resolved bundle versions.
- New console detail page (AC5 met via a list column on the existing page).
- REDECO/tenant-overlay/channel-rule authoring UI or seeding real rule content.

### Delivery

Single PR. `size:exception` acceptable (user decision) — migration, resolver,
evaluation wiring, re-evaluation path, and console column form one coherent
versioning slice; splitting them ships a bundle that is versioned but never
stamped or surfaced.

## Capabilities

### New Capabilities

- `policy-bundle`: versioned, immutable point-in-time policy bundles
  (append-only bundle + join with `effective_date`/`legal_basis`, current-bundle
  resolution, evaluation stamping, reproducible re-evaluation, console
  surfacing).

### Modified Capabilities

- None. Issue #3's `evidence-ledger` spec is unaffected: the ledger continues to
  hash the existing `policy_bundle_version` string field, now populated with a
  real value instead of `''` — no requirement change.

## Approach

Adopt the exploration's **Approach 1 (minimal wiring)**: reuse the proven
evidence-ledger append-only trigger for bundle immutability; add the
snapshot-metadata columns the issue requires; resolve and stamp the real bundle
version + FK id at evaluation time; provide a re-evaluation path that reruns the
existing pipeline against a requested version; and thread the version through to
the console. Keep the ledger hash chain untouched by reusing the existing string
field. Defer full data-driven rule interpretation as a documented follow-up.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `db/migrations/0000X_policy_bundle_versioning.sql` | New | Columns, append-only + TRUNCATE guard triggers, single-active constraint/index, evaluations FK, `vigia_app` grants |
| `db/queries/{policies,evaluations,interaction_events}.sql` | Modified | Resolve bundle, stamp version, surface column |
| `internal/db/*.sql.go` | Modified | sqlc regeneration |
| `internal/evaluation/service.go` | Modified | Bundle field on input; re-evaluation path |
| `internal/postgres/adapters.go` | Modified | Pass version/id through `CreateEvaluation` |
| `internal/httpapi/httpapi.go` | Modified | DTO + optional re-evaluation endpoint |
| `internal/core/types.go` | Modified | Evaluation/join struct fields |
| `apps/console/src/lib/api.ts`, `apps/console/src/app/interactions/InteractionsTable.tsx` | Modified | Bundle-version field + column |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Immutability trigger blocks legitimate `draft→active→superseded` status transitions | Med | Trigger carve-out allowing only `status` column updates along allowed transitions; design phase pins the rule |
| Changing `policy_bundle_version` semantics breaks ledger reproducibility/golden hashes | High | Keep the text version string for hashing; add a separate FK id for joins; do not touch ledger field order |
| AC4 read as full rule-driven detector selection, exceeding minimal scope | Med | Explicitly scope re-evaluation as rerun-and-stamp; document rule-interpreter as follow-up |
| Existing evaluations show empty bundle version post-migration | Low | No backfill by design; new evaluations stamped going forward, stated explicitly |
| Single-PR size exceeds 400-line budget | Med | `size:exception` pre-approved; slice is coherent |
| Concurrent bundle edits create two active bundles or duplicate version numbers | Low | `CreateBundleVersion` takes a `FOR UPDATE` lock on the prior-active row before superseding; a partial unique index on `(tenant_id, name) WHERE status = 'active'` and `UNIQUE(tenant_id, name, version)` enforce the invariant at the schema level regardless |

## Rollback Plan

- Roll back the migration via `make migrate-down` (revokes `vigia_app` grants,
  drops the single-active constraint/index, triggers, new columns, and the
  `evaluations.policy_bundle_id` FK).
- Revert `CreateEvaluationInput`/`EvaluationStore.CreateEvaluation` to not pass a
  bundle version; the column reverts to its `''` default.
- Remove the re-evaluation path and any new endpoint.
- Revert the sqlc-regenerated queries and console column.
- Issue #3 evidence ledger is unaffected (the string field simply returns to
  `''` for new evaluations); existing evaluation rows are untouched.

## Dependencies

- Existing `policy_bundles` / `policy_rules` / `policy_bundle_rules` scaffold
  (foundation migration).
- Issue #3 evidence-ledger append-only trigger pattern (reused, not modified).
- goose + sqlc migration/query workflow (`make migrate-up`, `make sqlc`).

## Success Criteria

- [ ] Editing a rule produces a **new** bundle version; prior versions remain
      queryable and unmutated (append-only trigger enforced) — AC1, AC3.
- [ ] Every new `Evaluation` records the real bundle version string + FK id
      used — AC2.
- [ ] Re-evaluating an interaction against a specified bundle version reruns the
      pipeline and stamps that version reproducibly — AC4.
- [ ] The console interactions list shows which bundle version judged each
      interaction — AC5.
- [ ] Table-driven + integration tests cover immutability, new-version-on-edit,
      stamping, and re-evaluation; ledger golden hashes remain stable.

## Proposal question round

No interactive question round was run; the orchestrator supplied the scope
decision (Approach 1 minimal wiring) with instruction not to re-ask. Assumptions
the spec/design should confirm:

- Bundle lifecycle uses a `status` flag with allowed transitions
  (`draft→active→superseded`); "current" = the active bundle per tenant+name.
  The exact transition set and whether the immutability trigger carves out
  `status` updates (vs. modeling status as new rows) is a **design decision**.
- Re-evaluation reruns the *currently wired* detectors/judge, not a rule-driven
  selection; AC4 reproducibility is proven by stamping, not by historical rule
  interpretation.
- `policy_bundle_version` stays a human-readable string for the ledger hash; a
  separate `policy_bundle_id` FK is added for joins.

If any of these should change, raise it before spec.

## Next recommended phase

Spec and Design (can run in parallel).
