package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/riverqueue/river/rivermigrate"

	"unicorn/internal/app"
	"unicorn/internal/domain"
	"unicorn/internal/platform/buildinfo"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/platform/logger"
	"unicorn/internal/seed"
)

const (
	configUsage   = "usage: server config {db-url|app-env} [--format=raw|migrate] [--config=path]"
	migrateUsage  = "usage: server migrate {river-up|river-down|river-reset|river-status} [--config=path]"
	seedUserUsage = "usage: server seed-user --email <email> [--role user|admin] [--display-name <name>] [--config=path]"
)

func main() {
	if err := run(os.Args[1:], os.Stdin); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader) error {
	if len(args) > 0 {
		switch args[0] {
		case "version":
			bi := buildinfo.Get()
			fmt.Printf("version=%s commit=%s date=%s go=%s\n", bi.Version, bi.Commit, bi.Date, bi.GoVersion)
			return nil
		case "config":
			return runConfigCmd(args[1:])
		case "migrate":
			return runMigrateCmd(args[1:])
		case "seed-user":
			return runSeedUserCmd(args[1:], stdin)
		case "run":
			args = args[1:]
		}
	}

	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	configPath := fs.String("config", "", "optional path to YAML configuration file")
	mode := fs.String("mode", "", "override runtime mode (all|api|worker)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(config.LoadOptions{ConfigPath: *configPath})
	if err != nil {
		return err
	}
	if *mode != "" {
		cfg.Server.Mode = config.ServerMode(*mode)
		if err = config.Validate(cfg); err != nil {
			return err
		}
	}

	log := logger.New(cfg.Log, os.Stdout)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	return application.Run(ctx)
}

func runSeedUserCmd(args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("seed-user", flag.ContinueOnError)
	configPath := fs.String("config", "", "optional path to YAML configuration file")
	email := fs.String("email", "", "email address of the user to create")
	role := fs.String("role", string(domain.RoleUser), "role of the user (user|admin)")
	displayName := fs.String("display-name", "", "display name of the user")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" || fs.NArg() > 0 {
		return errors.New(seedUserUsage)
	}

	password, err := readPassword(stdin)
	if err != nil {
		return err
	}

	cfg, err := config.Load(config.LoadOptions{ConfigPath: *configPath})
	if err != nil {
		return err
	}

	user, err := seed.Run(context.Background(), cfg, seed.UserInput{
		Email:       *email,
		Password:    password,
		Role:        domain.Role(*role),
		DisplayName: *displayName,
	})
	if err != nil {
		return err
	}

	fmt.Printf("created user %s (%s) role=%s\n", user.ID, user.Email, user.Role)
	return nil
}

func readPassword(stdin io.Reader) (string, error) {
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("password must be provided on stdin")
	}
	return password, nil
}

func runConfigCmd(args []string) error {
	if len(args) == 0 {
		return errors.New(configUsage)
	}
	sub := args[0]
	fs := flag.NewFlagSet("config "+sub, flag.ContinueOnError)
	configPath := fs.String("config", "", "optional path to YAML configuration file")
	format := fs.String("format", "raw", "output format for db-url: raw | migrate")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := config.Load(config.LoadOptions{ConfigPath: *configPath})
	if err != nil {
		return err
	}

	switch sub {
	case "db-url":
		switch *format {
		case "raw":
			fmt.Println(cfg.Database.URL)
		case "migrate":
			u, err := database.MigrateURL(cfg.Database.URL)
			if err != nil {
				return err
			}
			fmt.Println(u)
		default:
			return fmt.Errorf("unsupported format %q (expected raw or migrate)", *format)
		}
	case "app-env":
		fmt.Println(cfg.App.Env)
	default:
		return errors.New(configUsage)
	}
	return nil
}

func runMigrateCmd(args []string) error {
	if len(args) == 0 {
		return errors.New(migrateUsage)
	}
	sub := args[0]
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	configPath := fs.String("config", "", "optional path to YAML configuration file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := config.Load(config.LoadOptions{ConfigPath: *configPath})
	if err != nil {
		return err
	}

	ctx := context.Background()
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	switch sub {
	case "river-up":
		res, err := database.MigrateRiver(ctx, db, rivermigrate.DirectionUp, nil)
		if err != nil {
			return err
		}
		fmt.Printf("river-up complete (%d versions applied)\n", len(res.Versions))
	case "river-down":
		res, err := database.MigrateRiver(ctx, db, rivermigrate.DirectionDown, &rivermigrate.MigrateOpts{MaxSteps: 1})
		if err != nil {
			return err
		}
		fmt.Printf("river-down complete (%d versions reverted)\n", len(res.Versions))
	case "river-reset":
		res, err := database.MigrateRiver(ctx, db, rivermigrate.DirectionDown, &rivermigrate.MigrateOpts{TargetVersion: -1})
		if err != nil {
			return err
		}
		fmt.Printf("river-reset complete (%d versions reverted)\n", len(res.Versions))
	case "river-status":
		versions, err := database.ExistingRiverVersions(ctx, db)
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			fmt.Println("no river migrations applied")
			return nil
		}
		fmt.Printf("latest river migration version: %d (%d applied)\n", versions[len(versions)-1].Version, len(versions))
	default:
		return fmt.Errorf("unknown migrate subcommand %q", sub)
	}
	return nil
}
