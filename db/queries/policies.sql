-- name: CreatePolicyRule :one
INSERT INTO policy_rules (code, title, description, severity)
VALUES ($1, $2, $3, $4)
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
