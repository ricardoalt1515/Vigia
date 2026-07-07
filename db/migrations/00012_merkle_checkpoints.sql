-- +goose Up
-- +goose StatementBegin
ALTER TABLE evaluations ADD COLUMN judge_input_tokens bigint NOT NULL DEFAULT 0;
ALTER TABLE evaluations ADD COLUMN judge_output_tokens bigint NOT NULL DEFAULT 0;
ALTER TABLE evaluations ADD COLUMN judge_cache_read_input_tokens bigint NOT NULL DEFAULT 0;
ALTER TABLE evaluations ADD COLUMN judge_cache_creation_input_tokens bigint NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE merkle_checkpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    first_seq bigint NOT NULL,
    last_seq bigint NOT NULL,
    record_count bigint NOT NULL,
    root_hash text NOT NULL,
    chain_head_hash text NOT NULL,
    checkpoint_body bytea NOT NULL,
    rfc3161_token bytea NOT NULL,
    tsa_url text NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, first_seq, last_seq),
    CONSTRAINT merkle_checkpoints_range_check CHECK (first_seq > 0 AND last_seq >= first_seq),
    CONSTRAINT merkle_checkpoints_count_check CHECK (record_count = last_seq - first_seq + 1),
    CONSTRAINT merkle_checkpoints_root_hash_check CHECK (root_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT merkle_checkpoints_chain_head_hash_check CHECK (chain_head_hash ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_merkle_checkpoints_tenant_last_seq
    ON merkle_checkpoints (tenant_id, last_seq DESC);

ALTER TABLE merkle_checkpoints ENABLE ROW LEVEL SECURITY;
CREATE POLICY merkle_checkpoints_tenant_isolation ON merkle_checkpoints
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON merkle_checkpoints TO vigia_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
REVOKE SELECT, INSERT ON merkle_checkpoints FROM vigia_app;
DROP TABLE IF EXISTS merkle_checkpoints;
ALTER TABLE evaluations DROP COLUMN IF EXISTS judge_cache_creation_input_tokens;
ALTER TABLE evaluations DROP COLUMN IF EXISTS judge_cache_read_input_tokens;
ALTER TABLE evaluations DROP COLUMN IF EXISTS judge_output_tokens;
ALTER TABLE evaluations DROP COLUMN IF EXISTS judge_input_tokens;
-- +goose StatementEnd
