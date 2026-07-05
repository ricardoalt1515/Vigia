-- name: CreateComplaintCase :one
INSERT INTO complaint_cases (
    tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, idempotency_key
) VALUES ($1, $2, $3, 'open', $4, $5, $6, $7)
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
RETURNING id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at;

-- name: GetComplaintCaseByIdempotencyKey :one
SELECT id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at
FROM complaint_cases
WHERE tenant_id = $1 AND idempotency_key = $2;

-- name: GetComplaintCase :one
SELECT id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at
FROM complaint_cases
WHERE tenant_id = $1 AND id = $2;

-- name: ListOpenComplaintCases :many
SELECT id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at
FROM complaint_cases
WHERE tenant_id = $1 AND state = 'open'
ORDER BY opened_at ASC
LIMIT $2;

-- name: ListSLADueComplaintCases :many
SELECT id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at
FROM complaint_cases
WHERE tenant_id = $1 AND state IN ('open', 'awaiting_review') AND sla_due_at <= $2
ORDER BY sla_due_at ASC
LIMIT $3;

-- name: ListExpiredReviewComplaintCases :many
SELECT id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at
FROM complaint_cases
WHERE tenant_id = $1 AND state = 'awaiting_review' AND review_expires_at <= $2
ORDER BY review_expires_at ASC
LIMIT $3;

-- name: TransitionComplaintCaseToAwaitingReview :one
UPDATE complaint_cases
SET state = 'awaiting_review', review_expires_at = $3, updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND state = 'open'
RETURNING id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at;

-- name: TransitionComplaintCaseToEscalated :one
UPDATE complaint_cases
SET state = 'escalated', updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND state = ANY(sqlc.arg(from_states)::text[])
RETURNING id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at;

-- name: TransitionComplaintCaseToResolved :one
UPDATE complaint_cases
SET state = 'resolved', resolved_at = $3, updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND state = 'awaiting_review' AND review_expires_at > now()
RETURNING id, tenant_id, interaction_id, redeco_cause, state, opened_at, sla_due_at,
    calendar_version, review_expires_at, resolved_at, idempotency_key, created_at, updated_at;
