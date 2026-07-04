-- +goose Up
-- +goose StatementBegin
-- (a) Rule-snapshot provenance columns. effective_date starts nullable so
-- the backfill (step b) can populate it before the NOT NULL constraint
-- (step c) locks it in. legal_basis defaults to '' so pre-#6 semantics
-- (no legal basis recorded) are preserved for any row a NOT NULL DEFAULT
-- would otherwise reject.
ALTER TABLE policy_bundle_rules ADD COLUMN effective_date date;
ALTER TABLE policy_bundle_rules ADD COLUMN legal_basis text NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
-- (b) Backfill BEFORE the append-only guard triggers exist below: this
-- UPDATE must run while the table is still unguarded, or it would violate
-- its own future guard.
UPDATE policy_bundle_rules SET effective_date = created_at::date;
-- +goose StatementEnd

-- +goose StatementBegin
-- (c) Lock the backfilled column down to NOT NULL.
ALTER TABLE policy_bundle_rules ALTER COLUMN effective_date SET NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
-- (d) Composite FK stamping the resolved bundle onto every evaluation.
-- Nullable: pre-#6 rows and no-active-bundle evaluations keep NULL
-- (Decision 3's sentinel path).
ALTER TABLE evaluations ADD COLUMN policy_bundle_id uuid;
ALTER TABLE evaluations
    ADD CONSTRAINT evaluations_policy_bundle_id_fkey
    FOREIGN KEY (policy_bundle_id, tenant_id)
    REFERENCES policy_bundles(id, tenant_id);
CREATE INDEX idx_evaluations_policy_bundle_id ON evaluations (policy_bundle_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- (e) Schema-level lifecycle guard: status can only ever be one of these
-- three values.
ALTER TABLE policy_bundles ADD CONSTRAINT policy_bundles_status_check
    CHECK (status IN ('draft', 'active', 'superseded'));
-- +goose StatementEnd

-- +goose StatementBegin
-- (f) At most one active bundle per tenant+name, enforced independent of
-- trigger/lock logic (belt-and-suspenders alongside the FOR UPDATE lock in
-- CreateBundleVersion). Non-deferrable: Postgres partial unique indexes
-- cannot be DEFERRABLE, which is why CreateBundleVersion must supersede the
-- prior active row BEFORE inserting the new one.
CREATE UNIQUE INDEX policy_bundles_one_active_per_tenant_name
    ON policy_bundles (tenant_id, name) WHERE status = 'active';
-- +goose StatementEnd

-- +goose StatementBegin
-- (g) policy_bundles carve-out guard: blocks DELETE always, blocks UPDATE
-- unless it touches ONLY status along an allowed draft->active->superseded
-- transition. Created AFTER the backfill above so nothing already-existing
-- trips it.
CREATE FUNCTION policy_bundles_guard_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP <> 'UPDATE' THEN
    RAISE EXCEPTION 'policy_bundles is append-only: % not permitted', TG_OP USING ERRCODE='restrict_violation';
  END IF;
  IF ROW(NEW.id,NEW.tenant_id,NEW.name,NEW.version,NEW.created_at)
     IS DISTINCT FROM ROW(OLD.id,OLD.tenant_id,OLD.name,OLD.version,OLD.created_at) THEN
    RAISE EXCEPTION 'policy_bundles: only status may change' USING ERRCODE='restrict_violation';
  END IF;
  IF NOT (OLD.status=NEW.status
          OR (OLD.status='draft'  AND NEW.status='active')
          OR (OLD.status='active' AND NEW.status='superseded')) THEN
    RAISE EXCEPTION 'policy_bundles: illegal status transition % -> %', OLD.status, NEW.status USING ERRCODE='restrict_violation';
  END IF;
  RETURN NEW;
END; $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER policy_bundles_guard_update_delete
    BEFORE UPDATE OR DELETE ON policy_bundles
    FOR EACH ROW EXECUTE FUNCTION policy_bundles_guard_mutation();
-- +goose StatementEnd

-- +goose StatementBegin
-- A FOR EACH ROW trigger never fires on TRUNCATE, so TRUNCATE needs its own
-- FOR EACH STATEMENT trigger (mirrors evidence_records_no_truncate). The
-- same carve-out function is reused: its TG_OP <> 'UPDATE' branch already
-- raises on TRUNCATE without touching NEW/OLD.
CREATE TRIGGER policy_bundles_guard_truncate
    BEFORE TRUNCATE ON policy_bundles
    FOR EACH STATEMENT EXECUTE FUNCTION policy_bundles_guard_mutation();
-- +goose StatementEnd

-- +goose StatementBegin
-- ENABLE ALWAYS: the guard fires regardless of session_replication_role, so
-- the append-only guarantee holds even under replica-mode sessions (see
-- 00001_initial_foundation.sql's evidence_records precedent).
ALTER TABLE policy_bundles ENABLE ALWAYS TRIGGER policy_bundles_guard_update_delete;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE policy_bundles ENABLE ALWAYS TRIGGER policy_bundles_guard_truncate;
-- +goose StatementEnd

-- +goose StatementBegin
-- policy_bundle_rules is fully append-only (no carve-out): every rule
-- inclusion is content-identity, so any UPDATE/DELETE/TRUNCATE is blocked
-- outright. Reuses the existing evidence_records_block_mutation() function
-- (00001_initial_foundation.sql) verbatim — it ignores NEW/OLD and always
-- raises, so it is safely reusable across tables and across both trigger
-- shapes (row-level and statement-level).
CREATE TRIGGER policy_bundle_rules_no_update_delete
    BEFORE UPDATE OR DELETE ON policy_bundle_rules
    FOR EACH ROW EXECUTE FUNCTION evidence_records_block_mutation();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER policy_bundle_rules_no_truncate
    BEFORE TRUNCATE ON policy_bundle_rules
    FOR EACH STATEMENT EXECUTE FUNCTION evidence_records_block_mutation();
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE policy_bundle_rules ENABLE ALWAYS TRIGGER policy_bundle_rules_no_update_delete;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE policy_bundle_rules ENABLE ALWAYS TRIGGER policy_bundle_rules_no_truncate;
-- +goose StatementEnd

-- +goose StatementBegin
-- (h) policy_bundles/policy_bundle_rules are new-to-the-app tables for
-- vigia_app (00004_restricted_app_role.sql precedent): read-only SELECT,
-- writes stay owner-role-only (CreateBundleVersion runs through the owner
-- pool, mirroring cmd/seed).
GRANT SELECT ON policy_bundles, policy_bundle_rules TO vigia_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
REVOKE SELECT ON policy_bundles, policy_bundle_rules FROM vigia_app;
DROP TRIGGER IF EXISTS policy_bundle_rules_no_truncate ON policy_bundle_rules;
DROP TRIGGER IF EXISTS policy_bundle_rules_no_update_delete ON policy_bundle_rules;
DROP TRIGGER IF EXISTS policy_bundles_guard_truncate ON policy_bundles;
DROP TRIGGER IF EXISTS policy_bundles_guard_update_delete ON policy_bundles;
DROP FUNCTION IF EXISTS policy_bundles_guard_mutation();
DROP INDEX IF EXISTS policy_bundles_one_active_per_tenant_name;
ALTER TABLE policy_bundles DROP CONSTRAINT IF EXISTS policy_bundles_status_check;
DROP INDEX IF EXISTS idx_evaluations_policy_bundle_id;
ALTER TABLE evaluations DROP CONSTRAINT IF EXISTS evaluations_policy_bundle_id_fkey;
ALTER TABLE evaluations DROP COLUMN IF EXISTS policy_bundle_id;
ALTER TABLE policy_bundle_rules ALTER COLUMN effective_date DROP NOT NULL;
ALTER TABLE policy_bundle_rules DROP COLUMN IF EXISTS legal_basis;
ALTER TABLE policy_bundle_rules DROP COLUMN IF EXISTS effective_date;
-- +goose StatementEnd
