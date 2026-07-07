-- +goose Up
-- +goose StatementBegin
ALTER TABLE interaction_transcripts
    ADD COLUMN provider text NOT NULL DEFAULT '',
    ADD COLUMN adapter text NOT NULL DEFAULT '',
    ADD COLUMN service text NOT NULL DEFAULT '',
    ADD COLUMN language_code text NOT NULL DEFAULT '',
    ADD COLUMN provider_job_id text NOT NULL DEFAULT '',
    ADD COLUMN provider_request_id text NOT NULL DEFAULT '',
    ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}'::jsonb;

GRANT INSERT ON interaction_transcripts TO vigia_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
REVOKE INSERT ON interaction_transcripts FROM vigia_app;
ALTER TABLE interaction_transcripts
    DROP COLUMN metadata,
    DROP COLUMN provider_request_id,
    DROP COLUMN provider_job_id,
    DROP COLUMN language_code,
    DROP COLUMN service,
    DROP COLUMN adapter,
    DROP COLUMN provider;
-- +goose StatementEnd
