package handlers

import (
	"log/slog"
	"os"
	"redrx/internal/config"
	"redrx/internal/models"
	"redrx/internal/services"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func setupTestHandler() (*Handler, *gorm.DB) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.URL{}, &models.AuditLog{}, &models.Click{}, &models.User{})
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Config{
		SessionSecret: "test-secret-12345678901234567890123456789012",
	}

	audit := services.NewAuditService(db, logger)
	geoIP := services.NewGeoIPService(cfg, logger)
	stats := services.NewStatsService(db, logger, geoIP)
	shortener := services.NewShortenerService(db, audit)
	qr := services.NewQRService()

	// Use a dummy redis client (not connected) with no retries
	rdb := redis.NewClient(&redis.Options{
		Addr:       "localhost:1",
		MaxRetries: -1,
	})

	h := NewHandler(cfg, logger, db, rdb, shortener, stats, audit, qr)
	return h, db
}

func setupTestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	return h.SetupRouter(nil, "../../web/templates/*.html", "../../web/static")
}
