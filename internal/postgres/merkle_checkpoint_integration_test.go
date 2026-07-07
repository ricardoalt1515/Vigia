package postgres_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/postgres"
	"github.com/ricardoalt1515/vigia/internal/timestamping"
)

type fakeTimestampClient struct{}

func (fakeTimestampClient) Timestamp(ctx context.Context, payload []byte) (timestamping.Token, error) {
	sum := sha256.Sum256(payload)
	return timestamping.Token{RawToken: append([]byte("fake-rfc3161-token:"), sum[:]...), HashedMessage: sum[:]}, nil
}

func (fakeTimestampClient) VerifyTokenDigest(rawToken []byte, expectedDigest []byte) error {
	want := append([]byte("fake-rfc3161-token:"), expectedDigest...)
	if !bytes.Equal(rawToken, want) {
		return errors.New("fake token digest mismatch")
	}
	return nil
}

func TestMerkleCheckpointerCreatesTimestampedCheckpointForNewEvidence(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "merkle-checkpoint")
	for _, suffix := range []string{"a", "b"} {
		interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "merkle/checkpoint/"+suffix)
		evaluateOnce(t, ctx, pool, tenantID, interactionID)
	}

	checkpointer := postgres.NewMerkleCheckpointerFromPool(pool, fakeTimestampClient{}, "https://tsa.example.test")
	checkpoint, err := checkpointer.CreateCheckpoint(ctx, tenantID)
	if err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}
	if checkpoint.FirstSeq != 1 || checkpoint.LastSeq != 2 || checkpoint.RecordCount != 2 {
		t.Fatalf("checkpoint range = %+v, want seq 1..2 count 2", checkpoint)
	}
	if checkpoint.RootHash == "" || checkpoint.ChainHeadHash == "" {
		t.Fatalf("checkpoint hashes missing: %+v", checkpoint)
	}

	var storedToken []byte
	var storedBody []byte
	if err := pool.QueryRow(ctx, `
		SELECT rfc3161_token, checkpoint_body
		FROM merkle_checkpoints
		WHERE tenant_id = $1 AND first_seq = 1 AND last_seq = 2
	`, tenantID).Scan(&storedToken, &storedBody); err != nil {
		t.Fatalf("read merkle checkpoint: %v", err)
	}
	if len(storedToken) == 0 || len(storedBody) == 0 {
		t.Fatalf("stored token/body lengths = %d/%d, want non-zero", len(storedToken), len(storedBody))
	}
	wantBody := ledger.MerkleCheckpoint{
		TenantID:      checkpoint.TenantID,
		FirstSeq:      checkpoint.FirstSeq,
		LastSeq:       checkpoint.LastSeq,
		RecordCount:   checkpoint.RecordCount,
		RootHash:      checkpoint.RootHash,
		ChainHeadHash: checkpoint.ChainHeadHash,
		CreatedAt:     checkpoint.CreatedAt,
	}.CanonicalBytes()
	if !bytes.Equal(storedBody, wantBody) {
		t.Fatalf("stored checkpoint body = %s, want canonical bytes %s", string(storedBody), string(wantBody))
	}
	sum := sha256.Sum256(storedBody)
	if !bytes.Equal(storedToken, append([]byte("fake-rfc3161-token:"), sum[:]...)) {
		t.Fatalf("stored token is not bound to stored checkpoint body")
	}
}

func TestVerifyMerkleCheckpointsRejectsTamperedRFC3161Token(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "merkle-token-tamper")
	interactionID := seedInteraction(t, ctx, pool, tenantID, debtorID, "merkle/token-tamper")
	evaluateOnce(t, ctx, pool, tenantID, interactionID)

	checkpointer := postgres.NewMerkleCheckpointerFromPool(pool, fakeTimestampClient{}, "https://tsa.example.test")
	if _, err := checkpointer.CreateCheckpoint(ctx, tenantID); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE merkle_checkpoints SET rfc3161_token = 'broken'::bytea WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatalf("tamper token: %v", err)
	}
	if err := checkpointer.VerifyMerkleCheckpoints(ctx, tenantID); err == nil {
		t.Fatal("VerifyMerkleCheckpoints error = nil, want tampered token rejected")
	}
}

func TestMerkleCheckpointerContinuesAfterLatestCheckpoint(t *testing.T) {
	databaseURL := requireDatabaseURL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	tenantID, debtorID := seedTenantAndDebtor(t, ctx, pool, "merkle-checkpoint-incremental")
	firstInteraction := seedInteraction(t, ctx, pool, tenantID, debtorID, "merkle/incremental/first")
	evaluateOnce(t, ctx, pool, tenantID, firstInteraction)

	checkpointer := postgres.NewMerkleCheckpointerFromPool(pool, fakeTimestampClient{}, "https://tsa.example.test")
	first, err := checkpointer.CreateCheckpoint(ctx, tenantID)
	if err != nil {
		t.Fatalf("CreateCheckpoint first: %v", err)
	}
	if first.FirstSeq != 1 || first.LastSeq != 1 {
		t.Fatalf("first checkpoint = %+v, want seq 1", first)
	}

	secondInteraction := seedInteraction(t, ctx, pool, tenantID, debtorID, "merkle/incremental/second")
	evaluateOnce(t, ctx, pool, tenantID, secondInteraction)

	second, err := checkpointer.CreateCheckpoint(ctx, tenantID)
	if err != nil {
		t.Fatalf("CreateCheckpoint second: %v", err)
	}
	if second.FirstSeq != 2 || second.LastSeq != 2 || second.RecordCount != 1 {
		t.Fatalf("second checkpoint = %+v, want seq 2 only", second)
	}
}
