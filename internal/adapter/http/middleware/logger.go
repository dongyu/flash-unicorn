package middleware

import (
	"github.com/gofiber/contrib/v3/sentry"
	fiberzerolog "github.com/gofiber/contrib/v3/zerolog"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/rs/zerolog"

	"unicorn/internal/platform/logger"
)

func ContextLogger(base zerolog.Logger, extractor logger.TraceIDExtractor) fiber.Handler {
	return func(c fiber.Ctx) error {
		reqID := requestid.FromContext(c)
		if hub := sentry.GetHubFromContext(c); hub != nil && reqID != "" {
			hub.Scope().SetTag("request_id", reqID)
		}

		reqLog := base.With().
			Str("request_id", reqID).
			Logger()
		ctx := logger.EnrichContext(c.Context(), reqLog, extractor)
		c.SetContext(ctx)
		return c.Next()
	}
}

func AccessLogger(base zerolog.Logger) fiber.Handler {
	return fiberzerolog.New(fiberzerolog.Config{
		Logger: &base,
		GetLogger: func(c fiber.Ctx) zerolog.Logger {
			return *zerolog.Ctx(c.Context())
		},
		Skip: func(c fiber.Ctx) bool {
			return c.Path() == "/healthz"
		},
	})
}
