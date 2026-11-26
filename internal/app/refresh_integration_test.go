package app_test

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/testkit"
)

type refreshResponse struct {
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	TokenType    string           `json:"token_type"`
	ExpiresIn    int64            `json:"expires_in"`
	User         authUserResponse `json:"user"`
}

func refreshTokens(t *testing.T, ta *testkit.TestApp, refreshToken string) testkit.HTTPResult[refreshResponse] {
	t.Helper()
	return testkit.DoJSON[dto.RefreshRequest, refreshResponse](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/refresh",
		&dto.RefreshRequest{RefreshToken: refreshToken},
	)
}

func logoutTokens(t *testing.T, ta *testkit.TestApp, req dto.LogoutRequest) testkit.HTTPResult[any] {
	t.Helper()
	return testkit.DoJSON[dto.LogoutRequest, any](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/logout", &req,
	)
}

func disableUser(t *testing.T, ta *testkit.TestApp, userID uuid.UUID) {
	t.Helper()
	_, err := ta.DB.SQLite.ExecContext(t.Context(),
		"UPDATE users SET status = 'disabled' WHERE id = ?", userID.String())
	require.NoError(t, err)
}

func TestAuth_LoginIssuesRefreshToken(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	login := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)
	require.NotEmpty(t, login.Data.RefreshToken)
	require.NotEqual(t, login.Data.AccessToken, login.Data.RefreshToken)
}

func TestAuth_RefreshRotatesTokenPair(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{
		Email:       "user@example.com",
		Password:    loginPasswordFixture,
		DisplayName: "Refresh User",
	})

	login := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)

	refreshed := refreshTokens(t, ta, login.Data.RefreshToken)
	require.Equal(t, http.StatusOK, refreshed.StatusCode)
	require.NotEmpty(t, refreshed.Data.AccessToken)
	require.NotEmpty(t, refreshed.Data.RefreshToken)
	require.NotEqual(t, login.Data.RefreshToken, refreshed.Data.RefreshToken)
	require.Equal(t, dto.TokenTypeBearer, refreshed.Data.TokenType)
	require.Equal(t, int64(900), refreshed.Data.ExpiresIn)
	require.Equal(t, login.Data.User, refreshed.Data.User)

	me := testkit.DoJSONAuth[any, authUserResponse](
		t, ta.HTTP, http.MethodGet, "/api/v1/auth/me", refreshed.Data.AccessToken, nil,
	)
	require.Equal(t, http.StatusOK, me.StatusCode)
	require.Equal(t, login.Data.User, me.Data)

	rotated := refreshTokens(t, ta, refreshed.Data.RefreshToken)
	require.Equal(t, http.StatusOK, rotated.StatusCode)
	require.NotEqual(t, refreshed.Data.RefreshToken, rotated.Data.RefreshToken)
}

func TestAuth_RefreshRejectsReplayedTokenAndRevokesFamily(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	first := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, first.StatusCode)
	second := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, second.StatusCode)

	rotated := refreshTokens(t, ta, first.Data.RefreshToken)
	require.Equal(t, http.StatusOK, rotated.StatusCode)

	replay := refreshTokens(t, ta, first.Data.RefreshToken)
	problem := testkit.RequireProblem(t, replay, http.StatusUnauthorized)
	require.Equal(t, http.StatusText(http.StatusUnauthorized), problem.Title)
	require.Equal(t, "Bearer", replay.Header.Get("WWW-Authenticate"))
	require.NotContains(t, string(replay.RawBody), first.Data.RefreshToken)

	for _, token := range []string{rotated.Data.RefreshToken, second.Data.RefreshToken} {
		res := refreshTokens(t, ta, token)
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	}
}

func TestAuth_RefreshRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	clock := testkit.NewFakeClock(time.Date(2031, 6, 1, 12, 0, 0, 0, time.UTC))
	ta := testkit.NewTestApp(t, testkit.WithClock(clock))
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	login := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)

	clock.Advance(720*time.Hour + time.Second)

	expired := refreshTokens(t, ta, login.Data.RefreshToken)
	testkit.RequireProblem(t, expired, http.StatusUnauthorized)
}

func TestAuth_RefreshRejectsUnknownToken(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)

	unknown := refreshTokens(t, ta, uuid.NewString())
	testkit.RequireProblem(t, unknown, http.StatusUnauthorized)
}

func TestAuth_RefreshRejectsDisabledUser(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	user := testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	login := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)

	disableUser(t, ta, user.ID)

	disabled := refreshTokens(t, ta, login.Data.RefreshToken)
	testkit.RequireProblem(t, disabled, http.StatusUnauthorized)
}

func TestAuth_RefreshAllowsOnlyOneConcurrentRotation(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	login := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)

	const attempts = 2
	statuses := make([]int, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			res := testkit.DoJSON[dto.RefreshRequest, refreshResponse](
				t, ta.HTTP, http.MethodPost, "/api/v1/auth/refresh",
				&dto.RefreshRequest{RefreshToken: login.Data.RefreshToken},
			)
			statuses[index] = res.StatusCode
		}(i)
	}
	wg.Wait()

	okCount := 0
	for _, status := range statuses {
		switch status {
		case http.StatusOK:
			okCount++
		case http.StatusUnauthorized:
		default:
			require.Failf(t, "unexpected refresh status", "status = %d", status)
		}
	}
	require.Equal(t, 1, okCount, "statuses = %v", statuses)
}

func TestAuth_LogoutRevokesOnlyPresentedToken(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	first := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	second := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, first.StatusCode)
	require.Equal(t, http.StatusOK, second.StatusCode)

	logout := logoutTokens(t, ta, dto.LogoutRequest{RefreshToken: first.Data.RefreshToken})
	require.Equal(t, http.StatusNoContent, logout.StatusCode)
	require.Empty(t, logout.RawBody)

	stillValid := refreshTokens(t, ta, second.Data.RefreshToken)
	require.Equal(t, http.StatusOK, stillValid.StatusCode)

	revoked := refreshTokens(t, ta, first.Data.RefreshToken)
	testkit.RequireProblem(t, revoked, http.StatusUnauthorized)
}

func TestAuth_LogoutAllRevokesEverySession(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	first := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	second := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, first.StatusCode)
	require.Equal(t, http.StatusOK, second.StatusCode)

	logout := logoutTokens(t, ta, dto.LogoutRequest{RefreshToken: second.Data.RefreshToken, All: true})
	require.Equal(t, http.StatusNoContent, logout.StatusCode)

	for _, token := range []string{first.Data.RefreshToken, second.Data.RefreshToken} {
		res := refreshTokens(t, ta, token)
		testkit.RequireProblem(t, res, http.StatusUnauthorized)
	}
}

func TestAuth_LogoutIsIdempotentForUnknownToken(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)

	single := logoutTokens(t, ta, dto.LogoutRequest{RefreshToken: uuid.NewString()})
	require.Equal(t, http.StatusNoContent, single.StatusCode)

	all := logoutTokens(t, ta, dto.LogoutRequest{RefreshToken: uuid.NewString(), All: true})
	require.Equal(t, http.StatusNoContent, all.StatusCode)
}

func TestAuth_RefreshAndLogoutRequireTokenInBody(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)

	refresh := testkit.DoJSON[dto.RefreshRequest, any](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/refresh", &dto.RefreshRequest{},
	)
	testkit.RequireProblem(t, refresh, http.StatusUnprocessableEntity)

	logout := testkit.DoJSON[dto.LogoutRequest, any](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/logout", &dto.LogoutRequest{},
	)
	testkit.RequireProblem(t, logout, http.StatusUnprocessableEntity)
}
