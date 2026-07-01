-- name: CreateEvaluation :one
INSERT INTO evaluations (tenant_id, interaction_event_id, overall_outcome)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, interaction_event_id, overall_outcome, policy_bundle_version, created_at;

-- name: CountOutOfHoursEvaluations :one
SELECT count(*) FROM evaluations WHERE overall_outcome = 'fail';
