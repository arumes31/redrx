package models

import (
	"time"
)

type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    *uint     `gorm:"index" json:"user_id"`           // Nullable for anonymous actions (if any) or login attempts
	Action    string    `gorm:"size:50;not null" json:"action"` // e.g., "LOGIN", "CREATE_LINK", "DELETE_LINK"
	EntityID  string    `gorm:"size:50" json:"entity_id"`       // ID of the object affected (e.g. ShortCode or UserID)
	Details   string    `gorm:"type:text" json:"details"`       // JSON or text description
	IPAddress string    `gorm:"size:45" json:"ip_address"`
	Timestamp time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"timestamp"`
}
