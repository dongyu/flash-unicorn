package security_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"unicorn/internal/domain"
	"unicorn/internal/platform/security"
)

func TestBcryptHasher_UsesConfiguredCost(t *testing.T) {
	hasher := security.NewBcryptHasher(5)

	hash, err := hasher.Hash("s3cret")
	require.NoError(t, err)

	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	require.Equal(t, 5, cost)
}

func TestBcryptHasher_Verify(t *testing.T) {
	hasher := security.NewBcryptHasher(4)

	hash, err := hasher.Hash("s3cret")
	require.NoError(t, err)

	require.NoError(t, hasher.Verify("s3cret", hash))
	require.ErrorIs(t, hasher.Verify("wrong", hash), domain.ErrUnauthorized)
}
