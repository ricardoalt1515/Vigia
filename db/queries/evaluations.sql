-- name: CreateEvaluation :one
INSERT INTO evaluations (tenant_id, interaction_event_id, overall_outcome,
    requires_hitl, judge_model_id, rubric_version, policy_bundle_version, policy_bundle_id,
    judge_input_tokens, judge_output_tokens, judge_cache_read_input_tokens,
    judge_cache_creation_input_tokens)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, tenant_id, interaction_event_id, overall_outcome, policy_bundle_version,
    created_at, requires_hitl, judge_model_id, rubric_version, policy_bundle_id,
    judge_input_tokens, judge_output_tokens, judge_cache_read_input_tokens,
    judge_cache_creation_input_tokens;

-- name: CountOutOfHoursEvaluations :one
SELECT count(*) FROM evaluations WHERE overall_outcome = 'fail';

-- name: GetEvaluationByInteractionEventID :one
-- Used by cmd/seed to detect whether a pre-existing (re-run) interaction
-- still needs to be backfilled with an evaluation.
SELECT id, tenant_id, interaction_event_id, overall_outcome, policy_bundle_version,
    created_at, requires_hitl, judge_model_id, rubric_version, policy_bundle_id,
    judge_input_tokens, judge_output_tokens, judge_cache_read_input_tokens,
    judge_cache_creation_input_tokens
FROM evaluations
WHERE tenant_id = $1 AND interaction_event_id = $2;
