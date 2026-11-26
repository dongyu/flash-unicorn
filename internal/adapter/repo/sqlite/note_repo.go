package sqliterepo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"unicorn/internal/adapter/repo/dberr"
	sqlitegen "unicorn/internal/adapter/repo/sqlite/gen"
	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/domain"
)

type NoteRepository struct {
	db *sql.DB
}

func NewNoteRepository(db *sql.DB) *NoteRepository {
	return &NoteRepository{db: db}
}

var _ domain.NoteRepository = (*NoteRepository)(nil)

func (r *NoteRepository) queries(ctx context.Context) *sqlitegen.Queries {
	if tx, ok := txctx.SQLTxFromContext(ctx); ok {
		return sqlitegen.New(tx)
	}
	return sqlitegen.New(r.db)
}

func toDomainNote(row sqlitegen.Note) domain.Note {
	return domain.Note{
		ID:        row.ID,
		Title:     row.Title,
		Body:      row.Body,
		CreatedAt: row.CreatedAt.UTC(),
	}
}

func (r *NoteRepository) Create(ctx context.Context, note domain.Note) (domain.Note, error) {
	row, err := r.queries(ctx).CreateNote(ctx, sqlitegen.CreateNoteParams{
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
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Note{}, domain.ErrNotFound
		}
		return domain.Note{}, err
	}
	return toDomainNote(row), nil
}

func (r *NoteRepository) List(ctx context.Context, limit, offset int) ([]domain.Note, error) {
	rows, err := r.queries(ctx).ListNotes(ctx, sqlitegen.ListNotesParams{
		Limit:  int64(limit),
		Offset: int64(offset),
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
