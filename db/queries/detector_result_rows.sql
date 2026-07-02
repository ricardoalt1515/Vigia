-- name: CreateDetectorResultRow :one
INSERT INTO detector_result_rows (tenant_id, interaction_event_id, detector_code, outcome, severity, result_payload, evaluation_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, tenant_id, interaction_event_id, detector_code, outcome, severity, result_payload, evaluation_id, created_at;

-- name: ListDetectorResultRowsByTenant :many
SELECT id, tenant_id, interaction_event_id, detector_code, outcome, severity, result_payload, evaluation_id, created_at
FROM detector_result_rows
WHERE tenant_id = $1
ORDER BY created_at DESC;
