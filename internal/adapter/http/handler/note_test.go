package handler_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	httpadapter "unicorn/internal/adapter/http"
	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/adapter/http/handler"
	"unicorn/internal/domain"
	"unicorn/internal/testkit"
	usecasemock "unicorn/internal/testkit/mocks/usecase"
	"unicorn/internal/usecase"
)

func newTestNoteRouter(t *testing.T) (*fiber.App, *usecasemock.MockNoteUseCase) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockUC := usecasemock.NewMockNoteUseCase(ctrl)
	app := httpadapter.NewRouter(httpadapter.RouterDeps{
		Logger: zerolog.New(io.Discard),
		Routes: []httpadapter.RouteRegistrar{
			handler.NewNoteHandler(mockUC, handler.NewValidator()),
		},
	})
	return app, mockUC
}

func TestNoteHandler_Create_ValidationProblemDetails(t *testing.T) {
	app, _ := newTestNoteRouter(t)

	req := dto.CreateNoteRequest{Title: ""}
	res := testkit.DoJSON[dto.CreateNoteRequest, any](t, app, http.MethodPost, "/api/v1/notes", &req)
	problem := testkit.RequireProblem(t, res, http.StatusUnprocessableEntity)
	require.NotEmpty(t, problem.RequestID)
	require.NotEmpty(t, problem.Errors)
	require.Equal(t, "title", problem.Errors[0].Field)
}

func TestNoteHandler_GetByID_NotFoundProblemDetails(t *testing.T) {
	app, mockUC := newTestNoteRouter(t)

	id := uuid.MustParse("01930000-0000-7000-8000-000000000001")
	mockUC.EXPECT().GetNote(gomock.Any(), id).Return(domain.Note{}, domain.ErrNotFound)

	res := testkit.DoJSON[any, any](t, app, http.MethodGet, "/api/v1/notes/"+id.String(), nil)
	problem := testkit.RequireProblem(t, res, http.StatusNotFound)
	require.Equal(t, "/api/v1/notes/"+id.String(), problem.Instance)
}

func TestNoteHandler_Create_Success(t *testing.T) {
	app, mockUC := newTestNoteRouter(t)

	id := uuid.MustParse("01930000-0000-7000-8000-000000000002")
	now := time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC)

	mockUC.EXPECT().
		CreateNote(gomock.Any(), usecase.CreateNoteInput{Title: "Hello", Body: "World"}).
		Return(domain.Note{ID: id, Title: "Hello", Body: "World", CreatedAt: now}, nil)

	req := dto.CreateNoteRequest{Title: "Hello", Body: "World"}
	res := testkit.DoJSON[dto.CreateNoteRequest, dto.NoteResponse](t, app, http.MethodPost, "/api/v1/notes", &req)
	require.Equal(t, http.StatusCreated, res.StatusCode)
	require.Equal(t, "/api/v1/notes/"+id.String(), res.Header.Get("Location"))
	require.Equal(t, id, res.Data.ID)
	require.Equal(t, "Hello", res.Data.Title)
}

func TestNoteHandler_Create_MalformedJSON(t *testing.T) {
	app, _ := newTestNoteRouter(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/notes", strings.NewReader(`{"title":`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	testkit.RequireProblem(t, testkit.HTTPResult[any]{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		RawBody:    raw,
	}, http.StatusBadRequest)
}

func TestNoteHandler_GetByID_InternalErrorSanitized(t *testing.T) {
	app, mockUC := newTestNoteRouter(t)

	id := uuid.MustParse("01930000-0000-7000-8000-000000000003")
	mockUC.EXPECT().GetNote(gomock.Any(), id).Return(domain.Note{}, errors.New("secret db connection string leaked"))

	res := testkit.DoJSON[any, any](t, app, http.MethodGet, "/api/v1/notes/"+id.String(), nil)
	problem := testkit.RequireProblem(t, res, http.StatusInternalServerError)
	require.NotContains(t, problem.Detail, "secret")
}
