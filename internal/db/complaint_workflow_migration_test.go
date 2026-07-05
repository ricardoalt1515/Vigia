package db_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ricardoalt1515/vigia/internal/auth"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
)

func TestComplaintWorkflowMigrationDefinesTenantScopedCasesAndReviews(t *testing.T) {
	migration := readComplaintWorkflowMigration(t)

	for _, table := range []string{"complaint_cases", "human_reviews"} {
		block := createTableBlock(t, migration, table)
		if !strings.Contains(block, "tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE") {
			t.Fatalf("%s is missing non-null tenant_id FK", table)
		}
		if !strings.Contains(block, "UNIQUE (id, tenant_id)") {
			t.Fatalf("%s is missing composite tenant key", table)
		}
		if !strings.Contains(migration, "ALTER TABLE "+table+" ENABLE ROW LEVEL SECURITY") {
			t.Fatalf("%s does not enable RLS", table)
		}
		if !strings.Contains(migration, "CREATE POLICY "+table+"_tenant_isolation ON "+table) {
			t.Fatalf("%s is missing tenant isolation policy", table)
		}
	}
}

func TestComplaintWorkflowMigrationSeedsVersionedHolidayCalendar(t *testing.T) {
	migration := readComplaintWorkflowMigration(t)

	for _, required := range []string{
		"CREATE TABLE business_day_holidays",
		"calendar_version text NOT NULL",
		"holiday_date date NOT NULL",
		"UNIQUE (calendar_version, holiday_date)",
		"pending counsel confirmation",
		"mx-lft-art-74-2026a",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("migration missing holiday calendar fragment %q", required)
		}
	}
}

func TestComplaintWorkflowMigrationExtendsEvidenceRecordsForComplaintTransitions(t *testing.T) {
	migration := readComplaintWorkflowMigration(t)

	for _, required := range []string{
		"ALTER TABLE evidence_records ADD COLUMN record_kind text NOT NULL DEFAULT 'evaluation'",
		"ALTER TABLE evidence_records ALTER COLUMN interaction_event_id DROP NOT NULL",
		"ALTER TABLE evidence_records ALTER COLUMN evaluation_id DROP NOT NULL",
		"ALTER TABLE evidence_records ADD COLUMN complaint_case_id uuid",
		"FOREIGN KEY (complaint_case_id, tenant_id)",
		"evidence_records_exactly_one_record_kind_check",
		"evidence_records_one_evaluation_record",
		"evidence_records_one_complaint_transition_record",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("migration missing evidence extension fragment %q", required)
		}
	}
}

func TestComplaintWorkflowDownMigrationRefusesToEraseComplaintEvidence(t *testing.T) {
	migration := readComplaintWorkflowMigration(t)
	down := migration[strings.Index(migration, "-- +goose Down"):]

	for _, forbidden := range []string{
		"ALTER TABLE evidence_records DISABLE TRIGGER evidence_records_no_update_delete",
		"DELETE FROM evidence_records WHERE record_kind = 'complaint_transition'",
	} {
		if strings.Contains(down, forbidden) {
			t.Fatalf("down migration must not silently erase append-only complaint evidence; found %q", forbidden)
		}
	}
	for _, required := range []string{
		"IF EXISTS (SELECT 1 FROM evidence_records WHERE record_kind = 'complaint_transition') THEN",
		"RAISE EXCEPTION 'refusing to roll back complaint workflow while complaint_transition evidence exists'",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("down migration missing safe rollback guard fragment %q", required)
		}
	}
}

func TestComplaintWorkflowHumanReviewWinnerQueryRequiresSameCaseAndTenant(t *testing.T) {
	query := readRepoFile(t, "db", "queries", "human_reviews.sql")
	block := sqlQueryBlock(t, query, "-- name: MarkWinningHumanReviewProcessed :one")

	for _, required := range []string{
		"WHERE tenant_id = $1",
		"AND complaint_case_id = $2",
		"AND id = $3",
		"AND processed_at IS NULL",
		"AND superseded_at IS NULL",
	} {
		if !strings.Contains(block, required) {
			t.Fatalf("MarkWinningHumanReviewProcessed must require same-case unprocessed winner; missing %q in:\n%s", required, block)
		}
	}
}

