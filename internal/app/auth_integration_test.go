package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/domain"
	"unicorn/internal/platform/config"
	"unicorn/internal/testkit"
)

const loginPasswordFixture = "correct-horse-battery"

type authUserResponse struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

type loginResponse struct {
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	TokenType    string           `json:"token_type"`
	ExpiresIn    int64            `json:"expires_in"`
	User         authUserResponse `json:"user"`
}

func loginUser(t *testing.T, ta *testkit.TestApp, email, password string) testkit.HTTPResult[loginResponse] {
	t.Helper()
	return testkit.DoJSON[dto.LoginRequest, loginResponse](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/login",
		&dto.LoginRequest{Email: email, Password: password},
	)
}

func decodeAccessToken(t *testing.T, value string) jwt.MapClaims {
	t.Helper()
	token, _, err := jwt.NewParser().ParseUnverified(value, jwt.MapClaims{})
	require.NoError(t, err)
	claims, ok := token.Claims.(jwt.MapClaims)
	require.True(t, ok)
	return claims
}

func TestAuth_LoginMeAndProtectedNotes(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	user := testkit.SeedUser(t, ta, testkit.SeedUserParams{
		Email:       "User@Example.com",
		Password:    loginPasswordFixture,
		DisplayName: "Test User",
	})

	login := loginUser(t, ta, "USER@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)
	require.NotEmpty(t, login.Data.AccessToken)
	require.Equal(t, dto.TokenTypeBearer, login.Data.TokenType)
	require.Equal(t, int64(900), login.Data.ExpiresIn)
	require.Equal(t, authUserResponse{
		Email:       "user@example.com",
		DisplayName: "Test User",
		Role:        string(domain.RoleUser),
		Status:      string(domain.UserStatusActive),
	}, login.Data.User)

	claims := decodeAccessToken(t, login.Data.AccessToken)
	require.Equal(t, "unicorn-test", claims["iss"])
	require.Equal(t, user.ID.String(), claims["sub"])
	require.Equal(t, []any{string(domain.RoleUser)}, claims["roles"])
	require.NotEmpty(t, claims["jti"])

	token := login.Data.AccessToken

	me := testkit.DoJSONAuth[any, authUserResponse](t, ta.HTTP, http.MethodGet, "/api/v1/auth/me", token, nil)
	require.Equal(t, http.StatusOK, me.StatusCode)
	require.Equal(t, login.Data.User, me.Data)

	create := testkit.DoJSONAuth[dto.CreateNoteRequest, noteResponse](
		t, ta.HTTP, http.MethodPost, "/api/v1/notes", token,
		&dto.CreateNoteRequest{Title: "Authenticated", Body: "protected"},
	)
	require.Equal(t, http.StatusCreated, create.StatusCode)
	require.Equal(t, "Authenticated", create.Data.Title)

	get := testkit.DoJSONAuth[any, noteResponse](
		t, ta.HTTP, http.MethodGet, "/api/v1/notes/"+create.Data.ID.String(), token, nil,
	)
	require.Equal(t, http.StatusOK, get.StatusCode)
	require.Equal(t, create.Data.ID, get.Data.ID)

	list := testkit.DoJSONAuth[any, any](t, ta.HTTP, http.MethodGet, "/api/v1/notes", token, nil)
	require.Equal(t, http.StatusOK, list.StatusCode)
}

func TestAuth_LoginFailuresShareOneResponse(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})
	testkit.SeedUser(t, ta, testkit.SeedUserParams{
		Email:    "disabled@example.com",
		Password: loginPasswordFixture,
		Status:   domain.UserStatusDisabled,
	})
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "external@example.com"})

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{name: "unknown user", email: "missing@example.com", password: loginPasswordFixture},
		{name: "wrong password", email: "user@example.com", password: "definitely-wrong"},
		{name: "disabled user", email: "disabled@example.com", password: loginPasswordFixture},
		{name: "missing credential", email: "external@example.com", password: loginPasswordFixture},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := loginUser(t, ta, tt.email, tt.password)
			problem := testkit.RequireProblem(t, res, http.StatusUnauthorized)
			require.Equal(t, "Bearer", res.Header.Get("WWW-Authenticate"))
			require.Equal(t, "Unauthorized", problem.Title)
			require.Equal(t, "invalid credentials", problem.Detail)
		})
	}
}

func TestAuth_NotesRequireBearerToken(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)

	missing := testkit.DoJSON[any, any](t, ta.HTTP, http.MethodGet, "/api/v1/notes", nil)
	problem := testkit.RequireProblem(t, missing, http.StatusUnauthorized)
	require.Equal(t, "Bearer", missing.Header.Get("WWW-Authenticate"))
	require.NotEmpty(t, problem.RequestID)

	malformed := testkit.DoJSONAuth[any, any](t, ta.HTTP, http.MethodGet, "/api/v1/notes", "not-a-token", nil)
	testkit.RequireProblem(t, malformed, http.StatusUnauthorized)
	require.Equal(t, "Bearer", malformed.Header.Get("WWW-Authenticate"))
}

func TestAuth_ExpiredAccessTokenIsRejected(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2031, 2, 3, 4, 5, 6, 0, time.UTC)
	clock := testkit.NewFakeClock(fixedNow)
	ta := testkit.NewTestApp(t,
		testkit.WithClock(clock),
		testkit.WithConfig(func(cfg *config.Config) {
			cfg.Auth.AccessTTL = time.Minute
		}),
	)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "user@example.com", Password: loginPasswordFixture})

	login := loginUser(t, ta, "user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)
	require.Equal(t, int64(60), login.Data.ExpiresIn)

	clock.Advance(2 * time.Minute)

	me := testkit.DoJSONAuth[any, any](t, ta.HTTP, http.MethodGet, "/api/v1/auth/me", login.Data.AccessToken, nil)
	testkit.RequireProblem(t, me, http.StatusUnauthorized)
	require.Equal(t, "Bearer", me.Header.Get("WWW-Authenticate"))
}

func TestAuth_LoginRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)

	res := testkit.DoJSON[dto.LoginRequest, any](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/login",
		&dto.LoginRequest{Email: "not-an-email"},
	)
	problem := testkit.RequireProblem(t, res, http.StatusUnprocessableEntity)
	require.Len(t, problem.Errors, 2)
}
