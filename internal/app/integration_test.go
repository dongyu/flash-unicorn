package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertest"
	"github.com/stretchr/testify/require"

	"unicorn/internal/adapter/worker"
	"unicorn/internal/testkit"
)

type noteResponse struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func TestFullStack_NotesHTTPAndRiverQueue(t *testing.T) {
	t.Parallel()

	ta := testkit.NewTestApp(t)
	ctx := context.Background()

	healthRes := testkit.DoJSON[any, map[string]string](t, ta.HTTP, http.MethodGet, "/healthz", nil)
	require.Equal(t, http.StatusOK, healthRes.StatusCode)
	require.Equal(t, "ok", healthRes.Data["status"])

	readyRes := testkit.DoJSON[any, map[string]string](t, ta.HTTP, http.MethodGet, "/readyz", nil)
	require.Equal(t, http.StatusOK, readyRes.StatusCode)
	require.Equal(t, "ready", readyRes.Data["status"])

	createReq := map[string]string{
		"title": "Architecture Blueprint",
		"body":  "Clean Architecture with Fiber v3, sqlc, and River",
	}
	createRes := testkit.DoJSON[map[string]string, noteResponse](t, ta.HTTP, http.MethodPost, "/api/v1/notes", &createReq)
	require.Equal(t, http.StatusCreated, createRes.StatusCode)
	require.NotEqual(t, uuid.Nil, createRes.Data.ID)
	require.Equal(t, "Architecture Blueprint", createRes.Data.Title)

	rivertest.RequireInserted(ctx, t, ta.SQLiteDriver, worker.NoteCreatedArgs{NoteID: createRes.Data.ID}, nil)

	workRes := testkit.WorkSQLiteJob(ctx, t, ta, worker.NewNoteCreatedWorker(ta.App.NoteProcessor), worker.NoteCreatedArgs{NoteID: createRes.Data.ID})
	require.Equal(t, river.EventKindJobCompleted, workRes.EventKind)

	getRes := testkit.DoJSON[any, noteResponse](t, ta.HTTP, http.MethodGet, "/api/v1/notes/"+createRes.Data.ID.String(), nil)
	require.Equal(t, http.StatusOK, getRes.StatusCode)
	require.Equal(t, createRes.Data.ID, getRes.Data.ID)
	require.Equal(t, "Architecture Blueprint", getRes.Data.Title)

	type listResp struct {
		Items  []noteResponse `json:"items"`
		Limit  int            `json:"limit"`
		Offset int            `json:"offset"`
	}
	listRes := testkit.DoJSON[any, listResp](t, ta.HTTP, http.MethodGet, "/api/v1/notes?limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, listRes.StatusCode)
	require.Len(t, listRes.Data.Items, 1)
	require.Equal(t, 10, listRes.Data.Limit)
	require.Equal(t, 0, listRes.Data.Offset)
}

func TestFullStack_InjectedClockAndIDGeneratorDriveNoteCreation(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	ta := testkit.NewTestApp(t,
		testkit.WithClock(testkit.NewFakeClock(fixedNow)),
		testkit.WithIDGenerator(&testkit.SeqIDGenerator{}),
	)

	createReq := map[string]string{
		"title": "Clock Seam",
		"body":  "created_at comes from the injected clock",
	}
	createRes := testkit.DoJSON[map[string]string, noteResponse](t, ta.HTTP, http.MethodPost, "/api/v1/notes", &createReq)
	require.Equal(t, http.StatusCreated, createRes.StatusCode)
	require.Equal(t, uuid.MustParse("00000000-0000-7000-8000-000000000001"), createRes.Data.ID)
	require.True(t, fixedNow.Equal(createRes.Data.CreatedAt), "created_at = %s, want %s", createRes.Data.CreatedAt, fixedNow)
}
