package middleware

import (
	"net/http"
	"redrx/internal/models"
	"redrx/internal/repository"

	"github.com/gin-gonic/gin"
)

func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API Key required"})
			c.Abort()
			return
		}

		var user models.User
		if err := repository.DB.Where("api_key = ?", apiKey).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API Key"})
			c.Abort()
			return
		}

		// Set user_id in context for handlers to use
		// Note: This matches session key "user_id" but in context
		c.Set("user_id", user.ID)

		c.Next()
	}
}
