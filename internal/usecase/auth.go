package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"unicorn/internal/domain"
)

const dummyHashSeed = "unicorn-dummy-credential-0001"

type LoginInput struct {
	Email    string
	Password string
}

type LoginResult struct {
	AccessToken  domain.AccessToken
	RefreshToken domain.RefreshToken
	ExpiresIn    int64
	User         domain.User
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

type RegisterResult struct {
	AccessToken  domain.AccessToken
	RefreshToken domain.RefreshToken
	ExpiresIn    int64
	User         domain.User
}

type RefreshInput struct {
	RefreshToken domain.RefreshToken
}

type RefreshResult struct {
	AccessToken  domain.AccessToken
	RefreshToken domain.RefreshToken
	ExpiresIn    int64
	User         domain.User
}

type LogoutInput struct {
	RefreshToken domain.RefreshToken
	All          bool
}

type AuthUseCase interface {
	Register(ctx context.Context, in RegisterInput) (RegisterResult, error)
	Login(ctx context.Context, in LoginInput) (LoginResult, error)
	Refresh(ctx context.Context, in RefreshInput) (RefreshResult, error)
	Logout(ctx context.Context, in LogoutInput) error
	Me(ctx context.Context) (domain.User, error)
}

type authUseCase struct {
	repo          domain.UserRepository
	refreshTokens domain.RefreshTokenRepository
	hasher        domain.PasswordHasher
	issuer        domain.TokenIssuer
	refreshGen    domain.RefreshTokenGenerator
	refreshTTL    time.Duration
	clock         domain.Clock
	idGen         domain.IDGenerator
	dummyHash     string
}

func NewAuthUseCase(
	repo domain.UserRepository,
	refreshTokens domain.RefreshTokenRepository,
	hasher domain.PasswordHasher,
	issuer domain.TokenIssuer,
	refreshGen domain.RefreshTokenGenerator,
	refreshTTL time.Duration,
	clock domain.Clock,
	idGen domain.IDGenerator,
) (AuthUseCase, error) {
	dummyHash, err := hasher.Hash(dummyHashSeed)
	if err != nil {
		return nil, fmt.Errorf("hash dummy credential: %w", err)
	}
	return &authUseCase{
		repo:          repo,
		refreshTokens: refreshTokens,
		hasher:        hasher,
		issuer:        issuer,
		refreshGen:    refreshGen,
		refreshTTL:    refreshTTL,
		clock:         clock,
		idGen:         idGen,
		dummyHash:     dummyHash,
	}, nil
}

func (u *authUseCase) Register(ctx context.Context, in RegisterInput) (RegisterResult, error) {
	passwordHash, err := u.hasher.Hash(in.Password)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("hash password: %w", err)
	}

	id, err := u.idGen.NewID()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("generate user id: %w", err)
	}

	now := u.clock.Now().UTC()
	created, err := u.repo.Create(ctx, domain.User{
		ID:           id,
		Email:        domain.NormalizeEmail(in.Email),
		PasswordHash: passwordHash,
		DisplayName:  strings.TrimSpace(in.DisplayName),
		Role:         domain.RoleUser,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return RegisterResult{}, domain.ErrConflict
		}
		return RegisterResult{}, fmt.Errorf("persist user: %w", err)
	}

	accessToken, refreshToken, err := u.issueTokenPair(ctx, created, nil)
	if err != nil {
		return RegisterResult{}, err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", created.ID.String()).
		Msg("user registered")

	return RegisterResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    u.expiresIn(accessToken),
		User:         created,
	}, nil
}

func (u *authUseCase) Login(ctx context.Context, in LoginInput) (LoginResult, error) {
	email := domain.NormalizeEmail(in.Email)

	user, err := u.repo.GetByEmail(ctx, email)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		u.verifyDummy(in.Password)
		logLoginFailure(ctx, email, "unknown_user")
		return LoginResult{}, domain.ErrUnauthorized
	case err != nil:
		return LoginResult{}, fmt.Errorf("look up user by email: %w", err)
	}

	if user.PasswordHash == "" {
		u.verifyDummy(in.Password)
		logLoginFailure(ctx, email, "missing_credential")
		return LoginResult{}, domain.ErrUnauthorized
	}

	if verifyErr := u.hasher.Verify(in.Password, user.PasswordHash); verifyErr != nil {
		logLoginFailure(ctx, email, "password_mismatch")
		return LoginResult{}, domain.ErrUnauthorized
	}

	if user.Status != domain.UserStatusActive {
		logLoginFailure(ctx, email, "disabled_user")
		return LoginResult{}, domain.ErrUnauthorized
	}

	accessToken, refreshToken, err := u.issueTokenPair(ctx, user, nil)
	if err != nil {
		return LoginResult{}, err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", user.ID.String()).
		Msg("user logged in")

	return LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    u.expiresIn(accessToken),
		User:         user,
	}, nil
}

