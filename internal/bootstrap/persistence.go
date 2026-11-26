package bootstrap

import (
	"fmt"

	postgresrepo "unicorn/internal/adapter/repo/postgres"
	sqliterepo "unicorn/internal/adapter/repo/sqlite"
	"unicorn/internal/domain"
	"unicorn/internal/platform/database"
)

type Persistence struct {
	Tx            domain.TxManager
	Notes         domain.NoteRepository
	Users         domain.UserRepository
	RefreshTokens domain.RefreshTokenRepository
}

func OpenPersistence(db *database.DB) (Persistence, error) {
	switch db.Dialect {
	case database.DialectPostgres:
		return Persistence{
			Tx:            postgresrepo.NewTxManager(db.Postgres),
			Notes:         postgresrepo.NewNoteRepository(db.Postgres),
			Users:         postgresrepo.NewUserRepository(db.Postgres),
			RefreshTokens: postgresrepo.NewRefreshTokenRepository(db.Postgres),
		}, nil
	case database.DialectSQLite:
		return Persistence{
			Tx:            sqliterepo.NewTxManager(db.SQLite),
			Notes:         sqliterepo.NewNoteRepository(db.SQLite),
			Users:         sqliterepo.NewUserRepository(db.SQLite),
			RefreshTokens: sqliterepo.NewRefreshTokenRepository(db.SQLite),
		}, nil
	default:
		return Persistence{}, fmt.Errorf("unsupported dialect %q", db.Dialect)
	}
}
