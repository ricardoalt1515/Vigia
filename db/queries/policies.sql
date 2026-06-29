-- name: CreatePolicyRule :one
INSERT INTO policy_rules (code, title, description, severity)
VALUES ($1, $2, $3, $4)
RETURNING id, code, title, description, severity, created_at;

-- name: CreatePolicyBundle :one
INSERT INTO policy_bundles (tenant_id, name, version, status)
VALUES ($1, $2, $3, $4)
RETURNING id, tenant_id, name, version, status, created_at;

-- name: AddPolicyBundleRule :one
INSERT INTO policy_bundle_rules (tenant_id, policy_bundle_id, policy_rule_id)
VALUES ($1, $2, $3)
RETURNING tenant_id, policy_bundle_id, policy_rule_id, created_at;

-- name: ListPolicyBundleRulesByTenant :many
SELECT pbr.tenant_id, pbr.policy_bundle_id, pbr.policy_rule_id, pbr.created_at,
       pr.code, pr.title, pr.description, pr.severity
FROM policy_bundle_rules pbr
JOIN policy_rules pr ON pr.id = pbr.policy_rule_id
WHERE pbr.tenant_id = $1
ORDER BY pr.code;
