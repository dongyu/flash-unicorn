package seed_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"unicorn/internal/domain"
	"unicorn/internal/seed"
	"unicorn/internal/testkit"
	domainmock "unicorn/internal/testkit/mocks/domain"
)

func TestSeeder_CreatesActiveUserWithNormalizedEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	fixedNow := time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC)

	uc := seed.NewSeeder(repo, hasher, testkit.NewFakeClock(fixedNow), &testkit.SeqIDGenerator{})

	repo.EXPECT().
		GetByEmail(gomock.Any(), "admin@example.com").
		Return(domain.User{}, domain.ErrNotFound)
	hasher.EXPECT().
		Hash("s3cret").
		Return("bcrypt-hash", nil)
	repo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, user domain.User) (domain.User, error) {
			require.Equal(t, "admin@example.com", user.Email)
			require.Equal(t, "bcrypt-hash", user.PasswordHash)
			require.Equal(t, "Admin", user.DisplayName)
			require.Equal(t, domain.RoleAdmin, user.Role)
			require.Equal(t, domain.UserStatusActive, user.Status)
			require.Equal(t, fixedNow, user.CreatedAt)
			require.Equal(t, fixedNow, user.UpdatedAt)
			return user, nil
		})

	created, err := uc.Create(context.Background(), seed.UserInput{
		Email:       "  Admin@Example.COM ",
		Password:    "s3cret",
		Role:        domain.RoleAdmin,
		DisplayName: "  Admin  ",
	})
	require.NoError(t, err)
	require.Equal(t, "admin@example.com", created.Email)
	require.Equal(t, domain.RoleAdmin, created.Role)
	require.Equal(t, domain.UserStatusActive, created.Status)
}

func TestSeeder_DefaultsToUserRole(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	uc := seed.NewSeeder(repo, hasher, testkit.NewFakeClock(time.Now()), &testkit.SeqIDGenerator{})

	repo.EXPECT().GetByEmail(gomock.Any(), "user@example.com").Return(domain.User{}, domain.ErrNotFound)
	hasher.EXPECT().Hash("s3cret").Return("bcrypt-hash", nil)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, user domain.User) (domain.User, error) {
			require.Equal(t, domain.RoleUser, user.Role)
			require.Equal(t, "", user.DisplayName)
			return user, nil
		})

	_, err := uc.Create(context.Background(), seed.UserInput{
		Email:    "user@example.com",
		Password: "s3cret",
	})
	require.NoError(t, err)
}

func TestSeeder_ExistingEmailConflicts(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	uc := seed.NewSeeder(repo, hasher, testkit.NewFakeClock(time.Now()), &testkit.SeqIDGenerator{})

	repo.EXPECT().
		GetByEmail(gomock.Any(), "taken@example.com").
		Return(domain.User{Email: "taken@example.com"}, nil)

	_, err := uc.Create(context.Background(), seed.UserInput{
		Email:    "taken@example.com",
		Password: "s3cret",
	})
	require.ErrorIs(t, err, domain.ErrConflict)
}

func TestSeeder_RepositoryConflictIsReported(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	uc := seed.NewSeeder(repo, hasher, testkit.NewFakeClock(time.Now()), &testkit.SeqIDGenerator{})

	repo.EXPECT().GetByEmail(gomock.Any(), "race@example.com").Return(domain.User{}, domain.ErrNotFound)
	hasher.EXPECT().Hash("s3cret").Return("bcrypt-hash", nil)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(domain.User{}, domain.ErrConflict)

	_, err := uc.Create(context.Background(), seed.UserInput{
		Email:    "race@example.com",
		Password: "s3cret",
	})
	require.ErrorIs(t, err, domain.ErrConflict)
}

func TestSeeder_RejectsInvalidInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	uc := seed.NewSeeder(repo, hasher, testkit.NewFakeClock(time.Now()), &testkit.SeqIDGenerator{})

	for name, in := range map[string]seed.UserInput{
		"empty email":    {Password: "s3cret"},
		"unknown role":   {Email: "user@example.com", Password: "s3cret", Role: domain.Role("owner")},
		"unknown status": {Email: "user@example.com", Password: "s3cret", Status: domain.UserStatus("pending")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := uc.Create(context.Background(), in)
			require.ErrorIs(t, err, domain.ErrValidation)
		})
	}
}

func TestSeeder_HasherFailurePropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	hashErr := errors.New("bcrypt unavailable")

	uc := seed.NewSeeder(repo, hasher, testkit.NewFakeClock(time.Now()), &testkit.SeqIDGenerator{})

	repo.EXPECT().GetByEmail(gomock.Any(), "user@example.com").Return(domain.User{}, domain.ErrNotFound)
	hasher.EXPECT().Hash("s3cret").Return("", hashErr)

	_, err := uc.Create(context.Background(), seed.UserInput{
		Email:    "user@example.com",
		Password: "s3cret",
	})
	require.ErrorIs(t, err, hashErr)
}

func TestSeeder_CreatesUserWithoutCredential(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	uc := seed.NewSeeder(repo, hasher, testkit.NewFakeClock(time.Now()), &testkit.SeqIDGenerator{})

	repo.EXPECT().GetByEmail(gomock.Any(), "external@example.com").Return(domain.User{}, domain.ErrNotFound)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, user domain.User) (domain.User, error) {
			require.Equal(t, "", user.PasswordHash)
			require.Equal(t, domain.UserStatusActive, user.Status)
			return user, nil
		})

	_, err := uc.Create(context.Background(), seed.UserInput{Email: "external@example.com"})
	require.NoError(t, err)
}

func TestSeeder_HonoursExplicitStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	uc := seed.NewSeeder(repo, hasher, testkit.NewFakeClock(time.Now()), &testkit.SeqIDGenerator{})

	repo.EXPECT().GetByEmail(gomock.Any(), "off@example.com").Return(domain.User{}, domain.ErrNotFound)
	hasher.EXPECT().Hash("s3cret").Return("bcrypt-hash", nil)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, user domain.User) (domain.User, error) {
			require.Equal(t, domain.UserStatusDisabled, user.Status)
			return user, nil
		})

	_, err := uc.Create(context.Background(), seed.UserInput{
		Email:    "off@example.com",
		Password: "s3cret",
		Status:   domain.UserStatusDisabled,
	})
	require.NoError(t, err)
}
