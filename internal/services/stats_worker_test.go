package services

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"redrx/internal/config"
	"redrx/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestStatsService_EnrichClickData(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	geoIP := NewGeoIPService(config.Config{}, logger)
	service := NewStatsService(nil, logger, geoIP)

	t.Run("Enrich Mobile User Agent", func(t *testing.T) {
		click := &models.Click{
			Platform:  "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0 Mobile/15E148 Safari/604.1",
			IPAddress: "1.2.3.4",
		}
		service.enrichClickData(click)

		assert.Equal(t, "Mobile", click.DeviceType)
		assert.Contains(t, click.Browser, "Safari")
		assert.Equal(t, "1.2.3.0", click.IPAddress) // Masked
	})

	t.Run("Enrich Desktop User Agent", func(t *testing.T) {
		click := &models.Click{
			Platform:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
			IPAddress: "8.8.8.8",
		}
		service.enrichClickData(click)

		assert.Equal(t, "Desktop", click.DeviceType)
		assert.Contains(t, click.Browser, "Chrome")
		assert.Equal(t, "8.8.8.0", click.IPAddress)
	})

	t.Run("Enrich Bot User Agent", func(t *testing.T) {
		click := &models.Click{
			Platform:  "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			IPAddress: "66.249.66.1",
		}
		service.enrichClickData(click)

		assert.Equal(t, "Bot", click.DeviceType)
	})
}

func TestStatsService_MaskIP(t *testing.T) {
	service := &StatsService{}

	assert.Equal(t, "192.168.1.0", service.maskIP("192.168.1.55"))
	assert.Equal(t, "IPv6 (Masked)", service.maskIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334"))
	assert.Equal(t, "127.0.0.0", service.maskIP("127.0.0.1"))
	assert.Equal(t, "localhost", service.maskIP("localhost"))
}

func TestStatsService_Worker(t *testing.T) {
	db := setupTestDB()
	logger := slog.Default()
	geoIP := NewGeoIPService(config.Config{}, logger)
	service := NewStatsService(db, logger, geoIP)

	ctx, cancel := context.WithCancel(context.Background())
	go service.Start(ctx)

	t.Run("Record Click Async", func(t *testing.T) {
		click := models.Click{
			URLID:     1,
			IPAddress: "1.1.1.1",
			Platform:  "Test",
		}
		service.RecordClickAsync(click)
		
		time.Sleep(100 * time.Millisecond)
		
		var count int64
		db.Model(&models.Click{}).Count(&count)
		assert.Equal(t, int64(1), count)
	})

	t.Run("Channel Full", func(t *testing.T) {
		// Create a service with small channel
		smallService := &StatsService{
			clickChannel: make(chan models.Click, 1),
			logger:       logger,
		}
		smallService.RecordClickAsync(models.Click{}) // Fill
		smallService.RecordClickAsync(models.Click{}) // Should drop
	})

	t.Run("DB Error", func(t *testing.T) {
		dbErr := setupTestDB()
		dbErr.Migrator().DropTable(&models.Click{})
		serviceErr := NewStatsService(dbErr, logger, geoIP)
		
		ctxErr, cancelErr := context.WithCancel(context.Background())
		go serviceErr.Start(ctxErr)
		
		serviceErr.RecordClickAsync(models.Click{IPAddress: "1.1.1.1"})
		time.Sleep(100 * time.Millisecond)
		cancelErr()
	})

	cancel()
	time.Sleep(50 * time.Millisecond)
}
