package services

import (
	"log"

	"redrx/internal/models"
	"redrx/internal/repository"

	"github.com/mssola/user_agent"
)

var ClickChannel = make(chan models.Click, 1000)

type IPAPIResponse struct {
	Status     string `json:"status"`
	Country    string `json:"country"`
	RegionName string `json:"regionName"`
	City       string `json:"city"`
	Timezone   string `json:"timezone"`
	Message    string `json:"message"`
}

func StartStatsWorker() {
	go func() {
		for click := range ClickChannel {
			enrichClickData(&click)

			if err := repository.DB.Create(&click).Error; err != nil {
				log.Printf("Failed to record click stats: %v", err)
			}
		}
	}()
}

func RecordClickAsync(click models.Click) {
	select {
	case ClickChannel <- click:
		// Sent
	default:
		log.Println("Stats channel full, dropping click event")
	}
}

func enrichClickData(click *models.Click) {
	// 1. Parse User Agent
	ua := user_agent.New(click.Platform) // Platform field temporarily holds UA string from handler
	browserName, browserVer := ua.Browser()
	click.Browser = browserName + " " + browserVer
	click.OS = ua.OS()

	if ua.Mobile() {
		click.DeviceType = "Mobile"
	} else if ua.Bot() {
		click.DeviceType = "Bot"
	} else {
		click.DeviceType = "Desktop" // Default assumption
		// Tablets are harder to detect cleanly without better library, treating as Mobile usually
	}

	// 2. GeoIP Lookup (Local MaxMind DB)
	country, region, city := GetLocation(click.IPAddress)
	click.Country = country
	click.Region = region
	click.City = city

	// 3. Mask IP for Privacy (GDPR)
	click.IPAddress = maskIP(click.IPAddress)
}

func maskIP(ip string) string {
	// Simple string manipulation for IPv4
	// 192.168.1.50 -> 192.168.1.0
	// IPv6 is harder, just returning substring or "Masked"
	// Check for IPv4
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == '.' {
			return ip[:i] + ".0"
		}
		if ip[i] == ':' {
			// IPv6: naive masking, keep first segment or so?
			// Let's just keep first 3 segments if possible or simple truncation.
			// 2001:0db8:85a3:0000:0000:8a2e:0370:7334
			// Count colons?
			// 2001:0db8:85a3:0000:0000:8a2e:0370:7334
			return "IPv6 (Masked)"
		}
	}
	return ip
}
