package dto

import (
	"time"

	"github.com/google/uuid"

	"unicorn/internal/domain"
)

type CreateNoteRequest struct {
	Title string `json:"title" validate:"required,min=1,max=200"`
	Body  string `json:"body" validate:"max=10000"`
}

type ListNotesQuery struct {
	Limit  int `query:"limit" validate:"omitempty,min=1,max=100"`
	Offset int `query:"offset" validate:"omitempty,min=0"`
}

type NoteResponse struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func ToNoteResponse(n domain.Note) NoteResponse {
	return NoteResponse{
		ID:        n.ID,
		Title:     n.Title,
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
	}
}
