package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"redrx/internal/models"
	"redrx/internal/repository"
	"redrx/internal/services"

	"github.com/gin-gonic/gin"
)

func RedirectToURL(c *gin.Context) {
	shortCode := c.Param("short_code")

	var urlEntry models.URL
	ctx := context.Background()

	// 1. Redis Cache Lookup (Full Object)
	cacheHit := false
	if repository.Rdb != nil {
		val, err := repository.Rdb.Get(ctx, "url:"+shortCode).Result()
		if err == nil {
			if err := json.Unmarshal([]byte(val), &urlEntry); err == nil {
				cacheHit = true
			}
		}
	}

	// 2. DB Lookup (if Cache Miss)
	if !cacheHit {
		if err := repository.DB.Where("short_code = ?", shortCode).First(&urlEntry).Error; err != nil {
			c.HTML(http.StatusNotFound, "404.html", gin.H{"error": "Link not found"})
			return
		}
		// Write to Cache
		if repository.Rdb != nil {
			data, _ := json.Marshal(urlEntry)
			repository.Rdb.Set(ctx, "url:"+shortCode, data, 10*time.Minute)
		}
	}

	// 3. Validation
	if !urlEntry.IsEnabled {
		c.HTML(http.StatusGone, "410.html", gin.H{"error": "Link disabled"})
		return
	}

	now := time.Now()
	if urlEntry.ExpiresAt != nil && now.After(*urlEntry.ExpiresAt) {
		c.HTML(http.StatusGone, "410.html", gin.H{"error": "Link expired"})
		return
	}

	// 4. Password Check (if applicable)
	// This would require a redirect to an interstitial password page or checking session
	// keeping simple for now.

	// 5. Security Checks
	if urlEntry.AllowedIPs != "" {
		allowed := false
		clientIP := c.ClientIP()
		ips := strings.Split(urlEntry.AllowedIPs, ",")
		for _, ip := range ips {
			if strings.TrimSpace(ip) == clientIP {
				allowed = true
				break
			}
		}
		if !allowed {
			c.HTML(http.StatusForbidden, "403.html", gin.H{"error": "Access Denied"})
			return
		}
	}

	// 6. Async Stats
	if urlEntry.StatsEnabled {
		click := models.Click{
			URLID:     urlEntry.ID,
			Timestamp: time.Now(),
			IPAddress: c.ClientIP(),
			Referrer:  c.Request.Referer(),
			Platform:  c.Request.UserAgent(),
		}
		services.RecordClickAsync(click)
	}

	// 7. Splash Page / Warning
	if urlEntry.SplashPage || urlEntry.SensitiveWarning {
		c.HTML(http.StatusOK, "splash.html", gin.H{
			"URL":              urlEntry,
			"SensitiveWarning": urlEntry.SensitiveWarning,
		})
		return
	}

	// 8. Redirect
	c.Redirect(http.StatusFound, urlEntry.LongURL)
}
