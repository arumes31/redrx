package services

import (
	"log/slog"
	"os"
	"testing"

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
}

func TestStatsService_MaskIP(t *testing.T) {
	service := &StatsService{}

	assert.Equal(t, "192.168.1.0", service.maskIP("192.168.1.55"))
	assert.Equal(t, "IPv6 (Masked)", service.maskIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334"))
	assert.Equal(t, "127.0.0.0", service.maskIP("127.0.0.1"))
	assert.Equal(t, "localhost", service.maskIP("localhost"))
}
