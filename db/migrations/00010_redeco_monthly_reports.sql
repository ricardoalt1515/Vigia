-- +goose Up
-- +goose StatementBegin
CREATE TABLE despacho_penalizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    despacho_id uuid NOT NULL,
    complaint_case_id uuid NOT NULL,
    period_year integer NOT NULL,
    period_month integer NOT NULL,
    penalization text NOT NULL,
    resolution text NOT NULL,
    source_state text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, complaint_case_id, period_year, period_month),
    CONSTRAINT despacho_penalizations_period_month_check CHECK (period_month BETWEEN 1 AND 12),
    CONSTRAINT despacho_penalizations_penalization_check CHECK (penalization IN ('penalized', 'cleared', 'overridden')),
    FOREIGN KEY (despacho_id, tenant_id)
        REFERENCES despachos(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (complaint_case_id, tenant_id)
        REFERENCES complaint_cases(id, tenant_id) ON DELETE CASCADE
);

CREATE INDEX idx_despacho_penalizations_period
    ON despacho_penalizations (tenant_id, period_year, period_month, despacho_id);

ALTER TABLE despacho_penalizations ENABLE ROW LEVEL SECURITY;
CREATE POLICY despacho_penalizations_tenant_isolation ON despacho_penalizations
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON despacho_penalizations TO vigia_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
REVOKE SELECT, INSERT, UPDATE, DELETE ON despacho_penalizations FROM vigia_app;
DROP TABLE IF EXISTS despacho_penalizations;
-- +goose StatementEnd
