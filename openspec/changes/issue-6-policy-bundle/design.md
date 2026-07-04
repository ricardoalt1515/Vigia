# Design: Issue #6 Versioned, Immutable PolicyBundle with Reproducible Evaluations

## Technical Approach

Close the wiring/immutability gaps on the existing inert `policy_bundles` /
`policy_bundle_rules` / `evaluations` schema (Approach 1, minimal wiring). One
migration `00007_policy_bundle_versioning.sql` adds four append-only guard
triggers (row-level UPDATE/DELETE + statement-level TRUNCATE, one pair per
table, reusing the evidence-ledger `RAISE EXCEPTION` pattern), the
`effective_date`/`legal_basis` snapshot columns, a composite `policy_bundle_id`
FK on `evaluations`, a partial unique index enforcing one active bundle per
tenant+name, a `status` CHECK constraint, and `GRANT SELECT` for
`vigia_app` on both new-to-the-app tables (following 00004/00005/00006). A
`BundleResolver` seam resolves the active bundle for a tenant and stamps its real
version string + FK id onto every `Evaluation`; the ledger keeps hashing the
existing `policy_bundle_version` string (no Body field/order change). A
non-persisting `ReEvaluateInteraction` path reruns the wired detectors/judge
against a caller-supplied historical bundle to prove reproducibility. The version
is threaded to the console as an additive nullable column.

The load-bearing hazard is **ledger hash stability**: when no bundle is active the
stamped version stays `""`, so bodies serialize byte-identically to today and the
golden hash is untouched (Decision 3).

## Architecture Decisions

