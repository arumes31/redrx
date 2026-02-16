package models

import (
	"time"
)

type URL struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	UserID           *uint      `json:"user_id,omitempty"` // Nullable for anonymous
	User             *User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ShortCode        string     `gorm:"unique;not null;size:20;index" json:"short_code"`
	LongURL          string     `gorm:"not null;type:text" json:"long_url"`
	RotateTargets    string     `gorm:"type:text" json:"rotate_targets"` // Stored as JSON string
	IOSTargetURL     string     `gorm:"type:text" json:"ios_target_url,omitempty"`
	AndroidTargetURL string     `gorm:"type:text" json:"android_target_url,omitempty"`
	PasswordHash     string     `gorm:"size:255" json:"-"`
	PreviewMode      bool       `gorm:"default:true" json:"preview_mode"`
	StatsEnabled     bool       `gorm:"default:true" json:"stats_enabled"`
	IsEnabled        bool       `gorm:"default:true;index" json:"is_enabled"`
	ClicksCount      int        `gorm:"column:clicks;default:0" json:"clicks_count"`
	CreatedAt        time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	StartAt          *time.Time `json:"start_at,omitempty"`
	EndAt            *time.Time `json:"end_at,omitempty"`
	LastAccessedAt   *time.Time `json:"last_accessed_at,omitempty"`

	// Security & Features
	IsLocked         bool   `gorm:"default:false" json:"is_locked"`
	AllowedIPs       string `gorm:"type:text" json:"allowed_ips"` // Comma separated
	SplashPage       bool   `gorm:"default:false" json:"splash_page"`
	SensitiveWarning bool   `gorm:"default:false" json:"sensitive_warning"`

	Clicks []Click `gorm:"foreignKey:URLID;constraint:OnDelete:CASCADE" json:"clicks,omitempty"`
}

// TableName overrides the table name used by User to `profiles`
func (URL) TableName() string {
	return "urls"
}
