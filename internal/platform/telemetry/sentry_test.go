package telemetry_test

import (
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"

	"unicorn/internal/platform/buildinfo"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/telemetry"
)

func TestInitSentry_DropsRequestBodyFromEvents(t *testing.T) {
	cfg := config.SentryConfig{
		DSN:          "https://public@example.invalid/1",
		FlushTimeout: time.Second,
	}
	flush, err := telemetry.InitSentry(cfg, "test", buildinfo.Info{Version: "test"})
	require.NoError(t, err)
	t.Cleanup(flush)

	beforeSend := sentry.CurrentHub().Client().Options().BeforeSend
	require.NotNil(t, beforeSend)

	event := beforeSend(&sentry.Event{
		Message: "boom",
		Request: &sentry.Request{
			URL:  "http://example.test/api/v1/auth/login",
			Data: `{"password":"leaked-password","refresh_token":"leaked-token"}`,
		},
	}, nil)

	require.NotNil(t, event)
	require.Empty(t, event.Request.Data)
	require.Equal(t, "http://example.test/api/v1/auth/login", event.Request.URL)
}
