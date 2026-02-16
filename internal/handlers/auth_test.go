package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthHandlers(t *testing.T) {
	h, _ := setupTestHandler()
	r := setupTestRouter(h)
	
	r.POST("/api/register", h.RegisterUser)
	r.POST("/api/login", h.LoginUser)

	t.Run("Register success", func(t *testing.T) {
		body := map[string]string{
			"username": "testuser",
			"email":    "test@example.com",
			"password": "password123",
		}
		jsonBody, _ := json.Marshal(body)
		
		req, _ := http.NewRequest("POST", "/api/register", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("Login success", func(t *testing.T) {
		body := map[string]string{
			"username": "testuser",
			"password": "password123",
		}
		jsonBody, _ := json.Marshal(body)
		
		req, _ := http.NewRequest("POST", "/api/login", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp["api_key"])
	})

	t.Run("Login fail - wrong password", func(t *testing.T) {
		body := map[string]string{
			"username": "testuser",
			"password": "wrongpassword",
		}
		jsonBody, _ := json.Marshal(body)
		
		req, _ := http.NewRequest("POST", "/api/login", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