func TestComplaintWorkflowHumanReviewLookupRequiresMatchingDecision(t *testing.T) {
	query := readRepoFile(t, "db", "queries", "human_reviews.sql")
	block := sqlQueryBlock(t, query, "-- name: GetUnprocessedHumanReviewForCase :one")

	for _, required := range []string{
		"WHERE tenant_id = $1",
		"AND complaint_case_id = $2",
		"AND id = $3",
		"AND decision = $4",
		"AND processed_at IS NULL",
		"AND superseded_at IS NULL",
	} {
		if !strings.Contains(block, required) {
			t.Fatalf("GetUnprocessedHumanReviewForCase must require same-case unprocessed review with matching decision; missing %q in:\n%s", required, block)
		}
	}
}

func TestComplaintWorkflowUnprocessedReviewListRequiresAwaitingReviewCase(t *testing.T) {
	query := readRepoFile(t, "db", "queries", "human_reviews.sql")
	block := sqlQueryBlock(t, query, "-- name: ListUnprocessedHumanReviews :many")

	for _, required := range []string{
		"FROM human_reviews hr",
		"JOIN complaint_cases cc ON cc.id = hr.complaint_case_id AND cc.tenant_id = hr.tenant_id",
		"WHERE hr.tenant_id = $1",
		"AND cc.state = 'awaiting_review'",
		"AND hr.processed_at IS NULL",
		"AND hr.superseded_at IS NULL",
		"ORDER BY hr.created_at ASC",
	} {
		if !strings.Contains(block, required) {
			t.Fatalf("ListUnprocessedHumanReviews must only return unprocessed reviews for awaiting_review cases; missing %q in:\n%s", required, block)
		}
	}
}

func TestComplaintWorkflowUnprocessedReviewListSkipsTerminalCasesUnderLimitPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL complaint workflow integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL complaint workflow integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer conn.Close(ctx)

	queries := vigiaDB.New(conn)
	tenantID := createComplaintWorkflowTenant(t, ctx, conn, "complaint-review-window")
	interactionStale := createComplaintWorkflowInteraction(t, ctx, conn, tenantID, "review-window-stale")
	interactionValid := createComplaintWorkflowInteraction(t, ctx, conn, tenantID, "review-window-valid")

	staleCase := createAwaitingReviewCaseForQueryTest(t, ctx, queries, tenantID, interactionStale, "review-window-stale")
	validCase := createAwaitingReviewCaseForQueryTest(t, ctx, queries, tenantID, interactionValid, "review-window-valid")
	staleReview := insertHumanReviewForQueryTest(t, ctx, queries, tenantID, staleCase.ID, "approve", "stale-reviewer")
	validReview := insertHumanReviewForQueryTest(t, ctx, queries, tenantID, validCase.ID, "approve", "valid-reviewer")

	if _, err := conn.Exec(ctx, `UPDATE human_reviews SET created_at = now() - interval '2 hours' WHERE id = $1`, staleReview.ID); err != nil {
		t.Fatalf("age stale review: %v", err)
	}
	if _, err := conn.Exec(ctx, `UPDATE complaint_cases SET state = 'escalated', updated_at = now() WHERE id = $1 AND tenant_id = $2`, staleCase.ID, tenantID); err != nil {
		t.Fatalf("make first reviewed case terminal: %v", err)
	}

	reviews, err := queries.ListUnprocessedHumanReviews(ctx, vigiaDB.ListUnprocessedHumanReviewsParams{TenantID: mustUUID(t, tenantID), Limit: 1})
	if err != nil {
		t.Fatalf("list unprocessed reviews: %v", err)
	}
	if got, want := len(reviews), 1; got != want {
		t.Fatalf("review count under limit pressure = %d, want %d", got, want)
	}
	if reviews[0].ID != validReview.ID || reviews[0].ComplaintCaseID != validCase.ID {
		t.Fatalf("poll window returned review %s for case %s, want active review %s for awaiting_review case %s", reviews[0].ID.String(), reviews[0].ComplaintCaseID.String(), validReview.ID.String(), validCase.ID.String())
	}
}

