package postgresrepo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"unicorn/internal/adapter/repo/dberr"
	pggen "unicorn/internal/adapter/repo/postgres/gen"
	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/domain"
)

type NoteRepository struct {
	pool *pgxpool.Pool
}

func NewNoteRepository(pool *pgxpool.Pool) *NoteRepository {
	return &NoteRepository{pool: pool}
}

var _ domain.NoteRepository = (*NoteRepository)(nil)

func (r *NoteRepository) queries(ctx context.Context) *pggen.Queries {
	if tx, ok := txctx.PgxTxFromContext(ctx); ok {
		return pggen.New(tx)
	}
	return pggen.New(r.pool)
}

func toDomainNote(row pggen.Note) domain.Note {
	return domain.Note{
		ID:        row.ID,
		Title:     row.Title,
		Body:      row.Body,
		CreatedAt: row.CreatedAt.UTC(),
	}
}

func (r *NoteRepository) Create(ctx context.Context, note domain.Note) (domain.Note, error) {
	row, err := r.queries(ctx).CreateNote(ctx, pggen.CreateNoteParams{
		ID:        note.ID,
		Title:     note.Title,
		Body:      note.Body,
		CreatedAt: note.CreatedAt.UTC(),
	})
	if err != nil {
		if dberr.IsUniqueViolation(err) {
			return domain.Note{}, domain.ErrConflict
		}
		return domain.Note{}, err
	}
	return toDomainNote(row), nil
}

func (r *NoteRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Note, error) {
	row, err := r.queries(ctx).GetNoteByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Note{}, domain.ErrNotFound
		}
		return domain.Note{}, err
	}
	return toDomainNote(row), nil
}

func (r *NoteRepository) List(ctx context.Context, limit, offset int) ([]domain.Note, error) {
	rows, err := r.queries(ctx).ListNotes(ctx, pggen.ListNotesParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	notes := make([]domain.Note, 0, len(rows))
	for _, row := range rows {
		notes = append(notes, toDomainNote(row))
	}
	return notes, nil
}
