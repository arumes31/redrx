package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"redrx/internal/models"
	"redrx/internal/services"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestMiddlewares(t *testing.T) {
	h, db := setupTestHandler()
	r := setupTestRouter(h)

	t.Run("AuthRequired - Unauthorized Redirect", func(t *testing.T) {
		r.GET("/protected", h.AuthRequired(), func(c *gin.Context) {
			c.Status(200)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/protected", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/login", w.Header().Get("Location"))
	})

	t.Run("AuthRequired - Unauthorized API", func(t *testing.T) {
		r.GET("/api/v1/protected", h.AuthRequired(), func(c *gin.Context) {
			c.Status(200)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/protected", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("AuthRequired - API Key Success", func(t *testing.T) {
		user := models.User{Username: "apikeyuser", Email: "api1@err.com", APIKey: "valid-key"}
		db.Create(&user)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/protected", nil)
		req.Header.Set("X-API-Key", "valid-key")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("AuthRequired - Session Success", func(t *testing.T) {
		// Set session helper
		r.GET("/set-session-mw", func(c *gin.Context) {
			session := sessions.Default(c)
			session.Set("user_id", uint(999))
			session.Save()
			c.Status(200)
		})

		w1 := httptest.NewRecorder()
		req1, _ := http.NewRequest("GET", "/set-session-mw", nil)
		r.ServeHTTP(w1, req1)
		cookie := w1.Header().Get("Set-Cookie")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/protected", nil)
		req.Header.Set("Cookie", cookie)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("APIKeyAuth - Missing Key", func(t *testing.T) {
		r.GET("/api/key-only", h.APIKeyAuth(), func(c *gin.Context) {
			c.Status(200)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/key-only", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("APIKeyAuth - Invalid Key", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/key-only", nil)
		req.Header.Set("X-API-Key", "invalid")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("APIKeyAuth - Success", func(t *testing.T) {
		user := models.User{Username: "apikeyuser2", Email: "api2@err.com", APIKey: "valid-key-2"}
		db.Create(&user)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/key-only", nil)
		req.Header.Set("X-API-Key", "valid-key-2")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("RateLimitMiddleware", func(t *testing.T) {
		limiter := services.NewIPRateLimiter(rate.Limit(1), 1, h.logger)
		r.GET("/limited", h.RateLimitMiddleware(limiter), func(c *gin.Context) {
			c.Status(200)
		})

		// First request allowed
		w1 := httptest.NewRecorder()
		req1, _ := http.NewRequest("GET", "/limited", nil)
		r.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusOK, w1.Code)

		// Second request blocked
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("GET", "/limited", nil)
		r.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusTooManyRequests, w2.Code)
	})
}
