-- name: CreateInteractionEvent :one
INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone, created_at;

-- name: ListInteractionEventsByTenant :many
SELECT id, tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone, created_at
FROM interaction_events
WHERE tenant_id = $1
ORDER BY occurred_at DESC;

-- name: ListCurrentTenantInteractions :many
SELECT id, tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone, created_at
FROM interaction_events
ORDER BY occurred_at DESC
LIMIT $1;

-- name: ListCurrentTenantInteractionsWithOutcome :many
SELECT
    ie.id, ie.tenant_id, ie.debtor_id, ie.channel, ie.direction, ie.status,
    ie.occurred_at, ie.transcript_ref, ie.debtor_timezone, ie.created_at,
    e.overall_outcome
FROM interaction_events ie
LEFT JOIN LATERAL (
    SELECT overall_outcome
    FROM evaluations
    WHERE evaluations.interaction_event_id = ie.id
    ORDER BY created_at DESC
    LIMIT 1
) e ON true
ORDER BY ie.occurred_at DESC
LIMIT $1;
