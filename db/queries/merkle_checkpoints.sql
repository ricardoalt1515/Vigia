-- name: InsertMerkleCheckpoint :one
INSERT INTO merkle_checkpoints (tenant_id, first_seq, last_seq, record_count,
    root_hash, chain_head_hash, checkpoint_body, rfc3161_token, tsa_url, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, tenant_id, first_seq, last_seq, record_count, root_hash,
    chain_head_hash, checkpoint_body, rfc3161_token, tsa_url, created_at;

-- name: LatestMerkleCheckpoint :one
SELECT id, tenant_id, first_seq, last_seq, record_count, root_hash,
    chain_head_hash, checkpoint_body, rfc3161_token, tsa_url, created_at
FROM merkle_checkpoints
WHERE tenant_id = $1
ORDER BY last_seq DESC
LIMIT 1;

-- name: ListEvidenceRecordsByTenantAfterSeq :many
SELECT id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at,
    judge_rubric_version, judge_model_id, judge_confidence, record_kind,
    complaint_case_id, transition_kind, transition_from_state,
    transition_to_state, human_review_id
FROM evidence_records
WHERE tenant_id = $1 AND seq > $2
ORDER BY seq ASC;

-- name: ListEvidenceRecordsByTenantSeqRange :many
SELECT id, tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
    overall_outcome, policy_bundle_version, inputs_digest, created_at,
    judge_rubric_version, judge_model_id, judge_confidence, record_kind,
    complaint_case_id, transition_kind, transition_from_state,
    transition_to_state, human_review_id
FROM evidence_records
WHERE tenant_id = $1 AND seq >= $2 AND seq <= $3
ORDER BY seq ASC;

-- name: ListMerkleCheckpointsByTenant :many
SELECT id, tenant_id, first_seq, last_seq, record_count, root_hash,
    chain_head_hash, checkpoint_body, rfc3161_token, tsa_url, created_at
FROM merkle_checkpoints
WHERE tenant_id = $1
ORDER BY last_seq ASC;
