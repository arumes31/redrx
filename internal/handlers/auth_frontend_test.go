package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"redrx/internal/models"
	"redrx/pkg/utils"

	"github.com/stretchr/testify/assert"
)

func TestAuthFrontendHandlers(t *testing.T) {
	h, db := setupTestHandler()
	r := setupTestRouter(h)

	t.Run("Show Login", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/login", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Show Register", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/register", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Handle Login Success", func(t *testing.T) {
		pass := "password123"
		hash, _ := utils.HashPassword(pass)
		db.Create(&models.User{Username: "loginuser", Email: "login@err.com", PasswordHash: hash, APIKey: "key1"})

		form := url.Values{}
		form.Add("username", "loginuser")
		form.Add("password", pass)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/dashboard", w.Header().Get("Location"))
	})

	t.Run("Handle Login Fail", func(t *testing.T) {
		form := url.Values{}
		form.Add("username", "nonexistent")
		form.Add("password", "wrong")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Handle Login DB Error", func(t *testing.T) {
		db.Migrator().DropTable(&models.User{})
		defer db.AutoMigrate(&models.User{})

		form := url.Values{}
		form.Add("username", "any")
		form.Add("password", "any")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Handle Login Wrong Password", func(t *testing.T) {
		pass := "correct"
		hash, _ := utils.HashPassword(pass)
		db.Create(&models.User{Username: "passuser", Email: "pass@err.com", PasswordHash: hash, APIKey: "key4"})

		form := url.Values{}
		form.Add("username", "passuser")
		form.Add("password", "wrong")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Handle Register DB Error", func(t *testing.T) {
		db.Migrator().DropTable(&models.User{})
		defer db.AutoMigrate(&models.User{})

		form := url.Values{}
		form.Add("username", "any")
		form.Add("email", "any@any.com")
		form.Add("password", "anyany")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Handle Register Success", func(t *testing.T) {
		form := url.Values{}
		form.Add("username", "newuser")
		form.Add("email", "new@example.com")
		form.Add("password", "password123")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/login", w.Header().Get("Location"))
	})

	t.Run("Handle Register Conflict", func(t *testing.T) {
		db.Create(&models.User{Username: "existing", Email: "existing@example.com", APIKey: "key2"})

		form := url.Values{}
		form.Add("username", "existing")
		form.Add("email", "existing@example.com")
		form.Add("password", "password123")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("Handle Register Hash Error", func(t *testing.T) {
		form := url.Values{}
		form.Add("username", "hashuser")
		form.Add("email", "hash@err.com")
		form.Add("password", strings.Repeat("A", 100))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
