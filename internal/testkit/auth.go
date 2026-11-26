package testkit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"unicorn/internal/bootstrap"
	"unicorn/internal/domain"
	"unicorn/internal/platform/security"
	"unicorn/internal/seed"
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

	pers, err := bootstrap.OpenPersistence(ta.DB)
	require.NoError(t, err)

	user, err := seed.NewSeeder(
		pers.Users,
		security.NewBcryptHasher(ta.App.Config.Auth.BcryptCost),
		domain.SystemClock{},
		domain.UUIDv7Generator{},
	).Create(context.Background(), seed.UserInput{
		Email:       params.Email,
		Password:    params.Password,
		Role:        params.Role,
		DisplayName: params.DisplayName,
		Status:      params.Status,
	})
	require.NoError(t, err)
	return user
}

func AuthHeader(accessToken string) string {
	return "Bearer " + accessToken
}
