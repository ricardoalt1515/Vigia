-- name: CreateDespacho :one
INSERT INTO despachos (tenant_id, external_ref, display_name)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, external_ref, display_name, created_at, updated_at;

-- name: GetDespachoByTenant :one
SELECT id, tenant_id, external_ref, display_name, created_at, updated_at
FROM despachos
WHERE tenant_id = $1 AND id = $2;

-- name: ListDespachosByTenant :many
SELECT id, tenant_id, external_ref, display_name, created_at, updated_at
FROM despachos
WHERE tenant_id = $1
ORDER BY created_at DESC;
