package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"redrx/internal/config"
	"redrx/internal/handlers"
	"redrx/internal/repository"
	"redrx/internal/services"

	"github.com/gin-gonic/gin"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func Run(ctx context.Context) error {
	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Setup Logger
	var handler slog.Handler
	if cfg.AppEnv == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 3. Initialize Database
	db, err := repository.InitDB(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// 4. Initialize Redis
	rdb, err := repository.InitRedis(cfg.RedisURL, cfg.RedisPassword, 0)
	if err != nil {
		logger.Warn("Failed to connect to Redis", "error", err)
	}

	// 5. Run Migrations
	if strings.HasPrefix(cfg.DatabaseURL, "postgres") {
		logger.Info("Running database migrations...")
		if err := repository.RunMigrations(cfg.DatabaseURL, ""); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// 6. Initialize Services
	auditService := services.NewAuditService(db, logger)
	geoIPService := services.NewGeoIPService(cfg, logger)
	statsService := services.NewStatsService(db, logger, geoIPService)
	shortenerService := services.NewShortenerService(db, auditService)
	qrService := services.NewQRService()
	rateLimiter := services.NewIPRateLimiter(5, 10, logger)

	// 7. Initialize Handler
	h := handlers.NewHandler(cfg, logger, db, rdb, shortenerService, statsService, auditService, qrService)

	// 8. Setup Router
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := h.SetupRouter(rateLimiter, "web/templates/*", "./web/static")

	// 9. Start Server with Graceful Shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Background Context for workers
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	// Start Background Workers
	go auditService.Start(workerCtx)
	go statsService.Start(workerCtx)
	go geoIPService.Init()
	go geoIPService.StartUpdater(workerCtx)
	rateLimiter.StartCleanup(10 * time.Minute)

	// Initializing server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("Starting server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		logger.Info("Shutting down server...")
	}

	// Graceful shutdown timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	workerCancel()
	// Wait a tiny bit for workers
	time.Sleep(100 * time.Millisecond)

	logger.Info("Server exiting")
	return nil
}
