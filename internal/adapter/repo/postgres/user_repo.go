package postgresrepo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"unicorn/internal/adapter/repo/dberr"
	pggen "unicorn/internal/adapter/repo/postgres/gen"
	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

var _ domain.UserRepository = (*UserRepository)(nil)

func (r *UserRepository) queries(ctx context.Context) *pggen.Queries {
	if tx, ok := txctx.PgxTxFromContext(ctx); ok {
		return pggen.New(tx)
	}
	return pggen.New(r.pool)
}

func toDomainUser(row pggen.User) domain.User {
	return domain.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash.String,
		DisplayName:  row.DisplayName,
		Role:         domain.Role(row.Role),
		Status:       domain.UserStatus(row.Status),
		CreatedAt:    row.CreatedAt.UTC(),
		UpdatedAt:    row.UpdatedAt.UTC(),
	}
}

func (r *UserRepository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	row, err := r.queries(ctx).CreateUser(ctx, pggen.CreateUserParams{
		ID:    user.ID,
		Email: domain.NormalizeEmail(user.Email),
		PasswordHash: pgtype.Text{
			String: user.PasswordHash,
			Valid:  user.PasswordHash != "",
		},
		DisplayName: user.DisplayName,
		Role:        string(user.Role),
		Status:      string(user.Status),
		CreatedAt:   user.CreatedAt.UTC(),
		UpdatedAt:   user.UpdatedAt.UTC(),
	})
	if err != nil {
		if dberr.IsUniqueViolation(err) {
			return domain.User{}, domain.ErrConflict
		}
		return domain.User{}, err
	}
	return toDomainUser(row), nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := r.queries(ctx).GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, err
	}
	return toDomainUser(row), nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := r.queries(ctx).GetUserByEmail(ctx, domain.NormalizeEmail(email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, err
	}
	return toDomainUser(row), nil
}
