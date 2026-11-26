package logger

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"unicorn/internal/platform/config"
)

type TraceIDExtractor func(ctx context.Context) string

func New(cfg config.LogConfig, out io.Writer) zerolog.Logger {
	if out == nil {
		out = os.Stdout
	}

	level, err := zerolog.ParseLevel(strings.ToLower(cfg.Level))
	if err != nil {
		level = zerolog.InfoLevel
	}

	writer := out
	if strings.EqualFold(cfg.Format, "console") {
		writer = zerolog.ConsoleWriter{
			Out:        out,
			TimeFormat: time.RFC3339,
		}
	}

	return zerolog.New(writer).
		Level(level).
		With().
		Timestamp().
		Logger()
}

func EnrichContext(ctx context.Context, log zerolog.Logger, extractor TraceIDExtractor) context.Context {
	if extractor != nil {
		if traceID := extractor(ctx); traceID != "" {
			log = log.With().Str("trace_id", traceID).Logger()
		}
	}
	return log.WithContext(ctx)
}
