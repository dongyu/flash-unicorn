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

	type noteResp struct {
		ID        uuid.UUID `json:"id"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
	}
	createReq := map[string]string{
		"title": "Architecture Blueprint",
		"body":  "Clean Architecture with Fiber v3, sqlc, and River",
	}
	createRes := testkit.DoJSON[map[string]string, noteResp](t, ta.HTTP, http.MethodPost, "/api/v1/notes", &createReq)
	require.Equal(t, http.StatusCreated, createRes.StatusCode)
	require.NotEqual(t, uuid.Nil, createRes.Data.ID)
	require.Equal(t, "Architecture Blueprint", createRes.Data.Title)

	rivertest.RequireInserted(ctx, t, ta.SQLiteDriver, worker.NoteCreatedArgs{NoteID: createRes.Data.ID}, nil)

	workRes := testkit.WorkSQLiteJob(ctx, t, ta, worker.NewNoteCreatedWorker(ta.App.NoteProcessor), worker.NoteCreatedArgs{NoteID: createRes.Data.ID})
	require.Equal(t, river.EventKindJobCompleted, workRes.EventKind)

	getRes := testkit.DoJSON[any, noteResp](t, ta.HTTP, http.MethodGet, "/api/v1/notes/"+createRes.Data.ID.String(), nil)
	require.Equal(t, http.StatusOK, getRes.StatusCode)
	require.Equal(t, createRes.Data.ID, getRes.Data.ID)
	require.Equal(t, "Architecture Blueprint", getRes.Data.Title)

	type listResp struct {
		Items  []noteResp `json:"items"`
		Limit  int        `json:"limit"`
		Offset int        `json:"offset"`
	}
	listRes := testkit.DoJSON[any, listResp](t, ta.HTTP, http.MethodGet, "/api/v1/notes?limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, listRes.StatusCode)
	require.Len(t, listRes.Data.Items, 1)
	require.Equal(t, 10, listRes.Data.Limit)
	require.Equal(t, 0, listRes.Data.Offset)
}
