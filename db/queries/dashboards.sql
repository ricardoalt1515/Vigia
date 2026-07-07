-- name: DashboardByDespacho :many
-- Interaction-grain (never detector_result_rows row-grain) violation-rate
-- ranking, one evaluation per interaction_event enforced by evaluations'
-- UNIQUE(tenant_id, interaction_event_id). Tenant-scoped implicitly by RLS
-- (current_setting('app.tenant_id')), like CountOutOfHoursEvaluations --
-- no explicit tenant_id filter here. Unattributed interactions
-- (interaction_events.despacho_id IS NULL) are reported under an explicit
-- synthetic bucket (despacho_id NULL, despacho_name "unattributed") rather
-- than silently dropped or folded into a named despacho. "violations" counts
-- only rows with outcome = 'fail' (never != 'pass'), so 'review' (judge
-- uncertainty) and 'warn' (MX-REDECO-03 confirmed warn-level signal) rows
-- are excluded, matching the by-cause endpoint's predicate.
-- Wrapped in a derived table so the ORDER BY expression below can reference
-- "total"/"violations"/"despacho_name" as real output columns of `agg`:
-- referencing an output-list alias (rather than a bare matching identifier)
-- inside an arithmetic ORDER BY expression on the original query is resolved
-- against the FROM-list instead and fails with "column ... does not exist"
-- since no such source column exists.
SELECT despacho_id, despacho_name, total, violations
FROM (
    SELECT
        d.id AS despacho_id,
        COALESCE(d.display_name, 'unattributed') AS despacho_name,
        COUNT(DISTINCT e.interaction_event_id) AS total,
        COUNT(DISTINCT e.interaction_event_id) FILTER (
            WHERE EXISTS (
                SELECT 1 FROM detector_result_rows drr
                WHERE drr.interaction_event_id = e.interaction_event_id
                  AND drr.outcome = 'fail'
            )
        ) AS violations
    FROM evaluations e
    JOIN interaction_events ie ON ie.id = e.interaction_event_id
    LEFT JOIN despachos d ON d.id = ie.despacho_id
    GROUP BY d.id, d.display_name
) agg
ORDER BY
    (violations::numeric / NULLIF(total, 0)) DESC NULLS LAST,
    despacho_name ASC;

-- name: DashboardByCause :many
-- Per-REDECO-rule-code breakdown, tenant-scoped implicitly by RLS.
-- "violations" counts outcome = 'fail' rows only; "warnings" is a separate
-- count of outcome = 'warn' rows (non-zero in practice only for
-- MX-REDECO-03) so warn-level activity is visible without inflating
-- violations. Both are computed by the same SQL GROUP BY, never fetched and
-- counted in application code.
SELECT
    detector_code AS rule_code,
    COUNT(*) FILTER (WHERE outcome = 'fail') AS violations,
    COUNT(*) FILTER (WHERE outcome = 'warn') AS warnings
FROM detector_result_rows
GROUP BY detector_code
ORDER BY detector_code ASC;

-- name: DashboardCostQuality :one
-- Tenant-scoped GenAI cost/quality summary. Token counts come from the judge
-- response usage recorded on evaluations; quality comes from the judge row's
-- confidence plus the evaluation outcome/HITL folding.
SELECT
    COUNT(*) FILTER (WHERE judge_model_id <> '') AS judged_interactions,
    COALESCE(SUM(judge_input_tokens), 0)::bigint AS input_tokens,
    COALESCE(SUM(judge_output_tokens), 0)::bigint AS output_tokens,
    COALESCE(SUM(judge_cache_read_input_tokens), 0)::bigint AS cache_read_input_tokens,
    COALESCE(SUM(judge_cache_creation_input_tokens), 0)::bigint AS cache_creation_input_tokens,
    COUNT(*) FILTER (WHERE requires_hitl) AS hitl_required,
    COUNT(*) FILTER (WHERE overall_outcome = 'fail') AS failed_interactions,
    COALESCE(AVG(drr.confidence) FILTER (WHERE drr.confidence IS NOT NULL), 0)::float8 AS average_confidence
FROM evaluations e
LEFT JOIN detector_result_rows drr
  ON drr.evaluation_id = e.id
 AND drr.detector_code = 'MX-REDECO-05';
