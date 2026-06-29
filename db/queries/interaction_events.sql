-- name: CreateInteractionEvent :one
INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, created_at;

-- name: ListInteractionEventsByTenant :many
SELECT id, tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, created_at
FROM interaction_events
WHERE tenant_id = $1
ORDER BY occurred_at DESC;
