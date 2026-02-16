package handlers

import (
	"log/slog"
	"os"
	"redrx/internal/config"
	"redrx/internal/models"
	"redrx/internal/services"

	"github.com/glebarez/sqlite"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupTestHandler() (*Handler, *gorm.DB) {
	db, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	db.AutoMigrate(&models.URL{}, &models.AuditLog{}, &models.Click{}, &models.User{})
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Config{}

	audit := services.NewAuditService(db, logger)
	geoIP := services.NewGeoIPService(cfg, logger)
	stats := services.NewStatsService(db, logger, geoIP)
	shortener := services.NewShortenerService(db, audit)
	qr := services.NewQRService()

	h := NewHandler(cfg, logger, db, nil, shortener, stats, audit, qr)
	return h, db
}

func setupTestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))
	
	return r
}
