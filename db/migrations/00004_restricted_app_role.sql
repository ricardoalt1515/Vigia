-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'vigia_app') THEN
        CREATE ROLE vigia_app LOGIN PASSWORD 'vigia_app';
    END IF;
END
$$;

ALTER ROLE vigia_app WITH
    LOGIN
    PASSWORD 'vigia_app'
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOINHERIT
    NOREPLICATION
    NOBYPASSRLS;

DO $$
BEGIN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO vigia_app', current_database());
END
$$;

GRANT USAGE ON SCHEMA public TO vigia_app;

GRANT SELECT ON tenant_api_keys TO vigia_app;
GRANT SELECT ON interaction_events TO vigia_app;
GRANT SELECT ON evaluations TO vigia_app;
GRANT SELECT ON detector_result_rows TO vigia_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
REVOKE SELECT ON detector_result_rows FROM vigia_app;
REVOKE SELECT ON evaluations FROM vigia_app;
REVOKE SELECT ON interaction_events FROM vigia_app;
REVOKE SELECT ON tenant_api_keys FROM vigia_app;
REVOKE USAGE ON SCHEMA public FROM vigia_app;

DO $$
BEGIN
    EXECUTE format('REVOKE CONNECT ON DATABASE %I FROM vigia_app', current_database());
END
$$;

DROP ROLE IF EXISTS vigia_app;
-- +goose StatementEnd
