package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"redrx/internal/models"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShortenURLHandler(t *testing.T) {
	h, db := setupTestHandler()
	r := setupTestRouter(h)
	
	// Helper to set session
	r.GET("/set-session-shorten/:id", func(c *gin.Context) {
		session := sessions.Default(c)
		uid := uint(123)
		session.Set("user_id", uid)
		session.Save()
		c.Status(200)
	})

	t.Run("Successfully shorten URL", func(t *testing.T) {
		db.Create(&models.User{Username: "shortuser", APIKey: "shortkey"})

		body := map[string]string{
			"long_url": "https://example.com",
		}
		jsonBody, _ := json.Marshal(body)
		
		req, _ := http.NewRequest("POST", "/api/v1/shorten", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "shortkey")
		
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp["short_code"])
	})

	t.Run("Shorten with UserID in Session", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/set-session-shorten/123", nil)
		r.ServeHTTP(w, req)
		cookie := w.Header().Get("Set-Cookie")

		body := map[string]string{
			"long_url": "https://example.com",
		}
		jsonBody, _ := json.Marshal(body)
		
		req2, _ := http.NewRequest("POST", "/api/v1/shorten", bytes.NewBuffer(jsonBody))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Cookie", cookie)
		
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusCreated, w2.Code)
	})

	t.Run("Invalid URL", func(t *testing.T) {
		body := map[string]string{
			"long_url": "not-a-url",
		}
		jsonBody, _ := json.Marshal(body)
		
		req, _ := http.NewRequest("POST", "/api/v1/shorten", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "shortkey")
		
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("CreateShortURL Error", func(t *testing.T) {
		// Trigger DB error or custom code error
		db.Migrator().DropTable(&models.URL{})
		
		body := map[string]string{
			"long_url": "https://example.com",
		}
		jsonBody, _ := json.Marshal(body)
		
		req, _ := http.NewRequest("POST", "/api/v1/shorten", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "shortkey")
		
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
