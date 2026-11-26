package testkit

import (
	"context"
	"time"

	"github.com/google/uuid"

	"unicorn/internal/domain"
)

type RefreshTokenRepositorySuite struct {
	RepoSuite
}

func (s *RefreshTokenRepositorySuite) seedUser(id uuid.UUID) domain.User {
	user, err := s.harness.UserRepo.Create(context.Background(), domain.User{
		ID:        id,
		Email:     id.String() + "@example.com",
		Role:      domain.RoleUser,
		Status:    domain.UserStatusActive,
		CreatedAt: time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC),
	})
	s.Require().NoError(err)
	return user
}

func (s *RefreshTokenRepositorySuite) newToken(userID uuid.UUID, tokenHash string, rotatedFrom *uuid.UUID) domain.RefreshTokenRecord {
	id, err := uuid.NewV7()
	s.Require().NoError(err)
	return domain.RefreshTokenRecord{
		ID:            id,
		UserID:        userID,
		TokenHash:     tokenHash,
		ExpiresAt:     time.Date(2027, 1, 1, 9, 0, 0, 0, time.UTC),
		CreatedAt:     time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC),
		RotatedFromID: rotatedFrom,
	}
}

func (s *RefreshTokenRepositorySuite) TestCreateAndGetByHash() {
	ctx := context.Background()
	userID, err := uuid.NewV7()
	s.Require().NoError(err)
	s.seedUser(userID)

	candidate := s.newToken(userID, "hash-first-token", nil)
	created, err := s.harness.RefreshTokenRepo.Create(ctx, candidate)
	s.Require().NoError(err)
	s.Equal(candidate.ID, created.ID)
	s.Equal(userID, created.UserID)
	s.Equal("hash-first-token", created.TokenHash)
	s.WithinDuration(candidate.ExpiresAt, created.ExpiresAt, time.Second)
	s.WithinDuration(candidate.CreatedAt, created.CreatedAt, time.Second)
	s.Nil(created.RevokedAt)
	s.Nil(created.RotatedFromID)
	s.False(created.Revoked())
	s.False(created.Expired(time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)))

	got, err := s.harness.RefreshTokenRepo.GetByHash(ctx, "hash-first-token")
	s.Require().NoError(err)
	s.Equal(created.ID, got.ID)
	s.Equal(created.UserID, got.UserID)
	s.Equal(created.TokenHash, got.TokenHash)
	s.WithinDuration(created.ExpiresAt, got.ExpiresAt, time.Second)
	s.True(got.Expired(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)))
}

func (s *RefreshTokenRepositorySuite) TestCreateKeepsRotationLink() {
	ctx := context.Background()
	userID, err := uuid.NewV7()
	s.Require().NoError(err)
	s.seedUser(userID)

	first, err := s.harness.RefreshTokenRepo.Create(ctx, s.newToken(userID, "hash-old", nil))
	s.Require().NoError(err)

	second, err := s.harness.RefreshTokenRepo.Create(ctx, s.newToken(userID, "hash-new", &first.ID))
	s.Require().NoError(err)
	s.Require().NotNil(second.RotatedFromID)
	s.Equal(first.ID, *second.RotatedFromID)

	got, err := s.harness.RefreshTokenRepo.GetByHash(ctx, "hash-new")
	s.Require().NoError(err)
	s.Require().NotNil(got.RotatedFromID)
	s.Equal(first.ID, *got.RotatedFromID)
}

func (s *RefreshTokenRepositorySuite) TestGetByHashNotFound() {
	_, err := s.harness.RefreshTokenRepo.GetByHash(context.Background(), "hash-missing")
	s.Require().Error(err)
	s.ErrorIs(err, domain.ErrNotFound)
}

func (s *RefreshTokenRepositorySuite) TestCreateDuplicateHashConflicts() {
	ctx := context.Background()
	userID, err := uuid.NewV7()
	s.Require().NoError(err)
	s.seedUser(userID)

	_, err = s.harness.RefreshTokenRepo.Create(ctx, s.newToken(userID, "hash-duplicate", nil))
	s.Require().NoError(err)

	_, err = s.harness.RefreshTokenRepo.Create(ctx, s.newToken(userID, "hash-duplicate", nil))
	s.Require().Error(err)
	s.ErrorIs(err, domain.ErrConflict)
}

