-- +goose Up
-- +goose StatementBegin
-- Transcript content the judge reads (synthetic now; #11 STT writes here later).
CREATE TABLE interaction_transcripts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    interaction_event_id uuid NOT NULL,
    -- [{ "speaker": "...", "text": "..." }, ...] in transcript order.
    utterances jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, interaction_event_id),   -- at most one transcript per interaction
    FOREIGN KEY (interaction_event_id, tenant_id)
        REFERENCES interaction_events(id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX idx_interaction_transcripts_interaction_event_id
    ON interaction_transcripts (interaction_event_id);
ALTER TABLE interaction_transcripts ENABLE ROW LEVEL SECURITY;
CREATE POLICY interaction_transcripts_tenant_isolation ON interaction_transcripts
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT ON interaction_transcripts TO vigia_app;

-- Additive evaluation header fields (all backward-compatible for #2/#3 rows).
ALTER TABLE evaluations ADD COLUMN requires_hitl boolean NOT NULL DEFAULT false;
ALTER TABLE evaluations ADD COLUMN judge_model_id text NOT NULL DEFAULT '';
ALTER TABLE evaluations ADD COLUMN rubric_version text NOT NULL DEFAULT '';

-- Judge per-rule row records its confidence/score; nullable so detector rows
-- (and all pre-#4 rows) stay valid.
ALTER TABLE detector_result_rows ADD COLUMN confidence numeric(5,4);
ALTER TABLE detector_result_rows ADD COLUMN score numeric(5,4);

-- Judge sub-object of the hashed evidence Body. Nullable, added together: all
-- three NULL for judge-less records (byte-identical to pre-#4 bodies), all three
-- present for judged records. judge_confidence is stored as TEXT holding the
-- already-quantized 4-decimal string (e.g. '0.9500'), NOT numeric — see design.
ALTER TABLE evidence_records ADD COLUMN judge_rubric_version text;
ALTER TABLE evidence_records ADD COLUMN judge_model_id text;
ALTER TABLE evidence_records ADD COLUMN judge_confidence text;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE evidence_records DROP COLUMN IF EXISTS judge_confidence;
ALTER TABLE evidence_records DROP COLUMN IF EXISTS judge_model_id;
ALTER TABLE evidence_records DROP COLUMN IF EXISTS judge_rubric_version;
ALTER TABLE detector_result_rows DROP COLUMN IF EXISTS score;
ALTER TABLE detector_result_rows DROP COLUMN IF EXISTS confidence;
ALTER TABLE evaluations DROP COLUMN IF EXISTS rubric_version;
ALTER TABLE evaluations DROP COLUMN IF EXISTS judge_model_id;
ALTER TABLE evaluations DROP COLUMN IF EXISTS requires_hitl;
REVOKE SELECT ON interaction_transcripts FROM vigia_app;
DROP TABLE IF EXISTS interaction_transcripts;
-- +goose StatementEnd
