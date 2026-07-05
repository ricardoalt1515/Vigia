package db_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var tenantScopedTables = []string{
	"tenant_api_keys",
	"debtors",
	"interaction_events",
	"policy_bundles",
	"policy_bundle_rules",
	"detector_result_rows",
	"evidence_records",
	"ledger_chain_heads",
	"interaction_transcripts",
	"despachos",
}

func TestMigrationPreservesTenantScopedParentChildIntegrity(t *testing.T) {
	migration := readInitialMigration(t)

	relationships := []struct {
		name        string
		parentTable string
		childTable  string
		childFK     string
	}{
		{
			name:        "interaction events belong to same tenant as debtor",
			parentTable: "debtors",
			childTable:  "interaction_events",
			childFK:     "FOREIGN KEY (debtor_id, tenant_id)\n        REFERENCES debtors(id, tenant_id) ON DELETE CASCADE",
		},
		{
			name:        "detector result rows belong to same tenant as interaction event",
			parentTable: "interaction_events",
			childTable:  "detector_result_rows",
			childFK:     "FOREIGN KEY (interaction_event_id, tenant_id)\n        REFERENCES interaction_events(id, tenant_id) ON DELETE CASCADE",
		},
		{
			name:        "policy bundle rules belong to same tenant as policy bundle",
			parentTable: "policy_bundles",
			childTable:  "policy_bundle_rules",
			childFK:     "FOREIGN KEY (policy_bundle_id, tenant_id)\n        REFERENCES policy_bundles(id, tenant_id) ON DELETE CASCADE",
		},
	}

	for _, tt := range relationships {
		t.Run(tt.name, func(t *testing.T) {
			parentBlock := createTableBlock(t, migration, tt.parentTable)
			if !strings.Contains(parentBlock, "UNIQUE (id, tenant_id)") {
				t.Fatalf("%s does not declare a composite unique key for tenant-preserving references", tt.parentTable)
			}

			childBlock := createTableBlock(t, migration, tt.childTable)
			if !strings.Contains(childBlock, tt.childFK) {
				t.Fatalf("%s does not declare tenant-preserving foreign key %q", tt.childTable, tt.childFK)
			}
		})
	}
}

func TestTenantAPIKeyPoliciesSupportLookupAndTenantIsolation(t *testing.T) {
	migration := readInitialMigration(t)

	for _, required := range []string{
		"CREATE POLICY tenant_api_keys_tenant_isolation ON tenant_api_keys",
		"tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid",
		"CREATE POLICY tenant_api_keys_hash_lookup ON tenant_api_keys",
		"key_hash = nullif(current_setting('app.api_key_hash', true), '')",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("migration missing tenant_api_keys policy fragment %q", required)
		}
	}
}

func readInitialMigration(t *testing.T) string {
	t.Helper()
	migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "00001_initial_foundation.sql"))
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	return string(migrationSQL)
}

func createTableBlock(t *testing.T, migration, table string) string {
	t.Helper()

	startToken := "CREATE TABLE " + table + " ("
	start := strings.Index(migration, startToken)
	if start == -1 {
		t.Fatalf("migration does not declare table %s", table)
	}

	remainder := migration[start:]
	end := strings.Index(remainder, "\n);")
	if end == -1 {
		t.Fatalf("migration table %s does not have a closing statement", table)
	}

	return remainder[:end]
}

func TestTenantScopedTablesHaveTenantIDAndRLSEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL catalog check in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL catalog check")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, table := range tenantScopedTables {
		t.Run(table, func(t *testing.T) {
			var hasTenantID bool
			if err := db.QueryRowContext(ctx, `
				select exists (
					select 1
					from information_schema.columns
					where table_schema = 'public'
					  and table_name = $1
					  and column_name = 'tenant_id'
					  and udt_name = 'uuid'
					  and is_nullable = 'NO'
				)
			`, table).Scan(&hasTenantID); err != nil {
				t.Fatalf("query tenant_id metadata: %v", err)
			}
			if !hasTenantID {
				t.Fatalf("table %s does not have non-null uuid tenant_id", table)
			}

			var rlsEnabled bool
			if err := db.QueryRowContext(ctx, `
				select c.relrowsecurity
				from pg_class c
				join pg_namespace n on n.oid = c.relnamespace
				where n.nspname = 'public'
				  and c.relname = $1
				  and c.relkind = 'r'
			`, table).Scan(&rlsEnabled); err != nil {
				t.Fatalf("query RLS metadata: %v", err)
			}
			if !rlsEnabled {
				t.Fatalf("table %s does not have row-level security enabled", table)
			}
		})
	}
}

