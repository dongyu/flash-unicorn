package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

//go:generate mockgen -destination=../testkit/mocks/domain/mock_note_repository.go -package=domainmock unicorn/internal/domain NoteRepository

type Note struct {
	ID        uuid.UUID
	Title     string
	Body      string
	CreatedAt time.Time
}

type NoteRepository interface {
	Create(ctx context.Context, note Note) (Note, error)
	GetByID(ctx context.Context, id uuid.UUID) (Note, error)
	List(ctx context.Context, limit, offset int) ([]Note, error)
}
