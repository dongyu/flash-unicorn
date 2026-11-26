package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gofiber/contrib/v3/sentry"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/rs/zerolog"

	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/domain"
)

func NewErrorHandler() fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		problem := mapErrorToProblem(c, err)

		if problem.Status >= http.StatusInternalServerError {
			zerolog.Ctx(c.Context()).Error().
				Err(err).
				Int("status", problem.Status).
				Str("path", c.Path()).
				Msg("unhandled server error")

			if hub := sentry.GetHubFromContext(c); hub != nil {
				hub.CaptureException(err)
			}
		}

		if problem.Status == http.StatusUnauthorized {
			c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
		}

		if err := c.Status(problem.Status).JSON(problem); err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, "application/problem+json")
		return nil
	}
}

func mapErrorToProblem(c fiber.Ctx, err error) dto.ProblemDetails {
	base := dto.ProblemDetails{
		Instance:  c.Path(),
		RequestID: requestid.FromContext(c),
	}

	var valErr *domain.ValidationError
	if errors.As(err, &valErr) {
		base.Type = "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.21"
		base.Title = http.StatusText(http.StatusUnprocessableEntity)
		base.Status = http.StatusUnprocessableEntity
		base.Detail = valErr.Message
		base.Errors = valErr.Fields
		return base
	}

	var bindErr *fiber.BindError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &bindErr) || errors.As(err, &syntaxErr) || errors.As(err, &typeErr) || errors.Is(err, io.EOF) {
		base.Type = "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.1"
		base.Title = http.StatusText(http.StatusBadRequest)
		base.Status = http.StatusBadRequest
		base.Detail = err.Error()
		return base
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		base.Type = "about:blank"
		base.Title = http.StatusText(fiberErr.Code)
		if base.Title == "" {
			base.Title = "HTTP Error"
		}
		base.Status = fiberErr.Code
		base.Detail = fiberErr.Message
		return base
	}

	switch {
	case errors.Is(err, domain.ErrValidation):
		base.Type = "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.21"
		base.Title = http.StatusText(http.StatusUnprocessableEntity)
		base.Status = http.StatusUnprocessableEntity
		base.Detail = err.Error()
	case errors.Is(err, domain.ErrNotFound):
		base.Type = "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.5"
		base.Title = http.StatusText(http.StatusNotFound)
		base.Status = http.StatusNotFound
		base.Detail = err.Error()
	case errors.Is(err, domain.ErrConflict):
		base.Type = "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.10"
		base.Title = http.StatusText(http.StatusConflict)
		base.Status = http.StatusConflict
		base.Detail = err.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		base.Type = "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.2"
		base.Title = http.StatusText(http.StatusUnauthorized)
		base.Status = http.StatusUnauthorized
		base.Detail = err.Error()
	case errors.Is(err, domain.ErrForbidden):
		base.Type = "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.4"
		base.Title = http.StatusText(http.StatusForbidden)
		base.Status = http.StatusForbidden
		base.Detail = err.Error()
	default:
		base.Type = "https://datatracker.ietf.org/doc/html/rfc9110#section-15.6.1"
		base.Title = http.StatusText(http.StatusInternalServerError)
		base.Status = http.StatusInternalServerError
		base.Detail = "an unexpected internal error occurred"
	}

	return base
}
