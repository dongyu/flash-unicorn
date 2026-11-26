package handler

import (
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"

	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/domain"
	"unicorn/internal/usecase"
)

type AuthHandler struct {
	uc          usecase.AuthUseCase
	validate    *validator.Validate
	requireAuth fiber.Handler
}

func NewAuthHandler(uc usecase.AuthUseCase, validate *validator.Validate, requireAuth fiber.Handler) *AuthHandler {
	return &AuthHandler{
		uc:          uc,
		validate:    validate,
		requireAuth: requireAuth,
	}
}

func (h *AuthHandler) RegisterRoutes(api fiber.Router) {
	auth := api.Group("/auth")
	auth.Post("/login", h.Login)
	auth.Get("/me", h.requireAuth, h.Me)
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	req, err := BindBodyAndValidate[dto.LoginRequest](c, h.validate)
	if err != nil {
		return err
	}

	result, err := h.uc.Login(c.Context(), usecase.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return fiber.NewError(http.StatusUnauthorized, "invalid credentials")
		}
		return err
	}

	return c.Status(http.StatusOK).JSON(dto.LoginResponse{
		AccessToken: result.AccessToken.Value,
		TokenType:   dto.TokenTypeBearer,
		ExpiresIn:   result.ExpiresIn,
		User:        dto.ToAuthUserResponse(result.User),
	})
}

func (h *AuthHandler) Me(c fiber.Ctx) error {
	user, err := h.uc.Me(c.Context())
	if err != nil {
		return err
	}
	return c.Status(http.StatusOK).JSON(dto.ToAuthUserResponse(user))
}
