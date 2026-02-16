package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"redrx/internal/config"

	"github.com/oschwald/geoip2-golang"
)

type GeoIPService struct {
	cfg       config.Config
	logger    *slog.Logger
	geoReader *geoip2.Reader
	geoLock   sync.RWMutex
}

func NewGeoIPService(cfg config.Config, logger *slog.Logger) *GeoIPService {
	return &GeoIPService{
		cfg:    cfg,
		logger: logger,
	}
}

func (s *GeoIPService) Init() {
	if s.cfg.MaxMindAccountID == "" || s.cfg.MaxMindLicenseKey == "" {
		s.logger.Warn("GeoIP: MaxMind credentials not set. Lookups will be disabled.")
		return
	}

	dbPath := s.cfg.MaxMindDBPath
	dbDir := filepath.Dir(dbPath)

	if err := os.MkdirAll(dbDir, 0755); err != nil {
		s.logger.Error("GeoIP: Failed to create directory", "dir", dbDir, "error", err)
		return
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		s.logger.Info("GeoIP: Database missing, downloading...")
		if err := s.updateGeoDB(); err != nil {
			s.logger.Error("GeoIP: Initial download failed", "error", err)
		}
	}

	s.reloadReader(dbPath)
}

func (s *GeoIPService) StartUpdater(ctx context.Context) {
	if s.cfg.MaxMindAccountID == "" {
		return
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.logger.Info("GeoIP: Running scheduled update...")
			if err := s.updateGeoDB(); err != nil {
				s.logger.Error("GeoIP: Update failed", "error", err)
				continue
			}
			s.reloadReader(s.cfg.MaxMindDBPath)
		case <-ctx.Done():
			s.logger.Info("GeoIP: Updater stopping")
			return
		}
	}
}

func (s *GeoIPService) updateGeoDB() error {
	dbDir := filepath.Dir(s.cfg.MaxMindDBPath)
	confPath := filepath.Join(dbDir, "GeoIP.conf")

	content := fmt.Sprintf("AccountID %s\nLicenseKey %s\nEditionIDs %s\nDatabaseDirectory %s\n",
		s.cfg.MaxMindAccountID, s.cfg.MaxMindLicenseKey, s.cfg.MaxMindEditionIDs, dbDir)

	if err := os.WriteFile(confPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write GeoIP.conf: %w", err)
	}
	defer os.Remove(confPath)

	cmd := exec.Command("geoipupdate", "-v", "-f", confPath, "-d", dbDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("geoipupdate failed: %w, output: %s", err, string(output))
	}

	s.logger.Info("GeoIP: Database updated successfully")
	return nil
}

func (s *GeoIPService) reloadReader(path string) {
	s.geoLock.Lock()
	defer s.geoLock.Unlock()

	if s.geoReader != nil {
		s.geoReader.Close()
	}

	reader, err := geoip2.Open(path)
	if err != nil {
		s.logger.Error("GeoIP: Failed to open database", "path", path, "error", err)
		return
	}
	s.geoReader = reader

	meta := reader.Metadata()
	s.logger.Info("GeoIP: Loaded database", "epoch", meta.BuildEpoch)
}

func (s *GeoIPService) GetLocation(ipStr string) (country, region, city string) {
	if ipStr == "127.0.0.1" || ipStr == "::1" {
		return "Localhost", "Local", "Local"
	}

	s.geoLock.RLock()
	reader := s.geoReader
	s.geoLock.RUnlock()

	if reader == nil {
		return "Unknown", "", ""
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "Invalid IP", "", ""
	}

	record, err := reader.City(ip)
	if err != nil {
		s.logger.Error("GeoIP: Lookup error", "ip", ipStr, "error", err)
		return "Error", "", ""
	}

	if name, ok := record.Country.Names["en"]; ok {
		country = name
	} else {
		country = record.Country.IsoCode
	}

	if country == "" {
		country = "Unknown"
	}

	if len(record.Subdivisions) > 0 {
		if name, ok := record.Subdivisions[0].Names["en"]; ok {
			region = name
		}
	}

	if name, ok := record.City.Names["en"]; ok {
		city = name
	}

	return country, region, city
}
