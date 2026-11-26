package domain

import (
	"context"

	"github.com/google/uuid"
)

//go:generate mockgen -destination=../testkit/mocks/domain/mock_queue.go -package=domainmock unicorn/internal/domain JobEnqueuer,TxManager

type JobEnqueuer interface {
	EnqueueNoteCreated(ctx context.Context, noteID uuid.UUID) error
}

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
