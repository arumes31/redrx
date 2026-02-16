package models

import (
	"time"
)

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"unique;not null;size:80" json:"username"`
	Email        string    `gorm:"unique;not null;size:120" json:"email"`
	PasswordHash string    `gorm:"not null;size:255" json:"-"`
	APIKey       string    `gorm:"unique;index;size:36" json:"api_key"`
	CreatedAt    time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	URLs         []URL     `gorm:"foreignKey:UserID" json:"urls,omitempty"`
}
