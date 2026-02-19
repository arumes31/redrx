package services

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"redrx/internal/config"

	"github.com/oschwald/geoip2-golang"
	"github.com/oschwald/maxminddb-golang"
	"github.com/stretchr/testify/assert"
)

type mockGeoIPReader struct {
	cityFunc     func(ip net.IP) (*geoip2.City, error)
	metadataFunc func() maxminddb.Metadata
	closeFunc    func() error
}

func (m *mockGeoIPReader) City(ip net.IP) (*geoip2.City, error) { return m.cityFunc(ip) }
func (m *mockGeoIPReader) Metadata() maxminddb.Metadata        { return m.metadataFunc() }
func (m *mockGeoIPReader) Close() error                        { return m.closeFunc() }

func TestNewGeoIPService(t *testing.T) {
	cfg := config.Config{}
	logger := slog.Default()
	service := NewGeoIPService(cfg, logger)

	assert.NotNil(t, service)
	assert.Equal(t, cfg, service.cfg)
	assert.Equal(t, logger, service.logger)
}

func TestGeoIPService_Init_Disabled(t *testing.T) {
	cfg := config.Config{
		MaxMindAccountID: "",
	}
	service := NewGeoIPService(cfg, slog.Default())
	service.Init()
	assert.Nil(t, service.geoReader)
}

func TestGeoIPService_Init_InvalidPath(t *testing.T) {
	// This will try to create a directory in a place it probably can't, 
	// or we just check the error log path.
	cfg := config.Config{
		MaxMindAccountID: "test",
		MaxMindLicenseKey: "test",
		MaxMindDBPath: "/invalid/path/to/db.mmdb",
	}
	service := NewGeoIPService(cfg, slog.Default())
	service.Init()
	assert.Nil(t, service.geoReader)
}

func TestGeoIPService_Init_MkdirError(t *testing.T) {
	tempFile, err := os.CreateTemp("", "geoip-file")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// Try to use the file as a directory path
	cfg := config.Config{
		MaxMindAccountID:  "test",
		MaxMindLicenseKey: "test",
		MaxMindDBPath:     filepath.Join(tempFile.Name(), "db.mmdb"),
	}
	service := NewGeoIPService(cfg, slog.Default())
	service.Init()
	assert.Nil(t, service.geoReader)
}

func TestGeoIPService_GetLocation(t *testing.T) {
	service := NewGeoIPService(config.Config{}, slog.Default())

	t.Run("Localhost IPv4", func(t *testing.T) {
		c, r, city := service.GetLocation("127.0.0.1")
		assert.Equal(t, "Localhost", c)
		assert.Equal(t, "Local", r)
		assert.Equal(t, "Local", city)
	})

	t.Run("Localhost IPv6", func(t *testing.T) {
		c, r, city := service.GetLocation("::1")
		assert.Equal(t, "Localhost", c)
		assert.Equal(t, "Local", r)
		assert.Equal(t, "Local", city)
	})

	t.Run("Nil Reader", func(t *testing.T) {
		c, r, city := service.GetLocation("8.8.8.8")
		assert.Equal(t, "Unknown", c)
		assert.Equal(t, "", r)
		assert.Equal(t, "", city)
	})

	t.Run("Invalid IP", func(t *testing.T) {
		mock := &mockGeoIPReader{}
		service.geoReader = mock
		defer func() { service.geoReader = nil }()

		c, _, _ := service.GetLocation("not-an-ip")
		assert.Equal(t, "Invalid IP", c)
	})

	t.Run("Reader Success", func(t *testing.T) {
		mock := &mockGeoIPReader{
			cityFunc: func(ip net.IP) (*geoip2.City, error) {
				return &geoip2.City{
					Country: struct {
						Names             map[string]string `maxminddb:"names"`
						IsoCode           string            `maxminddb:"iso_code"`
						GeoNameID         uint              `maxminddb:"geoname_id"`
						IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
					}{
						Names:   map[string]string{"en": "United States"},
						IsoCode: "US",
					},
					City: struct {
						Names     map[string]string `maxminddb:"names"`
						GeoNameID uint              `maxminddb:"geoname_id"`
					}{
						Names: map[string]string{"en": "New York"},
					},
					Subdivisions: []struct {
						Names     map[string]string `maxminddb:"names"`
						IsoCode   string            `maxminddb:"iso_code"`
						GeoNameID uint              `maxminddb:"geoname_id"`
					}{
						{Names: map[string]string{"en": "New York"}},
					},
				}, nil
			},
		}
		service.geoReader = mock
		defer func() { service.geoReader = nil }()

		c, r, city := service.GetLocation("8.8.8.8")
		assert.Equal(t, "United States", c)
		assert.Equal(t, "New York", r)
		assert.Equal(t, "New York", city)
	})

	t.Run("Reader Success - Country IsoCode only", func(t *testing.T) {
		mock := &mockGeoIPReader{
			cityFunc: func(ip net.IP) (*geoip2.City, error) {
				return &geoip2.City{
					Country: struct {
						Names             map[string]string `maxminddb:"names"`
						IsoCode           string            `maxminddb:"iso_code"`
						GeoNameID         uint              `maxminddb:"geoname_id"`
						IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
					}{
						IsoCode: "FR",
					},
				}, nil
			},
		}
		service.geoReader = mock
		defer func() { service.geoReader = nil }()

		c, _, _ := service.GetLocation("8.8.8.8")
		assert.Equal(t, "FR", c)
	})

	t.Run("Reader Success - No Country Info", func(t *testing.T) {
		mock := &mockGeoIPReader{
			cityFunc: func(ip net.IP) (*geoip2.City, error) {
				return &geoip2.City{}, nil
			},
		}
		service.geoReader = mock
		defer func() { service.geoReader = nil }()

		c, _, _ := service.GetLocation("8.8.8.8")
		assert.Equal(t, "Unknown", c)
	})

	t.Run("Reader Error", func(t *testing.T) {
		mock := &mockGeoIPReader{
			cityFunc: func(ip net.IP) (*geoip2.City, error) {
				return nil, errors.New("db error")
			},
		}
		service.geoReader = mock
		defer func() { service.geoReader = nil }()

		c, _, _ := service.GetLocation("8.8.8.8")
		assert.Equal(t, "Error", c)
	})
}

