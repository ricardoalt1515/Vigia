package tenantdb

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const setLocalTenantSQL = "SELECT set_config('app.tenant_id', $1, true)"
const setLocalAPIKeyHashSQL = "SELECT set_config('app.api_key_hash', $1, true)"

type Beginner interface {
	Begin(ctx context.Context) (Tx, error)
}

type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type WorkFunc func(ctx context.Context, tx Tx) error

func WithTenantTx(ctx context.Context, db Beginner, tenantID string, work WorkFunc) error {
	return withConfigTx(ctx, db, setLocalTenantSQL, tenantID, work)
}

func WithAPIKeyHashTx(ctx context.Context, db Beginner, keyHash string, work WorkFunc) error {
	return withConfigTx(ctx, db, setLocalAPIKeyHashSQL, keyHash, work)
}

func withConfigTx(ctx context.Context, db Beginner, sql string, value string, work WorkFunc) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err := tx.Exec(ctx, sql, value); err != nil {
		return err
	}
	if err := work(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}
