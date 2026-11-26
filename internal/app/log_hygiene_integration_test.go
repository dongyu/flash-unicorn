package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/testkit"
)

const (
	seededCanaryEmail    = "seeded.canary@example.com"
	unknownCanaryEmail   = "unknown.canary@example.com"
	registeredCanaryMail = "registered.canary@example.com"
	canaryPassword       = "canary-password-0001"
)

var authLogAllowedKeys = map[string]struct{}{
	"level":      {},
	"time":       {},
	"message":    {},
	"request_id": {},
	"user_id":    {},
	"reason":     {},
}

type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logCapture) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logCapture) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func TestAuth_LogsNeverCarryCredentialsOrEmail(t *testing.T) {
	t.Parallel()

	capture := &logCapture{}
	ta := testkit.NewTestApp(t, testkit.WithLogger(zerolog.New(capture).Level(zerolog.DebugLevel)))
	user := testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: seededCanaryEmail, Password: canaryPassword})

	require.Equal(t, http.StatusUnauthorized, loginUser(t, ta, unknownCanaryEmail, canaryPassword).StatusCode)
	require.Equal(t, http.StatusUnauthorized, loginUser(t, ta, seededCanaryEmail, "wrong-guess-password").StatusCode)

	login := loginUser(t, ta, seededCanaryEmail, canaryPassword)
	require.Equal(t, http.StatusOK, login.StatusCode)

	me := testkit.DoJSONAuth[any, dto.AuthUserResponse](
		t, ta.HTTP, http.MethodGet, "/api/v1/auth/me", login.Data.AccessToken, nil,
	)
	require.Equal(t, http.StatusOK, me.StatusCode)

	register := registerUser(t, ta, dto.RegisterRequest{Email: registeredCanaryMail, Password: canaryPassword})
	require.Equal(t, http.StatusCreated, register.StatusCode)

	refresh := testkit.DoJSON[dto.RefreshRequest, dto.AuthTokenResponse](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/refresh",
		&dto.RefreshRequest{RefreshToken: register.Data.RefreshToken},
	)
	require.Equal(t, http.StatusOK, refresh.StatusCode)

	logout := testkit.DoJSON[dto.LogoutRequest, any](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/logout",
		&dto.LogoutRequest{RefreshToken: refresh.Data.RefreshToken},
	)
	require.Equal(t, http.StatusNoContent, logout.StatusCode)

	logs := capture.String()
	require.NotEmpty(t, logs)

	for _, secret := range []string{
		seededCanaryEmail,
		unknownCanaryEmail,
		registeredCanaryMail,
		canaryPassword,
		"wrong-guess-password",
		login.Data.AccessToken,
		login.Data.RefreshToken,
		register.Data.AccessToken,
		register.Data.RefreshToken,
		refresh.Data.AccessToken,
		refresh.Data.RefreshToken,
	} {
		require.NotContains(t, logs, secret)
	}

	rejected := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "log line: %s", line)
		if entry["message"] != "login rejected" {
			continue
		}
		for key := range entry {
			require.Contains(t, authLogAllowedKeys, key, "log line: %s", line)
		}
		rejected[entry["reason"].(string)] = entry
	}

	require.Contains(t, rejected, "unknown_user")
	require.NotContains(t, rejected["unknown_user"], "user_id")
	require.Contains(t, rejected, "password_mismatch")
	require.Equal(t, user.ID.String(), rejected["password_mismatch"]["user_id"])
}
