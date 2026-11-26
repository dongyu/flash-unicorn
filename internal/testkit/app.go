package testkit

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-migrate/migrate/v4"
	pgx5migrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertest"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"unicorn/db"
	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/app"
	"unicorn/internal/domain"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
)

type TestApp struct {
	App          *app.Application
	HTTP         *fiber.App
	DB           *database.DB
	SQLiteDriver *riversqlite.Driver
}

type Options struct {
	Config     config.Config
	AppOptions []app.Option
}

type Option func(*Options)

func WithServerMode(mode config.ServerMode) Option {
	return func(o *Options) {
		o.Config.Server.Mode = mode
	}
}

func WithClock(clock domain.Clock) Option {
	return func(o *Options) {
		o.AppOptions = append(o.AppOptions, app.WithClock(clock))
	}
}

func WithIDGenerator(idGen domain.IDGenerator) Option {
	return func(o *Options) {
		o.AppOptions = append(o.AppOptions, app.WithIDGenerator(idGen))
	}
}

func WithConfig(mutate func(cfg *config.Config)) Option {
	return func(o *Options) {
		mutate(&o.Config)
	}
}

const testSigningKey = "unicorn-test-signing-key-00000001"

func defaultTestConfig() config.Config {
	allowRegistration := true
	return config.Config{
		App: config.AppConfig{Env: "test"},
		Server: config.ServerConfig{
			Mode:            config.ModeAPI,
			HTTPAddr:        ":0",
			ShutdownTimeout: 5 * time.Second,
		},
		Database: config.DatabaseConfig{MaxOpenConns: 1},
		Log: config.LogConfig{
			Level:  "debug",
			Format: "json",
		},
		River: config.RiverConfig{
			MaxWorkers: 2,
		},
		Auth: config.AuthConfig{
			Issuer:            "unicorn-test",
			Secret:            testSigningKey,
			AccessTTL:         15 * time.Minute,
			Leeway:            30 * time.Second,
			BcryptCost:        4,
			AllowRegistration: &allowRegistration,
		},
	}
}

func NewTestApp(t *testing.T, opts ...Option) *TestApp {
	t.Helper()

	ctx := context.Background()
	setup := Options{Config: defaultTestConfig()}
	for _, opt := range opts {
		opt(&setup)
	}
	cfg := setup.Config

	dbPath := filepath.Join(t.TempDir(), "test.db")
	cfg.Database.URL = "sqlite://" + dbPath

	rawDB, err := database.Open(ctx, cfg.Database)
	require.NoError(t, err)
	RunTestMigrations(t, rawDB)
	require.NoError(t, rawDB.Close())

	testLog := zerolog.New(io.Discard)
	application, err := app.New(ctx, cfg, testLog, setup.AppOptions...)
	require.NoError(t, err)

	t.Cleanup(func() {
		if application.HTTPServer != nil {
			_ = application.HTTPServer.Shutdown()
		}
		_ = application.DB.Close()
	})

	return &TestApp{
		App:          application,
		HTTP:         application.HTTPServer,
		DB:           application.DB,
		SQLiteDriver: riversqlite.New(application.DB.SQLite),
	}
}

func RunTestMigrations(t *testing.T, dbConn *database.DB) {
	t.Helper()
	ctx := context.Background()

	switch dbConn.Dialect {
	case database.DialectSQLite:
		src, err := iofs.New(db.SQLiteMigrations, "sqlite/migrations")
		require.NoError(t, err)
		drv, err := sqlitemigrate.WithInstance(dbConn.SQLite, &sqlitemigrate.Config{})
		require.NoError(t, err)
		m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
		require.NoError(t, err)
		err = m.Up()
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			require.NoError(t, err)
		}

	case database.DialectPostgres:
		src, err := iofs.New(db.PostgresMigrations, "postgres/migrations")
		require.NoError(t, err)
		sqlDB := stdlib.OpenDBFromPool(dbConn.Postgres)
		t.Cleanup(func() { _ = sqlDB.Close() })
		drv, err := pgx5migrate.WithInstance(sqlDB, &pgx5migrate.Config{})
		require.NoError(t, err)
		m, err := migrate.NewWithInstance("iofs", src, "pgx5", drv)
		require.NoError(t, err)
		err = m.Up()
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			require.NoError(t, err)
		}

	default:
		t.Fatalf("unsupported dialect: %s", dbConn.Dialect)
	}

	_, err := database.MigrateRiver(ctx, dbConn, rivermigrate.DirectionUp, nil)
	require.NoError(t, err)
}

func PostgresRiverDriver(dbConn *database.DB) *riverpgxv5.Driver {
	return riverpgxv5.New(dbConn.Postgres)
}

func WorkSQLiteJob[T river.JobArgs](
	ctx context.Context,
	t *testing.T,
	ta *TestApp,
	w river.Worker[T],
	args T,
) *rivertest.WorkResult {
	t.Helper()

	tx, err := ta.DB.SQLite.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	txCtx := txctx.WithSQLTx(ctx, tx)

	testWorker := rivertest.NewWorker(t, ta.SQLiteDriver, &river.Config{}, w)
	res, err := testWorker.Work(txCtx, t, tx, args, nil)
	require.NoError(t, err)
	return res
}
