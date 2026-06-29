package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ricardoalt1515/vigia/internal/auth"
	vigiaDB "github.com/ricardoalt1515/vigia/internal/db"
	"github.com/ricardoalt1515/vigia/internal/httpapi"
	"github.com/ricardoalt1515/vigia/internal/tenantdb"
)

const defaultInteractionLimit int32 = 50

type TenantAPIKeyStore struct {
	db tenantdb.Beginner
}

func NewTenantAPIKeyStore(db tenantdb.Beginner) *TenantAPIKeyStore {
	return &TenantAPIKeyStore{db: db}
}

func NewTenantAPIKeyStoreFromPool(pool *pgxpool.Pool) *TenantAPIKeyStore {
	return NewTenantAPIKeyStore(poolBeginner{pool: pool})
}

func (s *TenantAPIKeyStore) LookupTenantAPIKeyByHash(ctx context.Context, hash string) (auth.TenantAPIKey, error) {
	var key auth.TenantAPIKey
	err := tenantdb.WithAPIKeyHashTx(ctx, s.db, hash, func(ctx context.Context, tx tenantdb.Tx) error {
		record, err := vigiaDB.New(tx).GetTenantAPIKeyByHash(ctx, hash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return auth.ErrAPIKeyNotFound
			}
			return err
		}

		var expiresAt *time.Time
		if record.ExpiresAt.Valid {
			expiresAt = &record.ExpiresAt.Time
		}
		key = auth.TenantAPIKey{
			ID:        uuidString(record.ID),
			TenantID:  uuidString(record.TenantID),
			KeyHash:   record.KeyHash,
			Status:    record.Status,
			ExpiresAt: expiresAt,
		}
		return nil
	})
	if err != nil {
		return auth.TenantAPIKey{}, err
	}
	return key, nil
}

type InteractionReader struct {
	db    tenantdb.Beginner
	limit int32
}

func NewInteractionReader(db tenantdb.Beginner) *InteractionReader {
	return &InteractionReader{db: db, limit: defaultInteractionLimit}
}

func NewInteractionReaderFromPool(pool *pgxpool.Pool) *InteractionReader {
	return NewInteractionReader(poolBeginner{pool: pool})
}

type poolBeginner struct {
	pool *pgxpool.Pool
}

func (b poolBeginner) Begin(ctx context.Context) (tenantdb.Tx, error) {
	return b.pool.Begin(ctx)
}

func (r *InteractionReader) ListInteractions(ctx context.Context, tenantID string) ([]httpapi.Interaction, error) {
	var items []httpapi.Interaction
	err := tenantdb.WithTenantTx(ctx, r.db, tenantID, func(ctx context.Context, tx tenantdb.Tx) error {
		rows, err := vigiaDB.New(tx).ListCurrentTenantInteractions(ctx, r.limit)
		if err != nil {
			return err
		}
		items = make([]httpapi.Interaction, 0, len(rows))
		for _, row := range rows {
			items = append(items, httpapi.Interaction{
				ID:         uuidString(row.ID),
				OccurredAt: row.OccurredAt.Time,
				Channel:    row.Channel,
				Direction:  row.Direction,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func uuidString(id pgtype.UUID) string {
	return id.String()
}
