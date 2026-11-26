package dto

import "unicorn/internal/domain"

type ProblemDetails struct {
	Type      string              `json:"type"`
	Title     string              `json:"title"`
	Status    int                 `json:"status"`
	Detail    string              `json:"detail,omitempty"`
	Instance  string              `json:"instance,omitempty"`
	RequestID string              `json:"request_id,omitempty"`
	Errors    []domain.FieldError `json:"errors,omitempty"`
}

type PageResponse[T any] struct {
	Items  []T `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type HealthResponse struct {
	Status string `json:"status"`
}
