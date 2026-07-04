-- name: CreateDebtor :one
-- date_of_birth (issue #7) is the durable DOB source, snapshotted onto
-- interaction_events.contacted_party_dob at ingest time by the caller.
INSERT INTO debtors (tenant_id, external_ref, display_name, timezone, date_of_birth)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, external_ref, display_name, timezone, date_of_birth, created_at, updated_at;

-- name: GetDebtorByTenant :one
SELECT id, tenant_id, external_ref, display_name, timezone, date_of_birth, created_at, updated_at
FROM debtors
WHERE tenant_id = $1 AND id = $2;

-- name: ListDebtorsByTenant :many
SELECT id, tenant_id, external_ref, display_name, timezone, date_of_birth, created_at, updated_at
FROM debtors
WHERE tenant_id = $1
ORDER BY created_at DESC;
