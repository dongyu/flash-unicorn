package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"unicorn/internal/domain"
	"unicorn/internal/testkit"
	domainmock "unicorn/internal/testkit/mocks/domain"
	"unicorn/internal/usecase"
)

const refreshTTLFixture = 720 * time.Hour

type stubRefreshTokenGenerator struct {
	issued []domain.RefreshToken
}

func (g *stubRefreshTokenGenerator) NewRefreshToken() (domain.RefreshToken, error) {
	token := domain.RefreshToken(fmt.Sprintf("generated-refresh-token-%d", len(g.issued)+1))
	g.issued = append(g.issued, token)
	return token, nil
}

func (g *stubRefreshTokenGenerator) HashRefreshToken(token domain.RefreshToken) string {
	return "hash:" + string(token)
}

func newAuthUseCaseWith(
	t *testing.T,
	repo domain.UserRepository,
	refreshTokens domain.RefreshTokenRepository,
	refreshGen domain.RefreshTokenGenerator,
	hasher domain.PasswordHasher,
	issuer domain.TokenIssuer,
	idGen domain.IDGenerator,
) usecase.AuthUseCase {
	t.Helper()
	uc, err := usecase.NewAuthUseCase(
		repo,
		refreshTokens,
		hasher,
		issuer,
		refreshGen,
		refreshTTLFixture,
		testkit.NewFakeClock(authFixedNow),
		idGen,
	)
	require.NoError(t, err)
	return uc
}

func storedRefreshToken(t *testing.T, userID uuid.UUID, tokenHash string, expiresAt time.Time) domain.RefreshTokenRecord {
	t.Helper()
	return domain.RefreshTokenRecord{
		ID:        uuid.MustParse("01930000-0000-7000-8000-0000000000a1"),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: authFixedNow.Add(-time.Hour),
	}
}

func TestAuthUseCase_LoginIssuesRefreshToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)
	refreshGen := &stubRefreshTokenGenerator{}
	issuer := &stubTokenIssuer{token: domain.AccessToken{
		Value:     "signed-access-token",
		ExpiresAt: authFixedNow.Add(15 * time.Minute),
	}}
	user := activeUser(t)

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	repo.EXPECT().GetByEmail(gomock.Any(), user.Email).Return(user, nil)
	hasher.EXPECT().Verify("correct-password", storedBcryptHash).Return(nil)
	refreshTokens.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, record domain.RefreshTokenRecord) (domain.RefreshTokenRecord, error) {
			require.Equal(t, user.ID, record.UserID)
			require.Equal(t, "hash:generated-refresh-token-1", record.TokenHash)
			require.Equal(t, authFixedNow.Add(refreshTTLFixture), record.ExpiresAt)
			require.Nil(t, record.RotatedFromID)
			return record, nil
		})

	uc := newAuthUseCaseWith(t, repo, refreshTokens, refreshGen, hasher, issuer, domain.UUIDv7Generator{})

	result, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    user.Email,
		Password: "correct-password",
	})
	require.NoError(t, err)
	require.Equal(t, domain.RefreshToken("generated-refresh-token-1"), result.RefreshToken)
}

func TestAuthUseCase_RefreshRotatesThePresentedToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)
	refreshGen := &stubRefreshTokenGenerator{}
	issuer := &stubTokenIssuer{token: domain.AccessToken{
		Value:     "rotated-access-token",
		ExpiresAt: authFixedNow.Add(15 * time.Minute),
	}}
	user := activeUser(t)
	stored := storedRefreshToken(t, user.ID, "hash:presented-token", authFixedNow.Add(24*time.Hour))

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().GetByHash(gomock.Any(), "hash:presented-token").Return(stored, nil)
	repo.EXPECT().GetByID(gomock.Any(), user.ID).Return(user, nil)
	refreshTokens.EXPECT().RevokeIfActive(gomock.Any(), "hash:presented-token", authFixedNow).Return(true, nil)
	refreshTokens.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, record domain.RefreshTokenRecord) (domain.RefreshTokenRecord, error) {
			require.Equal(t, user.ID, record.UserID)
			require.Equal(t, "hash:generated-refresh-token-1", record.TokenHash)
			require.Equal(t, authFixedNow.Add(refreshTTLFixture), record.ExpiresAt)
			require.NotNil(t, record.RotatedFromID)
			require.Equal(t, stored.ID, *record.RotatedFromID)
			return record, nil
		})

	uc := newAuthUseCaseWith(t, repo, refreshTokens, refreshGen, hasher, issuer, domain.UUIDv7Generator{})

	result, err := uc.Refresh(context.Background(), usecase.RefreshInput{
		RefreshToken: "presented-token",
	})
	require.NoError(t, err)
	require.Equal(t, "rotated-access-token", result.AccessToken.Value)
	require.Equal(t, int64(900), result.ExpiresIn)
	require.Equal(t, domain.RefreshToken("generated-refresh-token-1"), result.RefreshToken)
	require.Equal(t, user, result.User)
}

