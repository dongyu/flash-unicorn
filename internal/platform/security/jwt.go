package security

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"unicorn/internal/domain"
)

type JWTIssuer struct {
	issuer    string
	secret    []byte
	accessTTL time.Duration
	leeway    time.Duration
	clock     domain.Clock
	idGen     domain.IDGenerator
}

func NewJWTIssuer(
	issuer string,
	secret string,
	accessTTL time.Duration,
	leeway time.Duration,
	clock domain.Clock,
	idGen domain.IDGenerator,
) *JWTIssuer {
	return &JWTIssuer{
		issuer:    issuer,
		secret:    []byte(secret),
		accessTTL: accessTTL,
		leeway:    leeway,
		clock:     clock,
		idGen:     idGen,
	}
}

var _ domain.TokenIssuer = (*JWTIssuer)(nil)

type accessTokenClaims struct {
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

func (i *JWTIssuer) IssueAccessToken(_ context.Context, principal domain.Principal) (domain.AccessToken, error) {
	now := i.clock.Now().UTC()
	expiresAt := now.Add(i.accessTTL)

	tokenID, err := i.idGen.NewID()
	if err != nil {
		return domain.AccessToken{}, fmt.Errorf("generate access token id: %w", err)
	}

	roles := make([]string, 0, len(principal.Roles))
	for _, role := range principal.Roles {
		roles = append(roles, string(role))
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, accessTokenClaims{
		Roles: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   principal.UserID.String(),
			Issuer:    i.issuer,
			ID:        tokenID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})

	signed, err := token.SignedString(i.secret)
	if err != nil {
		return domain.AccessToken{}, fmt.Errorf("sign access token: %w", err)
	}

	return domain.AccessToken{Value: signed, ExpiresAt: expiresAt}, nil
}

func (i *JWTIssuer) VerifyAccessToken(_ context.Context, value string) (domain.Principal, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(i.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(i.leeway),
		jwt.WithTimeFunc(i.clock.Now),
	)

	claims := &accessTokenClaims{}
	if _, err := parser.ParseWithClaims(value, claims, func(*jwt.Token) (any, error) {
		return i.secret, nil
	}); err != nil {
		return domain.Principal{}, fmt.Errorf("parse access token: %w: %w", domain.ErrUnauthorized, err)
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return domain.Principal{}, fmt.Errorf("parse access token subject: %w", domain.ErrUnauthorized)
	}

	roles := make([]domain.Role, 0, len(claims.Roles))
	for _, raw := range claims.Roles {
		role := domain.Role(raw)
		if !role.Valid() {
			return domain.Principal{}, fmt.Errorf("parse access token role %q: %w", raw, domain.ErrUnauthorized)
		}
		roles = append(roles, role)
	}

	return domain.Principal{UserID: userID, Roles: roles}, nil
}
