package handlers

import (
	"net/http"
	"redrx/internal/models"
	"redrx/internal/services"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func (h *Handler) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user_id")
		if user == nil {
			// Check for API Key if session is missing
			apiKey := c.GetHeader("X-API-Key")
			if apiKey != "" {
				var u models.User
				if err := h.db.Where("api_key = ?", apiKey).First(&u).Error; err == nil {
					c.Set("user_id", u.ID)
					c.Next()
					return
				}
			}
			
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			} else {
				c.Redirect(http.StatusFound, "/login")
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *Handler) APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API Key required"})
			c.Abort()
			return
		}

		var user models.User
		if err := h.db.Where("api_key = ?", apiKey).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API Key"})
			c.Abort()
			return
		}

		c.Set("user_id", user.ID)
		c.Next()
	}
}

func (h *Handler) RateLimitMiddleware(limiter *services.IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		l := limiter.GetLimiter(ip)
		if !l.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded. Please try again later.",
			})
			return
		}
		c.Next()
	}
}
