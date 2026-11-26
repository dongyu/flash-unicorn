package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"unicorn/internal/domain"
)

const refreshTokenEntropyBytes = 32

type RandomRefreshTokenGenerator struct{}

func NewRefreshTokenGenerator() *RandomRefreshTokenGenerator {
	return &RandomRefreshTokenGenerator{}
}

var _ domain.RefreshTokenGenerator = (*RandomRefreshTokenGenerator)(nil)

func (g *RandomRefreshTokenGenerator) NewRefreshToken() (domain.RefreshToken, error) {
	buffer := make([]byte, refreshTokenEntropyBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return domain.RefreshToken(base64.RawURLEncoding.EncodeToString(buffer)), nil
}

func (g *RandomRefreshTokenGenerator) HashRefreshToken(token domain.RefreshToken) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
