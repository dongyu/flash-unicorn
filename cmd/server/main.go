package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/riverqueue/river/rivermigrate"

	"unicorn/internal/app"
	"unicorn/internal/platform/buildinfo"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/platform/logger"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
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
		if err := config.Validate(cfg); err != nil {
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

func runConfigCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: server config {db-url|app-env} [--format=raw|migrate] [--config=path]")
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
		return fmt.Errorf("usage: server config {db-url|app-env} [--format=raw|migrate] [--config=path]")
	}
	return nil
}

func runMigrateCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: server migrate {river-up|river-down|river-reset|river-status} [--config=path]")
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
