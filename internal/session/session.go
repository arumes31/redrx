// Package session implements signed cookie sessions.
//
// The payload is JSON, base64url-encoded and authenticated with HMAC-SHA256
// keyed on SECRET_KEY. Nothing secret is stored in it — only a user id, flash
// messages, a CSRF token and the set of password-protected links this visitor
// has unlocked — so signing without encryption is sufficient.
package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	cookieName = "redrx_session"
	// maxAge bounds both the cookie lifetime and the signature's validity.
	maxAge = 30 * 24 * time.Hour
	// maxCookieBytes leaves headroom under the 4 KiB browser limit.
	maxCookieBytes = 3800
)

// Flash is a one-shot message rendered on the next page load.
type Flash struct {
	Category string `json:"c"`
	Message  string `json:"m"`
}

// Data is the serialised session payload.
type Data struct {
	UserID   int64           `json:"uid,omitempty"`
	CSRF     string          `json:"csrf,omitempty"`
	Flashes  []Flash         `json:"fl,omitempty"`
	LinkAuth map[string]bool `json:"la,omitempty"`
	Issued   int64           `json:"iat,omitempty"`
}

// Session is the per-request view of the cookie, plus a dirty flag so unchanged
// sessions do not rewrite the cookie on every response.
type Session struct {
	Data
	dirty  bool
	secure bool
}

// Manager signs and verifies session cookies.
type Manager struct {
	key []byte
	// secure marks cookies Secure; disabled in debug so plain-HTTP dev works.
	secure bool
}

func NewManager(secretKey []byte, secure bool) *Manager {
	// Derive a distinct key so the session MAC is not the raw configured
	// secret, which may be reused for other purposes.
	sum := sha256.Sum256(append([]byte("redrx.session.v1|"), secretKey...))
	return &Manager{key: sum[:], secure: secure}
}

// Load reads and verifies the session cookie, returning an empty session when
// the cookie is absent, tampered with or expired.
func (m *Manager) Load(r *http.Request) *Session {
	s := &Session{secure: m.secure}
	s.LinkAuth = map[string]bool{}

	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return s
	}
	payload, ok := m.verify(c.Value)
	if !ok {
		return s
	}
	var d Data
	if err := json.Unmarshal(payload, &d); err != nil {
		return s
	}
	if d.Issued > 0 && time.Since(time.Unix(d.Issued, 0)) > maxAge {
		return s
	}
	s.Data = d
	if s.LinkAuth == nil {
		s.LinkAuth = map[string]bool{}
	}
	return s
}

// Save writes the cookie if the session changed. It must be called before any
// response body is written.
func (m *Manager) Save(w http.ResponseWriter, s *Session) {
	if s == nil || !s.dirty {
		return
	}
	s.dirty = false

	if s.isEmpty() {
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: m.secure, SameSite: http.SameSiteLaxMode,
		})
		return
	}

	s.Issued = time.Now().Unix()
	value, ok := m.shrinkToFit(s)
	if !ok {
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: value, Path: "/",
		MaxAge: int(maxAge.Seconds()), HttpOnly: true,
		Secure: m.secure, SameSite: http.SameSiteLaxMode,
	})
}

// shrinkToFit signs the session, dropping the least important state until the
// cookie fits. An oversized Set-Cookie is discarded by the browser in full, so
// emitting one would silently log the visitor out and lose every unlocked link
// — shedding some state is always better than shedding all of it.
func (m *Manager) shrinkToFit(s *Session) (string, bool) {
	value, ok := m.trySign(s)
	if !ok {
		return "", false
	}
	if len(value) <= maxCookieBytes {
		return value, true
	}

	// Flashes are one-shot and the largest single contributor, so they go first.
	if len(s.Flashes) > 0 {
		s.Flashes = nil
		if value, ok = m.trySign(s); !ok {
			return "", false
		}
		if len(value) <= maxCookieBytes {
			return value, true
		}
	}

	// Then the oldest link authorisations, which cost the visitor no more than
	// re-entering a link password. Map order is unspecified, so this evicts an
	// arbitrary entry each round rather than a strictly oldest one.
	for len(s.LinkAuth) > 0 {
		for code := range s.LinkAuth {
			delete(s.LinkAuth, code)
			break
		}
		if value, ok = m.trySign(s); !ok {
			return "", false
		}
		if len(value) <= maxCookieBytes {
			return value, true
		}
	}

	// Nothing left to shed: the identity alone is over the limit, which cannot
	// happen with the fields defined here.
	return value, len(value) <= maxCookieBytes
}

func (m *Manager) trySign(s *Session) (string, bool) {
	payload, err := json.Marshal(s.Data)
	if err != nil {
		return "", false
	}
	return m.sign(payload), true
}

func (m *Manager) sign(payload []byte) string {
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *Manager) verify(value string) ([]byte, bool) {
	body, sig, ok := strings.Cut(value, ".")
	if !ok {
		return nil, false
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(body))
	if subtle.ConstantTimeCompare(want, mac.Sum(nil)) != 1 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, false
	}
	return payload, true
}

func (s *Session) isEmpty() bool {
	return s.UserID == 0 && s.CSRF == "" && len(s.Flashes) == 0 && len(s.LinkAuth) == 0
}

// Login records the authenticated user and rotates the CSRF token, so a token
// captured before login cannot be replayed afterwards.
func (s *Session) Login(userID int64) {
	s.UserID = userID
	s.CSRF = ""
	s.dirty = true
}

// Logout clears all session state.
func (s *Session) Logout() {
	s.Data = Data{}
	s.LinkAuth = map[string]bool{}
	s.dirty = true
}

// AddFlash queues a message for the next rendered page.
func (s *Session) AddFlash(category, message string) {
	s.Flashes = append(s.Flashes, Flash{Category: category, Message: message})
	s.dirty = true
}

// TakeFlashes returns and clears the queued messages.
func (s *Session) TakeFlashes() []Flash {
	if len(s.Flashes) == 0 {
		return nil
	}
	out := s.Flashes
	s.Flashes = nil
	s.dirty = true
	return out
}

// AuthorizeLink records that the visitor entered the correct link password.
func (s *Session) AuthorizeLink(code string) {
	if s.LinkAuth == nil {
		s.LinkAuth = map[string]bool{}
	}
	s.LinkAuth[code] = true
	s.dirty = true
}

// IsLinkAuthorized reports whether this visitor already unlocked the link.
func (s *Session) IsLinkAuthorized(code string) bool { return s.LinkAuth[code] }

// CSRFToken returns the session's CSRF token, minting one on first use.
func (s *Session) CSRFToken() string {
	if s.CSRF == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return ""
		}
		s.CSRF = hex.EncodeToString(b)
		s.dirty = true
	}
	return s.CSRF
}

// ValidCSRF compares a submitted token against the session's in constant time.
func (s *Session) ValidCSRF(token string) bool {
	if s.CSRF == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(s.CSRF), []byte(token)) == 1
}
