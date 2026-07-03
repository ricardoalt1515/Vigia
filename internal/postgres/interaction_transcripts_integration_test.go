package postgres_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
)

// TestTranscriptContentRoundTripsSpeakerAndText covers *Transcript content
// storage carries speaker and text*: utterances written via
// CreateInteractionTranscript are retrievable byte-identically via
// GetInteractionTranscriptByInteraction.
func TestTranscriptContentRoundTripsSpeakerAndText(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "transcript-roundtrip")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "transcript/roundtrip")

	type utterance struct {
		Speaker string `json:"speaker"`
		Text    string `json:"text"`
	}
	want := []utterance{
		{Speaker: "agent", Text: "Buenos días, le hablamos de Vigía Cobranza."},
		{Speaker: "debtor", Text: "¿Quién habla?"},
	}
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal utterances: %v", err)
	}

	tenantUUID, err := parseTestUUID(tenantID)
	if err != nil {
		t.Fatalf("parse tenant uuid: %v", err)
	}
	interactionUUID, err := parseTestUUID(interactionID)
	if err != nil {
		t.Fatalf("parse interaction uuid: %v", err)
	}

	q := vigiaDB.New(pool)
	if _, err := q.CreateInteractionTranscript(ctx, vigiaDB.CreateInteractionTranscriptParams{
		TenantID:           tenantUUID,
		InteractionEventID: interactionUUID,
		Utterances:         payload,
	}); err != nil {
		t.Fatalf("CreateInteractionTranscript: %v", err)
	}

	row, err := q.GetInteractionTranscriptByInteraction(ctx, vigiaDB.GetInteractionTranscriptByInteractionParams{
		TenantID:           tenantUUID,
		InteractionEventID: interactionUUID,
	})
	if err != nil {
		t.Fatalf("GetInteractionTranscriptByInteraction: %v", err)
	}

	var got []utterance
	if err := json.Unmarshal(row.Utterances, &got); err != nil {
		t.Fatalf("unmarshal stored utterances: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("utterance %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestInteractionTranscriptsRLSIsolationAcrossTenants covers the design's
// RLS integration test for the new interaction_transcripts table: a
// restricted vigia_app role scoped to tenant A must not read tenant B's
// transcript rows.
func TestInteractionTranscriptsRLSIsolationAcrossTenants(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	appDatabaseURL := os.Getenv("APP_DATABASE_URL")
	if appDatabaseURL == "" {
		t.Skip("APP_DATABASE_URL (a role without BypassRLS) is required for the interaction_transcripts RLS isolation test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantA, debtorA := seedTenantAndDebtor(t, ctx, pool, "transcript-rls-a")
	tenantB, debtorB := seedTenantAndDebtor(t, ctx, pool, "transcript-rls-b")
	interactionA := seedInteraction(t, ctx, pool, tenantA, debtorA, "transcript-rls/tenant-a")
	interactionB := seedInteraction(t, ctx, pool, tenantB, debtorB, "transcript-rls/tenant-b")

	for _, pair := range []struct{ tenant, interaction string }{
		{tenantA, interactionA},
		{tenantB, interactionB},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO interaction_transcripts (tenant_id, interaction_event_id, utterances)
			VALUES ($1, $2, '[]'::jsonb)
		`, pair.tenant, pair.interaction); err != nil {
			t.Fatalf("seed transcript: %v", err)
		}
	}

	appPool, err := pgxpool.New(ctx, appDatabaseURL)
	if err != nil {
		t.Fatalf("connect app database: %v", err)
	}
	defer appPool.Close()

	tx, err := appPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin app tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantA); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}

	rows, err := tx.Query(ctx, `SELECT tenant_id FROM interaction_transcripts WHERE interaction_event_id = $1`, interactionB)
	if err != nil {
		t.Fatalf("query tenant B transcript under tenant A context: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("tenant B's interaction_transcripts row was readable under tenant A's RLS context")
	}
}

func parseTestUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(value)
	return id, err
}
