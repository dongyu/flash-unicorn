package app

import (
	"context"
	"fmt"

	"unicorn/internal/domain"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/platform/security"
	"unicorn/internal/usecase"
)

func SeedUser(ctx context.Context, cfg config.Config, in usecase.SeedUserInput) (domain.User, error) {
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return domain.User{}, fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	pers, err := buildPersistence(db)
	if err != nil {
		return domain.User{}, err
	}

	uc := usecase.NewSeedUserUseCase(
		pers.users,
		security.NewBcryptHasher(cfg.Auth.BcryptCost),
		domain.SystemClock{},
		domain.UUIDv7Generator{},
	)
	return uc.SeedUser(ctx, in)
}
