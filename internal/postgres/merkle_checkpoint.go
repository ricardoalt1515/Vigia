package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/ledger"
	"github.com/ricardoalt1515/vigia/internal/orchestrator"
	"github.com/ricardoalt1515/vigia/internal/tenantdb"
	"github.com/ricardoalt1515/vigia/internal/timestamping"
)

type MerkleCheckpointResult struct {
	ID            string
	TenantID      string
	FirstSeq      int64
	LastSeq       int64
	RecordCount   int64
	RootHash      string
	ChainHeadHash string
	TSAURL        string
	CreatedAt     time.Time
}

type MerkleCheckpointer struct {
	db     tenantdb.Beginner
	client timestamping.Client
	tsaURL string
	now    func() time.Time
}

func NewMerkleCheckpointer(db tenantdb.Beginner, client timestamping.Client, tsaURL string) *MerkleCheckpointer {
	return &MerkleCheckpointer{db: db, client: client, tsaURL: tsaURL, now: time.Now}
}

func NewMerkleCheckpointerFromPool(pool *pgxpool.Pool, client timestamping.Client, tsaURL string) *MerkleCheckpointer {
	return NewMerkleCheckpointer(poolBeginner{pool: pool}, client, tsaURL)
}

func (c *MerkleCheckpointer) ListMerkleCheckpointTenants(ctx context.Context) ([]string, error) {
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	rows, err := vigiaDB.New(tx).ListActiveTenants(ctx)
	if err != nil {
		return nil, err
	}
	tenantIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		tenantIDs = append(tenantIDs, uuidString(row.ID))
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	committed = true
	return tenantIDs, nil
}

func (c *MerkleCheckpointer) CreateMerkleCheckpoint(ctx context.Context, tenantID string) error {
	if err := c.VerifyMerkleCheckpoints(ctx, tenantID); err != nil {
		return err
	}
	_, err := c.CreateCheckpoint(ctx, tenantID)
	return err
}

func (c *MerkleCheckpointer) VerifyMerkleCheckpoints(ctx context.Context, tenantID string) error {
	return tenantdb.WithTenantTx(ctx, c.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		tenantUUID, err := parseUUID(tenantID)
		if err != nil {
			return err
		}
		q := vigiaDB.New(tx)
		checkpoints, err := q.ListMerkleCheckpointsByTenant(ctx, tenantUUID)
		if err != nil {
			return err
		}
		for _, checkpoint := range checkpoints {
			rows, err := q.ListEvidenceRecordsByTenantSeqRange(ctx, vigiaDB.ListEvidenceRecordsByTenantSeqRangeParams{TenantID: tenantUUID, Seq: checkpoint.FirstSeq, Seq_2: checkpoint.LastSeq})
			if err != nil {
				return err
			}
			records := make([]ledger.EvidenceRecord, 0, len(rows))
			for _, row := range rows {
				records = append(records, evidenceRowToRecord(row))
			}
			rebuilt, err := ledger.BuildMerkleCheckpoint(records, checkpoint.CreatedAt.Time)
			if err != nil {
				return err
			}
			if rebuilt.RootHash != checkpoint.RootHash || rebuilt.ChainHeadHash != checkpoint.ChainHeadHash || !bytes.Equal(rebuilt.CanonicalBytes(), checkpoint.CheckpointBody) {
				return fmt.Errorf("postgres: merkle checkpoint mismatch for seq %d..%d", checkpoint.FirstSeq, checkpoint.LastSeq)
			}
			verifier, ok := c.client.(timestamping.TokenVerifier)
			if !ok {
				return errors.New("postgres: timestamp client cannot verify persisted RFC3161 tokens")
			}
			digest := sha256.Sum256(checkpoint.CheckpointBody)
			if err := verifier.VerifyTokenDigest(checkpoint.Rfc3161Token, digest[:]); err != nil {
				return fmt.Errorf("postgres: invalid RFC3161 token for checkpoint seq %d..%d: %w", checkpoint.FirstSeq, checkpoint.LastSeq, err)
			}
		}
		return nil
	})
}

