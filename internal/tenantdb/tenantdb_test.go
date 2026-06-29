package tenantdb

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSetLocalTenantContext(t *testing.T) {
	ctx := context.Background()
	tenantID := "11111111-1111-1111-1111-111111111111"

	t.Run("sets API key hash context before lookup work and commits", func(t *testing.T) {
		keyHash := "key-hash"
		db := &fakeDB{tx: &fakeTx{}}

		err := WithAPIKeyHashTx(ctx, db, keyHash, func(_ context.Context, tx Tx) error {
			if got := db.tx.execs; len(got) != 1 || got[0].sql != setLocalAPIKeyHashSQL || got[0].arg != keyHash {
				t.Fatalf("api key hash context execs = %#v", got)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WithAPIKeyHashTx() error = %v", err)
		}
		if !db.tx.committed {
			t.Fatal("transaction was not committed")
		}
	})

	t.Run("sets tenant context before protected work and commits", func(t *testing.T) {
		db := &fakeDB{tx: &fakeTx{}}

		err := WithTenantTx(ctx, db, tenantID, func(_ context.Context, tx Tx) error {
			if got := db.tx.execs; len(got) != 1 || got[0].sql != setLocalTenantSQL || got[0].arg != tenantID {
				t.Fatalf("tenant context execs = %#v", got)
			}
			db.tx.protectedWorkObserved = true
			return nil
		})
		if err != nil {
			t.Fatalf("WithTenantTx() error = %v", err)
		}
		if !db.tx.committed {
			t.Fatal("transaction was not committed")
		}
		if db.tx.rolledBack {
			t.Fatal("successful transaction was rolled back")
		}
	})

	t.Run("rolls back when protected work fails", func(t *testing.T) {
		workErr := errors.New("protected work failed")
		db := &fakeDB{tx: &fakeTx{}}

		err := WithTenantTx(ctx, db, tenantID, func(context.Context, Tx) error {
			return workErr
		})
		if !errors.Is(err, workErr) {
			t.Fatalf("WithTenantTx() error = %v, want %v", err, workErr)
		}
		if !db.tx.rolledBack {
			t.Fatal("failed transaction was not rolled back")
		}
		if db.tx.committed {
			t.Fatal("failed transaction was committed")
		}
	})

	t.Run("rolls back when SET LOCAL fails", func(t *testing.T) {
		setErr := errors.New("set local failed")
		db := &fakeDB{tx: &fakeTx{execErr: setErr}}

		err := WithTenantTx(ctx, db, tenantID, func(context.Context, Tx) error {
			t.Fatal("protected work ran after SET LOCAL failed")
			return nil
		})
		if !errors.Is(err, setErr) {
			t.Fatalf("WithTenantTx() error = %v, want %v", err, setErr)
		}
		if !db.tx.rolledBack {
			t.Fatal("transaction did not roll back after SET LOCAL failure")
		}
	})
}

type fakeDB struct {
	tx *fakeTx
}

func (db *fakeDB) Begin(context.Context) (Tx, error) {
	return db.tx, nil
}

type fakeTx struct {
	execs                 []execCall
	execErr               error
	committed             bool
	rolledBack            bool
	protectedWorkObserved bool
}

type execCall struct {
	sql string
	arg string
}

func (tx *fakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	arg, _ := args[0].(string)
	tx.execs = append(tx.execs, execCall{sql: sql, arg: arg})
	return pgconn.CommandTag{}, tx.execErr
}

func (tx *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("fakeTx.Query should not be called in tenantdb unit tests")
}

func (tx *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("fakeTx.QueryRow should not be called in tenantdb unit tests")
}

func (tx *fakeTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *fakeTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}
