package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"unicorn/internal/domain"
)

const dummyHashSeed = "unicorn-dummy-credential-0001"

type LoginInput struct {
	Email    string
	Password string
}

type LoginResult struct {
	AccessToken domain.AccessToken
	ExpiresIn   int64
	User        domain.User
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

type RegisterResult struct {
	AccessToken domain.AccessToken
	ExpiresIn   int64
	User        domain.User
}

type AuthUseCase interface {
	Register(ctx context.Context, in RegisterInput) (RegisterResult, error)
	Login(ctx context.Context, in LoginInput) (LoginResult, error)
	Me(ctx context.Context) (domain.User, error)
}

type authUseCase struct {
	repo      domain.UserRepository
	hasher    domain.PasswordHasher
	issuer    domain.TokenIssuer
	clock     domain.Clock
	idGen     domain.IDGenerator
	dummyHash string
}

func NewAuthUseCase(
	repo domain.UserRepository,
	hasher domain.PasswordHasher,
	issuer domain.TokenIssuer,
	clock domain.Clock,
	idGen domain.IDGenerator,
) (AuthUseCase, error) {
	dummyHash, err := hasher.Hash(dummyHashSeed)
	if err != nil {
		return nil, fmt.Errorf("hash dummy credential: %w", err)
	}
	return &authUseCase{
		repo:      repo,
		hasher:    hasher,
		issuer:    issuer,
		clock:     clock,
		idGen:     idGen,
		dummyHash: dummyHash,
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

	principal := domain.Principal{UserID: created.ID, Roles: []domain.Role{created.Role}}
	accessToken, err := u.issuer.IssueAccessToken(ctx, principal)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("issue access token: %w", err)
	}

	expiresIn := int64(accessToken.ExpiresAt.Sub(u.clock.Now().UTC()).Round(time.Second) / time.Second)

	zerolog.Ctx(ctx).Info().
		Str("user_id", created.ID.String()).
		Msg("user registered")

	return RegisterResult{
		AccessToken: accessToken,
		ExpiresIn:   expiresIn,
		User:        created,
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

	principal := domain.Principal{UserID: user.ID, Roles: []domain.Role{user.Role}}
	accessToken, err := u.issuer.IssueAccessToken(ctx, principal)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue access token: %w", err)
	}

	expiresIn := int64(accessToken.ExpiresAt.Sub(u.clock.Now().UTC()).Round(time.Second) / time.Second)

	zerolog.Ctx(ctx).Info().
		Str("user_id", user.ID.String()).
		Msg("user logged in")

	return LoginResult{
		AccessToken: accessToken,
		ExpiresIn:   expiresIn,
		User:        user,
	}, nil
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

func (u *authUseCase) verifyDummy(password string) {
	_ = u.hasher.Verify(password, u.dummyHash)
}

func logLoginFailure(ctx context.Context, email, reason string) {
	zerolog.Ctx(ctx).Info().
		Str("email", email).
		Str("reason", reason).
		Msg("login rejected")
}
