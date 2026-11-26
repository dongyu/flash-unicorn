package domain

import (
	"context"
	"time"
)

type AccessToken struct {
	Value     string
	ExpiresAt time.Time
}

type RefreshToken string

type TokenIssuer interface {
	IssueAccessToken(ctx context.Context, principal Principal) (AccessToken, error)
	VerifyAccessToken(ctx context.Context, value string) (Principal, error)
}
