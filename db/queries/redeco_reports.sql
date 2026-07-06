-- name: ListRedecoMonthlyReportEntries :many
WITH latest_review AS (
    SELECT DISTINCT ON (hr.tenant_id, hr.complaint_case_id)
        hr.tenant_id,
        hr.complaint_case_id,
        hr.decision
    FROM human_reviews hr
    WHERE hr.processed_at IS NOT NULL
      AND hr.superseded_at IS NULL
    ORDER BY hr.tenant_id, hr.complaint_case_id, hr.processed_at DESC, hr.created_at DESC
)
SELECT
    cc.id AS complaint_case_id,
    cc.interaction_id,
    ie.despacho_id,
    COALESCE(d.display_name, 'unattributed')::text AS despacho_name,
    ie.channel,
    cc.redeco_cause AS cause,
    cc.state AS status,
    CASE
        WHEN cc.state = 'escalated' THEN 'escalated'
        WHEN latest_review.decision = 'approve' THEN 'approved'
        WHEN latest_review.decision = 'override' THEN 'overridden'
        ELSE 'resolved'
    END::text AS resolution,
    CASE
        WHEN cc.state = 'escalated' THEN 'penalized'
        WHEN latest_review.decision = 'override' THEN 'overridden'
        ELSE 'cleared'
    END::text AS penalization,
    ie.occurred_at,
    COALESCE(cc.resolved_at, cc.updated_at) AS closed_at
FROM complaint_cases cc
JOIN interaction_events ie ON ie.id = cc.interaction_id AND ie.tenant_id = cc.tenant_id
LEFT JOIN despachos d ON d.id = ie.despacho_id AND d.tenant_id = cc.tenant_id
LEFT JOIN latest_review ON latest_review.tenant_id = cc.tenant_id AND latest_review.complaint_case_id = cc.id
WHERE cc.tenant_id = $1
  AND cc.state IN ('resolved', 'escalated')
  AND COALESCE(cc.resolved_at, cc.updated_at) >= $2
  AND COALESCE(cc.resolved_at, cc.updated_at) < $3
ORDER BY closed_at ASC, cc.id ASC;

-- name: DeleteDespachoPenalizationsForPeriod :exec
DELETE FROM despacho_penalizations
WHERE tenant_id = $1 AND period_year = $2 AND period_month = $3;

-- name: UpsertDespachoPenalization :one
INSERT INTO despacho_penalizations (
    tenant_id, despacho_id, complaint_case_id, period_year, period_month,
    penalization, resolution, source_state
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, complaint_case_id, period_year, period_month) DO UPDATE
SET despacho_id = EXCLUDED.despacho_id,
    penalization = EXCLUDED.penalization,
    resolution = EXCLUDED.resolution,
    source_state = EXCLUDED.source_state,
    updated_at = now()
RETURNING id, tenant_id, despacho_id, complaint_case_id, period_year, period_month,
    penalization, resolution, source_state, created_at, updated_at;

-- name: ListDespachoPenalizationsForPeriod :many
SELECT id, tenant_id, despacho_id, complaint_case_id, period_year, period_month,
    penalization, resolution, source_state, created_at, updated_at
FROM despacho_penalizations
WHERE tenant_id = $1 AND period_year = $2 AND period_month = $3
ORDER BY despacho_id ASC, complaint_case_id ASC;
