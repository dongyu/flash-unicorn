package app_test

import (
	"context"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"unicorn/internal/app"
	"unicorn/internal/platform/config"
)

func TestNew_RejectsEmptyAuthSecret(t *testing.T) {
	_, err := app.New(context.Background(), config.Config{}, zerolog.New(io.Discard))
	require.ErrorContains(t, err, "auth.secret")
}
