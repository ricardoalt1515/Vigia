-- +goose Up
-- +goose StatementBegin
CREATE TABLE complaint_cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    interaction_id uuid NOT NULL,
    redeco_cause text NOT NULL,
    state text NOT NULL DEFAULT 'open',
    opened_at timestamptz NOT NULL,
    sla_due_at timestamptz NOT NULL,
    calendar_version text NOT NULL,
    review_expires_at timestamptz,
    resolved_at timestamptz,
    idempotency_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT complaint_cases_state_check CHECK (state IN ('open', 'awaiting_review', 'escalated', 'resolved')),
    CONSTRAINT complaint_cases_terminal_timestamp_check CHECK (
        (state = 'resolved' AND resolved_at IS NOT NULL)
        OR (state <> 'resolved' AND resolved_at IS NULL)
    ),
    FOREIGN KEY (interaction_id, tenant_id)
        REFERENCES interaction_events(id, tenant_id) ON DELETE CASCADE
);

CREATE INDEX idx_complaint_cases_tenant_state ON complaint_cases (tenant_id, state);
CREATE INDEX idx_complaint_cases_sla_due_at ON complaint_cases (sla_due_at) WHERE state IN ('open', 'awaiting_review');
CREATE INDEX idx_complaint_cases_review_expires_at ON complaint_cases (review_expires_at) WHERE state = 'awaiting_review';

ALTER TABLE complaint_cases ENABLE ROW LEVEL SECURITY;
CREATE POLICY complaint_cases_tenant_isolation ON complaint_cases
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE human_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    complaint_case_id uuid NOT NULL,
    decision text NOT NULL,
    reviewer text NOT NULL,
    notes text NOT NULL DEFAULT '',
    processed_at timestamptz,
    superseded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    CONSTRAINT human_reviews_decision_check CHECK (decision IN ('approve', 'override')),
    CONSTRAINT human_reviews_processed_or_superseded_check CHECK (
        processed_at IS NULL OR superseded_at IS NULL
    ),
    FOREIGN KEY (complaint_case_id, tenant_id)
        REFERENCES complaint_cases(id, tenant_id) ON DELETE CASCADE
);

CREATE INDEX idx_human_reviews_case_unprocessed
    ON human_reviews (tenant_id, complaint_case_id, created_at)
    WHERE processed_at IS NULL AND superseded_at IS NULL;

ALTER TABLE human_reviews ENABLE ROW LEVEL SECURITY;
CREATE POLICY human_reviews_tenant_isolation ON human_reviews
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- Jurisdiction-wide statutory calendar reference, not tenant data. Seed set is
-- pending counsel confirmation; see docs/regulatory-ruleset.md "Open legal
-- items to confirm with counsel". Ambiguous/unseeded dates are intentionally
-- treated by application code as business days so deadlines are never extended.
CREATE TABLE business_day_holidays (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    calendar_version text NOT NULL,
    holiday_date date NOT NULL,
    label text NOT NULL,
    source_note text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (calendar_version, holiday_date)
);

INSERT INTO business_day_holidays (calendar_version, holiday_date, label, source_note) VALUES
    ('mx-lft-art-74-2026a', DATE '2026-01-01', 'New Year''s Day', 'LFT Art. 74 statutory holiday; pending counsel confirmation'),
    ('mx-lft-art-74-2026a', DATE '2026-02-02', 'Constitution Day observed', 'LFT Art. 74 statutory holiday; pending counsel confirmation'),
    ('mx-lft-art-74-2026a', DATE '2026-03-16', 'Benito Juarez Day observed', 'LFT Art. 74 statutory holiday; pending counsel confirmation'),
    ('mx-lft-art-74-2026a', DATE '2026-05-01', 'Labor Day', 'LFT Art. 74 statutory holiday; pending counsel confirmation'),
    ('mx-lft-art-74-2026a', DATE '2026-09-16', 'Independence Day', 'LFT Art. 74 statutory holiday; pending counsel confirmation'),
    ('mx-lft-art-74-2026a', DATE '2026-11-16', 'Revolution Day observed', 'LFT Art. 74 statutory holiday; pending counsel confirmation'),
    ('mx-lft-art-74-2026a', DATE '2026-12-25', 'Christmas Day', 'LFT Art. 74 statutory holiday; pending counsel confirmation');

