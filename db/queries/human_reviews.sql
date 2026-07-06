-- name: InsertHumanReview :one
INSERT INTO human_reviews (tenant_id, complaint_case_id, decision, reviewer, notes)
SELECT $1, $2, $3, $4, $5
WHERE EXISTS (
    SELECT 1 FROM complaint_cases
    WHERE tenant_id = $1 AND id = $2 AND state = 'awaiting_review'
)
RETURNING id, tenant_id, complaint_case_id, decision, reviewer, notes,
    processed_at, superseded_at, created_at, updated_at;

-- name: ListUnprocessedHumanReviews :many
SELECT hr.id, hr.tenant_id, hr.complaint_case_id, hr.decision, hr.reviewer, hr.notes,
    hr.processed_at, hr.superseded_at, hr.created_at, hr.updated_at
FROM human_reviews hr
JOIN complaint_cases cc ON cc.id = hr.complaint_case_id AND cc.tenant_id = hr.tenant_id
WHERE hr.tenant_id = $1
  AND cc.state = 'awaiting_review'
  AND hr.processed_at IS NULL
  AND hr.superseded_at IS NULL
ORDER BY hr.created_at ASC
LIMIT $2;

-- name: GetUnprocessedHumanReviewForCase :one
SELECT id, tenant_id, complaint_case_id, decision, reviewer, notes,
    processed_at, superseded_at, created_at, updated_at
FROM human_reviews
WHERE tenant_id = $1
  AND complaint_case_id = $2
  AND id = $3
  AND decision = $4
  AND processed_at IS NULL
  AND superseded_at IS NULL;

-- name: MarkWinningHumanReviewProcessed :one
UPDATE human_reviews
SET processed_at = $4, updated_at = now()
WHERE tenant_id = $1
  AND complaint_case_id = $2
  AND id = $3
  AND processed_at IS NULL
  AND superseded_at IS NULL
RETURNING id, tenant_id, complaint_case_id, decision, reviewer, notes,
    processed_at, superseded_at, created_at, updated_at;

-- name: MarkOtherHumanReviewsSuperseded :execrows
UPDATE human_reviews
SET superseded_at = $4, updated_at = now()
WHERE tenant_id = $1
  AND complaint_case_id = $2
  AND id <> $3
  AND processed_at IS NULL
  AND superseded_at IS NULL;
