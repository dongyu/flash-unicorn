package security_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"unicorn/internal/domain"
	"unicorn/internal/platform/security"
	"unicorn/internal/testkit"
)

const (
	testIssuer     = "unicorn-test"
	testSigningKey = "unicorn-test-signing-key-0000000001"
	testAccessTTL  = 15 * time.Minute
	testLeeway     = 30 * time.Second
)

var fixedNow = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

type parsedClaims struct {
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

func newTestIssuer(clock domain.Clock) *security.JWTIssuer {
	return security.NewJWTIssuer(testIssuer, testSigningKey, testAccessTTL, testLeeway, clock, &testkit.SeqIDGenerator{})
}

func signToken(t *testing.T, method jwt.SigningMethod, key any, claims jwt.Claims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(method, claims).SignedString(key)
	require.NoError(t, err)
	return signed
}

func TestJWTIssuer_IssuesAccessTokenWithExpectedClaims(t *testing.T) {
	clock := testkit.NewFakeClock(fixedNow)
	issuer := newTestIssuer(clock)
	userID := uuid.MustParse("01930000-0000-7000-8000-0000000000aa")

	token, err := issuer.IssueAccessToken(context.Background(), domain.Principal{UserID: userID, Roles: []domain.Role{domain.RoleAdmin}})
	require.NoError(t, err)
	require.NotEmpty(t, token.Value)
	require.True(t, fixedNow.Add(testAccessTTL).Equal(token.ExpiresAt), "expires at %s", token.ExpiresAt)

	parsed, err := jwt.ParseWithClaims(token.Value, &parsedClaims{}, func(*jwt.Token) (any, error) {
		return []byte(testSigningKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	require.NoError(t, err)

	claims, ok := parsed.Claims.(*parsedClaims)
	require.True(t, ok)
	require.Equal(t, userID.String(), claims.Subject)
	require.Equal(t, testIssuer, claims.Issuer)
	require.NotEmpty(t, claims.ID)
	require.Equal(t, []string{"admin"}, claims.Roles)
	require.True(t, fixedNow.Equal(claims.IssuedAt.Time))
	require.True(t, fixedNow.Add(testAccessTTL).Equal(claims.ExpiresAt.Time))
}

func TestJWTIssuer_VerifiesIssuedToken(t *testing.T) {
	issuer := newTestIssuer(testkit.NewFakeClock(fixedNow))
	principal := domain.Principal{
		UserID: uuid.MustParse("01930000-0000-7000-8000-0000000000bb"),
		Roles:  []domain.Role{domain.RoleUser},
	}

	token, err := issuer.IssueAccessToken(context.Background(), principal)
	require.NoError(t, err)

	verified, err := issuer.VerifyAccessToken(context.Background(), token.Value)
	require.NoError(t, err)
	require.Equal(t, principal.UserID, verified.UserID)
	require.True(t, verified.HasRole(domain.RoleUser))
	require.False(t, verified.HasRole(domain.RoleAdmin))
}

func TestJWTIssuer_AcceptsTokenWithinLeeway(t *testing.T) {
	clock := testkit.NewFakeClock(fixedNow)
	issuer := newTestIssuer(clock)

	token, err := issuer.IssueAccessToken(context.Background(), domain.Principal{UserID: uuid.MustParse("01930000-0000-7000-8000-0000000000cc"), Roles: []domain.Role{domain.RoleUser}})
	require.NoError(t, err)

	clock.Advance(testAccessTTL + testLeeway - time.Second)
	_, err = issuer.VerifyAccessToken(context.Background(), token.Value)
	require.NoError(t, err)

	clock.Advance(2 * time.Second)
	_, err = issuer.VerifyAccessToken(context.Background(), token.Value)
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestJWTIssuer_RejectsInvalidTokens(t *testing.T) {
	userID := uuid.MustParse("01930000-0000-7000-8000-0000000000dd")
	validClaims := func() parsedClaims {
		return parsedClaims{
			Roles: []string{"user"},
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   userID.String(),
				Issuer:    testIssuer,
				IssuedAt:  jwt.NewNumericDate(fixedNow),
				ExpiresAt: jwt.NewNumericDate(fixedNow.Add(testAccessTTL)),
				ID:        "01930000-0000-7000-8000-0000000000ee",
			},
		}
	}

	tests := []struct {
		name  string
		token func(t *testing.T) string
	}{
		{
			name:  "malformed",
			token: func(*testing.T) string { return "not-a-jwt" },
		},
		{
			name: "wrong signing key",
			token: func(t *testing.T) string {
				return signToken(t, jwt.SigningMethodHS256, []byte("another-signing-key-000000000001"), validClaims())
			},
		},
		{
			name: "unexpected algorithm",
			token: func(t *testing.T) string {
				return signToken(t, jwt.SigningMethodHS512, []byte(testSigningKey), validClaims())
			},
		},
		{
			name: "wrong issuer",
			token: func(t *testing.T) string {
				claims := validClaims()
				claims.Issuer = "some-other-issuer"
				return signToken(t, jwt.SigningMethodHS256, []byte(testSigningKey), claims)
			},
		},
		{
			name: "missing expiration",
			token: func(t *testing.T) string {
				claims := validClaims()
				claims.ExpiresAt = nil
				return signToken(t, jwt.SigningMethodHS256, []byte(testSigningKey), claims)
			},
		},
		{
			name: "future issued at",
			token: func(t *testing.T) string {
				claims := validClaims()
				claims.IssuedAt = jwt.NewNumericDate(fixedNow.Add(time.Hour))
				return signToken(t, jwt.SigningMethodHS256, []byte(testSigningKey), claims)
			},
		},
		{
			name: "unknown role",
			token: func(t *testing.T) string {
				claims := validClaims()
				claims.Roles = []string{"root"}
				return signToken(t, jwt.SigningMethodHS256, []byte(testSigningKey), claims)
			},
		},
		{
			name: "invalid subject",
			token: func(t *testing.T) string {
				claims := validClaims()
				claims.Subject = "not-a-uuid"
				return signToken(t, jwt.SigningMethodHS256, []byte(testSigningKey), claims)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newTestIssuer(testkit.NewFakeClock(fixedNow)).VerifyAccessToken(context.Background(), tt.token(t))
			require.ErrorIs(t, err, domain.ErrUnauthorized)
		})
	}
}
