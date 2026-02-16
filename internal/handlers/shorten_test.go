package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortenURLHandler(t *testing.T) {
	h, _ := setupTestHandler()
	r := setupTestRouter(h)
	r.POST("/api/v1/shorten", h.ShortenURL)

	t.Run("Successfully shorten URL", func(t *testing.T) {
		body := map[string]string{
			"long_url": "https://example.com",
		}
		jsonBody, _ := json.Marshal(body)
		
		req, _ := http.NewRequest("POST", "/api/v1/shorten", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp["short_code"])
	})

	t.Run("Invalid URL", func(t *testing.T) {
		body := map[string]string{
			"long_url": "not-a-url",
		}
		jsonBody, _ := json.Marshal(body)
		
		req, _ := http.NewRequest("POST", "/api/v1/shorten", bytes.NewBuffer(jsonBody))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
