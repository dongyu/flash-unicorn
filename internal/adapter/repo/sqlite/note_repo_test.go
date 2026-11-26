package sqliterepo_test

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	queueadapter "unicorn/internal/adapter/repo/queue"
	sqliterepo "unicorn/internal/adapter/repo/sqlite"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/platform/queue"
	"unicorn/internal/testkit"
)

func TestNoteRepository_SQLiteContract(t *testing.T) {
	suite.Run(t, &testkit.NoteRepositorySuite{
		NewHarness: func() testkit.RepoHarness {
			ctx := context.Background()
			dbPath := filepath.Join(t.TempDir(), "contract.db")
			dbConn, err := database.Open(ctx, config.DatabaseConfig{
				URL:          "sqlite://" + dbPath,
				MaxOpenConns: 1,
			})
			require.NoError(t, err)
			testkit.RunTestMigrations(t, dbConn)

			riverClient, err := queue.NewClient(dbConn, config.RiverConfig{MaxWorkers: 1}, nil, zerolog.New(io.Discard), false)
			require.NoError(t, err)

			return testkit.RepoHarness{
				TxManager: sqliterepo.NewTxManager(dbConn.SQLite),
				NoteRepo:  sqliterepo.NewNoteRepository(dbConn.SQLite),
				Enqueuer:  queueadapter.NewEnqueuer(riverClient),
				Cleanup:   func() { _ = dbConn.Close() },
			}
		},
	})
}
