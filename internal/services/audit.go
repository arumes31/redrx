package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"redrx/internal/models"

	"gorm.io/gorm"
)

type AuditService struct {
	db           *gorm.DB
	auditChannel chan models.AuditLog
	logger       *slog.Logger
}

func NewAuditService(db *gorm.DB, logger *slog.Logger) *AuditService {
	return &AuditService{
		db:           db,
		auditChannel: make(chan models.AuditLog, 100),
		logger:       logger,
	}
}

func (s *AuditService) Start(ctx context.Context) {
	s.logger.Info("Audit worker starting")
	for {
		select {
		case entry := <-s.auditChannel:
			if err := s.db.Create(&entry).Error; err != nil {
				s.logger.Error("Failed to write audit log", "error", err)
			}
		case <-ctx.Done():
			s.logger.Info("Audit worker stopping")
			return
		}
	}
}

func (s *AuditService) LogAction(userID *uint, action, entityID string, details interface{}, ip string) {
	detailBytes, _ := json.Marshal(details)

	entry := models.AuditLog{
		UserID:    userID,
		Action:    action,
		EntityID:  entityID,
		Details:   string(detailBytes),
		IPAddress: ip,
		Timestamp: time.Now(),
	}

	select {
	case s.auditChannel <- entry:
	default:
		s.logger.Warn("Audit channel full, dropping log")
	}
}
