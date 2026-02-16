package handlers

import (
	"net/http"

	"redrx/internal/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ShowDashboard(c *gin.Context) {
	session := sessions.Default(c)
	userIDVal := session.Get("user_id")
	if userIDVal == nil {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	userID := userIDVal.(uint)

	// Fetch Stats
	var totalLinks int64
	h.db.Model(&models.URL{}).Where("user_id = ?", userID).Count(&totalLinks)

	var totalClicks int64
	h.db.Model(&models.URL{}).Where("user_id = ?", userID).Select("COALESCE(SUM(clicks_count), 0)").Scan(&totalClicks)

	var activeLinks int64
	h.db.Model(&models.URL{}).Where("user_id = ? AND is_enabled = ?", userID, true).Count(&activeLinks)

	// Fetch URLs (Limit 20 for now, implement pagination later)
	var urls []models.URL
	h.db.Where("user_id = ?", userID).Order("created_at desc").Limit(20).Find(&urls)

	// Fetch User Details (for API Key)
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		// Handle error (logout?)
		c.Redirect(http.StatusFound, "/logout")
		return
	}

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"User":        user,
		"TotalLinks":  totalLinks,
		"TotalClicks": totalClicks,
		"ActiveLinks": activeLinks,
		"URLs":        urls,
		"BaseURL":     c.Request.Host, // Useful for copy script
	})
}
