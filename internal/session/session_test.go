package session

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func roundTrip(t *testing.T, m *Manager, mutate func(*Session)) *Session {
	t.Helper()

	rec := httptest.NewRecorder()
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))
	mutate(s)
	m.Save(rec, s)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return m.Load(req)
}

func TestSessionRoundTrip(t *testing.T) {
	m := NewManager([]byte("secret"), true)

	got := roundTrip(t, m, func(s *Session) {
		s.Login(42)
		s.AddFlash("success", "hello there")
		s.AuthorizeLink("ABC123")
		s.CSRFToken()
	})

	if got.UserID != 42 {
		t.Errorf("UserID = %d, want 42", got.UserID)
	}
	if !got.IsLinkAuthorized("ABC123") {
		t.Error("link authorization did not survive the round trip")
	}
	if got.IsLinkAuthorized("OTHER") {
		t.Error("an unrelated link is reported as authorized")
	}

	flashes := got.TakeFlashes()
	if len(flashes) != 1 || flashes[0].Message != "hello there" || flashes[0].Category != "success" {
		t.Errorf("flashes = %+v, want the one queued message", flashes)
	}
	if len(got.TakeFlashes()) != 0 {
		t.Error("flashes were served twice")
	}
}

func TestTamperedCookieIsRejected(t *testing.T) {
	m := NewManager([]byte("secret"), true)

	rec := httptest.NewRecorder()
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))
	s.Login(7)
	m.Save(rec, s)

	cookie := rec.Result().Cookies()[0]

	// Flip a character in the payload; the MAC must no longer verify.
	body, sig, _ := strings.Cut(cookie.Value, ".")
	tampered := body[:len(body)-1] + string(body[len(body)-1]^1) + "." + sig

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: tampered})

	if got := m.Load(req); got.UserID != 0 {
		t.Errorf("a tampered cookie authenticated as user %d", got.UserID)
	}
}

func TestCookieFromAnotherKeyIsRejected(t *testing.T) {
	signer := NewManager([]byte("key-one"), true)
	verifier := NewManager([]byte("key-two"), true)

	rec := httptest.NewRecorder()
	s := signer.Load(httptest.NewRequest(http.MethodGet, "/", nil))
	s.Login(9)
	signer.Save(rec, s)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	if got := verifier.Load(req); got.UserID != 0 {
		t.Errorf("a cookie signed with a different SECRET_KEY was accepted as user %d", got.UserID)
	}
}

func TestCSRFTokenIsStableAndChecked(t *testing.T) {
	m := NewManager([]byte("secret"), true)
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))

	token := s.CSRFToken()
	if token == "" {
		t.Fatal("CSRFToken returned an empty string")
	}
	if s.CSRFToken() != token {
		t.Error("CSRFToken changed between calls within one session")
	}
	if !s.ValidCSRF(token) {
		t.Error("the session's own token failed validation")
	}
	for _, bad := range []string{"", "wrong", token + "x", token[:len(token)-1]} {
		if s.ValidCSRF(bad) {
			t.Errorf("ValidCSRF(%q) accepted a bad token", bad)
		}
	}
}

func TestLoginRotatesCSRFToken(t *testing.T) {
	m := NewManager([]byte("secret"), true)
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))

	before := s.CSRFToken()
	s.Login(3)
	after := s.CSRFToken()

	if before == after {
		t.Error("the CSRF token survived login; a pre-login token could be replayed")
	}
}

func TestLogoutClearsTheCookie(t *testing.T) {
	m := NewManager([]byte("secret"), true)

	rec := httptest.NewRecorder()
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))
	s.Login(11)
	s.Logout()
	m.Save(rec, s)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie instruction, got %d", len(cookies))
	}
	if cookies[0].MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want a negative value to expire the cookie", cookies[0].MaxAge)
	}
}

func TestCookieSecurityAttributes(t *testing.T) {
	m := NewManager([]byte("secret"), true)

	rec := httptest.NewRecorder()
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))
	s.Login(1)
	m.Save(rec, s)

	c := rec.Result().Cookies()[0]
	if !c.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}
	if !c.Secure {
		t.Error("session cookie is not Secure")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
}

func TestUnchangedSessionWritesNoCookie(t *testing.T) {
	m := NewManager([]byte("secret"), true)

	rec := httptest.NewRecorder()
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))
	m.Save(rec, s)

	if len(rec.Result().Cookies()) != 0 {
		t.Error("an untouched session still wrote a Set-Cookie header")
	}
}

// TestOversizedSessionIsShrunkNotDropped covers the visitor who has unlocked
// enough password-protected links to overflow the cookie. Emitting the
// oversized value would make the browser discard the whole cookie, logging them
// out; the session must instead shed link authorisations and keep the identity.
func TestOversizedSessionIsShrunkNotDropped(t *testing.T) {
	m := NewManager([]byte("secret"), true)

	rec := httptest.NewRecorder()
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))
	s.Login(42)
	s.CSRFToken()
	for i := 0; i < 400; i++ {
		s.AuthorizeLink(fmt.Sprintf("CODE%06d", i))
	}
	m.Save(rec, s)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	if n := len(cookies[0].Value); n > maxCookieBytes {
		t.Errorf("cookie value is %d bytes, over the %d limit browsers accept", n, maxCookieBytes)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	got := m.Load(req)

	if got.UserID != 42 {
		t.Errorf("UserID = %d after shrinking, want 42 — the login was lost", got.UserID)
	}
	if got.CSRF == "" {
		t.Error("CSRF token was lost while shrinking")
	}
	if len(got.LinkAuth) == 0 {
		t.Error("every link authorisation was evicted; expected as many as fit")
	}
}

// TestOversizedFlashesAreDroppedFirst keeps flashes ahead of link
// authorisations in the eviction order: a flash is shown once, an unlocked link
// costs a password re-entry.
func TestOversizedFlashesAreDroppedFirst(t *testing.T) {
	m := NewManager([]byte("secret"), true)

	rec := httptest.NewRecorder()
	s := m.Load(httptest.NewRequest(http.MethodGet, "/", nil))
	s.Login(7)
	s.AuthorizeLink("KEEPME")
	for i := 0; i < 200; i++ {
		s.AddFlash("info", strings.Repeat("x", 100))
	}
	m.Save(rec, s)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	if n := len(cookies[0].Value); n > maxCookieBytes {
		t.Errorf("cookie value is %d bytes, over the %d limit", n, maxCookieBytes)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	got := m.Load(req)

	if len(got.Flashes) != 0 {
		t.Errorf("Flashes = %d, want 0 — they should be dropped first", len(got.Flashes))
	}
	if !got.IsLinkAuthorized("KEEPME") {
		t.Error("the link authorisation was evicted before the flashes were")
	}
}
