package app_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/testkit"
)

const credentialRateLimitPerMinute = 10

func TestAuth_LoginIsRateLimitedPerIP(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	for attempt := 1; attempt <= credentialRateLimitPerMinute; attempt++ {
		res := loginUser(t, ta, "user@example.com", loginPasswordFixture)
		require.Equal(t, http.StatusOK, res.StatusCode, "login attempt %d", attempt)
	}

	blocked := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	problem := testkit.RequireProblem(t, blocked, http.StatusTooManyRequests)
	require.Equal(t, "Too Many Requests", problem.Title)
	require.Equal(t, "/api/v1/auth/login", problem.Instance)
	require.NotEmpty(t, problem.RequestID)
	require.NotEmpty(t, blocked.Header.Get("Retry-After"))
}

func TestAuth_RegisterConsumesTheSameCredentialRateLimit(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	for attempt := 1; attempt < credentialRateLimitPerMinute; attempt++ {
		res := registerUser(t, ta, dto.RegisterRequest{
			Email:    fmt.Sprintf("rate.limit%d@example.com", attempt),
			Password: loginPasswordFixture,
		})
		require.Equal(t, http.StatusCreated, res.StatusCode, "register attempt %d", attempt)
	}

	require.Equal(t, http.StatusOK, loginUser(t, ta, "user@example.com", loginPasswordFixture).StatusCode)

	blocked := registerUser(t, ta, dto.RegisterRequest{Email: "rate.limit.overflow@example.com", Password: loginPasswordFixture})
	testkit.RequireProblem(t, blocked, http.StatusTooManyRequests)
}

func TestAuth_RateLimitLeavesNonCredentialRoutesAlone(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	login := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)

	for attempt := 1; attempt < credentialRateLimitPerMinute; attempt++ {
		res := loginUser(t, ta, "user@example.com", "wrong-guess-password")
		require.Equal(t, http.StatusUnauthorized, res.StatusCode, "failed login attempt %d", attempt)
	}
	testkit.RequireProblem(t, loginUser(t, ta, "user@example.com", "wrong-guess-password"), http.StatusTooManyRequests)

	me := testkit.DoJSONAuth[any, dto.AuthUserResponse](
		t, ta.HTTP, http.MethodGet, "/api/v1/auth/me", login.Data.AccessToken, nil,
	)
	require.Equal(t, http.StatusOK, me.StatusCode)

	notes := testkit.DoJSONAuth[any, any](
		t, ta.HTTP, http.MethodGet, "/api/v1/notes", login.Data.AccessToken, nil,
	)
	require.Equal(t, http.StatusOK, notes.StatusCode)

	refresh := testkit.DoJSON[dto.RefreshRequest, dto.RefreshResponse](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/refresh",
		&dto.RefreshRequest{RefreshToken: login.Data.RefreshToken},
	)
	require.Equal(t, http.StatusOK, refresh.StatusCode)
}
