package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"redrx/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestShowStats(t *testing.T) {
	h, db := setupTestHandler()
	r := setupTestRouter(h)

	t.Run("404 Not Found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/MISSING/stats", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Show Stats Success", func(t *testing.T) {
		url := models.URL{
			ShortCode: "STATS",
			LongURL:   "https://google.com",
		}
		db.Create(&url)

		click := models.Click{
			URLID:     url.ID,
			IPAddress: "1.1.1.1",
			Country:   "Test Country",
		}
		db.Create(&click)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/STATS/stats", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}
