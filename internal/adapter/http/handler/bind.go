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
	return v
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
		return fmt.Sprintf("%s is required", fe.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", fe.Field(), fe.Param())
	default:
		return fmt.Sprintf("%s failed %s validation", fe.Field(), fe.Tag())
	}
}
