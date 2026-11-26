package usecase_test

import (
	"context"
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

var authFixedNow = time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)

const storedBcryptHash = "real-bcrypt-hash"

type stubTokenIssuer struct {
	issuedFor domain.Principal
	token     domain.AccessToken
	issueErr  error
}

func (s *stubTokenIssuer) IssueAccessToken(_ context.Context, principal domain.Principal) (domain.AccessToken, error) {
	s.issuedFor = principal
	return s.token, s.issueErr
}

func (s *stubTokenIssuer) VerifyAccessToken(context.Context, string) (domain.Principal, error) {
	return domain.Principal{}, nil
}

func newAuthUseCase(
	t *testing.T,
	repo domain.UserRepository,
	hasher domain.PasswordHasher,
	issuer domain.TokenIssuer,
) usecase.AuthUseCase {
	t.Helper()
	uc, err := usecase.NewAuthUseCase(repo, hasher, issuer, testkit.NewFakeClock(authFixedNow))
	require.NoError(t, err)
	return uc
}

func activeUser(t *testing.T) domain.User {
	t.Helper()
	return domain.User{
		ID:           uuid.MustParse("01930000-0000-7000-8000-0000000000f1"),
		Email:        "user@example.com",
		PasswordHash: storedBcryptHash,
		Role:         domain.RoleUser,
		Status:       domain.UserStatusActive,
	}
}

func TestAuthUseCase_LoginIssuesAccessTokenForActiveUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	issuer := &stubTokenIssuer{token: domain.AccessToken{
		Value:     "signed-access-token",
		ExpiresAt: authFixedNow.Add(15 * time.Minute),
	}}

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	repo.EXPECT().GetByEmail(gomock.Any(), "user@example.com").Return(activeUser(t), nil)
	hasher.EXPECT().Verify("correct-password", storedBcryptHash).Return(nil)

	uc := newAuthUseCase(t, repo, hasher, issuer)

	result, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    "  User@Example.COM ",
		Password: "correct-password",
	})
	require.NoError(t, err)
	require.Equal(t, "signed-access-token", result.AccessToken.Value)
	require.Equal(t, int64(900), result.ExpiresIn)
	require.Equal(t, "user@example.com", result.User.Email)
	require.Equal(t, domain.Principal{
		UserID: activeUser(t).ID,
		Roles:  []domain.Role{domain.RoleUser},
	}, issuer.issuedFor)
}

func TestAuthUseCase_LoginUnknownUserRunsDummyComparison(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	issuer := &stubTokenIssuer{}

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	repo.EXPECT().GetByEmail(gomock.Any(), "missing@example.com").Return(domain.User{}, domain.ErrNotFound)
	hasher.EXPECT().Verify("whatever-password", "dummy-bcrypt-hash").Return(domain.ErrUnauthorized)

	uc := newAuthUseCase(t, repo, hasher, issuer)

	_, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    "missing@example.com",
		Password: "whatever-password",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
	require.Empty(t, issuer.issuedFor.UserID)
}

func TestAuthUseCase_LoginWithoutCredentialRunsDummyComparison(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	issuer := &stubTokenIssuer{}

	user := activeUser(t)
	user.PasswordHash = ""

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	repo.EXPECT().GetByEmail(gomock.Any(), "user@example.com").Return(user, nil)
	hasher.EXPECT().Verify("whatever-password", "dummy-bcrypt-hash").Return(domain.ErrUnauthorized)

	uc := newAuthUseCase(t, repo, hasher, issuer)

	_, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    "user@example.com",
		Password: "whatever-password",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_LoginWrongPasswordFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	issuer := &stubTokenIssuer{}

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	repo.EXPECT().GetByEmail(gomock.Any(), "user@example.com").Return(activeUser(t), nil)
	hasher.EXPECT().Verify("wrong-password", storedBcryptHash).Return(domain.ErrUnauthorized)

	uc := newAuthUseCase(t, repo, hasher, issuer)

	_, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    "user@example.com",
		Password: "wrong-password",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_LoginDisabledUserFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)
	issuer := &stubTokenIssuer{}

	user := activeUser(t)
	user.Status = domain.UserStatusDisabled

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	repo.EXPECT().GetByEmail(gomock.Any(), "user@example.com").Return(user, nil)
	hasher.EXPECT().Verify("correct-password", storedBcryptHash).Return(nil)

	uc := newAuthUseCase(t, repo, hasher, issuer)

	_, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    "user@example.com",
		Password: "correct-password",
	})
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_MeLoadsUserFromPrincipalContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	user := activeUser(t)
	repo.EXPECT().GetByID(gomock.Any(), user.ID).Return(user, nil)

	uc := newAuthUseCase(t, repo, hasher, &stubTokenIssuer{})

	ctx := domain.WithPrincipal(context.Background(), domain.Principal{UserID: user.ID, Roles: []domain.Role{domain.RoleUser}})
	got, err := uc.Me(ctx)
	require.NoError(t, err)
	require.Equal(t, user, got)
}

func TestAuthUseCase_MeWithoutPrincipalFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)

	uc := newAuthUseCase(t, repo, hasher, &stubTokenIssuer{})

	_, err := uc.Me(context.Background())
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthUseCase_MeMissingUserFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domainmock.NewMockUserRepository(ctrl)
	hasher := domainmock.NewMockPasswordHasher(ctrl)

	hasher.EXPECT().Hash(gomock.Any()).Return("dummy-bcrypt-hash", nil)
	userID := uuid.MustParse("01930000-0000-7000-8000-0000000000f2")
	repo.EXPECT().GetByID(gomock.Any(), userID).Return(domain.User{}, domain.ErrNotFound)

	uc := newAuthUseCase(t, repo, hasher, &stubTokenIssuer{})

	ctx := domain.WithPrincipal(context.Background(), domain.Principal{UserID: userID})
	_, err := uc.Me(ctx)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}