// TestMigration00006AddsNullableJudgeColumns covers *Migration adds nullable
// judge columns without breaking existing rows*: the additive
// evaluations/detector_result_rows/evidence_records columns from migration
// 00006 must exist with the documented nullability.
func TestMigration00006AddsNullableJudgeColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL catalog check in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL catalog check")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tests := []struct {
		table      string
		column     string
		isNullable string
	}{
		{table: "evaluations", column: "requires_hitl", isNullable: "NO"},
		{table: "evaluations", column: "judge_model_id", isNullable: "NO"},
		{table: "evaluations", column: "rubric_version", isNullable: "NO"},
		{table: "detector_result_rows", column: "confidence", isNullable: "YES"},
		{table: "detector_result_rows", column: "score", isNullable: "YES"},
		{table: "evidence_records", column: "judge_rubric_version", isNullable: "YES"},
		{table: "evidence_records", column: "judge_model_id", isNullable: "YES"},
		{table: "evidence_records", column: "judge_confidence", isNullable: "YES"},
	}

	for _, tt := range tests {
		t.Run(tt.table+"."+tt.column, func(t *testing.T) {
			var isNullable string
			err := db.QueryRowContext(ctx, `
				SELECT is_nullable FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
			`, tt.table, tt.column).Scan(&isNullable)
			if err != nil {
				t.Fatalf("query column metadata for %s.%s: %v", tt.table, tt.column, err)
			}
			if isNullable != tt.isNullable {
				t.Fatalf("%s.%s is_nullable = %q, want %q", tt.table, tt.column, isNullable, tt.isNullable)
			}
		})
	}
}

