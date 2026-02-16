package middleware

import (
	"net/http"

	"redrx/internal/models"
	"redrx/internal/repository"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Check Session
		session := sessions.Default(c)
		userID := session.Get("user_id")
		if userID != nil {
			c.Next()
			return
		}

		// 2. Check API Key
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			var user models.User
			if err := repository.DB.Where("api_key = ?", apiKey).First(&user).Error; err == nil {
				// Set user_id in context (same key as session uses effectively when we need ID)
				// Session middleware sets it in session store, here we set context value.
				// Handlers using session.Get("user_id") will fail if we don't mock session?
				// Wait, session.Get reads from cookie store.
				// We need handlers to check Context FIRST, then Session?
				// OR we wrap session get logic in a helper "GetUserID(c)".

				// Let's set it in context "user_id".
				c.Set("user_id", user.ID)
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		c.Abort()
	}
}
