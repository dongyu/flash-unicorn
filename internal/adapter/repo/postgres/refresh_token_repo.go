package postgresrepo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"unicorn/internal/adapter/repo/dberr"
	pggen "unicorn/internal/adapter/repo/postgres/gen"
	"unicorn/internal/adapter/repo/txctx"
	"unicorn/internal/domain"
)

type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

var _ domain.RefreshTokenRepository = (*RefreshTokenRepository)(nil)

func (r *RefreshTokenRepository) queries(ctx context.Context) *pggen.Queries {
	if tx, ok := txctx.PgxTxFromContext(ctx); ok {
		return pggen.New(tx)
	}
	return pggen.New(r.pool)
}

func toDomainRefreshToken(row pggen.RefreshToken) domain.RefreshTokenRecord {
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
		rotatedFromID := uuid.UUID(row.RotatedFromID.Bytes)
		record.RotatedFromID = &rotatedFromID
	}
	return record
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token domain.RefreshTokenRecord) (domain.RefreshTokenRecord, error) {
	params := pggen.CreateRefreshTokenParams{
		ID:        token.ID,
		UserID:    token.UserID,
		TokenHash: token.TokenHash,
		ExpiresAt: token.ExpiresAt.UTC(),
		CreatedAt: token.CreatedAt.UTC(),
	}
	if token.RevokedAt != nil {
		params.RevokedAt = pgtype.Timestamptz{Time: token.RevokedAt.UTC(), Valid: true}
	}
	if token.RotatedFromID != nil {
		params.RotatedFromID = pgtype.UUID{Bytes: *token.RotatedFromID, Valid: true}
	}

	row, err := r.queries(ctx).CreateRefreshToken(ctx, params)
	if err != nil {
		if dberr.IsUniqueViolation(err) {
			return domain.RefreshTokenRecord{}, domain.ErrConflict
		}
		return domain.RefreshTokenRecord{}, err
	}
	return toDomainRefreshToken(row), nil
}

func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (domain.RefreshTokenRecord, error) {
	row, err := r.queries(ctx).GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RefreshTokenRecord{}, domain.ErrNotFound
		}
		return domain.RefreshTokenRecord{}, err
	}
	return toDomainRefreshToken(row), nil
}

func (r *RefreshTokenRepository) RevokeIfActive(ctx context.Context, tokenHash string, revokedAt time.Time) (bool, error) {
	affected, err := r.queries(ctx).RevokeRefreshToken(ctx, pggen.RevokeRefreshTokenParams{
		TokenHash: tokenHash,
		RevokedAt: pgtype.Timestamptz{Time: revokedAt.UTC(), Valid: true},
	})
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *RefreshTokenRepository) RevokeAllActiveForUser(ctx context.Context, userID uuid.UUID, revokedAt time.Time) error {
	_, err := r.queries(ctx).RevokeAllRefreshTokensForUser(ctx, pggen.RevokeAllRefreshTokensForUserParams{
		UserID:    userID,
		RevokedAt: pgtype.Timestamptz{Time: revokedAt.UTC(), Valid: true},
	})
	return err
}
