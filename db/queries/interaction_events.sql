-- name: GetInteractionEventByID :one
-- Evidence export lookup (issue #3): fetch a single interaction scoped to
-- its tenant.
SELECT id, tenant_id, debtor_id, channel, direction, status, occurred_at, transcript_ref, debtor_timezone, created_at
FROM interaction_events
WHERE id = $1 AND tenant_id = $2;

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
-- Evaluation runs synchronously, once, at ingest time for this change (#2):
-- at most one evaluations row per interaction and at most one
-- detector_result_rows row per evaluation, so a plain LEFT JOIN (no
-- LATERAL/window function) is sufficient and keeps sqlc's nullability
-- inference accurate for overall_outcome/reason.
SELECT
    ie.id, ie.tenant_id, ie.debtor_id, ie.channel, ie.direction, ie.status,
    ie.occurred_at, ie.transcript_ref, ie.debtor_timezone, ie.created_at,
    e.overall_outcome,
    dr.result_payload ->> 'rationale' AS reason
FROM interaction_events ie
LEFT JOIN evaluations e ON e.interaction_event_id = ie.id
LEFT JOIN detector_result_rows dr ON dr.evaluation_id = e.id
ORDER BY ie.occurred_at DESC
LIMIT $1;
