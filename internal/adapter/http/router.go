package httpadapter

import (
	"time"

	"github.com/gofiber/contrib/v3/sentry"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/rs/zerolog"

	"unicorn/internal/adapter/http/handler"
	"unicorn/internal/adapter/http/middleware"
	"unicorn/internal/platform/logger"
)

const defaultBodyLimitBytes = 4 * 1024 * 1024

type RouteRegistrar interface {
	RegisterRoutes(api fiber.Router)
}

type RouterDeps struct {
	Logger         zerolog.Logger
	Readiness      handler.ReadinessChecker
	TraceExtractor logger.TraceIDExtractor
	Routes         []RouteRegistrar
}

func NewRouter(deps RouterDeps) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "unicorn",
		ErrorHandler: middleware.NewErrorHandler(),
		BodyLimit:    defaultBodyLimitBytes,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	})

	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(sentry.New(sentry.Config{
		Repanic:         true,
		WaitForDelivery: false,
		Timeout:         2 * time.Second,
	}))
	app.Use(helmet.New())
	app.Use(cors.New())
	app.Use(requestid.New())
	app.Use(middleware.ContextLogger(deps.Logger, deps.TraceExtractor))
	app.Use(middleware.AccessLogger(deps.Logger))

	handler.NewHealthHandler(deps.Readiness).RegisterRoutes(app)

	apiV1 := app.Group("/api/v1")
	for _, r := range deps.Routes {
		if r != nil {
			r.RegisterRoutes(apiV1)
		}
	}

	return app
}
