package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AccessToken struct {
	Value     string
	ExpiresAt time.Time
}

type RefreshToken string

type RefreshTokenRecord struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	TokenHash     string
	ExpiresAt     time.Time
	CreatedAt     time.Time
	RevokedAt     *time.Time
	RotatedFromID *uuid.UUID
}

func (r RefreshTokenRecord) Revoked() bool {
	return r.RevokedAt != nil
}

func (r RefreshTokenRecord) Expired(now time.Time) bool {
	return !now.Before(r.ExpiresAt)
}

//go:generate mockgen -destination=../testkit/mocks/domain/mock_refresh_token.go -package=domainmock unicorn/internal/domain RefreshTokenRepository,RefreshTokenGenerator

type RefreshTokenRepository interface {
	Create(ctx context.Context, token RefreshTokenRecord) (RefreshTokenRecord, error)
	GetByHash(ctx context.Context, tokenHash string) (RefreshTokenRecord, error)
	RevokeIfActive(ctx context.Context, tokenHash string, revokedAt time.Time) (bool, error)
	RevokeAllActiveForUser(ctx context.Context, userID uuid.UUID, revokedAt time.Time) error
}

type RefreshTokenGenerator interface {
	NewRefreshToken() (RefreshToken, error)
	HashRefreshToken(token RefreshToken) string
}

type TokenIssuer interface {
	IssueAccessToken(ctx context.Context, principal Principal) (AccessToken, error)
	VerifyAccessToken(ctx context.Context, value string) (Principal, error)
}
