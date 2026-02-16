package services

import (
	"context"
	"log/slog"

	"redrx/internal/models"

	"github.com/mssola/user_agent"
	"gorm.io/gorm"
)

type StatsService struct {
	db           *gorm.DB
	logger       *slog.Logger
	clickChannel chan models.Click
	geoIPService *GeoIPService
}

func NewStatsService(db *gorm.DB, logger *slog.Logger, geoIPService *GeoIPService) *StatsService {
	return &StatsService{
		db:           db,
		logger:       logger,
		clickChannel: make(chan models.Click, 1000),
		geoIPService: geoIPService,
	}
}

func (s *StatsService) Start(ctx context.Context) {
	s.logger.Info("Stats worker starting")
	for {
		select {
		case click := <-s.clickChannel:
			s.enrichClickData(&click)

			if err := s.db.Create(&click).Error; err != nil {
				s.logger.Error("Failed to record click stats", "error", err)
			}
		case <-ctx.Done():
			s.logger.Info("Stats worker stopping")
			return
		}
	}
}

func (s *StatsService) RecordClickAsync(click models.Click) {
	select {
	case s.clickChannel <- click:
		// Sent
	default:
		s.logger.Warn("Stats channel full, dropping click event")
	}
}

func (s *StatsService) enrichClickData(click *models.Click) {
	// 1. Parse User Agent
	ua := user_agent.New(click.Platform)
	browserName, browserVer := ua.Browser()
	click.Browser = browserName + " " + browserVer
	click.OS = ua.OS()

	if ua.Mobile() {
		click.DeviceType = "Mobile"
	} else if ua.Bot() {
		click.DeviceType = "Bot"
	} else {
		click.DeviceType = "Desktop"
	}

	// 2. GeoIP Lookup
	country, region, city := s.geoIPService.GetLocation(click.IPAddress)
	click.Country = country
	click.Region = region
	click.City = city

	// 3. Mask IP for Privacy (GDPR)
	click.IPAddress = s.maskIP(click.IPAddress)
}

func (s *StatsService) maskIP(ip string) string {
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == '.' {
			return ip[:i] + ".0"
		}
		if ip[i] == ':' {
			return "IPv6 (Masked)"
		}
	}
	return ip
}
