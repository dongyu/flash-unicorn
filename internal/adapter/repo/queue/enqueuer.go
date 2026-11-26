package queueadapter

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/adapter/worker"
	"unicorn/internal/domain"
	"unicorn/internal/platform/database"
	"unicorn/internal/platform/queue"
)

type Enqueuer struct {
	client *queue.Client
}

func NewEnqueuer(client *queue.Client) *Enqueuer {
	return &Enqueuer{client: client}
}

var _ domain.JobEnqueuer = (*Enqueuer)(nil)

func (e *Enqueuer) EnqueueNoteCreated(ctx context.Context, noteID uuid.UUID) error {
	args := worker.NoteCreatedArgs{NoteID: noteID}

	switch e.client.Dialect {
	case database.DialectPostgres:
		if tx, ok := txctx.PgxTxFromContext(ctx); ok {
			_, err := e.client.Postgres.InsertTx(ctx, tx, args, nil)
			return err
		}
		_, err := e.client.Postgres.Insert(ctx, args, nil)
		return err

	case database.DialectSQLite:
		if tx, ok := txctx.SQLTxFromContext(ctx); ok {
			_, err := e.client.SQLite.InsertTx(ctx, tx, args, nil)
			return err
		}
		_, err := e.client.SQLite.Insert(ctx, args, nil)
		return err

	default:
		return fmt.Errorf("unsupported queue dialect %q", e.client.Dialect)
	}
}
