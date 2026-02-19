package services

import (
	"log/slog"
	"os"
	"redrx/pkg/utils"
	"strings"
	"testing"
	"time"

	"redrx/internal/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database: " + err.Error())
	}
	err = db.AutoMigrate(&models.URL{}, &models.AuditLog{}, &models.Click{}, &models.User{})
	if err != nil {
		panic("failed to migrate database: " + err.Error())
	}
	return db
}

func TestCreateShortURL(t *testing.T) {
	db := setupTestDB()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditService(db, logger)
	service := NewShortenerService(db, audit)

	t.Run("Create random short URL", func(t *testing.T) {
		dto := ShortenDTO{
			LongURL: "https://google.com",
		}
		url, err := service.CreateShortURL(dto)
		
		assert.NoError(t, err)
		assert.NotEmpty(t, url.ShortCode)
		assert.Equal(t, "https://google.com", url.LongURL)
	})

	t.Run("Collision Retry", func(t *testing.T) {
		calls := 0
		service.codeGenerator = func(int) string {
			calls++
			if calls == 1 {
				return "COLLIDE"
			}
			return "UNIQUE"
		}
		defer func() { service.codeGenerator = utils.GenerateShortCode }()

		// Create first URL
		db.Create(&models.URL{ShortCode: "COLLIDE", LongURL: "https://a.com"})

		dto := ShortenDTO{LongURL: "https://b.com"}
		url, err := service.CreateShortURL(dto)

		assert.NoError(t, err)
		assert.Equal(t, "UNIQUE", url.ShortCode)
		assert.Equal(t, 2, calls)
	})

	t.Run("Create custom short URL", func(t *testing.T) {
		dto := ShortenDTO{
			LongURL:    "https://yahoo.com",
			CustomCode: "YAHOO",
		}
		url, err := service.CreateShortURL(dto)
		
		assert.NoError(t, err)
		assert.Equal(t, "YAHOO", url.ShortCode)
	})

	t.Run("Duplicate custom code should fail", func(t *testing.T) {
		dto := ShortenDTO{
			LongURL:    "https://bing.com",
			CustomCode: "BING",
		}
		_, err := service.CreateShortURL(dto)
		assert.NoError(t, err)

		_, err = service.CreateShortURL(dto)
		assert.Error(t, err)
		assert.Equal(t, "custom code already taken", err.Error())
	})

	t.Run("Create short URL with Password and Expiry", func(t *testing.T) {
		hours := 24
		dto := ShortenDTO{
			LongURL:     "https://github.com",
			Password:    "secure-pass",
			ExpiryHours: &hours,
		}
		url, err := service.CreateShortURL(dto)
		
		assert.NoError(t, err)
		assert.NotEmpty(t, url.PasswordHash)
		assert.NotNil(t, url.ExpiresAt)
		assert.True(t, url.ExpiresAt.After(time.Now()))
	})

	t.Run("HashPassword Error", func(t *testing.T) {
		dto := ShortenDTO{
			LongURL:  "https://github.com",
			Password: strings.Repeat("A", 100), // Max is 72
		}
		_, err := service.CreateShortURL(dto)
		assert.Error(t, err)
	})

	t.Run("DB Create Error", func(t *testing.T) {
		dbErr := setupTestDB()
		dbErr.Migrator().DropTable(&models.URL{})
		auditErr := NewAuditService(dbErr, logger)
		serviceErr := NewShortenerService(dbErr, auditErr)
		
		dto := ShortenDTO{
			LongURL: "https://github.com",
		}
		_, err := serviceErr.CreateShortURL(dto)
		assert.Error(t, err)
	})

	t.Run("DB Error during custom code check", func(t *testing.T) {
		dbErr := setupTestDB()
		dbErr.Migrator().DropTable(&models.URL{})
		auditErr := NewAuditService(dbErr, logger)
		serviceErr := NewShortenerService(dbErr, auditErr)
		
		dto := ShortenDTO{
			LongURL:    "https://github.com",
			CustomCode: "MOCK",
		}
		_, err := serviceErr.CreateShortURL(dto)
		assert.Error(t, err)
	})

	t.Run("DB Error during random code check", func(t *testing.T) {
		dbErr := setupTestDB()
		dbErr.Migrator().DropTable(&models.URL{})
		auditErr := NewAuditService(dbErr, logger)
		serviceErr := NewShortenerService(dbErr, auditErr)
		
		dto := ShortenDTO{
			LongURL: "https://github.com",
		}
		_, err := serviceErr.CreateShortURL(dto)
		assert.Error(t, err)
	})
}
