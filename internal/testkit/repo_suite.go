package testkit

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"unicorn/internal/domain"
)

type RepoHarness struct {
	TxManager domain.TxManager
	NoteRepo  domain.NoteRepository
	UserRepo  domain.UserRepository
	Enqueuer  domain.JobEnqueuer
	Cleanup   func()
}

type NoteRepositorySuite struct {
	suite.Suite
	NewHarness func() RepoHarness
	harness    RepoHarness
}

func (s *NoteRepositorySuite) SetupTest() {
	s.harness = s.NewHarness()
}

func (s *NoteRepositorySuite) TearDownTest() {
	if s.harness.Cleanup != nil {
		s.harness.Cleanup()
	}
}

func (s *NoteRepositorySuite) TestCreateAndGetByID() {
	ctx := context.Background()
	id, err := uuid.NewV7()
	s.Require().NoError(err)
	now := time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC)

	created, err := s.harness.NoteRepo.Create(ctx, domain.Note{
		ID:        id,
		Title:     "Contract Note",
		Body:      "Stored across dialects",
		CreatedAt: now,
	})
	s.Require().NoError(err)
	s.Equal(id, created.ID)
	s.Equal("Contract Note", created.Title)
	s.Equal("Stored across dialects", created.Body)
	s.WithinDuration(now, created.CreatedAt, time.Second)

	got, err := s.harness.NoteRepo.GetByID(ctx, id)
	s.Require().NoError(err)
	s.Equal(created.ID, got.ID)
	s.Equal(created.Title, got.Title)
	s.Equal(created.Body, got.Body)
	s.WithinDuration(now, got.CreatedAt, time.Second)
}

func (s *NoteRepositorySuite) TestGetByIDNotFound() {
	ctx := context.Background()
	missingID, err := uuid.NewV7()
	s.Require().NoError(err)

	_, err = s.harness.NoteRepo.GetByID(ctx, missingID)
	s.Require().Error(err)
	s.ErrorIs(err, domain.ErrNotFound)
}

func (s *NoteRepositorySuite) TestListOrderingAndPagination() {
	ctx := context.Background()
	base := time.Date(2026, 11, 26, 1, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		id, err := uuid.NewV7()
		s.Require().NoError(err)
		_, err = s.harness.NoteRepo.Create(ctx, domain.Note{
			ID:        id,
			Title:     string(rune('A' + i)),
			Body:      "item",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
		s.Require().NoError(err)
	}

	page, err := s.harness.NoteRepo.List(ctx, 2, 1)
	s.Require().NoError(err)
	s.Require().Len(page, 2)
	s.Equal("B", page[0].Title)
	s.Equal("A", page[1].Title)
}

func (s *NoteRepositorySuite) TestWithinTxRollback() {
	ctx := context.Background()
	id, err := uuid.NewV7()
	s.Require().NoError(err)
	sentinel := errors.New("boom")

	err = s.harness.TxManager.WithinTx(ctx, func(txCtx context.Context) error {
		_, createErr := s.harness.NoteRepo.Create(txCtx, domain.Note{
			ID:        id,
			Title:     "Should Rollback",
			Body:      "never committed",
			CreatedAt: time.Now().UTC(),
		})
		s.Require().NoError(createErr)
		s.Require().NoError(s.harness.Enqueuer.EnqueueNoteCreated(txCtx, id))
		return sentinel
	})
	s.Require().ErrorIs(err, sentinel)

	_, getErr := s.harness.NoteRepo.GetByID(ctx, id)
	s.ErrorIs(getErr, domain.ErrNotFound)
}

func (s *NoteRepositorySuite) TestWithinTxNestedReuseAndCommit() {
	ctx := context.Background()
	id, err := uuid.NewV7()
	s.Require().NoError(err)

	err = s.harness.TxManager.WithinTx(ctx, func(txCtx context.Context) error {
		return s.harness.TxManager.WithinTx(txCtx, func(nestedCtx context.Context) error {
			_, createErr := s.harness.NoteRepo.Create(nestedCtx, domain.Note{
				ID:        id,
				Title:     "Committed Atomically",
				Body:      "with river job",
				CreatedAt: time.Now().UTC(),
			})
			if createErr != nil {
				return createErr
			}
			return s.harness.Enqueuer.EnqueueNoteCreated(nestedCtx, id)
		})
	})
	s.Require().NoError(err)

	got, err := s.harness.NoteRepo.GetByID(ctx, id)
	s.Require().NoError(err)
	s.Equal("Committed Atomically", got.Title)
}

func (s *NoteRepositorySuite) TestCreateDuplicateIDConflict() {
	ctx := context.Background()
	id, err := uuid.NewV7()
	s.Require().NoError(err)

	note := domain.Note{
		ID:        id,
		Title:     "First",
		Body:      "one",
		CreatedAt: time.Now().UTC(),
	}
	_, err = s.harness.NoteRepo.Create(ctx, note)
	s.Require().NoError(err)

	_, err = s.harness.NoteRepo.Create(ctx, note)
	s.Require().Error(err)
	s.ErrorIs(err, domain.ErrConflict)
}
