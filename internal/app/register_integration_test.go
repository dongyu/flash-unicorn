package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/domain"
	"unicorn/internal/platform/config"
	"unicorn/internal/testkit"
)

func registerUser(t *testing.T, ta *testkit.TestApp, req dto.RegisterRequest) testkit.HTTPResult[dto.RegisterResponse] {
	t.Helper()
	return testkit.DoJSON[dto.RegisterRequest, dto.RegisterResponse](
		t, ta.HTTP, http.MethodPost, "/api/v1/auth/register", &req,
	)
}

func TestAuth_RegisterIssuesTokensImmediately(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)

	res := registerUser(t, ta, dto.RegisterRequest{
		Email:       "New.User@Example.com",
		Password:    loginPasswordFixture,
		DisplayName: "New User",
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	require.NotEmpty(t, res.Data.AccessToken)
	require.Equal(t, dto.TokenTypeBearer, res.Data.TokenType)
	require.Equal(t, int64(900), res.Data.ExpiresIn)
	require.Equal(t, dto.AuthUserResponse{
		Email:       "new.user@example.com",
		DisplayName: "New User",
		Role:        domain.RoleUser,
		Status:      domain.UserStatusActive,
	}, res.Data.User)
	require.NotEmpty(t, res.Data.RefreshToken)

	claims := decodeAccessToken(t, res.Data.AccessToken)
	require.Equal(t, "unicorn-test", claims["iss"])
	require.Equal(t, []any{string(domain.RoleUser)}, claims["roles"])
	require.NotEmpty(t, claims["sub"])

	me := testkit.DoJSONAuth[any, dto.AuthUserResponse](
		t, ta.HTTP, http.MethodGet, "/api/v1/auth/me", res.Data.AccessToken, nil,
	)
	require.Equal(t, http.StatusOK, me.StatusCode)
	require.Equal(t, res.Data.User, me.Data)

	login := loginUser(t, ta, "new.user@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)
}

func TestAuth_RegisterDuplicateEmailConflicts(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	testkit.SeedUser(t, ta, testkit.SeedUserParams{
		Email:    "taken@example.com",
		Password: loginPasswordFixture,
	})

	res := registerUser(t, ta, dto.RegisterRequest{
		Email:    "TAKEN@example.com",
		Password: loginPasswordFixture,
	})
	problem := testkit.RequireProblem(t, res, http.StatusConflict)
	require.Equal(t, http.StatusText(http.StatusConflict), problem.Title)
	require.Equal(t, "resource conflict", problem.Detail)
	require.NotContains(t, string(res.RawBody), "password")
}

func TestAuth_RegisterEnforcesPasswordByteBounds(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)

	tests := []struct {
		name       string
		password   string
		wantStatus int
	}{
		{name: "shorter than eight bytes", password: strings.Repeat("a", 7), wantStatus: http.StatusUnprocessableEntity},
		{name: "exactly eight bytes", password: strings.Repeat("a", 8), wantStatus: http.StatusCreated},
		{name: "exactly seventy-two bytes", password: strings.Repeat("a", 72), wantStatus: http.StatusCreated},
		{name: "longer than seventy-two bytes", password: strings.Repeat("a", 73), wantStatus: http.StatusUnprocessableEntity},
		{name: "multibyte under seventy-two bytes", password: strings.Repeat("é", 25), wantStatus: http.StatusCreated},
		{name: "multibyte over seventy-two bytes", password: strings.Repeat("é", 40), wantStatus: http.StatusUnprocessableEntity},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := registerUser(t, ta, dto.RegisterRequest{
				Email:    fmt.Sprintf("user%d@example.com", i),
				Password: tt.password,
			})
			if tt.wantStatus == http.StatusCreated {
				require.Equal(t, http.StatusCreated, res.StatusCode)
				return
			}
			problem := testkit.RequireProblem(t, res, tt.wantStatus)
			require.Len(t, problem.Errors, 1)
			require.Equal(t, "password", problem.Errors[0].Field)
			require.Contains(t, problem.Errors[0].Message, "8 and 72 bytes")
		})
	}
}

func TestAuth_RegisterAbsentWhenDisabled(t *testing.T) {
	t.Parallel()

	disabled := false
	ta := testkit.NewTestApp(t, testkit.WithConfig(func(cfg *config.Config) {
		cfg.Auth.AllowRegistration = &disabled
	}))

	res := registerUser(t, ta, dto.RegisterRequest{
		Email:    "new.user@example.com",
		Password: loginPasswordFixture,
	})
	testkit.RequireProblem(t, res, http.StatusNotFound)

	testkit.SeedUser(t, ta, testkit.SeedUserParams{Email: "seeded@example.com", Password: loginPasswordFixture})
	login := loginUser(t, ta, "seeded@example.com", loginPasswordFixture)
	require.Equal(t, http.StatusOK, login.StatusCode)
}
