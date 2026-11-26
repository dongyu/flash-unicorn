package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	sqliterepo "unicorn/internal/adapter/repo/sqlite"
	"unicorn/internal/domain"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/testkit"
)

func TestRunSeedUser_CreatesUserFromStdin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seed.db")
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "sqlite://"+dbPath)
	t.Setenv("AUTH_BCRYPT_COST", "4")
	t.Setenv("CONFIG_FILE", "")

	ctx := context.Background()
	dbConn, err := database.Open(ctx, config.DatabaseConfig{URL: "sqlite://" + dbPath, MaxOpenConns: 1})
	require.NoError(t, err)
	testkit.RunTestMigrations(t, dbConn)
	require.NoError(t, dbConn.Close())

	err = run([]string{
		"seed-user",
		"--email", "Admin@Example.com",
		"--role", "admin",
		"--display-name", "Root",
	}, strings.NewReader("s3cret\n"))
	require.NoError(t, err)

	dbConn, err = database.Open(ctx, config.DatabaseConfig{URL: "sqlite://" + dbPath, MaxOpenConns: 1})
	require.NoError(t, err)
	defer func() { _ = dbConn.Close() }()

	user, err := sqliterepo.NewUserRepository(dbConn.SQLite).GetByEmail(ctx, "admin@example.com")
	require.NoError(t, err)
	require.Equal(t, "admin@example.com", user.Email)
	require.Equal(t, domain.RoleAdmin, user.Role)
	require.Equal(t, domain.UserStatusActive, user.Status)
	require.Equal(t, "Root", user.DisplayName)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("s3cret")))
}

func TestRunSeedUser_ExistingEmailFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seed.db")
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "sqlite://"+dbPath)
	t.Setenv("AUTH_BCRYPT_COST", "4")
	t.Setenv("CONFIG_FILE", "")

	ctx := context.Background()
	dbConn, err := database.Open(ctx, config.DatabaseConfig{URL: "sqlite://" + dbPath, MaxOpenConns: 1})
	require.NoError(t, err)
	testkit.RunTestMigrations(t, dbConn)
	require.NoError(t, dbConn.Close())

	args := []string{"seed-user", "--email", "admin@example.com"}
	require.NoError(t, run(args, strings.NewReader("s3cret\n")))

	err = run(args, strings.NewReader("other-secret\n"))
	require.ErrorIs(t, err, domain.ErrConflict)
}

func TestRunSeedUser_RejectsPasswordFlag(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("AUTH_BCRYPT_COST", "4")
	t.Setenv("CONFIG_FILE", "")

	err := run([]string{
		"seed-user",
		"--email", "admin@example.com",
		"--password", "s3cret",
	}, strings.NewReader(""))
	require.Error(t, err)
}

func TestRunSeedUser_RequiresPasswordOnStdin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seed.db")
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "sqlite://"+dbPath)
	t.Setenv("AUTH_BCRYPT_COST", "4")
	t.Setenv("CONFIG_FILE", "")

	ctx := context.Background()
	dbConn, err := database.Open(ctx, config.DatabaseConfig{URL: "sqlite://" + dbPath, MaxOpenConns: 1})
	require.NoError(t, err)
	testkit.RunTestMigrations(t, dbConn)
	require.NoError(t, dbConn.Close())

	err = run([]string{"seed-user", "--email", "admin@example.com"}, strings.NewReader(""))
	require.Error(t, err)
}
