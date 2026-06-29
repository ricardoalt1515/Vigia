-- name: CreateTenantAPIKey :one
INSERT INTO tenant_api_keys (tenant_id, key_hash, label, status, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, key_hash, label, status, created_at, expires_at, last_used_at;

-- name: ListTenantAPIKeysByTenant :many
SELECT id, tenant_id, key_hash, label, status, created_at, expires_at, last_used_at
FROM tenant_api_keys
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: GetTenantAPIKeyByHash :one
SELECT id, tenant_id, key_hash, label, status, created_at, expires_at, last_used_at
FROM tenant_api_keys
WHERE key_hash = $1;