func (s *RefreshTokenRepositorySuite) TestRevokeIfActiveAffectsOneWinner() {
	ctx := context.Background()
	userID, err := uuid.NewV7()
	s.Require().NoError(err)
	s.seedUser(userID)

	created, err := s.harness.RefreshTokenRepo.Create(ctx, s.newToken(userID, "hash-rotating", nil))
	s.Require().NoError(err)

	revokedAt := time.Date(2026, 12, 3, 10, 0, 0, 0, time.UTC)
	revoked, err := s.harness.RefreshTokenRepo.RevokeIfActive(ctx, created.TokenHash, revokedAt)
	s.Require().NoError(err)
	s.True(revoked)

	again, err := s.harness.RefreshTokenRepo.RevokeIfActive(ctx, created.TokenHash, revokedAt)
	s.Require().NoError(err)
	s.False(again)

	got, err := s.harness.RefreshTokenRepo.GetByHash(ctx, created.TokenHash)
	s.Require().NoError(err)
	s.Require().NotNil(got.RevokedAt)
	s.WithinDuration(revokedAt, *got.RevokedAt, time.Second)
	s.True(got.Revoked())
}

func (s *RefreshTokenRepositorySuite) TestRevokeIfActiveUnknownHashIsNoop() {
	revoked, err := s.harness.RefreshTokenRepo.RevokeIfActive(
		context.Background(),
		"hash-unknown",
		time.Date(2026, 12, 3, 10, 0, 0, 0, time.UTC),
	)
	s.Require().NoError(err)
	s.False(revoked)
}

func (s *RefreshTokenRepositorySuite) TestRevokeAllActiveForUserCascades() {
	ctx := context.Background()
	firstUserID, err := uuid.NewV7()
	s.Require().NoError(err)
	s.seedUser(firstUserID)
	secondUserID, err := uuid.NewV7()
	s.Require().NoError(err)
	s.seedUser(secondUserID)

	first, err := s.harness.RefreshTokenRepo.Create(ctx, s.newToken(firstUserID, "hash-first", nil))
	s.Require().NoError(err)
	second, err := s.harness.RefreshTokenRepo.Create(ctx, s.newToken(firstUserID, "hash-second", nil))
	s.Require().NoError(err)
	other, err := s.harness.RefreshTokenRepo.Create(ctx, s.newToken(secondUserID, "hash-other", nil))
	s.Require().NoError(err)

	revokedAt := time.Date(2026, 12, 3, 11, 0, 0, 0, time.UTC)
	s.Require().NoError(s.harness.RefreshTokenRepo.RevokeAllActiveForUser(ctx, firstUserID, revokedAt))

	for _, hash := range []string{first.TokenHash, second.TokenHash} {
		got, getErr := s.harness.RefreshTokenRepo.GetByHash(ctx, hash)
		s.Require().NoError(getErr)
		s.Require().NotNil(got.RevokedAt, "hash %s should be revoked", hash)
		s.WithinDuration(revokedAt, *got.RevokedAt, time.Second)
	}

	stillActive, err := s.harness.RefreshTokenRepo.GetByHash(ctx, other.TokenHash)
	s.Require().NoError(err)
	s.Nil(stillActive.RevokedAt)
}

func (s *RefreshTokenRepositorySuite) TestDeleteUserCascades() {
	ctx := context.Background()
	userID, err := uuid.NewV7()
	s.Require().NoError(err)
	s.seedUser(userID)

	_, err = s.harness.RefreshTokenRepo.Create(ctx, s.newToken(userID, "hash-cascaded", nil))
	s.Require().NoError(err)

	s.Require().NoError(s.harness.DeleteUser(userID))

	_, err = s.harness.RefreshTokenRepo.GetByHash(ctx, "hash-cascaded")
	s.ErrorIs(err, domain.ErrNotFound)
}
