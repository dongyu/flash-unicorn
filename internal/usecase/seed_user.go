package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"unicorn/internal/domain"
)

type SeedUserInput struct {
	Email       string
	Password    string
	Role        domain.Role
	DisplayName string
	Status      domain.UserStatus
}

type SeedUserUseCase interface {
	SeedUser(ctx context.Context, in SeedUserInput) (domain.User, error)
}

type seedUserUseCase struct {
	repo   domain.UserRepository
	hasher domain.PasswordHasher
	clock  domain.Clock
	idGen  domain.IDGenerator
}

func NewSeedUserUseCase(
	repo domain.UserRepository,
	hasher domain.PasswordHasher,
	clock domain.Clock,
	idGen domain.IDGenerator,
) SeedUserUseCase {
	return &seedUserUseCase{
		repo:   repo,
		hasher: hasher,
		clock:  clock,
		idGen:  idGen,
	}
}

func (u *seedUserUseCase) SeedUser(ctx context.Context, in SeedUserInput) (domain.User, error) {
	email := domain.NormalizeEmail(in.Email)
	role := in.Role
	if role == "" {
		role = domain.RoleUser
	}
	status := in.Status
	if status == "" {
		status = domain.UserStatusActive
	}

	if err := validateSeedUserInput(email, role, status); err != nil {
		return domain.User{}, err
	}

	_, err := u.repo.GetByEmail(ctx, email)
	switch {
	case err == nil:
		return domain.User{}, fmt.Errorf("user %q already exists: %w", email, domain.ErrConflict)
	case !errors.Is(err, domain.ErrNotFound):
		return domain.User{}, fmt.Errorf("look up user by email: %w", err)
	}

	var passwordHash string
	if in.Password != "" {
		hashed, hashErr := u.hasher.Hash(in.Password)
		if hashErr != nil {
			return domain.User{}, fmt.Errorf("hash password: %w", hashErr)
		}
		passwordHash = hashed
	}

	id, err := u.idGen.NewID()
	if err != nil {
		return domain.User{}, fmt.Errorf("generate user id: %w", err)
	}

	now := u.clock.Now().UTC()
	candidate := domain.User{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  strings.TrimSpace(in.DisplayName),
		Role:         role,
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	created, err := u.repo.Create(ctx, candidate)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.User{}, fmt.Errorf("user %q already exists: %w", email, domain.ErrConflict)
		}
		return domain.User{}, fmt.Errorf("persist user: %w", err)
	}
	return created, nil
}

func validateSeedUserInput(email string, role domain.Role, status domain.UserStatus) error {
	if email == "" {
		return domain.NewValidationError(
			"invalid seed user payload",
			domain.FieldError{Field: "email", Tag: "required", Message: "email must not be empty"},
		)
	}
	if !role.Valid() {
		return domain.NewValidationError(
			"invalid seed user payload",
			domain.FieldError{Field: "role", Tag: "oneof", Message: "role must be user or admin"},
		)
	}
	if !status.Valid() {
		return domain.NewValidationError(
			"invalid seed user payload",
			domain.FieldError{Field: "status", Tag: "oneof", Message: "status must be active or disabled"},
		)
	}
	return nil
}
