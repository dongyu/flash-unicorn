package telemetry

import (
	"fmt"

	"github.com/getsentry/sentry-go"

	"unicorn/internal/platform/buildinfo"
	"unicorn/internal/platform/config"
)

func InitSentry(cfg config.SentryConfig, appEnv string, bi buildinfo.Info) (func(), error) {
	if cfg.DSN == "" {
		return func() {}, nil
	}

	env := cfg.Environment
	if env == "" {
		env = appEnv
	}

	release := bi.Version
	if bi.Commit != "" && bi.Commit != "none" {
		release = fmt.Sprintf("%s@%s", bi.Version, bi.Commit)
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      env,
		Release:          release,
		EnableTracing:    cfg.EnableTracing,
		TracesSampleRate: cfg.TracesSampleRate,
		AttachStacktrace: true,
		BeforeSend:       dropRequestBody,
	})
	if err != nil {
		return nil, fmt.Errorf("init sentry: %w", err)
	}

	return func() {
		sentry.Flush(cfg.FlushTimeout)
	}, nil
}

func dropRequestBody(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event.Request != nil {
		event.Request.Data = ""
	}
	return event
}
