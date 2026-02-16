package handlers

import (
	"log/slog"
	"redrx/internal/config"
	"redrx/internal/services"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Handler struct {
	cfg              config.Config
	logger           *slog.Logger
	db               *gorm.DB
	rdb              *redis.Client
	shortenerService *services.ShortenerService
	statsService     *services.StatsService
	auditService     *services.AuditService
	qrService        *services.QRService
}

func NewHandler(
	cfg config.Config,
	logger *slog.Logger,
	db *gorm.DB,
	rdb *redis.Client,
	shortenerService *services.ShortenerService,
	statsService *services.StatsService,
	auditService *services.AuditService,
	qrService *services.QRService,
) *Handler {
	return &Handler{
		cfg:              cfg,
		logger:           logger,
		db:               db,
		rdb:              rdb,
		shortenerService: shortenerService,
		statsService:     statsService,
		auditService:     auditService,
		qrService:        qrService,
	}
}
