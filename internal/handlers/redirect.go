package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"redrx/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) RedirectToURL(c *gin.Context) {
	shortCode := c.Param("short_code")

	var urlEntry models.URL
	ctx := c.Request.Context()

	// 1. Redis Cache Lookup (Full Object)
	cacheHit := false
	if h.rdb != nil {
		val, err := h.rdb.Get(ctx, "url:"+shortCode).Result()
		if err == nil {
			if err := json.Unmarshal([]byte(val), &urlEntry); err == nil {
				cacheHit = true
			}
		}
	}

	// 2. DB Lookup (if Cache Miss)
	if !cacheHit {
		if err := h.db.Where("short_code = ?", shortCode).First(&urlEntry).Error; err != nil {
			c.HTML(http.StatusNotFound, "404.html", gin.H{"error": "Link not found"})
			return
		}
		// Write to Cache
		if h.rdb != nil {
			data, _ := json.Marshal(urlEntry)
			h.rdb.Set(ctx, "url:"+shortCode, data, 10*time.Minute)
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
		h.statsService.RecordClickAsync(click)
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
