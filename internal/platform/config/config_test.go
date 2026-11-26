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
  bcrypt_cost: 12
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
			return []string{"APP_ENV=production", "AUTH_BCRYPT_COST=10"}
		},
	})
	require.NoError(t, err)
	require.Equal(t, 10, cfg.Auth.BcryptCost)
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
