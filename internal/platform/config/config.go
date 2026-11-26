package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

type ServerMode string

const (
	ModeAll    ServerMode = "all"
	ModeAPI    ServerMode = "api"
	ModeWorker ServerMode = "worker"
)

type Config struct {
	App      AppConfig      `koanf:"app" validate:"required"`
	Server   ServerConfig   `koanf:"server" validate:"required"`
	Database DatabaseConfig `koanf:"database" validate:"required"`
	Log      LogConfig      `koanf:"log" validate:"required"`
	River    RiverConfig    `koanf:"river" validate:"required"`
	Sentry   SentryConfig   `koanf:"sentry" validate:"required"`
}

type AppConfig struct {
	Env string `koanf:"env" validate:"required,oneof=development test staging production"`
}

type ServerConfig struct {
	Mode            ServerMode    `koanf:"mode" validate:"required,oneof=all api worker"`
	HTTPAddr        string        `koanf:"http_addr" validate:"required"`
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout" validate:"required,gt=0"`
}

type DatabaseConfig struct {
	URL          string `koanf:"url" validate:"required"`
	MaxOpenConns int    `koanf:"max_open_conns" validate:"gte=1"`
}

type LogConfig struct {
	Level  string `koanf:"level" validate:"required,oneof=trace debug info warn error fatal panic"`
	Format string `koanf:"format" validate:"required,oneof=json console"`
}

type RiverConfig struct {
	MaxWorkers int `koanf:"max_workers" validate:"gte=1"`
}

type SentryConfig struct {
	DSN              string        `koanf:"dsn"`
	Environment      string        `koanf:"environment"`
	EnableTracing    bool          `koanf:"enable_tracing"`
	TracesSampleRate float64       `koanf:"traces_sample_rate" validate:"gte=0,lte=1"`
	FlushTimeout     time.Duration `koanf:"flush_timeout" validate:"gt=0"`
}

type LoadOptions struct {
	ConfigPath  string
	DotEnvPath  string
	EnvironFunc func() []string
}

func defaultConfig() Config {
	return Config{
		App: AppConfig{
			Env: "development",
		},
		Server: ServerConfig{
			Mode:            ModeAll,
			HTTPAddr:        ":3331",
			ShutdownTimeout: 15 * time.Second,
		},
		Database: DatabaseConfig{
			URL:          "sqlite://./tmp/dev.db",
			MaxOpenConns: 20,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		River: RiverConfig{
			MaxWorkers: 10,
		},
		Sentry: SentryConfig{
			DSN:              "",
			Environment:      "",
			EnableTracing:    false,
			TracesSampleRate: 0,
			FlushTimeout:     2 * time.Second,
		},
	}
}

var flatEnvMapping = map[string]string{
	"APP_ENV":                   "app.env",
	"SERVER_MODE":               "server.mode",
	"HTTP_ADDR":                 "server.http_addr",
	"SHUTDOWN_TIMEOUT":          "server.shutdown_timeout",
	"DATABASE_URL":              "database.url",
	"DATABASE_MAX_OPEN_CONNS":   "database.max_open_conns",
	"LOG_LEVEL":                 "log.level",
	"LOG_FORMAT":                "log.format",
	"RIVER_MAX_WORKERS":         "river.max_workers",
	"SENTRY_DSN":                "sentry.dsn",
	"SENTRY_ENVIRONMENT":        "sentry.environment",
	"SENTRY_ENABLE_TRACING":     "sentry.enable_tracing",
	"SENTRY_TRACES_SAMPLE_RATE": "sentry.traces_sample_rate",
	"SENTRY_FLUSH_TIMEOUT":      "sentry.flush_timeout",
}

func Load(opts LoadOptions) (Config, error) {
	k := koanf.New(".")

	if err := k.Load(structs.Provider(defaultConfig(), "koanf"), nil); err != nil {
		return Config{}, fmt.Errorf("load default config: %w", err)
	}

	dotEnvPath := opts.DotEnvPath
	if dotEnvPath == "" {
		dotEnvPath = ".env"
	}
	if _, err := os.Stat(dotEnvPath); err == nil && opts.EnvironFunc == nil {
		if err := godotenv.Load(dotEnvPath); err != nil {
			return Config{}, fmt.Errorf("load %s: %w", dotEnvPath, err)
		}
	}

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = os.Getenv("CONFIG_FILE")
	}
	if configPath != "" {
		if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
			return Config{}, fmt.Errorf("load yaml config %q: %w", configPath, err)
		}
	}

	envProvider := env.Provider(".", env.Opt{
		EnvironFunc: opts.EnvironFunc,
		TransformFunc: func(key, value string) (string, any) {
			if mapped, ok := flatEnvMapping[key]; ok {
				return mapped, value
			}
			const prefix = "UNICORN_"
			if strings.HasPrefix(key, prefix) {
				trimmed := strings.TrimPrefix(key, prefix)
				return strings.ToLower(strings.ReplaceAll(trimmed, "_", ".")), value
			}
			return "", nil
		},
	})
	if err := k.Load(envProvider, nil); err != nil {
		return Config{}, fmt.Errorf("load env config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Validate(cfg Config) error {
	v := validator.New(validator.WithRequiredStructEnabled())
	if err := v.Struct(cfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	if !strings.HasPrefix(cfg.Database.URL, "postgres://") &&
		!strings.HasPrefix(cfg.Database.URL, "postgresql://") &&
		!strings.HasPrefix(cfg.Database.URL, "sqlite://") {
		return errors.New("invalid configuration: database.url must start with postgres://, postgresql://, or sqlite://")
	}
	return nil
}
