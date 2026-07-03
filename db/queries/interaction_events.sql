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
-- Issue #4 rewrite: aggregates across detector_result_rows per evaluation
-- (worst-severity-wins for the displayed reason, bool_or threat_flagged for
-- MX-REDECO-05) instead of assuming at most one detector result per
-- evaluation. One row per interaction — the LATERAL subquery collapses
-- every detector_result_rows child before the join, so a second
-- detector/judge result never fans out the interaction row.
SELECT
    ie.id, ie.tenant_id, ie.debtor_id, ie.channel, ie.direction, ie.status,
    ie.occurred_at, ie.transcript_ref, ie.debtor_timezone, ie.created_at,
    e.overall_outcome,
    e.requires_hitl,
    CASE WHEN e.id IS NULL THEN NULL ELSE agg.threat_flagged END AS threat_flagged,
    agg.reason
FROM interaction_events ie
LEFT JOIN evaluations e ON e.interaction_event_id = ie.id
LEFT JOIN LATERAL (
    SELECT
        bool_or(dr.detector_code = 'MX-REDECO-05' AND dr.outcome IN ('fail', 'review'))
            AS threat_flagged,
        (array_agg(
            dr.result_payload ->> 'rationale'
            ORDER BY
                CASE dr.severity
                    WHEN 'critical' THEN 4 WHEN 'high' THEN 3
                    WHEN 'medium'  THEN 2 WHEN 'low'   THEN 1 ELSE 0 END DESC,
                dr.detector_code ASC
        ))[1] AS reason
    FROM detector_result_rows dr
    WHERE dr.evaluation_id = e.id
) agg ON true
ORDER BY ie.occurred_at DESC
LIMIT $1;
