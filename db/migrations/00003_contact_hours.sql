-- +goose Up
-- +goose StatementBegin
-- Decision 2: debtors.timezone is required with no lingering default.
-- Add-nullable -> backfill -> SET NOT NULL keeps existing (#1) rows valid
-- without leaving a default that would mask a missing timezone on future
-- inserts.
ALTER TABLE debtors ADD COLUMN timezone text;
UPDATE debtors SET timezone = 'America/Mexico_City' WHERE timezone IS NULL;
ALTER TABLE debtors ALTER COLUMN timezone SET NOT NULL;

-- Snapshot of the debtor's timezone at ingest time. Empty string means
-- "unresolved" and is intentionally loud: the detector fails closed on it.
ALTER TABLE interaction_events ADD COLUMN debtor_timezone text NOT NULL DEFAULT '';

CREATE TABLE evaluations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    interaction_event_id uuid NOT NULL,
    overall_outcome text NOT NULL,
    policy_bundle_version text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    FOREIGN KEY (interaction_event_id, tenant_id)
        REFERENCES interaction_events(id, tenant_id) ON DELETE CASCADE
);

ALTER TABLE evaluations ENABLE ROW LEVEL SECURITY;
CREATE POLICY evaluations_tenant_isolation ON evaluations
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- Additive: pre-existing (#1) detector_result_rows keep evaluation_id NULL.
ALTER TABLE detector_result_rows ADD COLUMN evaluation_id uuid;
ALTER TABLE detector_result_rows
    ADD CONSTRAINT detector_result_rows_evaluation_id_fkey
    FOREIGN KEY (evaluation_id, tenant_id)
    REFERENCES evaluations(id, tenant_id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE detector_result_rows DROP CONSTRAINT IF EXISTS detector_result_rows_evaluation_id_fkey;
ALTER TABLE detector_result_rows DROP COLUMN IF EXISTS evaluation_id;
DROP TABLE IF EXISTS evaluations;
ALTER TABLE interaction_events DROP COLUMN IF EXISTS debtor_timezone;
ALTER TABLE debtors DROP COLUMN IF EXISTS timezone;
-- +goose StatementEnd
