package database

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivermigrate"
	_ "modernc.org/sqlite"

	"unicorn/internal/platform/config"
)

type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectSQLite   Dialect = "sqlite"
)

type DB struct {
	Dialect  Dialect
	Postgres *pgxpool.Pool
	SQLite   *sql.DB
}

func ParseDialect(dsn string) (Dialect, error) {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return DialectPostgres, nil
	case strings.HasPrefix(dsn, "sqlite://"):
		return DialectSQLite, nil
	default:
		return "", fmt.Errorf("unsupported database url scheme: %q", dsn)
	}
}

func MigrateURL(dsn string) (string, error) {
	dialect, err := ParseDialect(dsn)
	if err != nil {
		return "", err
	}
	switch dialect {
	case DialectPostgres:
		if strings.HasPrefix(dsn, "postgresql://") {
			return "pgx5://" + strings.TrimPrefix(dsn, "postgresql://"), nil
		}
		return "pgx5://" + strings.TrimPrefix(dsn, "postgres://"), nil
	case DialectSQLite:
		return dsn, nil
	default:
		return "", fmt.Errorf("unsupported dialect %q", dialect)
	}
}

func Open(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	dialect, err := ParseDialect(cfg.URL)
	if err != nil {
		return nil, err
	}

	switch dialect {
	case DialectPostgres:
		poolCfg, err := pgxpool.ParseConfig(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("parse postgres dsn: %w", err)
		}
		if cfg.MaxOpenConns > 0 {
			if cfg.MaxOpenConns > math.MaxInt32 {
				return nil, fmt.Errorf("max open conns %d is outside the int32 range", cfg.MaxOpenConns)
			}
			poolCfg.MaxConns = int32(cfg.MaxOpenConns)
		}
		poolCfg.MaxConnLifetime = 30 * time.Minute
		poolCfg.MaxConnIdleTime = 5 * time.Minute
		poolCfg.HealthCheckPeriod = time.Minute

		pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
		if err != nil {
			return nil, fmt.Errorf("open postgres pool: %w", err)
		}
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		return &DB{Dialect: DialectPostgres, Postgres: pool}, nil

	case DialectSQLite:
		sqliteDSN, err := buildSQLiteDriverDSN(cfg.URL)
		if err != nil {
			return nil, err
		}
		sqlDB, err := sql.Open("sqlite", sqliteDSN)
		if err != nil {
			return nil, fmt.Errorf("open sqlite db: %w", err)
		}
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetConnMaxLifetime(0)
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(pingCtx); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("ping sqlite: %w", err)
		}
		return &DB{Dialect: DialectSQLite, SQLite: sqlDB}, nil

	default:
		return nil, fmt.Errorf("unsupported dialect %q", dialect)
	}
}

func buildSQLiteDriverDSN(raw string) (string, error) {
	trimmed := strings.TrimPrefix(raw, "sqlite://")
	pathPart, rawQuery, hasQuery := strings.Cut(trimmed, "?")
	if pathPart == "" {
		return "", fmt.Errorf("empty sqlite path in %q", raw)
	}

	isMemory := pathPart == ":memory:" || strings.HasPrefix(pathPart, "file::memory:")
	if !isMemory {
		dir := filepath.Dir(pathPart)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return "", fmt.Errorf("create sqlite parent dir %q: %w", dir, err)
			}
		}
	}

	queryValues := url.Values{}
	if hasQuery {
		parsed, err := url.ParseQuery(rawQuery)
		if err != nil {
			return "", fmt.Errorf("parse sqlite query params: %w", err)
		}
		queryValues = parsed
	}

	if !hasPragma(queryValues, "busy_timeout") {
		queryValues.Add("_pragma", "busy_timeout(5000)")
	}
	if !hasPragma(queryValues, "foreign_keys") {
		queryValues.Add("_pragma", "foreign_keys(1)")
	}
	if !isMemory && !hasPragma(queryValues, "journal_mode") {
		queryValues.Add("_pragma", "journal_mode(WAL)")
	}

	fileTarget := pathPart
	if !strings.HasPrefix(fileTarget, "file:") {
		fileTarget = "file:" + fileTarget
	}
	if encoded := queryValues.Encode(); encoded != "" {
		return fileTarget + "?" + encoded, nil
	}
	return fileTarget, nil
}

func hasPragma(values url.Values, prefix string) bool {
	for _, p := range values["_pragma"] {
		if strings.HasPrefix(strings.ToLower(p), strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

func (d *DB) Ping(ctx context.Context) error {
	switch d.Dialect {
	case DialectPostgres:
		return d.Postgres.Ping(ctx)
	case DialectSQLite:
		return d.SQLite.PingContext(ctx)
	default:
		return fmt.Errorf("unknown dialect %q", d.Dialect)
	}
}

func (d *DB) Close() error {
	if d == nil {
		return nil
	}
	if d.Postgres != nil {
		d.Postgres.Close()
	}
	if d.SQLite != nil {
		return d.SQLite.Close()
	}
	return nil
}

func MigrateRiver(ctx context.Context, db *DB, direction rivermigrate.Direction, opts *rivermigrate.MigrateOpts) (*rivermigrate.MigrateResult, error) {
	switch db.Dialect {
	case DialectPostgres:
		migrator, err := rivermigrate.New(riverpgxv5.New(db.Postgres), nil)
		if err != nil {
			return nil, fmt.Errorf("init postgres river migrator: %w", err)
		}
		return migrator.Migrate(ctx, direction, opts)
	case DialectSQLite:
		migrator, err := rivermigrate.New(riversqlite.New(db.SQLite), nil)
		if err != nil {
			return nil, fmt.Errorf("init sqlite river migrator: %w", err)
		}
		return migrator.Migrate(ctx, direction, opts)
	default:
		return nil, fmt.Errorf("unsupported dialect %q", db.Dialect)
	}
}

func ExistingRiverVersions(ctx context.Context, db *DB) ([]rivermigrate.Migration, error) {
	switch db.Dialect {
	case DialectPostgres:
		migrator, err := rivermigrate.New(riverpgxv5.New(db.Postgres), nil)
		if err != nil {
			return nil, err
		}
		return migrator.ExistingVersions(ctx)
	case DialectSQLite:
		migrator, err := rivermigrate.New(riversqlite.New(db.SQLite), nil)
		if err != nil {
			return nil, err
		}
		return migrator.ExistingVersions(ctx)
	default:
		return nil, fmt.Errorf("unsupported dialect %q", db.Dialect)
	}
}