func TestGeoIPService_StartUpdater_Loop(t *testing.T) {
	cfg := config.Config{
		MaxMindAccountID: "test",
		MaxMindDBPath:    "invalid",
	}
	service := NewGeoIPService(cfg, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	go service.StartUpdaterWithInterval(ctx, 10*time.Millisecond)

	time.Sleep(25 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)
}

func TestGeoIPService_StartUpdater_Interval_Disabled(t *testing.T) {
	service := NewGeoIPService(config.Config{}, slog.Default())
	service.StartUpdaterWithInterval(context.Background(), 10*time.Millisecond)
}

func TestGeoIPService_StartUpdater_Stop(t *testing.T) {
	cfg := config.Config{
		MaxMindAccountID: "test",
	}
	service := NewGeoIPService(cfg, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	
	go service.StartUpdater(ctx)
	
	time.Sleep(10 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)
	// If it doesn't hang, it's good
}

func TestGeoIPService_StartUpdater_Disabled(t *testing.T) {
	service := NewGeoIPService(config.Config{}, slog.Default())
	service.StartUpdater(context.Background()) // Should return immediately
}

func TestGeoIPService_ReloadReader_Error(t *testing.T) {
	service := NewGeoIPService(config.Config{}, slog.Default())
	service.reloadReader("non-existent-file")
	assert.Nil(t, service.geoReader)
}

func TestGeoIPService_ReloadReader_Exists(t *testing.T) {
	service := NewGeoIPService(config.Config{}, slog.Default())
	closed := false
	mock := &mockGeoIPReader{
		closeFunc: func() error {
			closed = true
			return nil
		},
	}
	service.geoReader = mock

	service.reloadReader("non-existent")
	assert.True(t, closed)
	assert.Nil(t, service.geoReader) // Because Open failed
}

func TestGeoIPService_UpdateGeoDB_Error(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "geoip-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.mmdb")
	cfg := config.Config{
		MaxMindAccountID: "test",
		MaxMindLicenseKey: "test",
		MaxMindDBPath: dbPath,
	}
	service := NewGeoIPService(cfg, slog.Default())
	
	err = service.updateGeoDB()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "geoipupdate failed")
}

func TestGeoIPService_UpdateGeoDB_WriteError(t *testing.T) {
	tempFile, err := os.CreateTemp("", "geoip-file-2")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	cfg := config.Config{
		MaxMindDBPath: filepath.Join(tempFile.Name(), "db.mmdb"),
	}
	service := NewGeoIPService(cfg, slog.Default())
	err = service.updateGeoDB()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write GeoIP.conf")
}
