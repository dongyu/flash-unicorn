package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"unicorn/internal/adapter/repo/dberr"
	sqlitegen "unicorn/internal/adapter/repo/sqlite/gen"
	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/domain"
)

type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

var _ domain.RefreshTokenRepository = (*RefreshTokenRepository)(nil)

func (r *RefreshTokenRepository) queries(ctx context.Context) *sqlitegen.Queries {
	if tx, ok := txctx.SQLTxFromContext(ctx); ok {
		return sqlitegen.New(tx)
	}
	return sqlitegen.New(r.db)
}

func toDomainRefreshToken(row sqlitegen.RefreshToken) (domain.RefreshTokenRecord, error) {
	record := domain.RefreshTokenRecord{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt.UTC(),
		CreatedAt: row.CreatedAt.UTC(),
	}
	if row.RevokedAt.Valid {
		revokedAt := row.RevokedAt.Time.UTC()
		record.RevokedAt = &revokedAt
	}
	if row.RotatedFromID.Valid {
		rotatedFromID, err := uuid.Parse(row.RotatedFromID.String)
		if err != nil {
			return domain.RefreshTokenRecord{}, err
		}
		record.RotatedFromID = &rotatedFromID
	}
	return record, nil
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token domain.RefreshTokenRecord) (domain.RefreshTokenRecord, error) {
	params := sqlitegen.CreateRefreshTokenParams{
		ID:        token.ID,
		UserID:    token.UserID,
		TokenHash: token.TokenHash,
		ExpiresAt: token.ExpiresAt.UTC(),
		CreatedAt: token.CreatedAt.UTC(),
	}
	if token.RevokedAt != nil {
		params.RevokedAt = sql.NullTime{Time: token.RevokedAt.UTC(), Valid: true}
	}
	if token.RotatedFromID != nil {
		params.RotatedFromID = sql.NullString{String: token.RotatedFromID.String(), Valid: true}
	}

	row, err := r.queries(ctx).CreateRefreshToken(ctx, params)
	if err != nil {
		if dberr.IsUniqueViolation(err) {
			return domain.RefreshTokenRecord{}, domain.ErrConflict
		}
		return domain.RefreshTokenRecord{}, err
	}
	return toDomainRefreshToken(row)
}

func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (domain.RefreshTokenRecord, error) {
	row, err := r.queries(ctx).GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RefreshTokenRecord{}, domain.ErrNotFound
		}
		return domain.RefreshTokenRecord{}, err
	}
	return toDomainRefreshToken(row)
}

func (r *RefreshTokenRepository) RevokeIfActive(ctx context.Context, tokenHash string, revokedAt time.Time) (bool, error) {
	affected, err := r.queries(ctx).RevokeRefreshToken(ctx, sqlitegen.RevokeRefreshTokenParams{
		TokenHash: tokenHash,
		RevokedAt: sql.NullTime{Time: revokedAt.UTC(), Valid: true},
	})
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *RefreshTokenRepository) RevokeAllActiveForUser(ctx context.Context, userID uuid.UUID, revokedAt time.Time) error {
	_, err := r.queries(ctx).RevokeAllRefreshTokensForUser(ctx, sqlitegen.RevokeAllRefreshTokensForUserParams{
		UserID:    userID,
		RevokedAt: sql.NullTime{Time: revokedAt.UTC(), Valid: true},
	})
	return err
}