| # | Decision | Choice | Rejected | Rationale |
|---|---|---|---|---|
| 1 | Immutability + lifecycle | `policy_bundles`: trigger blocks DELETE always and UPDATE unless it touches ONLY `status` along an allowed transition; a separate `BEFORE TRUNCATE FOR EACH STATEMENT` trigger blocks TRUNCATE always (mirrors `evidence_records_no_truncate`). `policy_bundle_rules`: fully append-only (row-level `BEFORE UPDATE OR DELETE` trigger blocking all mutation, plus its own statement-level `BEFORE TRUNCATE` trigger). All four triggers `ENABLE ALWAYS`. A `CHECK (status IN ('draft','active','superseded'))` constraint and a partial unique index `CREATE UNIQUE INDEX ... ON policy_bundles (tenant_id, name) WHERE status = 'active'` enforce at most one active bundle per tenant+name at the schema level, independent of trigger logic. | Status-as-new-rows (fully immutable bundle row); a single combined UPDATE/DELETE/TRUNCATE trigger function | Content columns (`name`,`version`) ARE the point-in-time snapshot identity and must never mutate; `status` is orthogonal lifecycle metadata the ledger never hashes. A carve-out is simpler than re-inserting the whole bundle to flip a flag and keeps "one active per name" resolvable. TRUNCATE requires a dedicated `FOR EACH STATEMENT` trigger — a `FOR EACH ROW` trigger never fires on TRUNCATE, so the evidence-ledger pattern of two separate triggers per table must be reused verbatim rather than assumed to be covered by the row-level guard. |
| 2 | Schema deltas | Add `effective_date date NOT NULL` (add-nullable → backfill `created_at::date` → SET NOT NULL, in that order, executed BEFORE the append-only/TRUNCATE guard triggers are created in the same migration — the backfill UPDATE must run while the table is still unguarded) and `legal_basis text NOT NULL DEFAULT ''` to `policy_bundle_rules`. Add `policy_bundle_id uuid` (nullable) to `evaluations` with `FOREIGN KEY (policy_bundle_id, tenant_id) REFERENCES policy_bundles(id, tenant_id)` + supporting index. Keep `policy_bundle_version text` verbatim. | Making `policy_bundle_id` NOT NULL; replacing the text version with a UUID-only ref; creating the guard triggers before the backfill | Nullable id + retained text version keep existing rows valid, keep the sentinel path (Decision 3), and never touch the hashed string. Follows the `debtors.timezone` add-nullable pattern (00003). Ordering matters: creating the `policy_bundle_rules` append-only triggers before the `effective_date` backfill would make the backfill's own UPDATE fail against its own guard. |
| 3 | Stamping + no-active-bundle | Resolve THE active bundle (`status='active'`) per tenant; stamp real `version`+`id`. When none is active → sentinel `version=""`, `id=NULL` (today's behavior). | Fail-closed when no active bundle | Failing would break `cmd/seed` and every golden-hash test (which run with no seeded bundle). Empty version → byte-identical Body → stable hash. Stamping is additive, never a hard requirement. |
| 4 | Resolver seam | New `BundleResolver` port injected into `evaluation.Service` (nil-safe: nil ⇒ no stamping). | Resolving in the httpapi handler and passing via input | The Service owns evaluation provenance; a nil resolver reproduces pre-#6 unit tests exactly, so existing table-driven tests stay green. |
| 5 | Re-evaluation | `ReEvaluateInteraction(ctx, interactionID, policyBundleID)` reruns the SAME wired detectors/judge and returns a stamped-but-**unpersisted** `core.Evaluation`; reproducibility asserted by determinism (rerun ⇒ identical outcome + `inputs_digest` + computed hash). Entry point: `POST /v1/interactions/{id}/reevaluate`. | Persisting a second evaluation row | `UNIQUE(tenant_id, interaction_event_id)` on `evaluations` and one-evidence-per-evaluation forbid a second persisted row. Non-persisting rerun proves the mechanism without relaxing the ledger. Persisted re-eval history is an explicit follow-up. |
| 6 | Bundle compilation | Store method `CreateBundleVersion(tenant, name, rules)` in one tenant tx: `SELECT ... FOR UPDATE` on the prior-active row scoped to `(tenant_id, name)` (serializes concurrent edits and version numbering), then UPDATE prior active → `superseded` (Decision 1 carve-out) FIRST, then INSERT the new bundle (`status='active'`, `version='vN'` where N = prior count+1, count scoped per `(tenant_id, name)`) + snapshot rule rows. Supersede-before-insert is mandatory: the partial unique index below is non-deferrable (Postgres partial unique indexes cannot be `DEFERRABLE`), so inserting the new active row while the prior one is still `active` would violate it on every ordinary edit. A `CREATE UNIQUE INDEX ... ON policy_bundles (tenant_id, name) WHERE status = 'active'` partial unique index enforces at most one active bundle per tenant+name at the schema level as the safety net if the lock is ever bypassed; `UNIQUE(tenant_id, name, version)` remains the safety net against duplicate version numbers. | Content-hash version / stored digest column; relying on the lock alone without a schema-level constraint | Reproducibility comes from append-only snapshot rows, not a stored digest; `vN` is human-readable and ledger-friendly. Content-hash versioning is a follow-up (no column in scope). The `FOR UPDATE` lock on the prior-active row prevents two concurrent `CreateBundleVersion` calls from both superseding the same row and racing to insert two actives; the partial unique index makes that guarantee schema-enforced, not just transaction-enforced. |
| 7 | Console surfacing | Add `e.policy_bundle_version` to `ListCurrentTenantInteractionsWithOutcome`; thread through httpapi `Interaction` DTO (`*string`), console `Interaction` type (`string | null`), and a table column. | New detail page | Additive nullable column mirrors the issue-#4 `threat_flagged`/`requires_hitl` pattern; AC5 met on the existing list page. |

## Data Flow

    Rules edit ─► CreateBundleVersion (supersede prior, then INSERT new) ─► policy_bundles/policy_bundle_rules (append-only)
                                                                              │
    EvaluateInteraction ─► BundleResolver.ActiveBundle ──────────────────────┘ (version+id | "",nil)
              │                       │
              ▼                       ▼
      detectors/judge      CreateEvaluationInput{PolicyBundleVersion, PolicyBundleID}
              │                       │
              └────► EvaluationStore.CreateEvaluation ─► evaluations(+FK) ─► ledger.Body.PolicyBundleVersion (hashed as-is)
                                                                              │
      ListInteractions ─► DTO.policy_bundle_version ─► console column ◄───────┘

## Interfaces / Contracts

```go
// internal/evaluation — nil-safe seam; nil resolver ⇒ no stamping (pre-#6 behavior).
type BundleResolver interface {
    ActiveBundle(ctx context.Context, tenantID string) (version, id string, found bool, err error)
}
// CreateEvaluationInput gains:  PolicyBundleVersion string;  PolicyBundleID *string
// Service gains: Resolver BundleResolver;  ReEvaluateInteraction(ctx, interactionID, policyBundleID string) (core.Evaluation, error)
```

```sql
-- policy_bundles carve-out trigger (non-obvious core)
CREATE FUNCTION policy_bundles_guard_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP <> 'UPDATE' THEN
    RAISE EXCEPTION 'policy_bundles is append-only: % not permitted', TG_OP USING ERRCODE='restrict_violation';
  END IF;
  IF ROW(NEW.id,NEW.tenant_id,NEW.name,NEW.version,NEW.created_at)
     IS DISTINCT FROM ROW(OLD.id,OLD.tenant_id,OLD.name,OLD.version,OLD.created_at) THEN
    RAISE EXCEPTION 'policy_bundles: only status may change' USING ERRCODE='restrict_violation';
  END IF;
  IF NOT (OLD.status=NEW.status
          OR (OLD.status='draft'  AND NEW.status='active')
          OR (OLD.status='active' AND NEW.status='superseded')) THEN
    RAISE EXCEPTION 'policy_bundles: illegal status transition % -> %', OLD.status, NEW.status USING ERRCODE='restrict_violation';
  END IF;
  RETURN NEW;
END; $$;
-- policy_bundle_rules reuse the evidence-style block-all fn (BEFORE UPDATE OR DELETE row-level trigger).
-- Each table also gets its own BEFORE TRUNCATE FOR EACH STATEMENT trigger
-- (mirrors evidence_records_no_truncate) since a FOR EACH ROW trigger never
-- fires on TRUNCATE. All four triggers are ENABLE ALWAYS.

-- Schema-level "one active bundle per tenant+name" guarantee (belt-and-suspenders
-- alongside the FOR UPDATE lock in Decision 6):
ALTER TABLE policy_bundles ADD CONSTRAINT policy_bundles_status_check
    CHECK (status IN ('draft','active','superseded'));
CREATE UNIQUE INDEX policy_bundles_one_active_per_tenant_name
    ON policy_bundles (tenant_id, name) WHERE status = 'active';
```

New queries (`db/queries/policies.sql`): `GetActiveBundleByTenant`,
`CountBundleVersions`. `CreatePolicyBundle` and `AddPolicyBundleRule` (both
already exist in `policies.sql`, MODIFIED not new) — `AddPolicyBundleRule`
gains `effective_date`/`legal_basis` params since `effective_date` is
`NOT NULL` with no default. `CreateEvaluation` (`evaluations.sql`) adds
`policy_bundle_version`, `policy_bundle_id` to INSERT + RETURNING.

## File Changes

| File | Action | Description |
|---|---|---|
| `db/migrations/00007_policy_bundle_versioning.sql` | Create | Columns (add-nullable → backfill → SET NOT NULL, before guard triggers), 4 guard triggers (row-level UPDATE/DELETE + statement-level TRUNCATE per table, `ENABLE ALWAYS`), `status` CHECK constraint, partial unique index (one active bundle per tenant+name), evaluations FK + index, `GRANT SELECT ON policy_bundles, policy_bundle_rules TO vigia_app`, Down (drops grants, index, constraint, triggers, FK, columns) |
| `db/queries/policies.sql` | Modify | `CreatePolicyBundle`, `AddPolicyBundleRule` (+`effective_date`/`legal_basis` params); new `GetActiveBundleByTenant`, `CountBundleVersions` |
| `db/queries/{evaluations,interaction_events}.sql` | Modify | Stamp version+id, surface column |
| `internal/db/*.sql.go`, `models.go` | Regenerate | `make sqlc` |
| `internal/core/types.go` | Modify | `Evaluation.PolicyBundleID`; `PolicyBundleRule.{EffectiveDate,LegalBasis}` |
| `internal/evaluation/service.go` | Modify | `BundleResolver`, stamp fields, `ReEvaluateInteraction` |
| `internal/postgres/adapters.go` | Modify | Pass version+id to `CreateEvaluation`; `BundleResolver`/`CreateBundleVersion` (with `FOR UPDATE` lock) adapters |
| `internal/httpapi/httpapi.go` | Modify | DTO field; `POST /v1/interactions/{id}/reevaluate` |
| `internal/db/migration_test.go` | Modify | Extend grant-check table list with `policy_bundles`, `policy_bundle_rules` |
| `apps/console/src/lib/api.ts`, `apps/console/src/app/interactions/InteractionsTable.tsx` | Modify | `policy_bundle_version` field + column |

## Testing Strategy (Strict TDD — `make test`)

| Layer | What | Approach |
|---|---|---|
| Unit | Stamping / resolver | Table-driven `Service` with fake resolver+store: active bundle ⇒ input carries version+id; nil/not-found ⇒ `""`/nil. |
| Unit | Re-eval determinism | Same interaction+bundle rerun ⇒ identical outcome + `inputs_digest`. |
| Unit | Ledger golden guard | Existing golden hash unchanged with `version=""`; new case: non-empty version changes the hash (stamping is hashed), `""` reproduces the pinned hex byte-identically. |
| Integration `_integration_test.go` | Triggers | Owner conn: UPDATE non-status / DELETE on bundles ⇒ exception; `draft→active`, `active→superseded` allowed; `active→draft` blocked; any UPDATE/DELETE on rules ⇒ exception. |
| Integration `_integration_test.go` | TRUNCATE guard | Owner conn: `TRUNCATE policy_bundles` and `TRUNCATE policy_bundle_rules` both fail with an exception from the statement-level trigger; rows remain intact after the failed attempt. |
| Integration | New-version-on-edit | `CreateBundleVersion` twice ⇒ two bundles, prior `superseded`, both rule snapshots intact; version numbers scoped per `(tenant_id, name)`. |
| Integration | Concurrent bundle creation | Two concurrent `CreateBundleVersion` calls for the same `(tenant_id, name)` ⇒ the `FOR UPDATE` lock serializes them (one blocks until the other commits); result is exactly one active bundle and no duplicate version, enforced by the partial unique index and `UNIQUE(tenant_id, name, version)` even if the lock were bypassed. |
| Integration | Stamping + RLS | Evaluate with active bundle ⇒ `evaluations.policy_bundle_version`/`policy_bundle_id` set, evidence hashes real version; tenant A cannot resolve tenant B's bundle. |
| Integration | Migration catalog | New columns/FK/triggers/CHECK/unique index present, RLS enabled, `vigia_app` has SELECT on `policy_bundles`/`policy_bundle_rules` and no write privileges (extend `migration_test.go` grant-check table list). |

## Migration / Rollout

Single migration; `make migrate-down` revokes `vigia_app` grants and drops the
partial unique index, CHECK constraint, triggers, columns, and FK. Reverting the
resolver wiring returns `policy_bundle_version` to `''` for new evaluations. No
backfill of historical evaluations (Decision 3). `size:exception` pre-approved.

## Open Questions

- None blocking. Spec should record: (a) re-evaluation is non-persisting in this
  slice; (b) no-active-bundle stamps the empty sentinel by design.
