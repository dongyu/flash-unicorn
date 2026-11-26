package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"unicorn/internal/domain"
	"unicorn/internal/testkit"
	domainmock "unicorn/internal/testkit/mocks/domain"
	"unicorn/internal/usecase"
)

func TestNoteUseCase_CreateNote_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	tx := domainmock.NewMockTxManager(ctrl)
	repo := domainmock.NewMockNoteRepository(ctrl)
	enqueuer := domainmock.NewMockJobEnqueuer(ctrl)

	fixedNow := time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC)
	clock := testkit.NewFakeClock(fixedNow)
	idGen := &testkit.SeqIDGenerator{}

	uc := usecase.NewNoteUseCase(tx, repo, enqueuer, clock, idGen)

	tx.EXPECT().
		WithinTx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})

	repo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, n domain.Note) (domain.Note, error) {
			require.Equal(t, "Hello", n.Title)
			require.Equal(t, "World", n.Body)
			require.Equal(t, fixedNow, n.CreatedAt)
			return n, nil
		})

	enqueuer.EXPECT().
		EnqueueNoteCreated(gomock.Any(), gomock.Any()).
		Return(nil)

	note, err := uc.CreateNote(context.Background(), usecase.CreateNoteInput{
		Title: "  Hello  ",
		Body:  "  World ",
	})
	require.NoError(t, err)
	require.Equal(t, "Hello", note.Title)
	require.Equal(t, fixedNow, note.CreatedAt)
}

func TestNoteUseCase_CreateNote_EmptyTitleValidation(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := usecase.NewNoteUseCase(
		domainmock.NewMockTxManager(ctrl),
		domainmock.NewMockNoteRepository(ctrl),
		domainmock.NewMockJobEnqueuer(ctrl),
		testkit.NewFakeClock(time.Now()),
		&testkit.SeqIDGenerator{},
	)

	_, err := uc.CreateNote(context.Background(), usecase.CreateNoteInput{Title: "   "})
	require.ErrorIs(t, err, domain.ErrValidation)
}

func TestNoteUseCase_CreateNote_RepoFailurePropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	tx := domainmock.NewMockTxManager(ctrl)
	repo := domainmock.NewMockNoteRepository(ctrl)
	enqueuer := domainmock.NewMockJobEnqueuer(ctrl)

	uc := usecase.NewNoteUseCase(tx, repo, enqueuer, testkit.NewFakeClock(time.Now()), &testkit.SeqIDGenerator{})
	dbErr := errors.New("write failed")

	tx.EXPECT().
		WithinTx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(domain.Note{}, dbErr)

	_, err := uc.CreateNote(context.Background(), usecase.CreateNoteInput{Title: "Valid"})
	require.ErrorIs(t, err, dbErr)
}
