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
