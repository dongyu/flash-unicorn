package handler_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	httpadapter "unicorn/internal/adapter/http"
	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/adapter/http/handler"
	"unicorn/internal/domain"
	"unicorn/internal/testkit"
	usecasemock "unicorn/internal/testkit/mocks/usecase"
	"unicorn/internal/usecase"
)

const (
	authPasswordMessage   = "password must be between 8 and 72 bytes"
	authTestEmail         = "user@example.com"
	authTestPassword      = "correct-horse-battery"
	authTestAccessToken   = "signed-access-token"
	authTestRefreshValue  = "opaque-refresh-token"
	authTestExpiresIn     = int64(900)
	authTestDisplayName   = "Test User"
	registerPath          = "/api/v1/auth/register"
	loginPath             = "/api/v1/auth/login"
	refreshPath           = "/api/v1/auth/refresh"
	mePath                = "/api/v1/auth/me"
	authTestRegisterBody  = `{"email":"user@example.com","password":"correct-horse-battery"}`
	authTestLoginBody     = `{"email":"user@example.com","password":"correct-horse-battery"}`
	authTestRefreshBody   = `{"refresh_token":"opaque-refresh-token"}`
	authTestTokenResponse = `{"access_token":"signed-access-token","refresh_token":"opaque-refresh-token","token_type":"Bearer","expires_in":900,"user":{"email":"user@example.com","display_name":"Test User","role":"user","status":"active"}}`
)

func newTestAuthRouter(t *testing.T, allowRegistration bool) (*fiber.App, *usecasemock.MockAuthUseCase) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockUC := usecasemock.NewMockAuthUseCase(ctrl)
	app := httpadapter.NewRouter(httpadapter.RouterDeps{
		Logger: zerolog.New(io.Discard),
		Routes: []httpadapter.RouteRegistrar{
			handler.NewAuthHandler(mockUC, handler.NewValidator(), allowRequest, allowRequest, allowRegistration),
		},
	})
	return app, mockUC
}

func authTestResult() usecase.AuthResult {
	return usecase.AuthResult{
		AccessToken: domain.AccessToken{
			Value:     authTestAccessToken,
			ExpiresAt: time.Date(2026, 12, 1, 10, 15, 0, 0, time.UTC),
		},
		RefreshToken: authTestRefreshValue,
		ExpiresIn:    authTestExpiresIn,
		User: domain.User{
			Email:       authTestEmail,
			DisplayName: authTestDisplayName,
			Role:        domain.RoleUser,
			Status:      domain.UserStatusActive,
		},
	}
}

func doRawJSON[T any](t *testing.T, app *fiber.App, target, body string) testkit.HTTPResult[T] {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return testkit.HTTPResult[T]{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		RawBody:    raw,
	}
}

func TestAuthHandler_Register_PasswordLengthBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     int
	}{
		{name: "7 bytes rejected", password: "1234567", want: http.StatusUnprocessableEntity},
		{name: "8 bytes accepted", password: "12345678", want: http.StatusCreated},
		{name: "72 bytes accepted", password: strings.Repeat("a", 72), want: http.StatusCreated},
		{name: "73 bytes rejected", password: strings.Repeat("a", 73), want: http.StatusUnprocessableEntity},
		{name: "2 runes 6 bytes rejected", password: strings.Repeat("密", 2), want: http.StatusUnprocessableEntity},
		{name: "3 runes 9 bytes accepted", password: strings.Repeat("密", 3), want: http.StatusCreated},
		{name: "24 runes 72 bytes accepted", password: strings.Repeat("密", 24), want: http.StatusCreated},
		{name: "25 runes 75 bytes rejected", password: strings.Repeat("密", 25), want: http.StatusUnprocessableEntity},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, mockUC := newTestAuthRouter(t, true)

			if tc.want == http.StatusCreated {
				mockUC.EXPECT().
					Register(gomock.Any(), usecase.RegisterInput{Email: authTestEmail, Password: tc.password}).
					Return(authTestResult(), nil)
			}

			res := testkit.DoJSON[dto.RegisterRequest, dto.AuthTokenResponse](
				t, app, http.MethodPost, registerPath,
				&dto.RegisterRequest{Email: authTestEmail, Password: tc.password},
			)
			require.Equal(t, tc.want, res.StatusCode)

			if tc.want != http.StatusUnprocessableEntity {
				return
			}
			problem := testkit.RequireProblem(t, res, http.StatusUnprocessableEntity)
			require.Len(t, problem.Errors, 1)
			require.Equal(t, "password", problem.Errors[0].Field)
			require.Equal(t, "passwordlen", problem.Errors[0].Tag)
			require.Equal(t, authPasswordMessage, problem.Errors[0].Message)
		})
	}
}