func TestComplaintWorkflowQueriesAndConstraintsAgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL complaint workflow integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL complaint workflow integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer conn.Close(ctx)

	queries := vigiaDB.New(conn)
	tenantA := createComplaintWorkflowTenant(t, ctx, conn, "complaint-a")
	tenantB := createComplaintWorkflowTenant(t, ctx, conn, "complaint-b")
	interactionA := createComplaintWorkflowInteraction(t, ctx, conn, tenantA, "interaction-a")
	interactionB := createComplaintWorkflowInteraction(t, ctx, conn, tenantB, "interaction-b")

	openedAt := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	slaDueAt := openedAt.Add(10 * 24 * time.Hour)
	caseA, err := queries.CreateComplaintCase(ctx, vigiaDB.CreateComplaintCaseParams{
		TenantID:        mustUUID(t, tenantA),
		InteractionID:   mustUUID(t, interactionA),
		RedecoCause:     "collection_dispute",
		OpenedAt:        pgtype.Timestamptz{Time: openedAt, Valid: true},
		SlaDueAt:        pgtype.Timestamptz{Time: slaDueAt, Valid: true},
		CalendarVersion: "mx-lft-art-74-2026a",
		IdempotencyKey:  "idem-a",
	})
	if err != nil {
		t.Fatalf("create tenant A complaint case via sqlc query: %v", err)
	}
	if _, err := queries.CreateComplaintCase(ctx, vigiaDB.CreateComplaintCaseParams{
		TenantID:        mustUUID(t, tenantA),
		InteractionID:   mustUUID(t, interactionA),
		RedecoCause:     "collection_dispute",
		OpenedAt:        pgtype.Timestamptz{Time: openedAt, Valid: true},
		SlaDueAt:        pgtype.Timestamptz{Time: slaDueAt, Valid: true},
		CalendarVersion: "mx-lft-art-74-2026a",
		IdempotencyKey:  "idem-a",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("duplicate idempotency insert error = %v, want pgx.ErrNoRows from ON CONFLICT DO NOTHING RETURNING", err)
	}
	if _, err := queries.CreateComplaintCase(ctx, vigiaDB.CreateComplaintCaseParams{
		TenantID:        mustUUID(t, tenantB),
		InteractionID:   mustUUID(t, interactionB),
		RedecoCause:     "collection_dispute",
		OpenedAt:        pgtype.Timestamptz{Time: openedAt, Valid: true},
		SlaDueAt:        pgtype.Timestamptz{Time: slaDueAt, Valid: true},
		CalendarVersion: "mx-lft-art-74-2026a",
		IdempotencyKey:  "idem-b",
	}); err != nil {
		t.Fatalf("create tenant B complaint case via sqlc query: %v", err)
	}

	openCases, err := queries.ListOpenComplaintCases(ctx, vigiaDB.ListOpenComplaintCasesParams{TenantID: mustUUID(t, tenantA), Limit: 20})
	if err != nil {
		t.Fatalf("list open tenant A cases: %v", err)
	}
	for _, item := range openCases {
		if item.TenantID.String() != tenantA {
			t.Fatalf("ListOpenComplaintCases returned tenant %s while scoped to %s", item.TenantID.String(), tenantA)
		}
	}
	if _, err := queries.TransitionComplaintCaseToAwaitingReview(ctx, vigiaDB.TransitionComplaintCaseToAwaitingReviewParams{
		TenantID:        mustUUID(t, tenantB),
		ID:              caseA.ID,
		ReviewExpiresAt: pgtype.Timestamptz{Time: openedAt.Add(3 * 24 * time.Hour), Valid: true},
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant transition error = %v, want pgx.ErrNoRows", err)
	}
	caseA, err = queries.TransitionComplaintCaseToAwaitingReview(ctx, vigiaDB.TransitionComplaintCaseToAwaitingReviewParams{
		TenantID:        mustUUID(t, tenantA),
		ID:              caseA.ID,
		ReviewExpiresAt: pgtype.Timestamptz{Time: openedAt.Add(3 * 24 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("transition tenant A case to awaiting review: %v", err)
	}
	if _, err := queries.InsertHumanReview(ctx, vigiaDB.InsertHumanReviewParams{
		TenantID:        mustUUID(t, tenantB),
		ComplaintCaseID: caseA.ID,
		Decision:        "approve",
		Reviewer:        "reviewer@example.test",
		Notes:           "wrong tenant",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant review insert error = %v, want pgx.ErrNoRows", err)
	}
	if _, err := queries.InsertHumanReview(ctx, vigiaDB.InsertHumanReviewParams{
		TenantID:        mustUUID(t, tenantA),
		ComplaintCaseID: caseA.ID,
		Decision:        "invalid",
		Reviewer:        "reviewer@example.test",
		Notes:           "invalid decision",
	}); err == nil {
		t.Fatal("expected invalid human review decision to fail CHECK constraint")
	}
}

func TestComplaintWorkflowEvidenceConstraintsAgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL complaint evidence integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for PostgreSQL complaint evidence integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer conn.Close(ctx)

	tenantID := createComplaintWorkflowTenant(t, ctx, conn, "complaint-evidence")
	interactionID := createComplaintWorkflowInteraction(t, ctx, conn, tenantID, "complaint-evidence-interaction")
	queries := vigiaDB.New(conn)
	caseRow, err := queries.CreateComplaintCase(ctx, vigiaDB.CreateComplaintCaseParams{
		TenantID:        mustUUID(t, tenantID),
		InteractionID:   mustUUID(t, interactionID),
		RedecoCause:     "collection_dispute",
		OpenedAt:        pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		SlaDueAt:        pgtype.Timestamptz{Time: time.Now().UTC().Add(10 * 24 * time.Hour), Valid: true},
		CalendarVersion: "mx-lft-art-74-2026a",
		IdempotencyKey:  "idem-evidence",
	})
	if err != nil {
		t.Fatalf("create complaint case for evidence test: %v", err)
	}

	insertComplaintEvidence := `
		INSERT INTO evidence_records (
			tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
			overall_outcome, policy_bundle_version, inputs_digest, created_at,
			record_kind, complaint_case_id, transition_kind
		) VALUES ($1, NULL, NULL, $2, $3, $4, 'awaiting_review', '', '', now(),
			'complaint_transition', $5, $6)
	`
	if _, err := conn.Exec(ctx, insertComplaintEvidence, tenantID, int64(1), "genesis", "hash-1", caseRow.ID, "request_review"); err != nil {
		t.Fatalf("insert complaint_transition evidence row: %v", err)
	}
	if _, err := conn.Exec(ctx, insertComplaintEvidence, tenantID, int64(2), "hash-1", "hash-2", caseRow.ID, "request_review"); err == nil {
		t.Fatal("expected duplicate (tenant_id, complaint_case_id, transition_kind) complaint evidence to fail partial unique index")
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO evidence_records (
			tenant_id, interaction_event_id, evaluation_id, seq, prev_hash, hash,
			overall_outcome, policy_bundle_version, inputs_digest, created_at,
			record_kind, complaint_case_id, transition_kind
		) VALUES ($1, $2, NULL, 3, 'hash-2', 'hash-3', 'awaiting_review', '', '', now(),
			'complaint_transition', $3, 'sla_breach')
	`, tenantID, interactionID, caseRow.ID); err == nil {
		t.Fatal("expected complaint_transition evidence with interaction_event_id to fail exactly-one CHECK")
	}
}

func TestComplaintWorkflowRLSAgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL complaint workflow RLS integration test in short mode")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	appDatabaseURL := os.Getenv("APP_DATABASE_URL")
	if databaseURL == "" || appDatabaseURL == "" {
		t.Skip("DATABASE_URL and APP_DATABASE_URL are required for PostgreSQL complaint workflow RLS integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	owner, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect owner database: %v", err)
	}
	defer owner.Close(ctx)
	app, err := pgx.Connect(ctx, appDatabaseURL)
	if err != nil {
		t.Fatalf("connect app database: %v", err)
	}
	defer app.Close(ctx)

	tenantA := createComplaintWorkflowTenant(t, ctx, owner, "complaint-rls-a")
	tenantB := createComplaintWorkflowTenant(t, ctx, owner, "complaint-rls-b")
	interactionA := createComplaintWorkflowInteraction(t, ctx, owner, tenantA, "complaint-rls-a-interaction")
	interactionB := createComplaintWorkflowInteraction(t, ctx, owner, tenantB, "complaint-rls-b-interaction")
	queries := vigiaDB.New(owner)
	createCaseForRLSTest(t, ctx, queries, tenantA, interactionA, "rls-a")
	createCaseForRLSTest(t, ctx, queries, tenantB, interactionB, "rls-b")

	tx, err := app.Begin(ctx)
	if err != nil {
		t.Fatalf("begin app tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantA); err != nil {
		t.Fatalf("set app tenant context: %v", err)
	}

	var visibleTenantCount int
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT tenant_id) FROM complaint_cases`).Scan(&visibleTenantCount); err != nil {
		t.Fatalf("query complaint_cases under app role: %v", err)
	}
	if visibleTenantCount != 1 {
		t.Fatalf("app role saw %d complaint_case tenants, want exactly 1", visibleTenantCount)
	}
	var crossTenantReviews int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM human_reviews hr
		JOIN complaint_cases cc ON cc.id = hr.complaint_case_id
		WHERE cc.tenant_id <> nullif(current_setting('app.tenant_id', true), '')::uuid
	`).Scan(&crossTenantReviews); err != nil {
		t.Fatalf("query human_reviews under app role: %v", err)
	}
	if crossTenantReviews != 0 {
		t.Fatalf("app role saw %d cross-tenant human_reviews, want 0", crossTenantReviews)
	}
}

func readComplaintWorkflowMigration(t *testing.T) string {
	t.Helper()
	return readRepoFile(t, "db", "migrations", "00009_complaint_workflow.sql")
}

func readRepoFile(t *testing.T, path ...string) string {
	t.Helper()
	parts := append([]string{"..", ".."}, path...)
	content, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read repo file %s: %v", filepath.Join(path...), err)
	}
	return string(content)
}

func sqlQueryBlock(t *testing.T, content, marker string) string {
	t.Helper()
	start := strings.Index(content, marker)
	if start < 0 {
		t.Fatalf("query marker %q not found", marker)
	}
	rest := content[start:]
	if next := strings.Index(rest[len(marker):], "-- name:"); next >= 0 {
		return rest[:len(marker)+next]
	}
	return rest
}

func createComplaintWorkflowTenant(t *testing.T, ctx context.Context, conn *pgx.Conn, prefix string) string {
	t.Helper()
	slug := prefix + "-" + auth.HashAPIKey(time.Now().String())[:12]
	var tenantID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO tenants (slug, name, status)
		VALUES ($1, $1, 'active')
		RETURNING id
	`, slug).Scan(&tenantID); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tenantID
}

func createComplaintWorkflowInteraction(t *testing.T, ctx context.Context, conn *pgx.Conn, tenantID, externalRef string) string {
	t.Helper()
	var debtorID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO debtors (tenant_id, external_ref, display_name, timezone)
		VALUES ($1, $2, $2, 'America/Mexico_City')
		RETURNING id
	`, tenantID, externalRef).Scan(&debtorID); err != nil {
		t.Fatalf("create debtor: %v", err)
	}
	var interactionID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO interaction_events (tenant_id, debtor_id, channel, direction, status, occurred_at, debtor_timezone)
		VALUES ($1, $2, 'phone', 'outbound', 'recorded', now(), 'America/Mexico_City')
		RETURNING id
	`, tenantID, debtorID).Scan(&interactionID); err != nil {
		t.Fatalf("create interaction: %v", err)
	}
	return interactionID
}

func createCaseForRLSTest(t *testing.T, ctx context.Context, queries *vigiaDB.Queries, tenantID, interactionID, idempotencyKey string) vigiaDB.ComplaintCase {
	t.Helper()
	caseRow := createAwaitingReviewCaseForQueryTest(t, ctx, queries, tenantID, interactionID, idempotencyKey)
	insertHumanReviewForQueryTest(t, ctx, queries, tenantID, caseRow.ID, "approve", "reviewer@example.test")
	return caseRow
}

func createAwaitingReviewCaseForQueryTest(t *testing.T, ctx context.Context, queries *vigiaDB.Queries, tenantID, interactionID, idempotencyKey string) vigiaDB.ComplaintCase {
	t.Helper()
	now := time.Now().UTC()
	caseRow, err := queries.CreateComplaintCase(ctx, vigiaDB.CreateComplaintCaseParams{
		TenantID:        mustUUID(t, tenantID),
		InteractionID:   mustUUID(t, interactionID),
		RedecoCause:     "collection_dispute",
		OpenedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		SlaDueAt:        pgtype.Timestamptz{Time: now.Add(10 * 24 * time.Hour), Valid: true},
		CalendarVersion: "mx-lft-art-74-2026a",
		IdempotencyKey:  idempotencyKey,
	})
	if err != nil {
		t.Fatalf("create complaint case for query test: %v", err)
	}
	caseRow, err = queries.TransitionComplaintCaseToAwaitingReview(ctx, vigiaDB.TransitionComplaintCaseToAwaitingReviewParams{
		TenantID:        mustUUID(t, tenantID),
		ID:              caseRow.ID,
		ReviewExpiresAt: pgtype.Timestamptz{Time: now.Add(3 * 24 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("transition complaint case for query test: %v", err)
	}
	return caseRow
}

func insertHumanReviewForQueryTest(t *testing.T, ctx context.Context, queries *vigiaDB.Queries, tenantID string, complaintCaseID pgtype.UUID, decision, reviewer string) vigiaDB.HumanReview {
	t.Helper()
	review, err := queries.InsertHumanReview(ctx, vigiaDB.InsertHumanReviewParams{
		TenantID:        mustUUID(t, tenantID),
		ComplaintCaseID: complaintCaseID,
		Decision:        decision,
		Reviewer:        reviewer,
		Notes:           "query coverage",
	})
	if err != nil {
		t.Fatalf("create human review for query test: %v", err)
	}
	return review
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return id
}
