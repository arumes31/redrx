package services

import (
	"log/slog"
	"os"
	"testing"

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
}
