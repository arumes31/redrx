package models

import (
	"time"
)

type Click struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	URLID      uint      `gorm:"not null;index" json:"url_id"`
	Timestamp  time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"timestamp"`
	IPAddress  string    `gorm:"size:45" json:"ip_address,omitempty"`
	Country    string    `gorm:"size:100;default:'Unknown'" json:"country"`
	City       string    `gorm:"size:100" json:"city"`
	Region     string    `gorm:"size:100" json:"region"`
	Browser    string    `gorm:"size:50" json:"browser"` // Parsed Browser Name
	OS         string    `gorm:"size:100" json:"os"`
	DeviceType string    `gorm:"size:50" json:"device_type"`
	Platform   string    `gorm:"size:255" json:"platform"` // Stores Raw User-Agent temporarily
	Referrer   string    `gorm:"size:255;default:'Direct'" json:"referrer"`
}