func (u *authUseCase) Refresh(ctx context.Context, in RefreshInput) (RefreshResult, error) {
	tokenHash := u.refreshGen.HashRefreshToken(in.RefreshToken)

	stored, err := u.refreshTokens.GetByHash(ctx, tokenHash)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		logRefreshFailure(ctx, uuid.Nil, "unknown_refresh_token")
		return RefreshResult{}, domain.ErrUnauthorized
	case err != nil:
		return RefreshResult{}, fmt.Errorf("look up refresh token: %w", err)
	}

	now := u.clock.Now().UTC()
	if stored.Revoked() {
		if revokeErr := u.refreshTokens.RevokeAllActiveForUser(ctx, stored.UserID, now); revokeErr != nil {
			return RefreshResult{}, fmt.Errorf("revoke refresh token family: %w", revokeErr)
		}
		logRefreshFailure(ctx, stored.UserID, "revoked_refresh_token")
		return RefreshResult{}, domain.ErrUnauthorized
	}

	if stored.Expired(now) {
		logRefreshFailure(ctx, stored.UserID, "expired_refresh_token")
		return RefreshResult{}, domain.ErrUnauthorized
	}

	user, err := u.repo.GetByID(ctx, stored.UserID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		logRefreshFailure(ctx, stored.UserID, "unknown_user")
		return RefreshResult{}, domain.ErrUnauthorized
	case err != nil:
		return RefreshResult{}, fmt.Errorf("load user: %w", err)
	}

	if user.Status != domain.UserStatusActive {
		logRefreshFailure(ctx, user.ID, "disabled_user")
		return RefreshResult{}, domain.ErrUnauthorized
	}

	rotated, err := u.refreshTokens.RevokeIfActive(ctx, tokenHash, now)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("revoke rotated refresh token: %w", err)
	}
	if !rotated {
		logRefreshFailure(ctx, user.ID, "concurrent_rotation")
		return RefreshResult{}, domain.ErrUnauthorized
	}

	accessToken, refreshToken, err := u.issueTokenPair(ctx, user, &stored.ID)
	if err != nil {
		return RefreshResult{}, err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", user.ID.String()).
		Msg("refresh token rotated")

	return RefreshResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    u.expiresIn(accessToken),
		User:         user,
	}, nil
}

func (u *authUseCase) Logout(ctx context.Context, in LogoutInput) error {
	tokenHash := u.refreshGen.HashRefreshToken(in.RefreshToken)
	now := u.clock.Now().UTC()

	if !in.All {
		if _, err := u.refreshTokens.RevokeIfActive(ctx, tokenHash, now); err != nil {
			return fmt.Errorf("revoke refresh token: %w", err)
		}
		return nil
	}

	stored, err := u.refreshTokens.GetByHash(ctx, tokenHash)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("look up refresh token: %w", err)
	}

	if err := u.refreshTokens.RevokeAllActiveForUser(ctx, stored.UserID, now); err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", stored.UserID.String()).
		Msg("all refresh tokens revoked")

	return nil
}

func (u *authUseCase) Me(ctx context.Context) (domain.User, error) {
	principal, ok := domain.PrincipalFromContext(ctx)
	if !ok {
		return domain.User{}, domain.ErrUnauthorized
	}

	user, err := u.repo.GetByID(ctx, principal.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, domain.ErrUnauthorized
		}
		return domain.User{}, fmt.Errorf("load user: %w", err)
	}
	return user, nil
}

func (u *authUseCase) issueTokenPair(
	ctx context.Context,
	user domain.User,
	rotatedFrom *uuid.UUID,
) (domain.AccessToken, domain.RefreshToken, error) {
	refreshToken, err := u.refreshGen.NewRefreshToken()
	if err != nil {
		return domain.AccessToken{}, "", fmt.Errorf("generate refresh token: %w", err)
	}

	recordID, err := u.idGen.NewID()
	if err != nil {
		return domain.AccessToken{}, "", fmt.Errorf("generate refresh token id: %w", err)
	}

	now := u.clock.Now().UTC()
	_, err = u.refreshTokens.Create(ctx, domain.RefreshTokenRecord{
		ID:            recordID,
		UserID:        user.ID,
		TokenHash:     u.refreshGen.HashRefreshToken(refreshToken),
		ExpiresAt:     now.Add(u.refreshTTL),
		CreatedAt:     now,
		RotatedFromID: rotatedFrom,
	})
	if err != nil {
		return domain.AccessToken{}, "", fmt.Errorf("persist refresh token: %w", err)
	}

	principal := domain.Principal{UserID: user.ID, Roles: []domain.Role{user.Role}}
	accessToken, err := u.issuer.IssueAccessToken(ctx, principal)
	if err != nil {
		return domain.AccessToken{}, "", fmt.Errorf("issue access token: %w", err)
	}

	return accessToken, refreshToken, nil
}

func (u *authUseCase) expiresIn(token domain.AccessToken) int64 {
	return int64(token.ExpiresAt.Sub(u.clock.Now().UTC()).Round(time.Second) / time.Second)
}

func (u *authUseCase) verifyDummy(password string) {
	_ = u.hasher.Verify(password, u.dummyHash)
}

func logRefreshFailure(ctx context.Context, userID uuid.UUID, reason string) {
	event := zerolog.Ctx(ctx).Info()
	if userID != uuid.Nil {
		event = event.Str("user_id", userID.String())
	}
	event.Str("reason", reason).Msg("refresh rejected")
}

func logLoginFailure(ctx context.Context, email, reason string) {
	zerolog.Ctx(ctx).Info().
		Str("email", email).
		Str("reason", reason).
		Msg("login rejected")
}
