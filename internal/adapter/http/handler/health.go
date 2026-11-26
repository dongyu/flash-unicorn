package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"

	"unicorn/internal/adapter/http/dto"
)

const readinessCheckTimeout = 2 * time.Second

type ReadinessChecker interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	checker ReadinessChecker
}

func NewHealthHandler(checker ReadinessChecker) *HealthHandler {
	return &HealthHandler{checker: checker}
}

func (h *HealthHandler) RegisterRoutes(app *fiber.App) {
	app.Get("/healthz", h.Liveness)
	app.Get("/readyz", h.Readiness)
}

func (h *HealthHandler) Liveness(c fiber.Ctx) error {
	return c.Status(http.StatusOK).JSON(dto.HealthResponse{Status: "ok"})
}

func (h *HealthHandler) Readiness(c fiber.Ctx) error {
	if h.checker != nil {
		pingCtx, cancel := context.WithTimeout(c.Context(), readinessCheckTimeout)
		defer cancel()
		if err := h.checker.Ping(pingCtx); err != nil {
			return fiber.NewError(http.StatusServiceUnavailable, "database unavailable")
		}
	}
	return c.Status(http.StatusOK).JSON(dto.HealthResponse{Status: "ready"})
}
