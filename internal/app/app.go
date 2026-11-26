package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"

	httpadapter "unicorn/internal/adapter/http"
	"unicorn/internal/adapter/http/handler"
	"unicorn/internal/adapter/http/middleware"
	queueadapter "unicorn/internal/adapter/repo/queue"
	"unicorn/internal/adapter/worker"
	"unicorn/internal/bootstrap"
	"unicorn/internal/platform/buildinfo"
	"unicorn/internal/platform/config"
	"unicorn/internal/platform/database"
	"unicorn/internal/platform/queue"
	"unicorn/internal/platform/security"
	"unicorn/internal/platform/telemetry"
	"unicorn/internal/usecase"
)

type Application struct {
	Config        config.Config
	Logger        zerolog.Logger
	DB            *database.DB
	RiverClient   *queue.Client
	HTTPServer    *fiber.App
	NoteUseCase   usecase.NoteUseCase
	NoteProcessor usecase.NoteJobProcessor
	flushSentry   func()
}

func New(ctx context.Context, cfg config.Config, log zerolog.Logger, opts ...Option) (*Application, error) {
	if cfg.Auth.Secret == "" {
		return nil, errors.New("auth.secret must not be empty")
	}

	deps := resolveDeps(opts)
	bi := buildinfo.Get()
	flushSentry, err := telemetry.InitSentry(cfg.Sentry, cfg.App.Env, bi)
	if err != nil {
		return nil, err
	}

	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		flushSentry()
		return nil, fmt.Errorf("open database: %w", err)
	}

	pers, err := bootstrap.OpenPersistence(db)
	if err != nil {
		_ = db.Close()
		flushSentry()
		return nil, err
	}

	noteProcessor := usecase.NewNoteJobProcessor(pers.Notes)
	workers := worker.RegisterWorkers(noteProcessor)
	registerWorkers := cfg.Server.Mode == config.ModeAll || cfg.Server.Mode == config.ModeWorker

	riverClient, err := queue.NewClient(db, cfg.River, workers, log, registerWorkers)
	if err != nil {
		_ = db.Close()
		flushSentry()
		return nil, fmt.Errorf("init river client: %w", err)
	}

	enqueuer := queueadapter.NewEnqueuer(riverClient)
	noteUC := usecase.NewNoteUseCase(
		pers.Tx,
		pers.Notes,
		enqueuer,
		deps.Clock,
		deps.IDGenerator,
	)

	var httpServer *fiber.App
	if cfg.Server.Mode == config.ModeAll || cfg.Server.Mode == config.ModeAPI {
		validate := handler.NewValidator()
		issuer := security.NewJWTIssuer(
			cfg.Auth.Issuer,
			cfg.Auth.Secret,
			cfg.Auth.AccessTTL,
			cfg.Auth.Leeway,
			deps.Clock,
			deps.IDGenerator,
		)
		authUC, err := usecase.NewAuthUseCase(
			pers.Users,
			pers.RefreshTokens,
			security.NewBcryptHasher(cfg.Auth.BcryptCost),
			issuer,
			security.NewRefreshTokenGenerator(),
			cfg.Auth.RefreshTTL,
			deps.Clock,
			deps.IDGenerator,
		)
		if err != nil {
			_ = db.Close()
			flushSentry()
			return nil, fmt.Errorf("build auth use case: %w", err)
		}

		requireAuth := middleware.RequireAuth(issuer)
		httpServer = httpadapter.NewRouter(httpadapter.RouterDeps{
			Logger:    log,
			Readiness: db,
			Routes: []httpadapter.RouteRegistrar{
				handler.NewNoteHandler(noteUC, validate, requireAuth),
				handler.NewAuthHandler(
					authUC,
					validate,
					requireAuth,
					middleware.NewAuthRateLimiter(),
					cfg.Auth.RegistrationAllowed(),
				),
			},
		})
	}

	return &Application{
		Config:        cfg,
		Logger:        log,
		DB:            db,
		RiverClient:   riverClient,
		HTTPServer:    httpServer,
		NoteUseCase:   noteUC,
		NoteProcessor: noteProcessor,
		flushSentry:   flushSentry,
	}, nil
}

func (a *Application) Run(ctx context.Context) error {
	runWorkers := a.Config.Server.Mode == config.ModeAll || a.Config.Server.Mode == config.ModeWorker
	runAPI := a.HTTPServer != nil
	bi := buildinfo.Get()

	if runWorkers {
		if err := a.RiverClient.Start(ctx); err != nil {
			_ = a.DB.Close()
			return fmt.Errorf("start river client: %w", err)
		}
		a.Logger.Info().
			Str("version", bi.Version).
			Str("commit", bi.Commit).
			Int("max_workers", a.Config.River.MaxWorkers).
			Msg("river worker started")
	}

	errCh := make(chan error, 1)
	if runAPI {
		go func() {
			a.Logger.Info().
				Str("version", bi.Version).
				Str("commit", bi.Commit).
				Str("addr", a.Config.Server.HTTPAddr).
				Str("mode", string(a.Config.Server.Mode)).
				Msg("http server listening")
			listenCfg := fiber.ListenConfig{
				DisableStartupMessage: true,
				ShutdownTimeout:       a.Config.Server.ShutdownTimeout,
			}
			if err := a.HTTPServer.Listen(a.Config.Server.HTTPAddr, listenCfg); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("fiber listen: %w", err)
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
		a.Logger.Info().Msg("shutdown signal received")
	case err := <-errCh:
		runErr = err
		a.Logger.Error().Err(err).Msg("subsystem exited unexpectedly")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.Config.Server.ShutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup
	var shutdownErr error
	var mu sync.Mutex
	recordErr := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		shutdownErr = errors.Join(shutdownErr, err)
		mu.Unlock()
	}

	if runAPI {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recordErr(a.HTTPServer.ShutdownWithContext(shutdownCtx))
		}()
	}
	if runWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recordErr(a.RiverClient.Stop(shutdownCtx))
		}()
	}
	wg.Wait()

	recordErr(a.DB.Close())
	if a.flushSentry != nil {
		a.flushSentry()
	}
	a.Logger.Info().Msg("shutdown complete")
	return errors.Join(runErr, shutdownErr)
}