func (c *MerkleCheckpointer) CreateCheckpoint(ctx context.Context, tenantID string) (MerkleCheckpointResult, error) {
	if c.client == nil {
		return MerkleCheckpointResult{}, errors.New("postgres: timestamp client is required")
	}
	if c.tsaURL == "" {
		return MerkleCheckpointResult{}, errors.New("postgres: TSA URL is required")
	}
	now := time.Now
	if c.now != nil {
		now = c.now
	}

	tenantUUID, records, err := c.loadCheckpointRecords(ctx, tenantID)
	if err != nil {
		return MerkleCheckpointResult{}, err
	}
	checkpoint, err := ledger.BuildMerkleCheckpoint(records, now())
	if err != nil {
		return MerkleCheckpointResult{}, err
	}

	canonical := checkpoint.CanonicalBytes()
	token, err := c.client.Timestamp(ctx, canonical)
	if err != nil {
		return MerkleCheckpointResult{}, err
	}
	if err := ledger.VerifyTimestampDigest(checkpoint, token.HashedMessage); err != nil {
		return MerkleCheckpointResult{}, err
	}
	return c.insertCheckpoint(ctx, tenantID, tenantUUID, checkpoint, canonical, token)
}

func (c *MerkleCheckpointer) loadCheckpointRecords(ctx context.Context, tenantID string) (pgtype.UUID, []ledger.EvidenceRecord, error) {
	var tenantUUID pgtype.UUID
	var records []ledger.EvidenceRecord
	err := tenantdb.WithTenantTx(ctx, c.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		parsedTenantUUID, err := parseUUID(tenantID)
		if err != nil {
			return err
		}
		tenantUUID = parsedTenantUUID
		q := vigiaDB.New(tx)

		lastSeq := int64(0)
		expectedPrevHash := ledger.GenesisPrevHash
		latest, err := q.LatestMerkleCheckpoint(ctx, tenantUUID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			lastSeq = latest.LastSeq
			expectedPrevHash = latest.ChainHeadHash
		}

		rows, err := q.ListEvidenceRecordsByTenantAfterSeq(ctx, vigiaDB.ListEvidenceRecordsByTenantAfterSeqParams{TenantID: tenantUUID, Seq: lastSeq})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ledger.ErrNoNewCheckpointRecords
		}
		records = make([]ledger.EvidenceRecord, 0, len(rows))
		for _, row := range rows {
			records = append(records, evidenceRowToRecord(row))
		}
		return verifyCheckpointRange(records, expectedPrevHash)
	})
	return tenantUUID, records, err
}

func (c *MerkleCheckpointer) insertCheckpoint(ctx context.Context, tenantID string, tenantUUID pgtype.UUID, checkpoint ledger.MerkleCheckpoint, canonical []byte, token timestamping.Token) (MerkleCheckpointResult, error) {
	var out MerkleCheckpointResult
	err := tenantdb.WithTenantTx(ctx, c.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		inserted, err := vigiaDB.New(tx).InsertMerkleCheckpoint(ctx, vigiaDB.InsertMerkleCheckpointParams{
			TenantID:       tenantUUID,
			FirstSeq:       checkpoint.FirstSeq,
			LastSeq:        checkpoint.LastSeq,
			RecordCount:    checkpoint.RecordCount,
			RootHash:       checkpoint.RootHash,
			ChainHeadHash:  checkpoint.ChainHeadHash,
			CheckpointBody: canonical,
			Rfc3161Token:   token.RawToken,
			TsaUrl:         c.tsaURL,
			CreatedAt:      pgtype.Timestamptz{Time: checkpoint.CreatedAt, Valid: true},
		})
		if err != nil {
			return err
		}
		out = merkleCheckpointResultFromRow(inserted)
		return nil
	})
	if err != nil {
		return MerkleCheckpointResult{}, err
	}
	return out, nil
}

func verifyCheckpointRange(records []ledger.EvidenceRecord, expectedPrevHash string) error {
	prevHash := expectedPrevHash
	for _, record := range records {
		if record.PrevHash != prevHash {
			return fmt.Errorf("postgres: cannot checkpoint broken evidence range at seq %d: prev_hash mismatch", record.Body.Seq)
		}
		if got := ledger.Hash(record.PrevHash, record.Body); got != record.Hash {
			return fmt.Errorf("postgres: cannot checkpoint broken evidence range at seq %d: hash mismatch", record.Body.Seq)
		}
		prevHash = record.Hash
	}
	return nil
}

var _ orchestrator.LedgerCheckpointStore = (*MerkleCheckpointer)(nil)

func merkleCheckpointResultFromRow(row vigiaDB.MerkleCheckpoint) MerkleCheckpointResult {
	return MerkleCheckpointResult{
		ID:            uuidString(row.ID),
		TenantID:      uuidString(row.TenantID),
		FirstSeq:      row.FirstSeq,
		LastSeq:       row.LastSeq,
		RecordCount:   row.RecordCount,
		RootHash:      row.RootHash,
		ChainHeadHash: row.ChainHeadHash,
		TSAURL:        row.TsaUrl,
		CreatedAt:     row.CreatedAt.Time,
	}
}
