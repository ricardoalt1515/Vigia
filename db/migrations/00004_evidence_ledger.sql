-- +goose Up
-- +goose StatementBegin
CREATE TABLE evidence_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    interaction_event_id uuid NOT NULL,
    evaluation_id uuid NOT NULL,
    seq bigint NOT NULL,
    prev_hash text NOT NULL,
    hash text NOT NULL,
    overall_outcome text NOT NULL,
    policy_bundle_version text NOT NULL DEFAULT '',
    inputs_digest text NOT NULL,
    -- No DEFAULT: the ledger inserts the exact microsecond-truncated value it hashed.
    created_at timestamptz NOT NULL,
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, seq),            -- monotonic per tenant; backstops the head lock
    UNIQUE (tenant_id, evaluation_id),  -- exactly one record per evaluation
    FOREIGN KEY (interaction_event_id, tenant_id)
        REFERENCES interaction_events(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (evaluation_id, tenant_id)
        REFERENCES evaluations(id, tenant_id) ON DELETE CASCADE
);

-- Composite FKs do not auto-index. get-by-interaction leads with
-- interaction_event_id; UNIQUE(tenant_id, seq) already covers list-by-seq and
-- UNIQUE(tenant_id, evaluation_id) covers evaluation lookup.
CREATE INDEX idx_evidence_records_interaction_event_id
    ON evidence_records (interaction_event_id);

ALTER TABLE evidence_records ENABLE ROW LEVEL SECURITY;
CREATE POLICY evidence_records_tenant_isolation ON evidence_records
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE ledger_chain_heads (
    tenant_id uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    last_seq bigint NOT NULL,
    last_hash text NOT NULL
);
ALTER TABLE ledger_chain_heads ENABLE ROW LEVEL SECURITY;
CREATE POLICY ledger_chain_heads_tenant_isolation ON ledger_chain_heads
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION evidence_records_block_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'evidence_records is append-only: % is not permitted', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER evidence_records_no_update_delete
    BEFORE UPDATE OR DELETE ON evidence_records
    FOR EACH ROW EXECUTE FUNCTION evidence_records_block_mutation();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER evidence_records_no_truncate
    BEFORE TRUNCATE ON evidence_records
    FOR EACH STATEMENT EXECUTE FUNCTION evidence_records_block_mutation();
-- +goose StatementEnd

-- Default trigger firing (ENABLE ORIGIN) is bypassed by
-- `SET session_replication_role = replica` (used by logical replication /
-- restore tooling). ENABLE ALWAYS makes both triggers fire regardless of
-- session_replication_role, so the write-once/no-truncate guarantee holds
-- even under replica-mode sessions.
-- +goose StatementBegin
ALTER TABLE evidence_records ENABLE ALWAYS TRIGGER evidence_records_no_update_delete;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE evidence_records ENABLE ALWAYS TRIGGER evidence_records_no_truncate;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS evidence_records_no_truncate ON evidence_records;
DROP TRIGGER IF EXISTS evidence_records_no_update_delete ON evidence_records;
DROP FUNCTION IF EXISTS evidence_records_block_mutation();
DROP TABLE IF EXISTS evidence_records;
DROP TABLE IF EXISTS ledger_chain_heads;
-- +goose StatementEnd
