package tenantdb

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

const setLocalTenantSQL = "SELECT set_config('app.tenant_id', $1, true)"

type Beginner interface {
	Begin(ctx context.Context) (Tx, error)
}

type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type WorkFunc func(ctx context.Context, tx Tx) error

func WithTenantTx(ctx context.Context, db Beginner, tenantID string, work WorkFunc) error {
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

	if _, err := tx.Exec(ctx, setLocalTenantSQL, tenantID); err != nil {
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
