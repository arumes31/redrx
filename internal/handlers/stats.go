package handlers

import (
	"net/http"

	"redrx/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ShowStats(c *gin.Context) {
	shortCode := c.Param("short_code")

	var urlEntry models.URL
	if err := h.db.Where("short_code = ?", shortCode).First(&urlEntry).Error; err != nil {
		c.HTML(http.StatusNotFound, "404.html", nil)
		return
	}

	// Fetch recent clicks (last 50)
	var recentClicks []models.Click
	h.db.Where("url_id = ?", urlEntry.ID).Order("timestamp desc").Limit(50).Find(&recentClicks)

	// Aggregations
	var countryStats []struct {
		Country string
		Count   int
	}
	h.db.Model(&models.Click{}).Where("url_id = ?", urlEntry.ID).Select("country, count(*) as count").Group("country").Order("count desc").Scan(&countryStats)

	var browserStats []struct {
		Browser string
		Count   int
	}
	h.db.Model(&models.Click{}).Where("url_id = ?", urlEntry.ID).Select("browser, count(*) as count").Group("browser").Order("count desc").Scan(&browserStats)

	var osStats []struct {
		OS    string
		Count int
	}
	h.db.Model(&models.Click{}).Where("url_id = ?", urlEntry.ID).Select("os, count(*) as count").Group("os").Order("count desc").Scan(&osStats)

	var deviceStats []struct {
		DeviceType string
		Count      int
	}
	h.db.Model(&models.Click{}).Where("url_id = ?", urlEntry.ID).Select("device_type, count(*) as count").Group("device_type").Order("count desc").Scan(&deviceStats)

	c.HTML(http.StatusOK, "stats.html", gin.H{
		"URL":          urlEntry,
		"RecentClicks": recentClicks,
		"BaseURL":      "https://" + c.Request.Host,
		"CountryStats": countryStats,
		"BrowserStats": browserStats,
		"OSStats":      osStats,
		"DeviceStats":  deviceStats,
	})
}
