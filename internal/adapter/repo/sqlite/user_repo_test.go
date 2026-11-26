package sqliterepo_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	sqliterepo "unicorn/internal/adapter/repo/sqlite"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/testkit"
)

func TestUserRepository_SQLiteContract(t *testing.T) {
	suite.Run(t, &testkit.UserRepositorySuite{
		RepoSuite: testkit.RepoSuite{
			NewHarness: func() testkit.RepoHarness {
				ctx := context.Background()
				dbPath := filepath.Join(t.TempDir(), "contract.db")
				dbConn, err := database.Open(ctx, config.DatabaseConfig{
					URL:          "sqlite://" + dbPath,
					MaxOpenConns: 1,
				})
				require.NoError(t, err)
				testkit.RunTestMigrations(t, dbConn)

				return testkit.RepoHarness{
					UserRepo: sqliterepo.NewUserRepository(dbConn.SQLite),
					Cleanup:  func() { _ = dbConn.Close() },
				}
			},
		},
	})
}
