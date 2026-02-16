package services

import (
	"encoding/json"
	"log"
	"time"

	"redrx/internal/models"
	"redrx/internal/repository"
)

var AuditChannel = make(chan models.AuditLog, 100)

func StartAuditWorker() {
	go func() {
		for entry := range AuditChannel {
			if err := repository.DB.Create(&entry).Error; err != nil {
				log.Printf("Failed to write audit log: %v", err)
			}
		}
	}()
}

func LogAction(userID *uint, action, entityID string, details interface{}, ip string) {
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
	case AuditChannel <- entry:
	default:
		log.Println("Audit channel full, dropping log")
	}
}
