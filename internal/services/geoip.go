package services

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"redrx/internal/config"

	"github.com/oschwald/geoip2-golang"
)

var (
	geoReader *geoip2.Reader
	geoLock   sync.RWMutex
)

// InitGeoIP initializes the GeoIP service:
// 1. Checks if DB exists, if not, downloads it.
// 2. Loads the DB.
// 3. Starts a background ticker to update it periodically.
func InitGeoIP(cfg config.Config) {
	if cfg.MaxMindAccountID == "" || cfg.MaxMindLicenseKey == "" {
		log.Println("GeoIP: MaxMind credentials not set. fast-fail or skip?")
		// For now, we log and skip. Lookups will fail gracefully.
		return
	}

	dbPath := cfg.MaxMindDBPath
	dbDir := filepath.Dir(dbPath)

	// Ensure directory exists
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Printf("GeoIP: Failed to create directory %s: %v", dbDir, err)
		return
	}

	// 1. Initial Download if missing
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Println("GeoIP: Database missing, downloading...")
		if err := updateGeoDB(cfg); err != nil {
			log.Printf("GeoIP: Initial download failed: %v", err)
		}
	} else {
		log.Println("GeoIP: Database exists, skipping initial download")
	}

	// 2. Load Reader
	reloadReader(dbPath)

	// 3. Background Updater (every 24h)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			log.Println("GeoIP: Running scheduled update...")
			if err := updateGeoDB(cfg); err != nil {
				log.Printf("GeoIP: Update failed: %v", err)
				continue
			}
			reloadReader(dbPath)
		}
	}()
}

func updateGeoDB(cfg config.Config) error {
	dbDir := filepath.Dir(cfg.MaxMindDBPath)
	confPath := filepath.Join(dbDir, "GeoIP.conf")

	// Create temp config for geoipupdate tool
	// Note: geoipupdate looks for "EditionIDs", not "MaxMindEditionIDs"
	content := fmt.Sprintf("AccountID %s\nLicenseKey %s\nEditionIDs %s\nDatabaseDirectory %s\n",
		cfg.MaxMindAccountID, cfg.MaxMindLicenseKey, cfg.MaxMindEditionIDs, dbDir)

	if err := os.WriteFile(confPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write GeoIP.conf: %w", err)
	}

	// Clean up conf file after update (optional, but good practice if it contains secrets)
	// defer os.Remove(confPath) // Creating it in the persistent vol might be useful for debug though.

	// Run geoipupdate
	cmd := exec.Command("geoipupdate", "-v", "-f", confPath, "-d", dbDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("geoipupdate failed: %w, output: %s", err, string(output))
	}

	log.Println("GeoIP: Database updated successfully")
	return nil
}

func reloadReader(path string) {
	geoLock.Lock()
	defer geoLock.Unlock()

	if geoReader != nil {
		geoReader.Close()
	}

	reader, err := geoip2.Open(path)
	if err != nil {
		log.Printf("GeoIP: Failed to open database: %v", err)
		return
	}
	geoReader = reader

	meta := reader.Metadata()
	log.Printf("GeoIP: Loaded database (Epoch: %d)", meta.BuildEpoch)
}

// GetLocation returns Country, Region, City for an IP
func GetLocation(ipStr string) (country, region, city string) {
	// Handle local IPs
	if ipStr == "127.0.0.1" || ipStr == "::1" {
		return "Localhost", "Local", "Local"
	}

	geoLock.RLock()
	reader := geoReader
	geoLock.RUnlock()

	if reader == nil {
		return "Unknown", "", ""
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "Invalid IP", "", ""
	}

	record, err := reader.City(ip)
	if err != nil {
		log.Printf("GeoIP: Lookup error: %v", err)
		return "Error", "", ""
	}

	country = record.Country.IsoCode // or record.Country.Names["en"]
	if country == "" {
		country = "Unknown"
	} else {
		// Prefer full name? The dashboard handles ISO codes usually?
		// Previous implementation used "CountryName" string from ip-api
		// Let's use Name for consistency with previous behavior or ISO?
		// ip-api "country" field is usually Full Name "United States".
		// MaxMind Country.Names["en"] gives "United States".
		if name, ok := record.Country.Names["en"]; ok {
			country = name
		}
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