// TestMigration00006PreservesEvaluationsUniqueConstraint covers *UNIQUE
// (tenant_id, interaction_event_id) constraint is preserved*: a second
// evaluation insert for the same (tenant_id, interaction_event_id) pair
// must still fail after migration 00006 applies.
func TestMigration00006PreservesEvaluationsUniqueConstraint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL catalog check in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL catalog check")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	suffix := time.Now().Format("150405.000000000")
	var tenantID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO tenants (slug, name, status) VALUES ($1, $1, 'active') RETURNING id
	`, "mig6-unique-"+suffix).Scan(&tenantID); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	var debtorID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO debtors (tenant_id, external_ref, display_name, timezone)
		VALUES ($1, $2, $2, 'America/Mexico_City') RETURNING id
	`, tenantID, "mig6-unique-debtor-"+suffix).Scan(&debtorID); err != nil {
		t.Fatalf("create debtor: %v", err)
	}
	var interactionID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, debtor_timezone)
		VALUES ($1, $2, 'phone', 'outbound', 'recorded', now(), 'America/Mexico_City') RETURNING id
	`, tenantID, debtorID).Scan(&interactionID); err != nil {
		t.Fatalf("create interaction: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO evaluations (tenant_id, interaction_event_id, overall_outcome) VALUES ($1, $2, 'pass')
	`, tenantID, interactionID); err != nil {
		t.Fatalf("first evaluation insert: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO evaluations (tenant_id, interaction_event_id, overall_outcome) VALUES ($1, $2, 'fail')
	`, tenantID, interactionID)
	if err == nil {
		t.Fatal("expected second evaluation insert for the same (tenant_id, interaction_event_id) to fail")
	}
}

func TestRestrictedAppRoleIsLeastPrivilege(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL app role catalog check in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL app role catalog check")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var superuser, createDB, createRole, bypassRLS bool
	if err := db.QueryRowContext(ctx, `
		select rolsuper, rolcreatedb, rolcreaterole, rolbypassrls
		from pg_roles
		where rolname = 'vigia_app'
	`).Scan(&superuser, &createDB, &createRole, &bypassRLS); err != nil {
		t.Fatalf("query vigia_app role attributes: %v", err)
	}
	if superuser || createDB || createRole || bypassRLS {
		t.Fatalf("vigia_app privileges = superuser:%t createdb:%t createrole:%t bypassrls:%t, want all false", superuser, createDB, createRole, bypassRLS)
	}

	var ownedTables int
	if err := db.QueryRowContext(ctx, `
		select count(*)
		from pg_tables
		where schemaname = 'public'
		  and tableowner = 'vigia_app'
	`).Scan(&ownedTables); err != nil {
		t.Fatalf("query vigia_app table ownership: %v", err)
	}
	if ownedTables != 0 {
		t.Fatalf("vigia_app owns %d public tables, want 0", ownedTables)
	}

	for _, table := range []string{
		"tenant_api_keys",
		"interaction_events",
		"evaluations",
		"detector_result_rows",
		"evidence_records",
		"ledger_chain_heads",
		"interaction_transcripts",
		"policy_bundles",
		"policy_bundle_rules",
		"despachos",
	} {
		t.Run(table, func(t *testing.T) {
			var canSelect bool
			if err := db.QueryRowContext(ctx, `select has_table_privilege('vigia_app', 'public.' || $1, 'SELECT')`, table).Scan(&canSelect); err != nil {
				t.Fatalf("query SELECT grant: %v", err)
			}
			if !canSelect {
				t.Fatalf("vigia_app cannot SELECT from %s", table)
			}

			var canInsert bool
			if err := db.QueryRowContext(ctx, `select has_table_privilege('vigia_app', 'public.' || $1, 'INSERT')`, table).Scan(&canInsert); err != nil {
				t.Fatalf("query INSERT grant: %v", err)
			}
			if canInsert {
				t.Fatalf("vigia_app can INSERT into %s; current RLS tests only require SELECT", table)
			}
		})
	}
}

// TestMigration00007PolicyBundleVersioningCatalog covers issue #6's schema
// half: the CHECK constraint, the partial unique index, and all four
// append-only guard triggers (row-level UPDATE/DELETE + statement-level
// TRUNCATE per table) must exist and be ENABLE ALWAYS after migration 00007.
func TestMigration00007PolicyBundleVersioningCatalog(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL catalog check in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL catalog check")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("policy_bundles_status_check exists", func(t *testing.T) {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'policy_bundles_status_check'
				  AND conrelid = 'policy_bundles'::regclass
			)
		`).Scan(&exists); err != nil {
			t.Fatalf("query constraint: %v", err)
		}
		if !exists {
			t.Fatal("policy_bundles_status_check constraint does not exist")
		}
	})

	t.Run("policy_bundles_one_active_per_tenant_name partial unique index exists", func(t *testing.T) {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE indexname = 'policy_bundles_one_active_per_tenant_name'
				  AND tablename = 'policy_bundles'
			)
		`).Scan(&exists); err != nil {
			t.Fatalf("query index: %v", err)
		}
		if !exists {
			t.Fatal("policy_bundles_one_active_per_tenant_name index does not exist")
		}
	})

	guardTriggers := []struct {
		table   string
		trigger string
	}{
		{table: "policy_bundles", trigger: "policy_bundles_guard_update_delete"},
		{table: "policy_bundles", trigger: "policy_bundles_guard_truncate"},
		{table: "policy_bundle_rules", trigger: "policy_bundle_rules_no_update_delete"},
		{table: "policy_bundle_rules", trigger: "policy_bundle_rules_no_truncate"},
	}
	for _, tt := range guardTriggers {
		t.Run(tt.trigger+" is ENABLE ALWAYS", func(t *testing.T) {
			var enabled string
			if err := db.QueryRowContext(ctx, `
				SELECT tgenabled FROM pg_trigger
				WHERE tgname = $1 AND tgrelid = $2::regclass
			`, tt.trigger, tt.table).Scan(&enabled); err != nil {
				t.Fatalf("query trigger %s on %s: %v", tt.trigger, tt.table, err)
			}
			// pg_trigger.tgenabled = 'A' means ENABLE ALWAYS.
			if enabled != "A" {
				t.Fatalf("trigger %s tgenabled = %q, want %q (ENABLE ALWAYS)", tt.trigger, enabled, "A")
			}
		})
	}
}
