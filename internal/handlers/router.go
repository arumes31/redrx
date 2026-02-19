package handlers

import (
	"encoding/json"
	"html/template"
	"redrx/internal/services"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func (h *Handler) SetupRouter(rateLimiter *services.IPRateLimiter, templatePath string, staticPath string) *gin.Engine {
	r := gin.Default()

	r.SetFuncMap(template.FuncMap{
		"json": func(v interface{}) template.JS {
			a, _ := json.Marshal(v)
			return template.JS(a)
		},
	})

	if templatePath != "" {
		r.LoadHTMLGlob(templatePath)
	}
	if staticPath != "" {
		r.Static("/static", staticPath)
	}

	// Middleware
	if rateLimiter != nil {
		r.Use(h.RateLimitMiddleware(rateLimiter))
	}
	
	store := cookie.NewStore([]byte(h.cfg.SessionSecret))
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

	return r
}
