-- name: CreateDebtor :one
INSERT INTO debtors (tenant_id, external_ref, display_name)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, external_ref, display_name, created_at, updated_at;

-- name: GetDebtorByTenant :one
SELECT id, tenant_id, external_ref, display_name, created_at, updated_at
FROM debtors
WHERE tenant_id = $1 AND id = $2;

-- name: ListDebtorsByTenant :many
SELECT id, tenant_id, external_ref, display_name, created_at, updated_at
FROM debtors
WHERE tenant_id = $1
ORDER BY created_at DESC;
