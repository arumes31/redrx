package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"redrx/internal/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShowDashboard(t *testing.T) {
	h, db := setupTestHandler()
	r := setupTestRouter(h)

	// Helper to set session
	r.GET("/set-session-db/:id", func(c *gin.Context) {
		session := sessions.Default(c)
		uid := uint(1)
		session.Set("user_id", uid)
		session.Save()
		c.Status(200)
	})

	t.Run("Unauthorized Redirect", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/dashboard", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/login", w.Header().Get("Location"))
	})

	t.Run("Show Dashboard - No User ID in Session", func(t *testing.T) {
		r2 := gin.New()
		// We need sessions middleware but we don't set user_id
		store := cookie.NewStore([]byte("secret"))
		r2.Use(sessions.Sessions("mysession", store))
		r2.GET("/test", h.ShowDashboard)
		
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r2.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/login", w.Header().Get("Location"))
	})

	t.Run("Show Dashboard Success", func(t *testing.T) {
		// Create User
		user := models.User{Username: "dashuser", Email: "dash@err.com", APIKey: "key3"}
		db.Create(&user)

		// Set session
		w1 := httptest.NewRecorder()
		req1, _ := http.NewRequest("GET", "/set-session-db/1", nil)
		r.ServeHTTP(w1, req1)
		cookie := w1.Header().Get("Set-Cookie")

		// Create some URLs
		db.Create(&models.URL{UserID: &user.ID, ShortCode: "D1", LongURL: "https://a.com", IsEnabled: true, ClicksCount: 5})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/dashboard", nil)
		req.Header.Set("Cookie", cookie)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("User Not Found Error", func(t *testing.T) {
		// Set session for non-existent user
		r.GET("/set-session-missing", func(c *gin.Context) {
			session := sessions.Default(c)
			session.Set("user_id", uint(9999))
			session.Save()
			c.Status(200)
		})

		w1 := httptest.NewRecorder()
		req1, _ := http.NewRequest("GET", "/set-session-missing", nil)
		r.ServeHTTP(w1, req1)
		cookie := w1.Header().Get("Set-Cookie")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/dashboard", nil)
		req.Header.Set("Cookie", cookie)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/logout", w.Header().Get("Location"))
	})
}
