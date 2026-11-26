package handler

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"

	"unicorn/internal/domain"
)

const (
	passwordLengthTag = "passwordlen"
	minPasswordBytes  = 8
	maxPasswordBytes  = 72
)

func NewValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		for _, tagName := range []string{"json", "query", "uri"} {
			if raw := fld.Tag.Get(tagName); raw != "" {
				name := strings.SplitN(raw, ",", 2)[0]
				if name != "" && name != "-" {
					return name
				}
			}
		}
		return fld.Name
	})
	if err := v.RegisterValidation(passwordLengthTag, passwordLengthWithinBounds); err != nil {
		panic(fmt.Sprintf("register %s validation: %v", passwordLengthTag, err))
	}
	return v
}

func passwordLengthWithinBounds(fl validator.FieldLevel) bool {
	length := len(fl.Field().String())
	return length >= minPasswordBytes && length <= maxPasswordBytes
}

func BindBodyAndValidate[T any](c fiber.Ctx, v *validator.Validate) (T, error) {
	var dst T
	if err := c.Bind().Body(&dst); err != nil {
		return dst, err
	}
	if err := validateStruct(v, dst); err != nil {
		return dst, err
	}
	return dst, nil
}

func BindQueryAndValidate[T any](c fiber.Ctx, v *validator.Validate) (T, error) {
	var dst T
	if err := c.Bind().Query(&dst); err != nil {
		return dst, err
	}
	if err := validateStruct(v, dst); err != nil {
		return dst, err
	}
	return dst, nil
}

func validateStruct(v *validator.Validate, target any) error {
	if v == nil {
		return nil
	}
	err := v.Struct(target)
	if err == nil {
		return nil
	}
	var valErrs validator.ValidationErrors
	if !errors.As(err, &valErrs) {
		return domain.NewValidationError(err.Error())
	}

	fields := make([]domain.FieldError, 0, len(valErrs))
	for _, fe := range valErrs {
		fields = append(fields, domain.FieldError{
			Field:   fe.Field(),
			Tag:     fe.Tag(),
			Message: formatValidationMessage(fe),
		})
	}
	return domain.NewValidationError("request validation failed", fields...)
}

func formatValidationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fe.Field() + " is required"
	case "min":
		return fmt.Sprintf("%s must be at least %s", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", fe.Field(), fe.Param())
	case passwordLengthTag:
		return fmt.Sprintf(
			"%s must be between %d and %d bytes",
			fe.Field(), minPasswordBytes, maxPasswordBytes,
		)
	default:
		return fmt.Sprintf("%s failed %s validation", fe.Field(), fe.Tag())
	}
}
