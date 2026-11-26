package testkit

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"unicorn/internal/domain"
)

type UserRepositorySuite struct {
	suite.Suite
	NewHarness func() RepoHarness
	harness    RepoHarness
}

func (s *UserRepositorySuite) SetupTest() {
	s.harness = s.NewHarness()
}

func (s *UserRepositorySuite) TearDownTest() {
	if s.harness.Cleanup != nil {
		s.harness.Cleanup()
	}
}

func (s *UserRepositorySuite) TestCreateAndGetByEmailAndID() {
	ctx := context.Background()
	id, err := uuid.NewV7()
	s.Require().NoError(err)
	now := time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC)

	created, err := s.harness.UserRepo.Create(ctx, domain.User{
		ID:           id,
		Email:        "  Alice@Example.COM ",
		PasswordHash: "bcrypt-hash",
		DisplayName:  "Alice",
		Role:         domain.RoleAdmin,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	s.Require().NoError(err)
	s.Equal(id, created.ID)
	s.Equal("alice@example.com", created.Email)
	s.Equal("bcrypt-hash", created.PasswordHash)
	s.Equal("Alice", created.DisplayName)
	s.Equal(domain.RoleAdmin, created.Role)
	s.Equal(domain.UserStatusActive, created.Status)
	s.WithinDuration(now, created.CreatedAt, time.Second)
	s.WithinDuration(now, created.UpdatedAt, time.Second)

	byEmail, err := s.harness.UserRepo.GetByEmail(ctx, " ALICE@example.com")
	s.Require().NoError(err)
	s.Equal(created.ID, byEmail.ID)
	s.Equal(created.Email, byEmail.Email)

	byID, err := s.harness.UserRepo.GetByID(ctx, id)
	s.Require().NoError(err)
	s.Equal(created.ID, byID.ID)
	s.Equal(created.Email, byID.Email)
}

func (s *UserRepositorySuite) TestCreateWithoutPasswordHash() {
	ctx := context.Background()
	id, err := uuid.NewV7()
	s.Require().NoError(err)

	_, err = s.harness.UserRepo.Create(ctx, domain.User{
		ID:        id,
		Email:     "external@example.com",
		Role:      domain.RoleUser,
		Status:    domain.UserStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	s.Require().NoError(err)

	got, err := s.harness.UserRepo.GetByID(ctx, id)
	s.Require().NoError(err)
	s.Empty(got.PasswordHash)
}

func (s *UserRepositorySuite) TestGetByEmailNotFound() {
	_, err := s.harness.UserRepo.GetByEmail(context.Background(), "missing@example.com")
	s.Require().Error(err)
	s.ErrorIs(err, domain.ErrNotFound)
}

func (s *UserRepositorySuite) TestGetByIDNotFound() {
	id, err := uuid.NewV7()
	s.Require().NoError(err)

	_, err = s.harness.UserRepo.GetByID(context.Background(), id)
	s.Require().Error(err)
	s.ErrorIs(err, domain.ErrNotFound)
}

func (s *UserRepositorySuite) TestCreateDuplicateEmailConflict() {
	ctx := context.Background()
	base := domain.User{
		Email:     "duplicate@example.com",
		Role:      domain.RoleUser,
		Status:    domain.UserStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	firstID, err := uuid.NewV7()
	s.Require().NoError(err)
	base.ID = firstID
	_, err = s.harness.UserRepo.Create(ctx, base)
	s.Require().NoError(err)

	secondID, err := uuid.NewV7()
	s.Require().NoError(err)
	base.ID = secondID
	base.Email = "DUPLICATE@example.com"
	_, err = s.harness.UserRepo.Create(ctx, base)
	s.Require().Error(err)
	s.ErrorIs(err, domain.ErrConflict)
}
