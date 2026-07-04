-- +goose Up
-- +goose StatementBegin
-- (a) despachos: minimal identity table, mirrors debtors. 1 tenant : N
-- despachos (despacho-registry spec: cardinality + RLS scenarios).
CREATE TABLE despachos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    external_ref text NOT NULL,
    display_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, external_ref)
);

ALTER TABLE despachos ENABLE ROW LEVEL SECURITY;
CREATE POLICY despachos_tenant_isolation ON despachos
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- despachos is a new-to-the-app table (00004_restricted_app_role.sql
-- precedent): read-only SELECT for vigia_app; writes stay owner-role-only
-- (seed-only for this change, per the despacho-registry spec's non-goals).
GRANT SELECT ON despachos TO vigia_app;
-- +goose StatementEnd

-- +goose StatementBegin
-- (b) Nullable despacho attribution FK on interaction_events. Optional per
-- interaction; composite FK enforces tenant-consistency (an interaction
-- cannot reference another tenant's despacho).
ALTER TABLE interaction_events ADD COLUMN despacho_id uuid;
ALTER TABLE interaction_events
    ADD CONSTRAINT interaction_events_despacho_id_fkey
    FOREIGN KEY (despacho_id, tenant_id)
    REFERENCES despachos(id, tenant_id);
CREATE INDEX idx_interaction_events_despacho_id ON interaction_events (despacho_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- (c) Detector-input snapshot columns on interaction_events (mirrors the
-- debtor_timezone precedent from 00003_contact_hours.sql): all
-- nullable/additive; NULL/empty means "unresolved" and each detector fails
-- closed on it per its own logic (see the deterministic-detectors spec).
ALTER TABLE interaction_events ADD COLUMN contact_party_relationship text;
ALTER TABLE interaction_events ADD COLUMN contacted_party_dob date;
ALTER TABLE interaction_events ADD COLUMN authorized_channels text[];
ALTER TABLE interaction_events ADD COLUMN payment_recipient text;
ALTER TABLE interaction_events ADD COLUMN disclosure_provided boolean;
-- +goose StatementEnd

-- +goose StatementBegin
-- (d) debtors.date_of_birth: durable DOB source, snapshotted onto
-- interaction_events.contacted_party_dob at ingest (cmd/seed today; a
-- future HTTP ingest endpoint would need its own vigia_app grant). Nullable:
-- existing debtors predate this field and DOB is not yet mandatory to
-- collect.
ALTER TABLE debtors ADD COLUMN date_of_birth date;
-- +goose StatementEnd

-- +goose StatementBegin
-- (e) One-time backfill for the "contact-hours" -> "MX-REDECO-04" rename
-- (cmd/api/main.go and cmd/seed/main.go wire the new code going forward, in
-- a later PR of this change). detector_result_rows has no append-only
-- guard, so this in-place backfill is safe and prevents the by-cause
-- dashboard from showing a split contact-hours/MX-REDECO-04 bucket for
-- pre-migration rows. Assumption: this only matters for pre-production
-- data -- no production traffic predates this rename.
--
-- This backfill is intentionally ONE-WAY: Down does not reverse it (see the
-- Down section below for why).
UPDATE detector_result_rows SET detector_code = 'MX-REDECO-04' WHERE detector_code = 'contact-hours';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- (e) is intentionally NOT reversed here: the backfill is one-way. Once
-- cmd/api/main.go and cmd/seed/main.go are wired to MX-REDECO-04 (a later
-- PR of this change), rows with detector_code = 'MX-REDECO-04' can include
-- genuine post-rename rows that were never 'contact-hours'. A Down that
-- blindly rewrote detector_code = 'MX-REDECO-04' back to 'contact-hours'
-- would corrupt those genuine rows -- there is no reliable predicate to
-- distinguish "backfilled from contact-hours" from "inserted as
-- MX-REDECO-04" after the rename ships. Rolling back this migration
-- therefore leaves detector_code values untouched; this is acceptable
-- because this migration only matters for pre-production data (no
-- production traffic predates the rename).
ALTER TABLE debtors DROP COLUMN IF EXISTS date_of_birth;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE interaction_events DROP COLUMN IF EXISTS disclosure_provided;
ALTER TABLE interaction_events DROP COLUMN IF EXISTS payment_recipient;
ALTER TABLE interaction_events DROP COLUMN IF EXISTS authorized_channels;
ALTER TABLE interaction_events DROP COLUMN IF EXISTS contacted_party_dob;
ALTER TABLE interaction_events DROP COLUMN IF EXISTS contact_party_relationship;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_interaction_events_despacho_id;
ALTER TABLE interaction_events DROP CONSTRAINT IF EXISTS interaction_events_despacho_id_fkey;
ALTER TABLE interaction_events DROP COLUMN IF EXISTS despacho_id;
-- +goose StatementEnd

-- +goose StatementBegin
REVOKE SELECT ON despachos FROM vigia_app;
DROP TABLE IF EXISTS despachos;
-- +goose StatementEnd
