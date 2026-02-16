package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"redrx/internal/config"
	"redrx/internal/handlers"
	"redrx/internal/repository"
	"redrx/internal/services"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
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
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}

	// 4. Initialize Redis
	rdb, err := repository.InitRedis(cfg.RedisURL, cfg.RedisPassword, 0)
	if err != nil {
		logger.Warn("Failed to connect to Redis", "error", err)
	}

	// 5. Run Migrations
	logger.Info("Running database migrations...")
	if err := repository.RunMigrations(cfg.DatabaseURL); err != nil {
		logger.Warn("Migration warning", "error", err)
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

	r := gin.Default()
	r.SetFuncMap(template.FuncMap{
		"json": func(v interface{}) template.JS {
			a, _ := json.Marshal(v)
			return template.JS(a)
		},
	})
	r.LoadHTMLGlob("web/templates/*")
	r.Static("/static", "./web/static")

	// Middleware
	r.Use(h.RateLimitMiddleware(rateLimiter))
	store := cookie.NewStore([]byte(cfg.SessionSecret))
	r.Use(sessions.Sessions("redrx_session", store))

	// Routes
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// Public Routes
	r.GET("/", h.ShowIndex)
	r.POST("/", h.HandleShortenForm)
	r.GET("/login", h.ShowLogin)
	r.POST("/login", h.HandleLoginForm)
	r.GET("/register", h.ShowRegister)
	r.POST("/register", h.HandleRegisterForm)
	r.POST("/api/register", h.RegisterUser)
	r.POST("/api/login", h.LoginUser)
	r.POST("/logout", h.LogoutUser)

	// Protected Routes
	authorized := r.Group("/")
	authorized.Use(h.AuthRequired())
	{
		authorized.GET("/dashboard", h.ShowDashboard)
		authorized.POST("/api/v1/shorten", h.ShortenURL)
		authorized.POST("/api/v1/auth/apikey", h.GenerateNewAPIKey)
		authorized.DELETE("/api/v1/auth/account", h.DeleteAccount)
	}

	// Catch-all Redirects
	r.GET("/:short_code", h.RedirectToURL)
	r.GET("/:short_code/stats", h.ShowStats)

	// 9. Start Server with Graceful Shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Background Context for services
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Background Workers
	go auditService.Start(ctx)
	go statsService.Start(ctx)
	go geoIPService.Init() // Init might take time (download)
	go geoIPService.StartUpdater(ctx)
	rateLimiter.StartCleanup(10 * time.Minute)

	// Initializing server in a goroutine so that it doesn't block the graceful shutdown handling
	go func() {
		logger.Info("Starting server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	// Stop background workers
	cancel()
	// Wait a bit for workers to finish if needed
	time.Sleep(500 * time.Millisecond)

	logger.Info("Server exiting")
}
