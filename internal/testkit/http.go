package testkit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"unicorn/internal/adapter/http/dto"
)

type HTTPResult[T any] struct {
	StatusCode int
	Header     http.Header
	RawBody    []byte
	Data       T
}

func DoJSON[Req any, Res any](t *testing.T, app *fiber.App, method, target string, body *Req) HTTPResult[Res] {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}

	req := httptest.NewRequestWithContext(t.Context(), method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	rawResp, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var out Res
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.StatusCode != http.StatusNoContent {
		if len(bytes.TrimSpace(rawResp)) > 0 {
			require.NoError(t, json.Unmarshal(rawResp, &out), "response body: %s", string(rawResp))
		}
	}

	return HTTPResult[Res]{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		RawBody:    rawResp,
		Data:       out,
	}
}

func RequireProblem[T any](t *testing.T, res HTTPResult[T], expectedStatus int) dto.ProblemDetails {
	t.Helper()
	require.Equal(t, expectedStatus, res.StatusCode)
	require.Contains(t, res.Header.Get("Content-Type"), "application/problem+json")

	var problem dto.ProblemDetails
	require.NoError(t, json.Unmarshal(res.RawBody, &problem), "problem body: %s", string(res.RawBody))
	require.Equal(t, expectedStatus, problem.Status)
	return problem
}
