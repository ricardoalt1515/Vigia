-- name: CreateEvaluation :one
INSERT INTO evaluations (tenant_id, interaction_event_id, overall_outcome)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, interaction_event_id, overall_outcome, policy_bundle_version, created_at;

-- name: CountOutOfHoursEvaluations :one
SELECT count(*) FROM evaluations WHERE overall_outcome = 'fail';

-- name: GetEvaluationByInteractionEventID :one
-- Used by cmd/seed to detect whether a pre-existing (re-run) interaction
-- still needs to be backfilled with an evaluation.
SELECT id, tenant_id, interaction_event_id, overall_outcome, policy_bundle_version, created_at
FROM evaluations
WHERE tenant_id = $1 AND interaction_event_id = $2;
