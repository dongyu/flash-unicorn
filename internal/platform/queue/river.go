package queue

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"

	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
)

type LoggingMiddleware struct {
	river.MiddlewareDefaults
	BaseLogger zerolog.Logger
}

func (m *LoggingMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	start := time.Now()
	jobLog := m.BaseLogger.With().
		Int64("job_id", job.ID).
		Str("job_kind", job.Kind).
		Str("queue", job.Queue).
		Int("attempt", job.Attempt).
		Logger()

	hub := sentry.CurrentHub().Clone()
	hub.Scope().SetTags(map[string]string{
		"job_id":   strconv.FormatInt(job.ID, 10),
		"job_kind": job.Kind,
		"queue":    job.Queue,
		"attempt":  strconv.Itoa(job.Attempt),
	})
	ctx = sentry.SetHubOnContext(ctx, hub)
	ctx = jobLog.WithContext(ctx)

	err := doInner(ctx)
	latency := time.Since(start)

	if err != nil {
		jobLog.Error().Err(err).Dur("latency", latency).Msg("river job failed")
		return err
	}
	jobLog.Info().Dur("latency", latency).Msg("river job completed")
	return nil
}

type riverErrorHandler struct {
	log zerolog.Logger
}

func (h *riverErrorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	h.log.Error().
		Err(err).
		Int64("job_id", job.ID).
		Str("job_kind", job.Kind).
		Int("attempt", job.Attempt).
		Msg("river error handler invoked")

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("job_id", strconv.FormatInt(job.ID, 10))
		scope.SetTag("job_kind", job.Kind)
		scope.SetContext("river_job", sentry.Context{
			"id":      job.ID,
			"kind":    job.Kind,
			"queue":   job.Queue,
			"attempt": job.Attempt,
		})
		hub.CaptureException(err)
	})
	return nil
}

func (h *riverErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	h.log.Error().
		Interface("panic", panicVal).
		Str("stack", trace).
		Int64("job_id", job.ID).
		Str("job_kind", job.Kind).
		Msg("river worker panicked")

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("job_id", strconv.FormatInt(job.ID, 10))
		scope.SetTag("job_kind", job.Kind)
		scope.SetContext("river_job", sentry.Context{
			"id":      job.ID,
			"kind":    job.Kind,
			"queue":   job.Queue,
			"attempt": job.Attempt,
			"stack":   trace,
		})
		hub.RecoverWithContext(ctx, panicVal)
	})
	return nil
}

type zerologSlogHandler struct {
	log   zerolog.Logger
	attrs []slog.Attr
}

func (h *zerologSlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return mapSlogLevel(level) >= h.log.GetLevel()
}

func (h *zerologSlogHandler) Handle(_ context.Context, r slog.Record) error {
	ev := h.log.WithLevel(mapSlogLevel(r.Level))
	for _, a := range h.attrs {
		ev = ev.Any(a.Key, a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		ev = ev.Any(a.Key, a.Value.Any())
		return true
	})
	ev.Msg(r.Message)
	return nil
}

func (h *zerologSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &zerologSlogHandler{log: h.log, attrs: merged}
}

func (h *zerologSlogHandler) WithGroup(name string) slog.Handler {
	return &zerologSlogHandler{
		log:   h.log.With().Str("slog_group", name).Logger(),
		attrs: h.attrs,
	}
}

func mapSlogLevel(l slog.Level) zerolog.Level {
	switch {
	case l >= slog.LevelError:
		return zerolog.ErrorLevel
	case l >= slog.LevelWarn:
		return zerolog.WarnLevel
	case l >= slog.LevelInfo:
		return zerolog.InfoLevel
	default:
		return zerolog.DebugLevel
	}
}

type Client struct {
	Dialect  database.Dialect
	Postgres *river.Client[pgx.Tx]
	SQLite   *river.Client[*sql.Tx]
}

func NewClient(
	db *database.DB,
	cfg config.RiverConfig,
	workers *river.Workers,
	log zerolog.Logger,
	registerWorkers bool,
) (*Client, error) {
	riverLog := log.With().Str("component", "river").Logger()
	riverCfg := &river.Config{
		Logger:       slog.New(&zerologSlogHandler{log: riverLog}),
		ErrorHandler: &riverErrorHandler{log: riverLog},
		Middleware: []rivertype.Middleware{
			&LoggingMiddleware{BaseLogger: log},
		},
	}
	if registerWorkers {
		riverCfg.Queues = map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: cfg.MaxWorkers},
		}
		riverCfg.Workers = workers
	}

	switch db.Dialect {
	case database.DialectPostgres:
		client, err := river.NewClient(riverpgxv5.New(db.Postgres), riverCfg)
		if err != nil {
			return nil, fmt.Errorf("create postgres river client: %w", err)
		}
		return &Client{Dialect: database.DialectPostgres, Postgres: client}, nil

	case database.DialectSQLite:
		client, err := river.NewClient(riversqlite.New(db.SQLite), riverCfg)
		if err != nil {
			return nil, fmt.Errorf("create sqlite river client: %w", err)
		}
		return &Client{Dialect: database.DialectSQLite, SQLite: client}, nil

	default:
		return nil, fmt.Errorf("unsupported dialect %q", db.Dialect)
	}
}

func (c *Client) Start(ctx context.Context) error {
	switch c.Dialect {
	case database.DialectPostgres:
		return c.Postgres.Start(ctx)
	case database.DialectSQLite:
		return c.SQLite.Start(ctx)
	default:
		return fmt.Errorf("unsupported dialect %q", c.Dialect)
	}
}

func (c *Client) Stop(ctx context.Context) error {
	switch c.Dialect {
	case database.DialectPostgres:
		return c.Postgres.Stop(ctx)
	case database.DialectSQLite:
		return c.SQLite.Stop(ctx)
	default:
		return fmt.Errorf("unsupported dialect %q", c.Dialect)
	}
}
