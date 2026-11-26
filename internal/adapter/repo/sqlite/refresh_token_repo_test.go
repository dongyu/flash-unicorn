package sqliterepo_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	sqliterepo "unicorn/internal/adapter/repo/sqlite"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/testkit"
)

func TestRefreshTokenRepository_SQLiteContract(t *testing.T) {
	suite.Run(t, &testkit.RefreshTokenRepositorySuite{
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
					UserRepo:         sqliterepo.NewUserRepository(dbConn.SQLite),
					RefreshTokenRepo: sqliterepo.NewRefreshTokenRepository(dbConn.SQLite),
					DeleteUser: func(userID uuid.UUID) error {
						_, execErr := dbConn.SQLite.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID.String())
						return execErr
					},
					Cleanup: func() { _ = dbConn.Close() },
				}
			},
		},
	})
}
