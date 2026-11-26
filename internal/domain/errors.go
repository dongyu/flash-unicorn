package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("resource conflict")
	ErrValidation   = errors.New("validation failed")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

type FieldError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag,omitempty"`
	Message string `json:"message"`
}

type ValidationError struct {
	Message string
	Fields  []FieldError
}

func (e *ValidationError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", ErrValidation.Error(), e.Message)
	}
	return ErrValidation.Error()
}

func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

func NewValidationError(message string, fields ...FieldError) error {
	return &ValidationError{
		Message: message,
		Fields:  fields,
	}
}