GRANT SELECT, INSERT, UPDATE ON complaint_cases TO vigia_app;
GRANT SELECT, INSERT, UPDATE ON human_reviews TO vigia_app;
GRANT SELECT ON business_day_holidays TO vigia_app;
GRANT SELECT, INSERT ON evidence_records TO vigia_app;
GRANT SELECT, INSERT, UPDATE ON ledger_chain_heads TO vigia_app;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE evidence_records ADD COLUMN record_kind text NOT NULL DEFAULT 'evaluation';
ALTER TABLE evidence_records ADD COLUMN complaint_case_id uuid;
ALTER TABLE evidence_records ADD COLUMN transition_kind text;
ALTER TABLE evidence_records ADD COLUMN transition_from_state text;
ALTER TABLE evidence_records ADD COLUMN transition_to_state text;
ALTER TABLE evidence_records ADD COLUMN human_review_id uuid;
ALTER TABLE evidence_records ALTER COLUMN interaction_event_id DROP NOT NULL;
ALTER TABLE evidence_records ALTER COLUMN evaluation_id DROP NOT NULL;
ALTER TABLE evidence_records DROP CONSTRAINT IF EXISTS evidence_records_tenant_id_evaluation_id_key;
ALTER TABLE evidence_records
    ADD CONSTRAINT evidence_records_record_kind_check
    CHECK (record_kind IN ('evaluation', 'complaint_transition'));
ALTER TABLE evidence_records
    ADD CONSTRAINT evidence_records_complaint_case_id_fkey
    FOREIGN KEY (complaint_case_id, tenant_id)
    REFERENCES complaint_cases(id, tenant_id) ON DELETE CASCADE;
ALTER TABLE evidence_records
    ADD CONSTRAINT evidence_records_exactly_one_record_kind_check
    CHECK (
        (record_kind = 'evaluation'
            AND evaluation_id IS NOT NULL
            AND interaction_event_id IS NOT NULL
            AND complaint_case_id IS NULL
            AND transition_kind IS NULL)
        OR
        (record_kind = 'complaint_transition'
            AND complaint_case_id IS NOT NULL
            AND evaluation_id IS NULL
            AND interaction_event_id IS NULL
            AND transition_kind IS NOT NULL
            AND transition_from_state IS NOT NULL
            AND transition_to_state IS NOT NULL)
    );
CREATE UNIQUE INDEX evidence_records_one_evaluation_record
    ON evidence_records (tenant_id, evaluation_id)
    WHERE record_kind = 'evaluation';
CREATE UNIQUE INDEX evidence_records_one_complaint_transition_record
    ON evidence_records (tenant_id, complaint_case_id, transition_kind)
    WHERE record_kind = 'complaint_transition';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM evidence_records WHERE record_kind = 'complaint_transition') THEN
        RAISE EXCEPTION 'refusing to roll back complaint workflow while complaint_transition evidence exists'
            USING ERRCODE = 'restrict_violation';
    END IF;
END;
$$;
DROP INDEX IF EXISTS evidence_records_one_complaint_transition_record;
DROP INDEX IF EXISTS evidence_records_one_evaluation_record;
ALTER TABLE evidence_records DROP CONSTRAINT IF EXISTS evidence_records_exactly_one_record_kind_check;
ALTER TABLE evidence_records DROP CONSTRAINT IF EXISTS evidence_records_complaint_case_id_fkey;
ALTER TABLE evidence_records DROP CONSTRAINT IF EXISTS evidence_records_record_kind_check;
ALTER TABLE evidence_records ALTER COLUMN evaluation_id SET NOT NULL;
ALTER TABLE evidence_records ALTER COLUMN interaction_event_id SET NOT NULL;
ALTER TABLE evidence_records ADD CONSTRAINT evidence_records_tenant_id_evaluation_id_key UNIQUE (tenant_id, evaluation_id);
ALTER TABLE evidence_records DROP COLUMN IF EXISTS human_review_id;
ALTER TABLE evidence_records DROP COLUMN IF EXISTS transition_to_state;
ALTER TABLE evidence_records DROP COLUMN IF EXISTS transition_from_state;
ALTER TABLE evidence_records DROP COLUMN IF EXISTS transition_kind;
ALTER TABLE evidence_records DROP COLUMN IF EXISTS complaint_case_id;
ALTER TABLE evidence_records DROP COLUMN IF EXISTS record_kind;
REVOKE INSERT, UPDATE ON ledger_chain_heads FROM vigia_app;
REVOKE INSERT ON evidence_records FROM vigia_app;
REVOKE SELECT ON business_day_holidays FROM vigia_app;
REVOKE SELECT, INSERT, UPDATE ON human_reviews FROM vigia_app;
REVOKE SELECT, INSERT, UPDATE ON complaint_cases FROM vigia_app;
DROP TABLE IF EXISTS business_day_holidays;
DROP TABLE IF EXISTS human_reviews;
DROP TABLE IF EXISTS complaint_cases;
-- +goose StatementEnd
