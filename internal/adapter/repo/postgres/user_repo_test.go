//go:build integration

package postgresrepo_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	postgresrepo "unicorn/internal/adapter/repo/postgres"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/testkit"
)

func TestUserRepository_PostgresContract(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping PostgreSQL contract suite")
	}

	suite.Run(t, &testkit.UserRepositorySuite{
		NewHarness: func() testkit.RepoHarness {
			ctx := context.Background()
			dbConn, err := database.Open(ctx, config.DatabaseConfig{
				URL:          dsn,
				MaxOpenConns: 5,
			})
			require.NoError(t, err)
			testkit.RunTestMigrations(t, dbConn)

			_, err = dbConn.Postgres.Exec(ctx, "TRUNCATE TABLE users, notes, river_job RESTART IDENTITY CASCADE")
			require.NoError(t, err)

			return testkit.RepoHarness{
				UserRepo: postgresrepo.NewUserRepository(dbConn.Postgres),
				Cleanup:  func() { _ = dbConn.Close() },
			}
		},
	})
}