func TestAuthUseCase_RefreshRevokesFamilyWhenPresentedTokenIsAlreadyRevoked(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)
	user := activeUser(t)
	revokedAt := authFixedNow.Add(-time.Minute)
	stored := storedRefreshToken(t, user.ID, "hash:replayed-token", authFixedNow.Add(24*time.Hour))
	stored.RevokedAt = &revokedAt

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().GetByHash(gomock.Any(), "hash:replayed-token").Return(stored, nil)
	refreshTokens.EXPECT().RevokeAllActiveForUser(gomock.Any(), user.ID, authFixedNow).Return(nil)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	_, err := uc.Refresh(context.Background(), usecase.RefreshInput{RefreshToken: "replayed-token"})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_RefreshRejectsExpiredToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)
	user := activeUser(t)
	stored := storedRefreshToken(t, user.ID, "hash:expired-token", authFixedNow.Add(-time.Second))

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().GetByHash(gomock.Any(), "hash:expired-token").Return(stored, nil)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	_, err := uc.Refresh(context.Background(), usecase.RefreshInput{RefreshToken: "expired-token"})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_RefreshRejectsUnknownToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().GetByHash(gomock.Any(), "hash:unknown-token").Return(domain.RefreshTokenRecord{}, domain.ErrNotFound)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	_, err := uc.Refresh(context.Background(), usecase.RefreshInput{RefreshToken: "unknown-token"})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_RefreshRejectsDisabledUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)
	user := activeUser(t)
	user.Status = domain.UserStatusDisabled
	stored := storedRefreshToken(t, user.ID, "hash:disabled-token", authFixedNow.Add(24*time.Hour))

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().GetByHash(gomock.Any(), "hash:disabled-token").Return(stored, nil)
	repo.EXPECT().GetByID(gomock.Any(), user.ID).Return(user, nil)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	_, err := uc.Refresh(context.Background(), usecase.RefreshInput{RefreshToken: "disabled-token"})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_RefreshLosingTheRotationRaceIsRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)
	user := activeUser(t)
	stored := storedRefreshToken(t, user.ID, "hash:racing-token", authFixedNow.Add(24*time.Hour))

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().GetByHash(gomock.Any(), "hash:racing-token").Return(stored, nil)
	repo.EXPECT().GetByID(gomock.Any(), user.ID).Return(user, nil)
	refreshTokens.EXPECT().RevokeIfActive(gomock.Any(), "hash:racing-token", authFixedNow).Return(false, nil)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	_, err := uc.Refresh(context.Background(), usecase.RefreshInput{RefreshToken: "racing-token"})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_LogoutRevokesPresentedToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().RevokeIfActive(gomock.Any(), "hash:logout-token", authFixedNow).Return(true, nil)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	require.NoError(t, uc.Logout(context.Background(), usecase.LogoutInput{RefreshToken: "logout-token"}))
}

func TestAuthUseCase_LogoutIsIdempotentForUnknownToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().RevokeIfActive(gomock.Any(), "hash:unknown-token", authFixedNow).Return(false, nil)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	require.NoError(t, uc.Logout(context.Background(), usecase.LogoutInput{RefreshToken: "unknown-token"}))
}

func TestAuthUseCase_LogoutAllRevokesEveryTokenOfTheUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)
	user := activeUser(t)
	stored := storedRefreshToken(t, user.ID, "hash:logout-all-token", authFixedNow.Add(24*time.Hour))

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().GetByHash(gomock.Any(), "hash:logout-all-token").Return(stored, nil)
	refreshTokens.EXPECT().RevokeAllActiveForUser(gomock.Any(), user.ID, authFixedNow).Return(nil)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	require.NoError(t, uc.Logout(context.Background(), usecase.LogoutInput{RefreshToken: "logout-all-token", All: true}))
}

func TestAuthUseCase_LogoutAllIsIdempotentForUnknownToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	refreshTokens := domainmock.NewMockRefreshTokenRepository(ctrl)

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	refreshTokens.EXPECT().GetByHash(gomock.Any(), "hash:unknown-token").Return(domain.RefreshTokenRecord{}, domain.ErrNotFound)

	uc := newAuthUseCaseWith(t, repo, refreshTokens, &stubRefreshTokenGenerator{}, hasher, &stubTokenIssuer{}, domain.UUIDv7Generator{})

	require.NoError(t, uc.Logout(context.Background(), usecase.LogoutInput{RefreshToken: "unknown-token", All: true}))
}
