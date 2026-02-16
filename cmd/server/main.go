package main

import (
	"encoding/json"
	"html/template"
	"log"

	"redrx/internal/config"
	"redrx/internal/handlers"
	"redrx/internal/middleware"
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
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. Initialize Database
	db, err := repository.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	_ = db // Suppress unused var for now

	// Initialize Redis
	_, err = repository.InitRedis(cfg.RedisURL, "", 0)
	if err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
	}

	// 3. Run Migrations
	log.Println("Running database migrations...")
	if err := repository.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Printf("Migration warning (or error): %v", err)
	}

	// 4. Start Background Services
	go services.InitGeoIP(cfg) // Run in goroutine to not block startup if download is slow
	services.StartStatsWorker()
	services.StartAuditWorker()

	// 5. Setup Router
	r := gin.Default()
	r.SetFuncMap(template.FuncMap{
		"json": func(v interface{}) template.JS {
			a, _ := json.Marshal(v)
			return template.JS(a)
		},
	})
	r.LoadHTMLGlob("web/templates/*")
	r.Static("/static", "./web/static")

	// Global Rate Limit
	r.Use(middleware.RateLimitMiddleware())

	// Session Middleware
	store := cookie.NewStore([]byte("secret")) // TODO: Load from config
	r.Use(sessions.Sessions("mysession", store))

	// Routes
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "healthy",
		})
	})

	// Frontend Routes
	r.GET("/", handlers.ShowIndex)
	r.POST("/", handlers.HandleShortenForm)

	r.GET("/login", handlers.ShowLogin)
	r.POST("/login", handlers.HandleLoginForm)

	r.GET("/register", handlers.ShowRegister)
	r.POST("/register", handlers.HandleRegisterForm)

	// Auth Routes (API)
	r.POST("/api/register", handlers.RegisterUser)
	r.POST("/api/login", handlers.LoginUser)
	r.POST("/logout", handlers.LogoutUser)

	// Protected Routes Group
	authorized := r.Group("/")
	authorized.Use(middleware.AuthRequired())
	{
		authorized.GET("/dashboard", handlers.ShowDashboard)
		authorized.POST("/api/v1/shorten", handlers.ShortenURL)

		// Account Management
		authorized.POST("/api/v1/auth/apikey", handlers.GenerateNewAPIKey)
		authorized.DELETE("/api/v1/auth/account", handlers.DeleteAccount)
	}

	// Redirect Route (Catch-all for short codes)
	r.GET("/:short_code", handlers.RedirectToURL)
	r.GET("/:short_code/stats", handlers.ShowStats)

	// 6. Start Server
	log.Printf("Starting server on port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
