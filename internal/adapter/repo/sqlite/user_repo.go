package sqliterepo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"unicorn/internal/adapter/repo/dberr"
	sqlitegen "unicorn/internal/adapter/repo/sqlite/gen"
	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/domain"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

var _ domain.UserRepository = (*UserRepository)(nil)

func (r *UserRepository) queries(ctx context.Context) *sqlitegen.Queries {
	if tx, ok := txctx.SQLTxFromContext(ctx); ok {
		return sqlitegen.New(tx)
	}
	return sqlitegen.New(r.db)
}

func toDomainUser(row sqlitegen.User) domain.User {
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
	row, err := r.queries(ctx).CreateUser(ctx, sqlitegen.CreateUserParams{
		ID:    user.ID,
		Email: domain.NormalizeEmail(user.Email),
		PasswordHash: sql.NullString{
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
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, err
	}
	return toDomainUser(row), nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := r.queries(ctx).GetUserByEmail(ctx, domain.NormalizeEmail(email))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, err
	}
	return toDomainUser(row), nil
}
