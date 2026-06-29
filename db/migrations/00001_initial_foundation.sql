-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenant_api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key_hash text NOT NULL UNIQUE,
    label text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    last_used_at timestamptz
);

CREATE TABLE debtors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    external_ref text NOT NULL,
    display_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, external_ref)
);

CREATE TABLE interaction_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    debtor_id uuid NOT NULL,
    channel text NOT NULL,
    direction text NOT NULL,
    status text NOT NULL DEFAULT 'recorded',
    occurred_at timestamptz NOT NULL,
    transcript_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    FOREIGN KEY (debtor_id, tenant_id)
        REFERENCES debtors(id, tenant_id) ON DELETE CASCADE
);

CREATE TABLE policy_rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    title text NOT NULL,
    description text NOT NULL DEFAULT '',
    severity text NOT NULL DEFAULT 'medium',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE policy_bundles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    version text NOT NULL,
    status text NOT NULL DEFAULT 'draft',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, name, version)
);

CREATE TABLE policy_bundle_rules (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    policy_bundle_id uuid NOT NULL,
    policy_rule_id uuid NOT NULL REFERENCES policy_rules(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (policy_bundle_id, policy_rule_id),
    FOREIGN KEY (policy_bundle_id, tenant_id)
        REFERENCES policy_bundles(id, tenant_id) ON DELETE CASCADE
);

CREATE TABLE detector_result_rows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    interaction_event_id uuid NOT NULL,
    detector_code text NOT NULL,
    outcome text NOT NULL,
    severity text NOT NULL,
    result_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (interaction_event_id, tenant_id)
        REFERENCES interaction_events(id, tenant_id) ON DELETE CASCADE
);

ALTER TABLE tenant_api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE debtors ENABLE ROW LEVEL SECURITY;
ALTER TABLE interaction_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_bundles ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_bundle_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE detector_result_rows ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_api_keys_tenant_isolation ON tenant_api_keys
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY debtors_tenant_isolation ON debtors
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY interaction_events_tenant_isolation ON interaction_events
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY policy_bundles_tenant_isolation ON policy_bundles
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY policy_bundle_rules_tenant_isolation ON policy_bundle_rules
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY detector_result_rows_tenant_isolation ON detector_result_rows
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS detector_result_rows;
DROP TABLE IF EXISTS policy_bundle_rules;
DROP TABLE IF EXISTS policy_bundles;
DROP TABLE IF EXISTS policy_rules;
DROP TABLE IF EXISTS interaction_events;
DROP TABLE IF EXISTS debtors;
DROP TABLE IF EXISTS tenant_api_keys;
DROP TABLE IF EXISTS tenants;
DROP EXTENSION IF EXISTS pgcrypto;
-- +goose StatementEnd
