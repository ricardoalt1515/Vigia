-- name: CreatePolicyRule :one
INSERT INTO policy_rules (code, title, description, severity)
VALUES ($1, $2, $3, $4)
RETURNING id, code, title, description, severity, created_at;

-- name: UpsertPolicyRule :one
-- Idempotent catalog seeding (issue #7 Design Decision "catalog + bundle
-- seeding idempotency"): policy_rules.code is UNIQUE, so a plain
-- CreatePolicyRule would fail on re-seed. ON CONFLICT (code) DO UPDATE keeps
-- a single row per rule code and refreshes title/description/severity to
-- the current catalog values on every seed run.
INSERT INTO policy_rules (code, title, description, severity)
VALUES ($1, $2, $3, $4)
ON CONFLICT (code) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    severity = EXCLUDED.severity
RETURNING id, code, title, description, severity, created_at;

-- name: CreatePolicyBundle :one
INSERT INTO policy_bundles (tenant_id, name, version, status)
VALUES ($1, $2, $3, $4)
RETURNING id, tenant_id, name, version, status, created_at;

-- name: AddPolicyBundleRule :one
INSERT INTO policy_bundle_rules (tenant_id, policy_bundle_id, policy_rule_id, effective_date, legal_basis)
VALUES ($1, $2, $3, $4, $5)
RETURNING tenant_id, policy_bundle_id, policy_rule_id, created_at, effective_date, legal_basis;

-- name: ListPolicyBundleRulesByTenant :many
SELECT pbr.tenant_id, pbr.policy_bundle_id, pbr.policy_rule_id, pbr.created_at,
       pbr.effective_date, pbr.legal_basis,
       pr.code, pr.title, pr.description, pr.severity
FROM policy_bundle_rules pbr
JOIN policy_rules pr ON pr.id = pbr.policy_rule_id
WHERE pbr.tenant_id = $1
ORDER BY pr.code;

-- name: GetActiveBundleByTenant :one
-- Resolves THE active bundle for a tenant (Design Decision 3/4): today's
-- BundleResolver seam resolves per-tenant only, with no bundle "name" input.
-- If a tenant were to ever activate more than one named bundle
-- simultaneously this returns a SQL "too many rows" error rather than
-- silently picking one; that constraint is out of scope for issue #6.
SELECT id, tenant_id, name, version, status, created_at
FROM policy_bundles
WHERE tenant_id = $1 AND status = 'active';

-- name: CountBundleVersions :one
-- Scoped to (tenant_id, name): CreateBundleVersion numbers new versions per
-- bundle name, not globally per tenant.
SELECT count(*) FROM policy_bundles WHERE tenant_id = $1 AND name = $2;

-- name: GetPolicyBundleByID :one
-- ReEvaluateInteraction's historical-bundle validation (issue #6): scoped to
-- (id, tenant_id) so a foreign-tenant bundle id naturally resolves to
-- pgx.ErrNoRows — the same "not found" outcome as a truly unknown id, never
-- leaking whether the bundle exists for a different tenant.
SELECT id, tenant_id, name, version, status, created_at
FROM policy_bundles
WHERE id = $1 AND tenant_id = $2;

-- name: LockActivePolicyBundle :one
-- CreateBundleVersion's serialization point (Design Decision 6): locks the
-- prior active row scoped to (tenant_id, name) so two concurrent
-- CreateBundleVersion calls for the same bundle name never both supersede
-- and insert at once. Returns pgx.ErrNoRows when no prior active bundle
-- exists yet (the first version for this name) — the caller proceeds
-- without a row to supersede.
SELECT id, tenant_id, name, version, status, created_at
FROM policy_bundles
WHERE tenant_id = $1 AND name = $2 AND status = 'active'
FOR UPDATE;

-- name: SupersedePolicyBundle :exec
-- Status-only update along the allowed active->superseded transition (the
-- one carve-out policy_bundles_guard_mutation permits). MUST run before the
-- new active row is inserted: the partial unique index
-- policy_bundles_one_active_per_tenant_name is non-deferrable.
UPDATE policy_bundles SET status = 'superseded' WHERE id = $1 AND tenant_id = $2;
