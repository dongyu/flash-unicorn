package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/domain"
)

type TxManager struct {
	db *sql.DB
}

func NewTxManager(db *sql.DB) *TxManager {
	return &TxManager{db: db}
}

var _ domain.TxManager = (*TxManager)(nil)

func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := txctx.SQLTxFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite tx: %w", err)
	}

	txCtx := txctx.WithSQLTx(ctx, tx)
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(txCtx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("commit sqlite tx: %w", err)
	}
	committed = true
	return nil
}
