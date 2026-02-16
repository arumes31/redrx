package services

import (
	"errors"
	"time"

	"redrx/internal/models"
	"redrx/internal/repository"
	"redrx/pkg/utils"

	"gorm.io/gorm"
)

type ShortenDTO struct {
	UserID           *uint
	LongURL          string
	CustomCode       string
	ExpiryHours      *int
	Password         string
	IOSTargetURL     string
	AndroidTargetURL string
	IPAddress        string // For Audit Log

	// Security
	IsLocked         bool
	AllowedIPs       string
	SplashPage       bool
	SensitiveWarning bool
}

func CreateShortURL(dto ShortenDTO) (*models.URL, error) {
	// 1. Determine Short Code
	var shortCode string
	if dto.CustomCode != "" {
		// Check availability
		var existing models.URL
		if err := repository.DB.Where("short_code = ?", dto.CustomCode).First(&existing).Error; err == nil {
			return nil, errors.New("custom code already taken")
		}
		shortCode = dto.CustomCode
	} else {
		// Generate unique code
		for {
			shortCode = utils.GenerateShortCode(6)
			var existing models.URL
			if err := repository.DB.Where("short_code = ?", shortCode).First(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
		}
	}

	// 2. Prepare Data
	var passwordHash string
	if dto.Password != "" {
		hash, err := utils.HashPassword(dto.Password)
		if err != nil {
			return nil, err
		}
		passwordHash = hash
	}

	var expiresAt *time.Time
	if dto.ExpiryHours != nil && *dto.ExpiryHours > 0 {
		t := time.Now().Add(time.Duration(*dto.ExpiryHours) * time.Hour)
		expiresAt = &t
	}

	newURL := models.URL{
		UserID:           dto.UserID,
		ShortCode:        shortCode,
		LongURL:          dto.LongURL,
		IOSTargetURL:     dto.IOSTargetURL,
		AndroidTargetURL: dto.AndroidTargetURL,
		PasswordHash:     passwordHash,
		ExpiresAt:        expiresAt,
		CreatedAt:        time.Now(),
		IsEnabled:        true,
		PreviewMode:      true, // Default
		StatsEnabled:     true, // Default
		IsLocked:         dto.IsLocked,
		AllowedIPs:       dto.AllowedIPs,
		SplashPage:       dto.SplashPage,
		SensitiveWarning: dto.SensitiveWarning,
	}

	if err := repository.DB.Create(&newURL).Error; err != nil {
		return nil, err
	}

	// Audit Log
	LogAction(dto.UserID, "CREATE_LINK", newURL.ShortCode, map[string]interface{}{
		"long_url": dto.LongURL,
	}, dto.IPAddress)

	return &newURL, nil
}
