package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"redrx/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestRedirectToURL(t *testing.T) {
	h, db := setupTestHandler()
	r := setupTestRouter(h)

	t.Run("404 Not Found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/NONEXISTENT", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Successful Redirect", func(t *testing.T) {
		url := models.URL{
			ShortCode: "GOOGLE",
			LongURL:   "https://google.com",
			IsEnabled: true,
			StatsEnabled: true,
		}
		db.Create(&url)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/GOOGLE", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "https://google.com", w.Header().Get("Location"))
	})

	t.Run("Link Disabled", func(t *testing.T) {
		url := models.URL{
			ShortCode: "DISABLED",
			LongURL:   "https://google.com",
			IsEnabled: true,
		}
		db.Create(&url)
		db.Model(&url).Update("is_enabled", false)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/DISABLED", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusGone, w.Code)
	})

	t.Run("Link Expired", func(t *testing.T) {
		past := time.Now().Add(-1 * time.Hour)
		url := models.URL{
			ShortCode: "EXPIRED",
			LongURL:   "https://google.com",
			IsEnabled: true,
			ExpiresAt: &past,
		}
		db.Create(&url)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/EXPIRED", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusGone, w.Code)
	})

	t.Run("Access Denied (IP)", func(t *testing.T) {
		url := models.URL{
			ShortCode:  "LOCKED",
			LongURL:    "https://google.com",
			IsEnabled:  true,
			AllowedIPs: "1.1.1.1",
		}
		db.Create(&url)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/LOCKED", nil)
		req.RemoteAddr = "8.8.8.8:1234"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Allowed IP", func(t *testing.T) {
		url := models.URL{
			ShortCode:  "IPOK",
			LongURL:    "https://google.com",
			IsEnabled:  true,
			AllowedIPs: "8.8.8.8",
		}
		db.Create(&url)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/IPOK", nil)
		req.RemoteAddr = "8.8.8.8:1234"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
	})

	t.Run("Allowed IP (Multiple)", func(t *testing.T) {
		url := models.URL{
			ShortCode:  "MULTIIP",
			LongURL:    "https://google.com",
			IsEnabled:  true,
			AllowedIPs: "1.1.1.1, 8.8.8.8",
		}
		db.Create(&url)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/MULTIIP", nil)
		req.RemoteAddr = "8.8.8.8:1234"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
	})

	t.Run("Splash Page", func(t *testing.T) {
		url := models.URL{
			ShortCode:  "SPLASH",
			LongURL:    "https://google.com",
			IsEnabled:  true,
			SplashPage: true,
		}
		db.Create(&url)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/SPLASH", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Sensitive Warning", func(t *testing.T) {
		url := models.URL{
			ShortCode:        "SENSITIVE",
			LongURL:          "https://google.com",
			IsEnabled:        true,
			SensitiveWarning: true,
		}
		db.Create(&url)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/SENSITIVE", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Sensitive Content")
	})

	t.Run("Stats Disabled", func(t *testing.T) {
		url := models.URL{
			ShortCode:    "NOSTATS",
			LongURL:      "https://google.com",
			IsEnabled:    true,
			StatsEnabled: false,
		}
		db.Create(&url)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/NOSTATS", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
	})
}
