-- name: CreateTenant :one
INSERT INTO tenants (slug, name, status)
VALUES ($1, $2, $3)
RETURNING id, slug, name, status, created_at, updated_at;

-- name: GetTenantBySlug :one
SELECT id, slug, name, status, created_at, updated_at
FROM tenants
WHERE slug = $1;

-- name: ListActiveTenants :many
SELECT id, slug, name, status, created_at, updated_at
FROM tenants
WHERE status = 'active'
ORDER BY created_at ASC, id ASC;
