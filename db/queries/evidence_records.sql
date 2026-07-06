-- name: InsertEvidenceRecord :one
INSERT INTO evidence_records (tenant_id, interaction_event_id, evaluation_id, seq,
    prev_hash, hash, overall_outcome, policy_bundle_version, inputs_digest, created_at,
    judge_rubric_version, judge_model_id, judge_confidence, record_kind)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'evaluation')
RETURNING id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at,
    judge_rubric_version, judge_model_id, judge_confidence, record_kind,
    complaint_case_id, transition_kind, transition_from_state, transition_to_state, human_review_id;

-- name: InsertComplaintTransitionEvidenceRecord :one
INSERT INTO evidence_records (tenant_id, interaction_event_id, evaluation_id, seq,
    prev_hash, hash, overall_outcome, policy_bundle_version, inputs_digest, created_at,
    record_kind, complaint_case_id, transition_kind, transition_from_state, transition_to_state,
    human_review_id)
VALUES ($1,NULL,NULL,$2,$3,$4,$5,'',$6,$7,'complaint_transition',$8,$9,$10,$11,$12)
RETURNING id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at,
    judge_rubric_version, judge_model_id, judge_confidence, record_kind,
    complaint_case_id, transition_kind, transition_from_state, transition_to_state, human_review_id;

-- name: ListEvidenceRecordsByTenant :many
-- Store-backed VerifyChain: replay a tenant's chain ordered by seq.
SELECT id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at,
    judge_rubric_version, judge_model_id, judge_confidence, record_kind,
    complaint_case_id, transition_kind, transition_from_state, transition_to_state, human_review_id
FROM evidence_records WHERE tenant_id = $1 ORDER BY seq ASC;

-- name: GetEvidenceRecordByInteraction :one
-- Export endpoint lookup.
SELECT id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at,
    judge_rubric_version, judge_model_id, judge_confidence, record_kind,
    complaint_case_id, transition_kind, transition_from_state, transition_to_state, human_review_id
FROM evidence_records WHERE tenant_id = $1 AND interaction_event_id = $2;

-- name: ListDetectorResultRowsByEvaluation :many
-- Package detector layer for evidence export, sorted by detector_code.
SELECT detector_code, outcome, severity, result_payload
FROM detector_result_rows WHERE evaluation_id = $1 ORDER BY detector_code ASC;
