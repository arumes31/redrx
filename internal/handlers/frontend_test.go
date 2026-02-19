package handlers

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"redrx/internal/models"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestFrontendHandlers(t *testing.T) {
	h, db := setupTestHandler()
	r := setupTestRouter(h)

	t.Run("Show Index", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Shorten Link")
	})

	t.Run("Handle Shorten Form - Success", func(t *testing.T) {
		form := url.Values{}
		form.Add("long_url", "https://example.com")
		form.Add("custom_code", "FRONTEND")
		
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "URL Shortened Successfully")
		assert.Contains(t, w.Body.String(), "FRONTEND")
	})

	t.Run("Handle Shorten Form - Invalid Input", func(t *testing.T) {
		form := url.Values{}
		form.Add("long_url", "not-a-url")
		
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid Input")
	})

	t.Run("Handle Shorten Form - Success with Session", func(t *testing.T) {
		// Set session
		r.GET("/set-session-fe", func(c *gin.Context) {
			session := sessions.Default(c)
			session.Set("user_id", uint(789))
			session.Save()
			c.Status(200)
		})

		w1 := httptest.NewRecorder()
		req1, _ := http.NewRequest("GET", "/set-session-fe", nil)
		r.ServeHTTP(w1, req1)
		cookie := w1.Header().Get("Set-Cookie")

		form := url.Values{}
		form.Add("long_url", "https://example.com")
		
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", cookie)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Handle Shorten Form - With Logo", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		
		part, _ := writer.CreateFormFile("logo_file", "logo.png")
		// Create a real valid image
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		png.Encode(part, img)
		
		writer.WriteField("long_url", "https://example.com")
		writer.Close()

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Handle Shorten Form - With Invalid Logo", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		
		part, _ := writer.CreateFormFile("logo_file", "logo.txt")
		part.Write([]byte("not an image"))
		
		writer.WriteField("long_url", "https://example.com")
		writer.Close()

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Handle Shorten Form - DB Error", func(t *testing.T) {
		db.Migrator().DropTable(&models.URL{})
		defer db.AutoMigrate(&models.URL{})

		form := url.Values{}
		form.Add("long_url", "https://example.com")
		
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code) // Should render index.html with Error
	})
}
