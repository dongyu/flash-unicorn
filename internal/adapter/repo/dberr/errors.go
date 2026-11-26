package dberr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"modernc.org/sqlite"
)

const (
	pgUniqueViolation          = "23505"
	sqliteConstraint           = 19
	sqliteConstraintPrimaryKey = 1555
	sqliteConstraintUnique     = 2067
)

func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}

	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		return code == sqliteConstraintUnique || code == sqliteConstraintPrimaryKey || code == sqliteConstraint
	}

	return false
}
