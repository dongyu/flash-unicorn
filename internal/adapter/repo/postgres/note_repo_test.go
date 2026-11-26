//go:build integration

package postgresrepo_test

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	postgresrepo "unicorn/internal/adapter/repo/postgres"
	queueadapter "unicorn/internal/adapter/repo/queue"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/platform/queue"
	"unicorn/internal/testkit"
)

func TestNoteRepository_PostgresContract(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping PostgreSQL contract suite")
	}

	suite.Run(t, &testkit.NoteRepositorySuite{
		RepoSuite: testkit.RepoSuite{
			NewHarness: func() testkit.RepoHarness {
				ctx := context.Background()
				dbConn, err := database.Open(ctx, config.DatabaseConfig{
					URL:          dsn,
					MaxOpenConns: 5,
				})
				require.NoError(t, err)
				testkit.RunTestMigrations(t, dbConn)

				_, err = dbConn.Postgres.Exec(ctx, "TRUNCATE TABLE notes, river_job RESTART IDENTITY CASCADE")
				require.NoError(t, err)

				riverClient, err := queue.NewClient(dbConn, config.RiverConfig{MaxWorkers: 1}, nil, zerolog.New(io.Discard), false)
				require.NoError(t, err)

				return testkit.RepoHarness{
					TxManager: postgresrepo.NewTxManager(dbConn.Postgres),
					NoteRepo:  postgresrepo.NewNoteRepository(dbConn.Postgres),
					Enqueuer:  queueadapter.NewEnqueuer(riverClient),
					Cleanup:   func() { _ = dbConn.Close() },
				}
			},
		},
	})
}
