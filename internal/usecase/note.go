package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"unicorn/internal/domain"
)

//go:generate mockgen -destination=../testkit/mocks/usecase/mock_note_usecase.go -package=usecasemock unicorn/internal/usecase NoteUseCase,NoteJobProcessor,AuthUseCase

type CreateNoteInput struct {
	Title string
	Body  string
}

type ListNotesInput struct {
	Limit  int
	Offset int
}

type NoteUseCase interface {
	CreateNote(ctx context.Context, in CreateNoteInput) (domain.Note, error)
	GetNote(ctx context.Context, id uuid.UUID) (domain.Note, error)
	ListNotes(ctx context.Context, in ListNotesInput) ([]domain.Note, error)
}

type NoteJobProcessor interface {
	ProcessNoteCreated(ctx context.Context, noteID uuid.UUID) error
}

type noteUseCase struct {
	tx       domain.TxManager
	repo     domain.NoteRepository
	enqueuer domain.JobEnqueuer
	clock    domain.Clock
	idGen    domain.IDGenerator
}

func NewNoteUseCase(
	tx domain.TxManager,
	repo domain.NoteRepository,
	enqueuer domain.JobEnqueuer,
	clock domain.Clock,
	idGen domain.IDGenerator,
) NoteUseCase {
	return &noteUseCase{
		tx:       tx,
		repo:     repo,
		enqueuer: enqueuer,
		clock:    clock,
		idGen:    idGen,
	}
}

func (u *noteUseCase) CreateNote(ctx context.Context, in CreateNoteInput) (domain.Note, error) {
	title := strings.TrimSpace(in.Title)
	body := strings.TrimSpace(in.Body)
	if title == "" {
		return domain.Note{}, domain.NewValidationError(
			"invalid note payload",
			domain.FieldError{Field: "title", Tag: "required", Message: "title must not be empty"},
		)
	}

	id, err := u.idGen.NewID()
	if err != nil {
		return domain.Note{}, fmt.Errorf("generate note id: %w", err)
	}

	candidate := domain.Note{
		ID:        id,
		Title:     title,
		Body:      body,
		CreatedAt: u.clock.Now().UTC(),
	}

	var created domain.Note
	err = u.tx.WithinTx(ctx, func(txCtx context.Context) error {
		var txErr error
		created, txErr = u.repo.Create(txCtx, candidate)
		if txErr != nil {
			return fmt.Errorf("persist note: %w", txErr)
		}
		if txErr = u.enqueuer.EnqueueNoteCreated(txCtx, created.ID); txErr != nil {
			return fmt.Errorf("enqueue note created job: %w", txErr)
		}
		return nil
	})
	if err != nil {
		return domain.Note{}, err
	}

	zerolog.Ctx(ctx).Info().
		Str("note_id", created.ID.String()).
		Msg("note created and background job enqueued")

	return created, nil
}

func (u *noteUseCase) GetNote(ctx context.Context, id uuid.UUID) (domain.Note, error) {
	return u.repo.GetByID(ctx, id)
}

func (u *noteUseCase) ListNotes(ctx context.Context, in ListNotesInput) ([]domain.Note, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := in.Offset
	if offset < 0 {
		offset = 0
	}
	return u.repo.List(ctx, limit, offset)
}

type noteJobProcessor struct {
	repo domain.NoteRepository
}

func NewNoteJobProcessor(repo domain.NoteRepository) NoteJobProcessor {
	return &noteJobProcessor{repo: repo}
}

func (p *noteJobProcessor) ProcessNoteCreated(ctx context.Context, noteID uuid.UUID) error {
	note, err := p.repo.GetByID(ctx, noteID)
	if err != nil {
		return fmt.Errorf("load note %s for processing: %w", noteID, err)
	}

	zerolog.Ctx(ctx).Info().
		Str("note_id", note.ID.String()).
		Str("title", note.Title).
		Msg("processed note_created background job")

	return nil
}