func TestAuthHandler_Register_SuccessResponseContract(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().
		Register(gomock.Any(), usecase.RegisterInput{Email: authTestEmail, Password: authTestPassword}).
		Return(authTestResult(), nil)

	res := testkit.DoJSON[dto.RegisterRequest, dto.AuthTokenResponse](
		t, app, http.MethodPost, registerPath,
		&dto.RegisterRequest{Email: authTestEmail, Password: authTestPassword},
	)
	require.Equal(t, http.StatusCreated, res.StatusCode)
	require.JSONEq(t, authTestTokenResponse, string(res.RawBody))
}

func TestAuthHandler_Login_SuccessResponseContract(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().
		Login(gomock.Any(), usecase.LoginInput{Email: authTestEmail, Password: authTestPassword}).
		Return(authTestResult(), nil)

	res := testkit.DoJSON[dto.LoginRequest, dto.AuthTokenResponse](
		t, app, http.MethodPost, loginPath,
		&dto.LoginRequest{Email: authTestEmail, Password: authTestPassword},
	)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.JSONEq(t, authTestTokenResponse, string(res.RawBody))
}

func TestAuthHandler_Refresh_SuccessResponseContract(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().
		Refresh(gomock.Any(), usecase.RefreshInput{RefreshToken: domain.RefreshToken(authTestRefreshValue)}).
		Return(authTestResult(), nil)

	res := testkit.DoJSON[dto.RefreshRequest, dto.AuthTokenResponse](
		t, app, http.MethodPost, refreshPath,
		&dto.RefreshRequest{RefreshToken: authTestRefreshValue},
	)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.JSONEq(t, authTestTokenResponse, string(res.RawBody))
}

func TestAuthHandler_Login_MalformedJSON(t *testing.T) {
	app, _ := newTestAuthRouter(t, true)

	res := doRawJSON[any](t, app, loginPath, `{"email":`)
	problem := testkit.RequireProblem(t, res, http.StatusBadRequest)
	require.Equal(t, loginPath, problem.Instance)
}

func TestAuthHandler_Login_UnknownFieldIsIgnored(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().
		Login(gomock.Any(), usecase.LoginInput{Email: authTestEmail, Password: authTestPassword}).
		Return(authTestResult(), nil)

	res := doRawJSON[any](t, app, loginPath, `{"email":"user@example.com","password":"correct-horse-battery","role":"admin"}`)
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestAuthHandler_Register_DisabledServesNoRoute(t *testing.T) {
	app, _ := newTestAuthRouter(t, false)

	res := testkit.DoJSON[dto.RegisterRequest, any](
		t, app, http.MethodPost, registerPath,
		&dto.RegisterRequest{Email: authTestEmail, Password: authTestPassword},
	)
	problem := testkit.RequireProblem(t, res, http.StatusNotFound)
	require.Equal(t, registerPath, problem.Instance)
}

func TestAuthHandler_Login_UsecaseUnauthorizedMapsTo401(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().
		Login(gomock.Any(), gomock.Any()).
		Return(usecase.AuthResult{}, domain.ErrUnauthorized)

	res := doRawJSON[any](t, app, loginPath, authTestLoginBody)
	problem := testkit.RequireProblem(t, res, http.StatusUnauthorized)
	require.Equal(t, "invalid credentials", problem.Detail)
	require.Equal(t, "Bearer", res.Header.Get(fiber.HeaderWWWAuthenticate))
}

func TestAuthHandler_Refresh_UsecaseUnauthorizedMapsTo401(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().
		Refresh(gomock.Any(), gomock.Any()).
		Return(usecase.AuthResult{}, domain.ErrUnauthorized)

	res := doRawJSON[any](t, app, refreshPath, authTestRefreshBody)
	problem := testkit.RequireProblem(t, res, http.StatusUnauthorized)
	require.Equal(t, "invalid refresh token", problem.Detail)
}

func TestAuthHandler_Register_UsecaseConflictMapsTo409(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().
		Register(gomock.Any(), gomock.Any()).
		Return(usecase.AuthResult{}, domain.ErrConflict)

	res := doRawJSON[any](t, app, registerPath, authTestRegisterBody)
	testkit.RequireProblem(t, res, http.StatusConflict)
}

func TestAuthHandler_Me_UsecaseUnauthorizedMapsTo401(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().Me(gomock.Any()).Return(domain.User{}, domain.ErrUnauthorized)

	res := testkit.DoJSON[any, any](t, app, http.MethodGet, mePath, nil)
	testkit.RequireProblem(t, res, http.StatusUnauthorized)
}

func TestAuthHandler_Login_UsecaseInternalErrorSanitized(t *testing.T) {
	app, mockUC := newTestAuthRouter(t, true)

	mockUC.EXPECT().
		Login(gomock.Any(), gomock.Any()).
		Return(usecase.AuthResult{}, errors.New("secret db connection string leaked"))

	res := doRawJSON[any](t, app, loginPath, authTestLoginBody)
	problem := testkit.RequireProblem(t, res, http.StatusInternalServerError)
	require.NotContains(t, problem.Detail, "secret")
}
