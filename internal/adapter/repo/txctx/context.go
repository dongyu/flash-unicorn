package txctx

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
)

type pgxTxKey struct{}
type sqlTxKey struct{}

func WithPgxTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, pgxTxKey{}, tx)
}

func PgxTxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(pgxTxKey{}).(pgx.Tx)
	return tx, ok && tx != nil
}

func WithSQLTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, sqlTxKey{}, tx)
}

func SQLTxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(sqlTxKey{}).(*sql.Tx)
	return tx, ok && tx != nil
}
