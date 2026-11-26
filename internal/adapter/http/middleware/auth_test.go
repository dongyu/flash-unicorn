package middleware_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"unicorn/internal/adapter/http/middleware"
	"unicorn/internal/domain"
	"unicorn/internal/platform/security"
	"unicorn/internal/testkit"
)

const middlewareSigningKey = "middleware-signing-key-0000000001"

func allowRequest(c fiber.Ctx) error {
	return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
}

func newAuthMiddlewareApp(t *testing.T) (*fiber.App, *security.JWTIssuer) {
	t.Helper()

	issuer := security.NewJWTIssuer(
		"middleware-test",
		middlewareSigningKey,
		15*time.Minute,
		30*time.Second,
		testkit.NewFakeClock(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)),
		&testkit.SeqIDGenerator{},
	)

	app := fiber.New(fiber.Config{ErrorHandler: middleware.NewErrorHandler()})
	app.Get("/authed", middleware.RequireAuth(issuer), allowRequest)
	app.Get("/admin", middleware.RequireAuth(issuer), middleware.RequireRole(domain.RoleAdmin), allowRequest)
	app.Get("/role-only", middleware.RequireRole(domain.RoleAdmin), allowRequest)
	return app, issuer
}

func issueToken(t *testing.T, issuer *security.JWTIssuer, roles ...domain.Role) domain.AccessToken {
	t.Helper()
	token, err := issuer.IssueAccessToken(t.Context(), domain.Principal{
		UserID: uuid.MustParse("01930000-0000-7000-8000-0000000000aa"),
		Roles:  roles,
	})
	require.NoError(t, err)
	return token
}

func TestRequireAuth_AllowsValidBearerToken(t *testing.T) {
	app, issuer := newAuthMiddlewareApp(t)
	token := issueToken(t, issuer, domain.RoleUser)

	res := testkit.DoJSONAuth[any, any](t, app, http.MethodGet, "/authed", token.Value, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestRequireAuth_RejectsMissingBearerToken(t *testing.T) {
	app, _ := newAuthMiddlewareApp(t)

	res := testkit.DoJSON[any, any](t, app, http.MethodGet, "/authed", nil)
	testkit.RequireProblem(t, res, http.StatusUnauthorized)
	require.Equal(t, "Bearer", res.Header.Get("WWW-Authenticate"))
}

func TestRequireRole_AllowsMatchingRole(t *testing.T) {
	app, issuer := newAuthMiddlewareApp(t)
	token := issueToken(t, issuer, domain.RoleAdmin)

	res := testkit.DoJSONAuth[any, any](t, app, http.MethodGet, "/admin", token.Value, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestRequireRole_ForbidsMissingRole(t *testing.T) {
	app, issuer := newAuthMiddlewareApp(t)
	token := issueToken(t, issuer, domain.RoleUser)

	res := testkit.DoJSONAuth[any, any](t, app, http.MethodGet, "/admin", token.Value, nil)
	testkit.RequireProblem(t, res, http.StatusForbidden)
	require.Empty(t, res.Header.Get("WWW-Authenticate"))
}

func TestRequireRole_RejectsMissingPrincipal(t *testing.T) {
	app, _ := newAuthMiddlewareApp(t)

	res := testkit.DoJSON[any, any](t, app, http.MethodGet, "/role-only", nil)
	testkit.RequireProblem(t, res, http.StatusUnauthorized)
	require.Equal(t, "Bearer", res.Header.Get("WWW-Authenticate"))
}
