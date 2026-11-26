package security_test

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	"unicorn/internal/platform/security"
)

func TestRefreshTokenGenerator_NewRefreshTokenIsOpaqueAndUnique(t *testing.T) {
	t.Parallel()

	generator := security.NewRefreshTokenGenerator()

	first, err := generator.NewRefreshToken()
	require.NoError(t, err)
	second, err := generator.NewRefreshToken()
	require.NoError(t, err)

	require.NotEqual(t, first, second)

	decoded, err := base64.RawURLEncoding.DecodeString(string(first))
	require.NoError(t, err)
	require.Len(t, decoded, 32)
}

func TestRefreshTokenGenerator_HashRefreshTokenIsSHA256Hex(t *testing.T) {
	t.Parallel()

	generator := security.NewRefreshTokenGenerator()

	require.Equal(t,
		"9c5d99bdca43b58ffcca642347b2e6356a511b27747e0bf7c9064feeef4b6e0a",
		generator.HashRefreshToken("opaque-refresh-token-fixture"),
	)
	require.NotEqual(t, generator.HashRefreshToken("first"), generator.HashRefreshToken("second"))
}
