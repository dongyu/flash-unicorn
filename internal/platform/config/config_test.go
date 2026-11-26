package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"unicorn/internal/platform/config"
)

const fixturePostgresDSN = "postgres://yaml:yaml@localhost:5432/yaml?sslmode=disable" //nolint:gosec

const fixtureSigningKey = "fixture-signing-key-0123456789abcdef"

func TestLoad_YAMLThenEnvOverrides(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "config.yaml")
	yamlContent := fmt.Sprintf(`
app:
  env: production
server:
  mode: worker
  http_addr: ":9090"
  shutdown_timeout: 30s
database:
  url: "%s"
  max_open_conns: 15
log:
  level: warn
  format: json
river:
  max_workers: 25
auth:
  issuer: yaml-issuer
  secret: fixture-signing-key-0123456789abcdef
  access_ttl: 5m
  leeway: 10s
  bcrypt_cost: 12
  allow_registration: true
sentry:
  dsn: "https://examplePublicKey@o0.ingest.sentry.io/0"
  enable_tracing: true
  traces_sample_rate: 0.25
`, fixturePostgresDSN)
	require.NoError(t, os.WriteFile(yamlPath, []byte(yamlContent), 0o600))

	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: yamlPath,
		EnvironFunc: func() []string {
			return []string{
				"SERVER_MODE=api",
				"DATABASE_URL=sqlite://./tmp/override.db",
				"SENTRY_TRACES_SAMPLE_RATE=0.5",
			}
		},
	})
	require.NoError(t, err)

	require.Equal(t, "production", cfg.App.Env)
	require.Equal(t, ":9090", cfg.Server.HTTPAddr)
	require.Equal(t, 30*time.Second, cfg.Server.ShutdownTimeout)
	require.Equal(t, 25, cfg.River.MaxWorkers)
	require.True(t, cfg.Sentry.EnableTracing)

	require.Equal(t, config.ModeAPI, cfg.Server.Mode)
	require.Equal(t, "sqlite://./tmp/override.db", cfg.Database.URL)
	require.InDelta(t, 0.5, cfg.Sentry.TracesSampleRate, 0.0001)
	require.Equal(t, "yaml-issuer", cfg.Auth.Issuer)
	require.Equal(t, fixtureSigningKey, cfg.Auth.Secret)
	require.Equal(t, 5*time.Minute, cfg.Auth.AccessTTL)
	require.Equal(t, 10*time.Second, cfg.Auth.Leeway)
	require.True(t, cfg.Auth.RegistrationAllowed())
}

func TestLoad_RejectsUnsupportedDatabaseScheme(t *testing.T) {
	_, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{
				"DATABASE_URL=mysql://user:pass@localhost:3306/db",
			}
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "postgres://")
}

func TestLoad_AuthBcryptCostDefaultsToFour(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string { return nil },
	})
	require.NoError(t, err)
	require.Equal(t, 4, cfg.Auth.BcryptCost)
}

func TestLoad_AuthBcryptCostFromEnv(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{"AUTH_BCRYPT_COST=10"}
		},
	})
	require.NoError(t, err)
	require.Equal(t, 10, cfg.Auth.BcryptCost)
}

func TestLoad_RejectsWeakBcryptCostInProduction(t *testing.T) {
	_, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{"APP_ENV=production", "AUTH_BCRYPT_COST=6"}
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "auth.bcrypt_cost")
}

func TestLoad_AcceptsProductionBcryptCostAtFloor(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{
				"APP_ENV=production",
				"AUTH_BCRYPT_COST=10",
				"AUTH_SECRET=" + fixtureSigningKey,
				"AUTH_ALLOW_REGISTRATION=true",
			}
		},
	})
	require.NoError(t, err)
	require.Equal(t, 10, cfg.Auth.BcryptCost)
}

func TestLoad_AuthDefaults(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string { return nil },
	})
	require.NoError(t, err)
	require.Equal(t, "unicorn", cfg.Auth.Issuer)
	require.GreaterOrEqual(t, len(cfg.Auth.Secret), 32)
	require.Equal(t, 15*time.Minute, cfg.Auth.AccessTTL)
	require.Equal(t, 30*time.Second, cfg.Auth.Leeway)
}

func TestLoad_AuthFromEnv(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{
				"AUTH_ISSUER=acme",
				"AUTH_SECRET=" + fixtureSigningKey,
				"AUTH_ACCESS_TTL=5m",
				"AUTH_LEEWAY=10s",
			}
		},
	})
	require.NoError(t, err)
	require.Equal(t, "acme", cfg.Auth.Issuer)
	require.Equal(t, fixtureSigningKey, cfg.Auth.Secret)
	require.Equal(t, 5*time.Minute, cfg.Auth.AccessTTL)
	require.Equal(t, 10*time.Second, cfg.Auth.Leeway)
}

func TestLoad_RejectsShortAuthSecret(t *testing.T) {
	_, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{"AUTH_SECRET=too-short"}
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "auth.secret")
}

func TestLoad_RejectsDevelopmentSecretOutsideDevelopment(t *testing.T) {
	for _, env := range []string{"test", "staging", "production"} {
		t.Run(env, func(t *testing.T) {
			_, err := config.Load(config.LoadOptions{
				EnvironFunc: func() []string {
					return []string{"APP_ENV=" + env, "AUTH_BCRYPT_COST=10"}
				},
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "auth.secret")
		})
	}
}

func TestLoad_AcceptsCustomAuthSecretOutsideDevelopment(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{"APP_ENV=staging", "AUTH_SECRET=" + fixtureSigningKey}
		},
	})
	require.NoError(t, err)
	require.Equal(t, fixtureSigningKey, cfg.Auth.Secret)
}

func TestLoad_RejectsBcryptCostOutsideRange(t *testing.T) {
	for _, raw := range []string{"3", "15"} {
		t.Run(raw, func(t *testing.T) {
			_, err := config.Load(config.LoadOptions{
				EnvironFunc: func() []string {
					return []string{"AUTH_BCRYPT_COST=" + raw}
				},
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "bcrypt_cost")
		})
	}
}

func TestLoad_RegistrationEnabledByDefaultOutsideProduction(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string { return nil },
	})
	require.NoError(t, err)
	require.True(t, cfg.Auth.RegistrationAllowed())
}

func TestLoad_RegistrationDisabledByDefaultInProduction(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{
				"APP_ENV=production",
				"AUTH_BCRYPT_COST=10",
				"AUTH_SECRET=" + fixtureSigningKey,
			}
		},
	})
	require.NoError(t, err)
	require.False(t, cfg.Auth.RegistrationAllowed())
}

func TestLoad_AcceptsProductionRegistrationExplicitlyEnabled(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{
				"APP_ENV=production",
				"AUTH_BCRYPT_COST=10",
				"AUTH_SECRET=" + fixtureSigningKey,
				"AUTH_ALLOW_REGISTRATION=true",
			}
		},
	})
	require.NoError(t, err)
	require.True(t, cfg.Auth.RegistrationAllowed())
}

func TestLoad_AcceptsRegistrationDisabledByEnv(t *testing.T) {
	cfg, err := config.Load(config.LoadOptions{
		EnvironFunc: func() []string {
			return []string{"AUTH_ALLOW_REGISTRATION=false"}
		},
	})
	require.NoError(t, err)
	require.False(t, cfg.Auth.RegistrationAllowed())
}
