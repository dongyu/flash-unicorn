package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"unicorn/internal/domain"
)

const bearerScheme = "Bearer "

func RequireAuth(issuer domain.TokenIssuer) fiber.Handler {
	return func(c fiber.Ctx) error {
		value, ok := bearerToken(c.Get(fiber.HeaderAuthorization))
		if !ok {
			return domain.ErrUnauthorized
		}

		principal, err := issuer.VerifyAccessToken(c.Context(), value)
		if err != nil {
			return domain.ErrUnauthorized
		}

		c.SetContext(domain.WithPrincipal(c.Context(), principal))
		return c.Next()
	}
}

func RequireRole(role domain.Role) fiber.Handler {
	return func(c fiber.Ctx) error {
		principal, ok := domain.PrincipalFromContext(c.Context())
		if !ok {
			return domain.ErrUnauthorized
		}
		if !principal.HasRole(role) {
			return domain.ErrForbidden
		}
		return c.Next()
	}
}

func bearerToken(header string) (string, bool) {
	if len(header) <= len(bearerScheme) || !strings.EqualFold(header[:len(bearerScheme)], bearerScheme) {
		return "", false
	}
	token := strings.TrimSpace(header[len(bearerScheme):])
	return token, token != ""
}
