package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sqliterepo "unicorn/internal/adapter/repo/sqlite"
	"unicorn/internal/domain"
	"unicorn/internal/platform/security"
)

type SeedUserParams struct {
	Email       string
	Password    string
	Role        domain.Role
	DisplayName string
	Status      domain.UserStatus
}

func SeedUser(t *testing.T, ta *TestApp, params SeedUserParams) domain.User {
	t.Helper()

	role := params.Role
	if role == "" {
		role = domain.RoleUser
	}
	status := params.Status
	if status == "" {
		status = domain.UserStatusActive
	}

	passwordHash := ""
	if params.Password != "" {
		hash, err := security.NewBcryptHasher(ta.App.Config.Auth.BcryptCost).Hash(params.Password)
		require.NoError(t, err)
		passwordHash = hash
	}

	id, err := uuid.NewV7()
	require.NoError(t, err)
	now := time.Now().UTC()

	user, err := sqliterepo.NewUserRepository(ta.DB.SQLite).Create(context.Background(), domain.User{
		ID:           id,
		Email:        domain.NormalizeEmail(params.Email),
		PasswordHash: passwordHash,
		DisplayName:  params.DisplayName,
		Role:         role,
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	return user
}

func AuthHeader(accessToken string) string {
	return "Bearer " + accessToken
}
