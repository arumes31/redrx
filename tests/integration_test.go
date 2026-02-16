package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"redrx/internal/config"
	"redrx/internal/handlers"
	"redrx/internal/models"
	"redrx/internal/repository"
	"redrx/internal/services"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var (
	testDB      *gorm.DB
	testHandler *handlers.Handler
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.Default()

	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))

	r.POST("/api/register", testHandler.RegisterUser)
	r.POST("/api/login", testHandler.LoginUser)
	r.POST("/api/v1/shorten", testHandler.ShortenURL)
	r.GET("/:short_code", testHandler.RedirectToURL)
	return r
}

func TestMain(m *testing.M) {
	if err := os.Chdir(".."); err != nil {
		panic("Failed to change to project root: " + err.Error())
	}

	cfg, _ := config.LoadConfig()
	cfg.DatabaseURL = "postgres://redrx:securepassword@127.0.0.1:5432/redrx_db?sslmode=disable"
	cfg.RedisURL = "127.0.0.1:6379"

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	db, err := repository.InitDB(cfg)
	if err != nil {
		panic("Failed to initialize database: " + err.Error())
	}
	testDB = db

	if err := repository.RunMigrations(cfg.DatabaseURL); err != nil {
		panic("Migration failed: " + err.Error())
	}

	// Initialize Services
	auditService := services.NewAuditService(db, logger)
	geoIPService := services.NewGeoIPService(cfg, logger)
	statsService := services.NewStatsService(db, logger, geoIPService)
	shortenerService := services.NewShortenerService(db, auditService)
	qrService := services.NewQRService()

	// Start workers for test
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go auditService.Start(ctx)
	go statsService.Start(ctx)

	testHandler = handlers.NewHandler(cfg, logger, db, nil, shortenerService, statsService, auditService, qrService)

	// Clean up user for test
	testDB.Where("username = ?", "testuser_integration").Delete(&models.User{})

	code := m.Run()
	os.Exit(code)
}

func TestRegisterAndLogin(t *testing.T) {
	r := setupRouter()

	// 1. Register
	w := httptest.NewRecorder()
	reqBody, _ := json.Marshal(map[string]string{
		"username": "testuser_integration",
		"email":    "test_int@example.com",
		"password": "password123",
	})
	req, _ := http.NewRequest("POST", "/api/register", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.True(t, w.Code == http.StatusCreated || w.Code == http.StatusConflict)

	// 2. Login
	w = httptest.NewRecorder()
	loginBody, _ := json.Marshal(map[string]string{
		"username": "testuser_integration",
		"password": "password123",
	})
	req, _ = http.NewRequest("POST", "/api/login", bytes.NewBuffer(loginBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestShortenAndRedirect(t *testing.T) {
	r := setupRouter()

	w := httptest.NewRecorder()
	shortenBody, _ := json.Marshal(map[string]string{
		"long_url": "https://example.com/integration-test",
	})
	req, _ := http.NewRequest("POST", "/api/v1/shorten", bytes.NewBuffer(shortenBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	shortCode := response["short_code"].(string)
	assert.NotEmpty(t, shortCode)

	// Test Redirect
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/"+shortCode, nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "https://example.com/integration-test", w.Result().Header.Get("Location"))
}
