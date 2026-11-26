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
	uc                usecase.AuthUseCase
	validate          *validator.Validate
	requireAuth       fiber.Handler
	credentialLimiter fiber.Handler
	allowRegistration bool
}

func NewAuthHandler(
	uc usecase.AuthUseCase,
	validate *validator.Validate,
	requireAuth fiber.Handler,
	credentialLimiter fiber.Handler,
	allowRegistration bool,
) *AuthHandler {
	return &AuthHandler{
		uc:                uc,
		validate:          validate,
		requireAuth:       requireAuth,
		credentialLimiter: credentialLimiter,
		allowRegistration: allowRegistration,
	}
}

func (h *AuthHandler) RegisterRoutes(api fiber.Router) {
	auth := api.Group("/auth")
	if h.allowRegistration {
		auth.Post("/register", h.credentialLimiter, h.Register)
	}
	auth.Post("/login", h.credentialLimiter, h.Login)
	auth.Post("/refresh", h.Refresh)
	auth.Post("/logout", h.Logout)
	auth.Get("/me", h.requireAuth, h.Me)
}

func (h *AuthHandler) Register(c fiber.Ctx) error {
	req, err := BindBodyAndValidate[dto.RegisterRequest](c, h.validate)
	if err != nil {
		return err
	}

	result, err := h.uc.Register(c.Context(), usecase.RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		return err
	}

	return c.Status(http.StatusCreated).JSON(dto.ToAuthTokenResponse(result))
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

	return c.Status(http.StatusOK).JSON(dto.ToAuthTokenResponse(result))
}

func (h *AuthHandler) Refresh(c fiber.Ctx) error {
	req, err := BindBodyAndValidate[dto.RefreshRequest](c, h.validate)
	if err != nil {
		return err
	}

	result, err := h.uc.Refresh(c.Context(), usecase.RefreshInput{
		RefreshToken: domain.RefreshToken(req.RefreshToken),
	})
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return fiber.NewError(http.StatusUnauthorized, "invalid refresh token")
		}
		return err
	}

	return c.Status(http.StatusOK).JSON(dto.ToAuthTokenResponse(result))
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	req, err := BindBodyAndValidate[dto.LogoutRequest](c, h.validate)
	if err != nil {
		return err
	}

	if err := h.uc.Logout(c.Context(), usecase.LogoutInput{
		RefreshToken: domain.RefreshToken(req.RefreshToken),
		All:          req.All,
	}); err != nil {
		return err
	}

	return c.SendStatus(http.StatusNoContent)
}

func (h *AuthHandler) Me(c fiber.Ctx) error {
	user, err := h.uc.Me(c.Context())
	if err != nil {
		return err
	}
	return c.Status(http.StatusOK).JSON(dto.ToAuthUserResponse(user))
}
