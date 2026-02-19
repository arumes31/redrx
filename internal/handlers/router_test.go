package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouter_Health(t *testing.T) {
	h, _ := setupTestHandler()
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthy")
}

func TestSetupRouter_Minimal(t *testing.T) {
	h, _ := setupTestHandler()
	r := h.SetupRouter(nil, "", "")
	assert.NotNil(t, r)
}
